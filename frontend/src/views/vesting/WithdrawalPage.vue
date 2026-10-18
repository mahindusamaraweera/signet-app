<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import api from '@/api/client'
import { assetDisplay } from '@/utils/assetDisplay'
import { canCancelRequest } from '@/utils/withdrawalDisplay'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'

const security = ref(null), addresses = ref([]), requests = ref([])
const amount = ref(''), addressId = ref(''), password = ref(''), emailCode = ref(''), authCode = ref('')
const prepared = ref(null), challenge = ref(''), stage = ref('draft'), busy = ref(false)
const historyError = ref('')
const error = ref(''), message = ref(''), uncertain = ref(false)
const cancelTarget = ref(null), cancelPassword = ref(''), cancelAcknowledged = ref(false)
const cancelDialog = ref(null), cancelInput = ref(null)
let returnFocus = null
watch(cancelTarget, async target => {
  await nextTick()
  if (target) {
    returnFocus = document.activeElement
    cancelDialog.value?.showModal()
    cancelInput.value?.focus()
  } else {
    cancelDialog.value?.close()
    cancelPassword.value = ''; cancelAcknowledged.value = false
    returnFocus?.focus()
  }
})
function closeCancellation() { if (!busy.value) cancelTarget.value = null }
let stopped = false
const enabled = computed(() => security.value?.withdrawal_submission_enabled === true)
const activeAddresses = computed(() => addresses.value.filter(a => !a.revoked_at && Date.parse(a.eligible_at) <= Date.now()))
const pendingQueue = computed(() => requests.value.some(r => r.queue_status === 'pending'))
const cancellationEnabled = computed(() => security.value?.withdrawal_cancellation_enabled === true)

