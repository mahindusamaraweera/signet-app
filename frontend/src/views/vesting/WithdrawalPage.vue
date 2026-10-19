<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import api from '@/api/client'
import WithdrawalCountdown from '@/components/WithdrawalCountdown.vue'
import { useWithdrawalClock } from '@/composables/useWithdrawalClock'
import { remainingSeconds, withdrawalErrorMessage, deadlineMillis } from '@/utils/withdrawalTime'
import { loadWithdrawalPayouts } from '@/utils/withdrawalHistory'
import { verifyAndQueueWithdrawal } from '@/utils/withdrawalSubmission'
import { assetDisplay } from '@/utils/assetDisplay'
import { canCancelRequest, withdrawalRequestNotice, hasOutstandingWithdrawal, withdrawalStatusLabel, withdrawalAmount } from '@/utils/withdrawalDisplay'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'

const { now, sync } = useWithdrawalClock()
const security = ref(null), addresses = ref([]), requests = ref([])
const amount = ref(''), addressId = ref(''), password = ref(''), emailCode = ref(''), authCode = ref('')
const prepared = ref(null), challenge = ref(''), stage = ref('draft'), busy = ref(false)
const historyError = ref('')
const showVerificationHistory = ref(false)
const verificationHistory = computed(() => requests.value.filter(r => !r.queue_status && !r.remote_withdrawal_id && ['expired', 'invalidated'].includes(r.authorization_status)))
const visibleRequests = computed(() => showVerificationHistory.value ? requests.value : requests.value.filter(r => !verificationHistory.value.includes(r)))
const emailInput = ref(null)
watch(busy, async value => {
  if (!value && challenge.value && stage.value === 'prepared' && !emailCode.value) {
    await nextTick(); if (!stopped) emailInput.value?.focus()
  }
})
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
const availableAddresses = computed(() => addresses.value.filter(a => !a.revoked_at))
const selectedAddress = computed(() => availableAddresses.value.find(a => a.id === addressId.value))
const accountWaiting = computed(() => security.value?.hold_until && remainingSeconds(security.value.hold_until, now.value) !== 0)
const addressWaiting = computed(() => selectedAddress.value && remainingSeconds(selectedAddress.value.eligible_at, now.value) !== 0)
const securityReady = computed(() => security.value?.email_verified && security.value?.authenticator_confirmed && !security.value?.withdrawals_frozen)
const admissionChecked = computed(() => security.value?.withdrawal_admission_checked === true)
const cooldownWaiting = computed(() => security.value?.withdrawal_cooldown_until && remainingSeconds(security.value.withdrawal_cooldown_until, now.value) !== 0)
const pendingQueue = computed(() => requests.value.some(r => ['pending', 'cancel_requested', 'cancellation_review_required'].includes(r.queue_status) || (r.queue_status === 'submitted' && !r.remote_withdrawal_id))
  || (admissionChecked.value ? !!security.value.active_withdrawal_id : hasOutstandingWithdrawal(requests.value)))
const previewExpired = computed(() => prepared.value && remainingSeconds(prepared.value.expires_at, now.value) === 0)
const cancellationEnabled = computed(() => security.value?.withdrawal_cancellation_enabled === true)

// Keep the password only in this mounted flow; never persist it across reloads.
watch(previewExpired, expired => { if (expired) password.value = '' })