function clearProof() { prepared.value = null; challenge.value = ''; emailCode.value = authCode.value = password.value = '' }
async function requestStatus() {
  const { data } = await api.get('/blockchain/withdrawals/requests')
  if (stopped) return
  requests.value = (data.requests || []).map(r => ({ ...r, payout: null }))
  historyError.value = ''
  try {
    const { data: history } = await api.get('/blockchain/withdrawals')
    if (stopped) return
    const payouts = new Map((history.withdrawals || []).map(p => [String(p.id), p]))
    requests.value = requests.value.map(r => ({ ...r, payout: payouts.get(String(r.remote_withdrawal_id)) || null }))
  } catch {
    if (stopped) return
    historyError.value = 'Payout status unavailable. Check payout history before taking further action.'
  }
  if (prepared.value) {
    const row = requests.value.find(r => r.id === prepared.value.id)
    if (row?.queue_status === 'pending' || row?.queue_status === 'submitted') {
      clearProof(); stage.value = 'queued'; message.value = 'Request queued. Check its reservation and payout history; do not submit it again.'
    } else if (row?.queue_status === 'cancellation_review_required') {
      clearProof(); stage.value = 'queued'; message.value = 'Cancellation could not be confirmed safely. Check payout history and contact support for reconciliation; do not assume a refund.'
    } else if (row?.queue_status === 'cancel_requested' || row?.queue_status === 'cancelled') {
      clearProof(); stage.value = 'queued'; message.value = row.queue_status === 'cancelled' ? 'Cancellation confirmed. Check custody history and available balance.' : 'Cancellation is pending confirmation. Do not assume a refund yet.'
    } else if (row?.authorization_status === 'authorized') stage.value = 'authorized'
    else if (row?.authorization_status === 'expired' || row?.authorization_status === 'invalidated') { clearProof(); stage.value = 'draft' }
  }
  uncertain.value = false
}
async function refresh() {
  if (busy.value || stopped) return
  busy.value = true; error.value = ''
  try {
    const { data } = await api.get('/blockchain/security'); if (stopped) return; security.value = data
    const { data: saved } = await api.get('/blockchain/security/addresses'); if (stopped) return; addresses.value = saved.addresses || []
    await requestStatus()
  } catch (e) { if (!stopped) { error.value = e?.response?.data?.message || 'Status unavailable. Do not repeat a withdrawal until its status is known.'; uncertain.value = true } }
  finally { if (!stopped) busy.value = false }
}
async function act(kind) {
  if (busy.value || stopped || uncertain.value || !enabled.value) return
  busy.value = true; error.value = ''; message.value = ''
  const currentPassword = password.value; password.value = ''
  try {
    if (kind === 'prepare') {
      const { data } = await api.post('/blockchain/withdrawals/prepare', { password: currentPassword, address_id: addressId.value, amount: amount.value })
      if (stopped) return; prepared.value = data; stage.value = 'prepared'
    } else {
      const body = { password: currentPassword, id: prepared.value.id, proof: prepared.value.proof, terms: prepared.value.terms }
      if (kind === 'email') {
        const { data } = await api.post('/blockchain/withdrawals/email', body)
        if (stopped) return; challenge.value = data.challenge_id; message.value = 'Verification email sent. Enter your password again and both codes.'
      } else if (kind === 'confirm') {
        await api.post('/blockchain/withdrawals/confirm', { ...body, challenge_id: challenge.value, email_code: emailCode.value, authenticator_code: authCode.value })
        if (stopped) return; stage.value = 'authorized'; emailCode.value = authCode.value = ''; message.value = 'Approved. Confirm queue submission below.'
      } else if (kind === 'queue') {
        await api.post('/blockchain/withdrawals/queue', body)
        if (stopped) return; clearProof(); stage.value = 'queued'; message.value = 'Queued for custody reservation, security checks and ledger audit. This is not a completed payout.'
        await requestStatus()
      }
    }
  } catch (e) {
    if (stopped) return
    error.value = e?.response?.data?.message || 'Response uncertain. Check request status before retrying.'
    uncertain.value = true
    try { await requestStatus() } catch { /* Keep submission blocked until status can be checked. */ }
  } finally {
    // Do not retain one-time verification codes after a confirmation attempt,
    // including an uncertain response or validation failure.
    if (kind === 'confirm') emailCode.value = authCode.value = ''
    if (!stopped) busy.value = false
  }
}
onMounted(refresh)
async function cancelRequest() {
  if (busy.value || stopped || !cancelTarget.value || !cancelAcknowledged.value || !cancellationEnabled.value) return
  busy.value = true; error.value = ''; const id = cancelTarget.value.id, currentPassword = cancelPassword.value
  cancelPassword.value = ''
  try {
    await api.post('/blockchain/withdrawals/cancel', { id, password: currentPassword })
    if (stopped) return
    cancelTarget.value = null; cancelAcknowledged.value = false
    message.value = 'Cancellation requested. Wait for confirmed status; funds are not necessarily refunded yet.'
    await requestStatus()
  } catch (e) {
    if (!stopped) { error.value = e?.response?.data?.message || 'Cancellation response uncertain. Check request status.'; uncertain.value = true; try { await requestStatus() } catch {} }
  } finally { if (!stopped) busy.value = false }
}
onBeforeUnmount(() => { stopped = true; clearProof(); cancelPassword.value = ''; cancelTarget.value = null })
</script>

<template>
  <DashboardLayout><div class="wallet-form-page">
    <h1>Withdraw SIGNET</h1>
    <p>Withdraw to your verified BSC address. A 3% fee is included in your requested amount; you receive 97%.</p>
    <p><RouterLink to="/wallet/security">Set up email, Authenticator and withdrawal address</RouterLink> · <RouterLink to="/wallet/deposits">Withdrawal history</RouterLink></p>
    <p v-if="!enabled" class="alert alert-warning">Withdrawal submission is not enabled. Your records remain available below.</p>
    <p v-if="error" role="alert" class="alert alert-danger">{{ error }}</p><p v-if="message" role="status" class="alert alert-info">{{ message }}</p>
    <button class="btn btn-outline-secondary mb-3" :disabled="busy" @click="refresh">Check status</button>
    <section v-if="enabled && stage !== 'queued'" class="card p-3 mb-4">
      <template v-if="!prepared">
        <label for="withdraw-address">Verified eligible address</label><select id="withdraw-address" v-model="addressId" class="form-select mb-3" :disabled="busy"><option value="">Select address</option><option v-for="a in activeAddresses" :key="a.id" :value="a.id">{{ a.address }}</option></select>
        <label for="withdraw-amount">Gross SIGNET amount (fee included)</label><input id="withdraw-amount" v-model="amount" inputmode="decimal" class="form-control mb-3" :disabled="busy" />
      </template>
      <template v-else><p class="text-break">Destination: {{ prepared.terms.destination }}</p><p>Gross: {{ prepared.terms.gross }} SIGNET<br>3% fee: {{ prepared.terms.fee }} SIGNET<br>You receive: {{ prepared.terms.net }} SIGNET</p><p>Approval expires: {{ prepared.expires_at }}</p></template>
      <label for="withdraw-password">Current password (enter again for each action)</label><input id="withdraw-password" v-model="password" type="password" autocomplete="current-password" maxlength="72" class="form-control mb-3" :disabled="busy" />
      <button v-if="!prepared" class="btn btn-primary" :disabled="busy || uncertain || pendingQueue || !password || !addressId || !/^[0-9]{1,18}(\.[0-9]{1,18})?$/.test(amount)" @click="act('prepare')">Review exact withdrawal</button>
      <template v-else-if="stage === 'prepared'">
        <button class="btn btn-primary mb-3" :disabled="busy || uncertain || !password" @click="act('email')">Send withdrawal verification email</button>
        <template v-if="challenge"><label for="withdraw-email">Email code</label><input id="withdraw-email" v-model="emailCode" maxlength="6" inputmode="numeric" autocomplete="one-time-code" class="form-control mb-3" :disabled="busy" /><label for="withdraw-auth">Authenticator code</label><input id="withdraw-auth" v-model="authCode" maxlength="6" inputmode="numeric" autocomplete="off" class="form-control mb-3" :disabled="busy" /><button class="btn btn-primary" :disabled="busy || uncertain || !password || !/^\d{6}$/.test(emailCode) || !/^\d{6}$/.test(authCode)" @click="act('confirm')">Verify this exact request</button></template>
      </template>
      <button v-else-if="stage === 'authorized'" class="btn btn-primary" :disabled="busy || uncertain || !password" @click="act('queue')">Queue approved withdrawal</button>
    </section>
    <h2 class="h5 mt-4">Recent withdrawal requests</h2><p>Payout status comes from custody. Submitted means reserved, awaiting processing. Older payouts are available in payout and fee history.</p>
    <p v-if="historyError" role="status" class="alert alert-warning">{{ historyError }}</p>
    <p v-if="!busy && !error && !requests.length" class="wallet-empty">No withdrawal requests yet.</p>
    <div class="withdrawal-list">
      <article v-for="r in requests" :key="r.id" class="withdrawal-record">
        <div class="withdrawal-heading">
          <strong>{{ assetDisplay(r.payout?.net_amount || r.terms.net, 'SIGNET') }} <small>SIGNET</small></strong>
          <span class="withdrawal-badge" :class="{ completed: r.payout?.status === 'completed' }">{{ r.payout?.status?.replaceAll('_', ' ') || (r.queue_status || r.authorization_status).replaceAll('_', ' ') }}</span>
        </div>
        <p class="small mb-2">Requested {{ assetDisplay(r.terms.gross, 'SIGNET') }} · Fee {{ assetDisplay(r.terms.fee, 'SIGNET') }} SIGNET</p>
        <p v-if="r.remote_withdrawal_id && !r.payout" class="small">Payout status not loaded. Check withdrawal history before trying again.</p>
        <p v-if="r.queue_status === 'cancellation_review_required'" class="text-warning small">Cancellation needs review. Contact support; a refund is not confirmed.</p>
        <details><summary>Withdrawal details<span v-if="r.remote_withdrawal_id"> · #{{ r.remote_withdrawal_id }}</span></summary>
          <p class="text-break mt-2 mb-1">To: {{ r.terms.destination }}</p>
          <p v-if="r.payout?.completed_at" class="small mb-1">Completed: {{ new Date(r.payout.completed_at).toLocaleString() }}</p>
          <a v-if="/^0x[a-fA-F0-9]{64}$/.test(r.payout?.tx_hash || '')" :href="'https://bscscan.com/tx/' + r.payout.tx_hash" target="_blank" rel="noopener noreferrer">View transaction ↗</a>
          <p v-else class="small mb-0">No payout transaction recorded.</p>
        </details>
        <button v-if="cancellationEnabled && canCancelRequest(r)" class="btn btn-outline-danger btn-sm mt-3" :disabled="busy" @click="error = ''; cancelTarget = r; cancelPassword = ''; cancelAcknowledged = false">Request cancellation</button>
      </article>
    </div>
    <Teleport to="body">
    <dialog ref="cancelDialog" class="withdrawal-dialog" aria-labelledby="cancel-title" @cancel.prevent="closeCancellation" @click="e => { if (e.target === cancelDialog) closeCancellation() }">
    <section v-if="cancelTarget">

      <div class="withdrawal-heading mb-3"><h2 id="cancel-title">Cancel withdrawal?</h2><button type="button" class="btn btn-outline-secondary btn-sm" aria-label="Close cancellation" :disabled="busy" @click="closeCancellation">✕</button></div><p class="text-break">{{ assetDisplay(cancelTarget.terms.gross, 'SIGNET') }} SIGNET → {{ cancelTarget.terms.destination }}</p>
      <p>Cancellation is possible only before payout starts. A pending cancellation is not a confirmed refund.</p>
      <p v-if="error" role="alert" class="alert alert-danger">{{ error }}</p>
      <label for="cancel-password">Current password</label><input ref="cancelInput" id="cancel-password" v-model="cancelPassword" type="password" autocomplete="current-password" maxlength="72" class="form-control mb-3" :disabled="busy" />
      <label><input v-model="cancelAcknowledged" type="checkbox" :disabled="busy" /> I want to cancel this exact request.</label>
      <div class="mt-3"><button class="btn btn-danger" :disabled="busy || !cancelPassword || !cancelAcknowledged" @click="cancelRequest">Request cancellation</button><button class="btn btn-outline-secondary ms-2" :disabled="busy" @click="cancelTarget = null; cancelPassword = ''; cancelAcknowledged = false">Keep request</button></div>
    </section>
    </dialog>
    </Teleport>
  </div></DashboardLayout>