function clearProof() { prepared.value = null; challenge.value = ''; emailCode.value = authCode.value = password.value = '' }
async function requestStatus() {
  const { data } = await api.get('/blockchain/withdrawals/requests')
  if (stopped) return
  requests.value = (data.requests || []).map(r => ({ ...r, payout: null }))
  historyError.value = ''
  try {
    const payouts = await loadWithdrawalPayouts(api, requests.value.map(r => r.remote_withdrawal_id))
    if (stopped) return
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
    const { data } = await api.get('/blockchain/security'); if (stopped) return; security.value = data; sync(data.server_time)
    const { data: saved } = await api.get('/blockchain/security/addresses'); if (stopped) return; addresses.value = saved.addresses || []
    await requestStatus()
  } catch (e) { if (!stopped) { error.value = e?.response?.data?.message || 'Status unavailable. Do not repeat a withdrawal until its status is known.'; uncertain.value = true } }
  finally { if (!stopped) busy.value = false }
}
async function startNewWithdrawal() {
  if (busy.value || stopped) return
  busy.value = true; error.value = ''
  try {
    await requestStatus()
    if (stopped) return
    if (pendingQueue.value) { message.value = 'An existing withdrawal is still processing or needs review. Check its status before starting another.'; return }
    clearProof(); stage.value = 'draft'
    message.value = 'Enter the amount and password to start a new withdrawal. Previous requests remain in history.'
  } catch { if (!stopped) { uncertain.value = true; error.value = 'Request status unavailable. Check status before starting another withdrawal.' } }
  finally { if (!stopped) busy.value = false }
}
async function act(kind) {
  if (busy.value || stopped || uncertain.value || !enabled.value) return
  if ((kind === 'prepare' || kind === 'queue') && (!admissionChecked.value || pendingQueue.value || cooldownWaiting.value)) { error.value = 'Check status. An existing withdrawal or cooldown must end before another request.'; return }
  if (prepared.value && previewExpired.value && kind !== 'prepare') { error.value = 'This verification period ended. Check status and start again.'; return }
  busy.value = true; error.value = ''; message.value = ''
  const currentPassword = password.value
  try {
    if (kind === 'prepare') {
      const { data } = await api.post('/blockchain/withdrawals/prepare', { password: currentPassword, address_id: addressId.value, amount: amount.value })
      if (stopped) return; prepared.value = data; stage.value = 'prepared'
    } else {
      const body = { password: currentPassword, id: prepared.value.id, proof: prepared.value.proof, terms: prepared.value.terms }
      if (kind === 'email') {
        challenge.value = ''; emailCode.value = authCode.value = ''
        const { data } = await api.post('/blockchain/withdrawals/email', body)
        if (stopped) return; challenge.value = data.challenge_id; message.value = 'Email code sent. Enter the newest email code, a fresh authenticator code, then verify and submit.'
      } else if (kind === 'confirm') {
        const queued = await verifyAndQueueWithdrawal(api, body, { challenge_id: challenge.value, email_code: emailCode.value, authenticator_code: authCode.value }, () => {
          if (stopped) return false
          stage.value = 'authorized'; emailCode.value = authCode.value = ''
          return true
        })
        if (!queued) return
        if (stopped) return
        clearProof(); stage.value = 'queued'; message.value = 'Withdrawal submitted. Security checks and payment processing will continue automatically.'
        await requestStatus()
      } else if (kind === 'queue') {
        await api.post('/blockchain/withdrawals/queue', body)
        if (stopped) return; clearProof(); stage.value = 'queued'; message.value = 'Queued for custody reservation, security checks and ledger audit. This is not a completed payout.'
        await requestStatus()
      }
    }
  } catch (e) {
    if (stopped) return
    const rejection = e?.response?.data
    if (security.value && rejection?.code === 'withdrawal_admission_unavailable') security.value.withdrawal_admission_checked = false
    if (security.value && rejection?.code === 'withdrawal_already_processing' && rejection.active_withdrawal_id) security.value.active_withdrawal_id = rejection.active_withdrawal_id
    if (deadlineMillis(rejection?.available_at) !== null) {
      if (rejection.code === 'withdrawal_cooldown_active' && security.value) security.value.withdrawal_cooldown_until = rejection.available_at
      if (rejection.code === 'withdrawal_security_wait' && security.value) security.value.hold_until = rejection.available_at
      if (rejection.code === 'withdrawal_address_wait') {
        const target = addresses.value.find(a => a.id === (prepared.value?.terms?.address_id || addressId.value))
        if (target) target.eligible_at = rejection.available_at
      }
    }
    error.value = withdrawalErrorMessage(rejection)
    uncertain.value = true
    try { await requestStatus() } catch { /* Keep submission blocked until status can be checked. */ }
  } finally {
    // Do not retain one-time verification codes after a confirmation attempt,
    // including an uncertain response or validation failure.
    if (kind === 'confirm') emailCode.value = authCode.value = ''
    if (!prepared.value || previewExpired.value) password.value = ''
    if (!stopped) busy.value = false
  }
}
let statusTimer
onMounted(() => {
  refresh()
  statusTimer = window.setInterval(() => {
    // Read-only refresh between flows; never interrupt verification or retry a submission.
    if (!busy.value && !prepared.value && !uncertain.value && !error.value && !cancelTarget.value && document.visibilityState === 'visible') refresh()
  }, 20000)
})
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
onBeforeUnmount(() => { stopped = true; window.clearInterval(statusTimer); clearProof(); cancelPassword.value = ''; cancelTarget.value = null })
</script>