</template>

<style scoped>
.wallet-form-page { max-width: 880px; margin-inline: auto; }
.withdrawal-list { display: grid; gap: 12px; }
.withdrawal-record { padding: 18px; border: 1px solid #29434c; border-radius: 16px; background: #0b1921; }
.withdrawal-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.withdrawal-heading strong { font-size: 1.25rem; }
.withdrawal-heading small { font-size: .75rem; color: #a9c0ca; }
.withdrawal-badge { font-size: .8rem; text-transform: capitalize; padding: 4px 10px; border-radius: 20px; background: #29323a; }
.withdrawal-badge.completed { background: #123e32; color: #66e5b7; }
.withdrawal-record p { color: #afc4cc; }
.withdrawal-record summary { cursor: pointer; color: #6bdded; font-size: .9rem; }
.withdrawal-dialog { width: min(480px, calc(100% - 24px)); max-height: calc(100dvh - 32px); overflow-y: auto; margin: auto; padding: 24px; border: 1px solid #385a64; border-radius: 20px; background: #0b1921; color: #e5f2f5; box-shadow: 0 24px 80px #0009; }
.withdrawal-dialog::backdrop { background: #000a; backdrop-filter: blur(3px); }
.withdrawal-dialog h2 { font-size: 1.3rem; margin: 0; }
.withdrawal-dialog .text-break { font-size: .9rem; }
@media (max-width: 576px) { .withdrawal-record { padding: 14px; } .withdrawal-dialog { padding: 18px; } }
</style>