<template>
  <DashboardLayout><div class="wallet-form-page">
    <h1>Withdraw SIGNET</h1>
    <p>Withdraw to your verified BSC address. A 3% fee is included in your requested amount; you receive 97%.</p>
    <p><RouterLink to="/wallet/security">Withdrawal security</RouterLink> · <RouterLink to="/wallet/deposits">Withdrawal history</RouterLink></p>
    <p v-if="!enabled" class="alert alert-warning">Withdrawal submission is not enabled. Your records remain available below.</p>
    <p v-if="error" role="alert" class="alert alert-danger">{{ error }}</p><p v-if="message" role="status" class="alert alert-info">{{ message }}</p>
    <p v-if="security?.withdrawals_frozen" role="status" class="alert alert-warning">Withdrawals are frozen for this account. Contact support.</p>
    <p v-else-if="security && !securityReady" role="status" class="alert alert-warning">Complete email verification and authenticator setup in <RouterLink to="/wallet/security">Withdrawal security</RouterLink>.</p>
    <WithdrawalCountdown v-if="accountWaiting" :deadline="security.hold_until" :now="now" kind="account" compact />
    <WithdrawalCountdown v-if="security?.withdrawal_cooldown_until" :deadline="security.withdrawal_cooldown_until" :now="now" kind="cooldown" compact />
    <p v-if="enabled && security && !admissionChecked" role="status" class="alert alert-warning">Current withdrawal status and cooldown could not be confirmed. Check status before requesting a withdrawal.</p>
    <button class="btn btn-outline-secondary mb-3" :disabled="busy" @click="refresh">Check status</button>
    <p v-if="pendingQueue" role="status" class="alert alert-info">An existing withdrawal is processing or needs review. Check its status below before starting another.</p>
    <button v-if="enabled && (prepared || stage === 'queued')" class="btn btn-outline-secondary mb-3" :disabled="busy || pendingQueue" @click="startNewWithdrawal">{{ prepared ? 'Check status and start again' : 'Start another withdrawal' }}</button>
    <ol v-if="enabled" class="withdrawal-steps" aria-label="Withdrawal progress">
      <li :aria-current="stage !== 'queued' && !prepared ? 'step' : undefined">1. Review</li>
      <li :aria-current="prepared ? 'step' : undefined">2. Verify codes</li>
      <li :aria-current="stage === 'queued' ? 'step' : undefined">3. Processing</li>
    </ol>
    <section v-if="enabled && stage !== 'queued'" class="card p-3 mb-4 withdrawal-method">
      <h2 class="h5">{{ !prepared ? '1. Review withdrawal' : stage === 'authorized' ? '2. Submit approved withdrawal' : '2. Verify codes' }}</h2>
      <WithdrawalCountdown v-if="prepared" :deadline="prepared.expires_at" :now="now" kind="verification" compact />
      <template v-if="!prepared">
        <label for="withdraw-address">Withdrawal address</label><select id="withdraw-address" v-model="addressId" class="form-select mb-3" :disabled="busy"><option value="">Select address</option><option v-for="a in availableAddresses" :key="a.id" :value="a.id">{{ a.address }}{{ remainingSeconds(a.eligible_at, now) === 0 ? '' : ' · Waiting' }}</option></select>
        <WithdrawalCountdown v-if="selectedAddress && addressWaiting" :deadline="selectedAddress.eligible_at" :now="now" compact />
        <p v-if="!availableAddresses.length" class="small">No verified address found. <RouterLink to="/wallet/security">Verify a withdrawal address</RouterLink>.</p>
        <label for="withdraw-amount">Amount in SIGNET (includes 3% fee)</label><input id="withdraw-amount" v-model="amount" inputmode="decimal" class="form-control mb-3" :disabled="busy" />
      </template>
      <template v-else>
        <dl class="withdrawal-totals">
          <div><dt>Requested</dt><dd>{{ withdrawalAmount(prepared.terms.gross) }} SIGNET</dd></div>
          <div><dt>Fee · 3%</dt><dd>{{ withdrawalAmount(prepared.terms.fee) }} SIGNET</dd></div>
          <div class="withdrawal-net"><dt>You receive</dt><dd>{{ withdrawalAmount(prepared.terms.net) }} SIGNET</dd></div>
        </dl>
        <div class="withdrawal-destination"><span class="small">Destination · BSC</span><p class="text-break">{{ prepared.terms.destination }}</p></div>
        <p v-if="previewExpired" role="status" class="alert alert-warning">Verification expired. Choose “Check status and start again” above to restart with the same amount and address.</p>
      </template>
      <template v-if="!prepared"><label for="withdraw-password">Current password</label><input id="withdraw-password" v-model="password" type="password" autocomplete="current-password" maxlength="72" class="form-control mb-3" :disabled="busy" /></template>
      <button v-if="!prepared" class="btn btn-primary" :disabled="busy || uncertain || !admissionChecked || pendingQueue || cooldownWaiting || !securityReady || accountWaiting || addressWaiting || !password || !selectedAddress || !/^[0-9]{1,18}(\.[0-9]{1,18})?$/.test(amount)" @click="act('prepare')">{{ pendingQueue ? 'Withdrawal already processing' : cooldownWaiting ? 'Withdrawal cooldown active' : accountWaiting || addressWaiting ? 'Waiting period active' : 'Review exact withdrawal' }}</button>
      <template v-else-if="stage === 'prepared'">
        <button class="btn btn-primary mb-3" :disabled="busy || uncertain || previewExpired || !password" @click="act('email')">{{ busy ? 'Please wait…' : challenge ? 'Resend email code' : 'Send verification email' }}</button>
        <template v-if="challenge"><div class="withdrawal-codes"><div><label for="withdraw-email">Email code</label><input ref="emailInput" id="withdraw-email" v-model="emailCode" maxlength="6" inputmode="numeric" autocomplete="one-time-code" class="form-control mb-3" :disabled="busy" /></div><div><label for="withdraw-auth">Authenticator code</label><input id="withdraw-auth" v-model="authCode" maxlength="6" inputmode="numeric" autocomplete="off" class="form-control mb-3" :disabled="busy" /></div></div><p class="small">Enter the newest email code and a fresh Authenticator code.</p><button class="btn btn-primary" :disabled="busy || uncertain || previewExpired || !password || !/^\d{6}$/.test(emailCode) || !/^\d{6}$/.test(authCode)" @click="act('confirm')">{{ busy ? 'Verifying and submitting…' : 'Verify and submit withdrawal' }}</button></template>
      </template>
      <button v-else-if="stage === 'authorized'" class="btn btn-primary" :disabled="busy || uncertain || !admissionChecked || pendingQueue || cooldownWaiting || previewExpired || !password" @click="act('queue')">Submit approved withdrawal</button>
    </section>
    <h2 class="h5 mt-4">Withdrawal activity</h2><p class="small">Updates automatically while this page is open and verification is not in progress. Submitted requests still need custody processing; only Completed confirms payment.</p>
    <button v-if="verificationHistory.length" class="btn btn-outline-secondary btn-sm mb-3" :aria-expanded="showVerificationHistory" @click="showVerificationHistory = !showVerificationHistory">{{ showVerificationHistory ? 'Hide' : 'Show' }} expired verification attempts ({{ verificationHistory.length }})</button>
    <p v-if="historyError" role="status" class="alert alert-warning">{{ historyError }}</p>
    <p v-if="!busy && !error && !visibleRequests.length" class="wallet-empty">No current withdrawal requests. Older attempts are available above.</p>
    <div class="withdrawal-list">
      <article v-for="r in visibleRequests" :key="r.id" class="withdrawal-record">
        <div class="withdrawal-heading">
          <strong>{{ withdrawalAmount(r.payout?.net_amount || r.terms.net) }} <small>SIGNET</small></strong>
          <span class="withdrawal-badge" :class="{ completed: r.payout?.status === 'completed' }">{{ withdrawalStatusLabel(r) }}</span>
        </div>
        <p class="small mb-2">Requested {{ withdrawalAmount(r.terms.gross) }} · Fee {{ withdrawalAmount(r.terms.fee) }} SIGNET</p>
        <p v-if="withdrawalRequestNotice(r)" class="small">{{ withdrawalRequestNotice(r) }}</p>
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
.withdrawal-method { gap: 4px; }
.withdrawal-method .form-control, .withdrawal-method .form-select { min-height: 44px; }
.withdrawal-method label { font-size: .9rem; margin-bottom: 6px; }
.withdrawal-method .btn { min-height: 44px; }
.withdrawal-totals { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 12px 0; }
.withdrawal-totals > div { padding: 12px; background: #0b1921; border-radius: 8px; overflow-wrap: anywhere; }
.withdrawal-totals dt { font-size: .8rem; color: #afc4cc; font-weight: 400; }
.withdrawal-totals dd { margin: 6px 0 0; font-weight: 600; }
.withdrawal-net { border: 1px solid #3f8d80; }
.withdrawal-destination { margin-bottom: 16px; }
.withdrawal-destination summary { cursor: pointer; }
.withdrawal-destination p { font-size: .85rem; margin: 8px 0; }
.withdrawal-codes { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
@media (max-width: 576px) { .withdrawal-totals { grid-template-columns: 1fr; gap: 6px; } .withdrawal-totals > div { display: flex; justify-content: space-between; gap: 12px; } .withdrawal-totals dd { margin: 0; } .withdrawal-codes { grid-template-columns: 1fr; gap: 0; } }

.withdrawal-steps { display: flex; flex-wrap: wrap; gap: 12px; padding: 0; list-style: none; margin: 0 0 20px; }
.withdrawal-steps li { padding: 8px 14px; border: 1px solid #526570; border-radius: 8px; }
.withdrawal-steps [aria-current="step"] { color: #8be8f2; border-color: #8be8f2; font-weight: 700; }
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
