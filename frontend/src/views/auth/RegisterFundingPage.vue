<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import api from '@/api/client'
import PurchaseActivation from '@/components/PurchaseActivation.vue'
import { useAuthStore } from '@/store/auth'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'
import QRCode from 'qrcode'
import PartialDepositReminder from '@/components/PartialDepositReminder.vue'
import { usdtDisplay, usdtDeficitDisplay } from '@/utils/usdtDisplay'
import { activationAction } from '@/utils/purchaseActivation'
import RegistrationProgress from '@/components/RegistrationProgress.vue'

const authStore = useAuthStore()
const checking = ref(false)
const loadingFunding = ref(false)
function activationBusy(value) {
  checking.value = value
  if (value) clearTimeout(timer)
}
const wallet = ref(null)
const fundingReady = computed(() => activationAction(purchase.value, wallet.value)?.ready && !error.value)
const purchase = ref(null)
const error = ref('')
const qr = ref('')
const deposits = ref([])
const historyError = ref('')
const copyMessage = ref('')
async function copyAddress() {
  try { await navigator.clipboard.writeText(wallet.value.address); copyMessage.value = 'Address copied' }
  catch { copyMessage.value = 'Copy unavailable. Select and copy the address below.' }
}
let timer
let stopped = false
let refreshInFlight = null
async function refresh() {
  if (stopped || checking.value) return
  if (refreshInFlight) return refreshInFlight
  loadingFunding.value = true
  refreshInFlight = loadFunding()
  try { await refreshInFlight } finally { refreshInFlight = null; loadingFunding.value = false }
}
async function loadFunding() {
  clearTimeout(timer)
  try {
    const [w, p] = await Promise.all([api.post('/blockchain/wallet'), api.get('/blockchain/purchase')])
    if (stopped) return
    wallet.value = w.data.wallet
    purchase.value = p.data.purchase
    if (purchase.value?.status === 'completed') await authStore.bootstrap()
    error.value = ''
    qr.value = wallet.value?.address ? await QRCode.toDataURL(wallet.value.address) : ''
    try {
      const { data } = await api.get('/blockchain/deposits')
      if (!stopped) { deposits.value = data.deposits || []; historyError.value = '' }
    } catch { if (!stopped) historyError.value = 'Deposit history is temporarily unavailable.' }
  } catch (err) {
    if (!stopped) error.value = err?.response?.data?.message || 'Funding service unavailable. Your account is preserved; please retry.'
  } finally {
    if (!stopped && !checking.value && purchase.value?.status !== 'completed') timer = setTimeout(refresh, 15000)
  }
}
onMounted(refresh)
onUnmounted(() => { stopped = true; clearTimeout(timer) })
</script>

<template>
  <AuthCardLayout back-text="Back to login" back-to="/login" card-class="funding-card p-4 p-lg-5 w-100">
    <PartialDepositReminder :purchase="fundingReady ? null : purchase" />
    <RegistrationProgress :step="4" />
    <div class="funding-kicker">SIGNET · PACKAGE ACTIVATION</div>
    <h1 class="h3 mb-2">Fund your package</h1>
    <p class="funding-intro">Fund your package with USDT. We’ll handle activation after your deposit is confirmed.</p>
    <div v-if="purchase?.required_usdt" class="funding-total">
      <span>Total required deposit</span>
      <strong :title="purchase.required_usdt">{{ usdtDisplay(purchase.required_usdt) }} <small>USDT</small></strong>
      <p>Includes your package and service fee</p>
    </div>
    <p v-if="error" class="alert alert-warning" role="alert">{{ error }}</p>
    <template v-if="wallet">
      <div class="funding-network">USDT <span>BSC · Chain {{ wallet.chain_id }}</span></div>
      <p v-if="wallet.provisioning_status !== 'ready'">Your deposit address is being prepared.</p>
      <template v-else>
        <div class="funding-address-panel">
          <img v-if="qr" :src="qr" alt="Personal USDT deposit address QR code" width="176" height="176" />
          <div><h2 class="h6">Your personal deposit address</h2><p>Scan with your wallet, or copy the address.</p>
            <code class="funding-address">{{ wallet.address }}</code>
            <button type="button" class="btn btn-primary mt-3" @click="copyAddress">Copy address</button>
            <p v-if="copyMessage" class="small mt-2 mb-0" role="status">{{ copyMessage }}</p>
          </div>
        </div>
        <p class="funding-safety">Send only USDT on this BSC network. Other assets or networks may not be recoverable.</p>
      </template>
    </template>
    <template v-if="purchase">
      <dl class="funding-breakdown">
        <div><dt>Package</dt><dd :title="purchase.package_price">{{ usdtDisplay(purchase.package_price) }} USDT</dd></div>
        <div v-if="purchase.service_fee_usdt"><dt>Service fee</dt><dd :title="purchase.service_fee_usdt">{{ usdtDisplay(purchase.service_fee_usdt) }} USDT</dd></div>
        <div><dt>Confirmed available</dt><dd :title="wallet?.available ?? purchase.available_usdt">{{ usdtDisplay(wallet?.available ?? purchase.available_usdt) }} USDT</dd></div>
      </dl>
      <div v-if="purchase.status === 'awaiting_deposit' && (purchase.last_sync_error || !purchase.last_attempt_at)" class="alert alert-warning" role="status">
        <strong>Waiting for automatic activation checks</strong>
        <p class="mb-0 mt-1">{{ purchase.last_sync_error === 'placement_required' ? 'Your placement needs review before this purchase can proceed.' : purchase.last_sync_error ? 'The activation service could not complete its latest check. Your funds remain recorded. Contact support if this persists.' : 'The activation worker has not recorded its first check yet.' }}</p>
        <p class="small mt-2 mb-0">If your deposit is credited below, do not send another payment just because the package status has not updated.</p>
      </div>
      <div v-else-if="purchase.status === 'awaiting_deposit' && fundingReady" class="alert alert-info" role="status">
        Your confirmed USDT balance covers the package and service fee. Activation runs automatically, or you can use the button below. No further deposit is needed.
      </div>
      <div v-else-if="purchase.status === 'awaiting_deposit'" class="alert alert-info" role="status">
        <strong :title="purchase.remaining_usdt">{{ usdtDeficitDisplay(purchase.remaining_usdt) }} USDT remaining</strong>
        <p class="mb-0 mt-1">Deposit the remaining amount. Activation starts automatically once sufficient funds are confirmed.</p>
        <details class="small mt-2"><summary>Exact remaining amount</summary>{{ purchase.remaining_usdt }} USDT</details>
        <p v-if="purchase.deposit_seen" class="mb-0 mt-2">Your deposit has been recorded. Your account and placement are protected while you complete funding.</p>
      </div>
      <div v-else-if="purchase.status === 'completed'" class="alert alert-success" role="status">
        <template v-if="purchase.purchase_kind === 'topup'">Your top-up is complete and your income cap has increased. No purchaser bonus or new vesting was created.</template>
        <template v-else>Your package is active. SIGNET has been allocated to your custody and vesting schedule.</template>
        Excess USDT remains available in your wallet.
      </div>
      <div v-else-if="purchase.status === 'awaiting_cap_review'" class="alert alert-warning" role="status">
        Funding is confirmed, but activation is held for a team income-cap check. Your funds and financial history are preserved. Do not deposit again for this hold; contact support.
      </div>
      <div v-else-if="purchase.status === 'expired'" class="alert alert-warning">This purchase has expired. Do not send funds to its old deposit address. Contact support if you already sent a payment.</div>
      <div v-else class="alert alert-info" role="status">Your package is processing automatically: {{ purchase.status }}. No further purchase action is needed.</div>
    </template>
    <PurchaseActivation :purchase="purchase" :wallet="wallet" :disabled="loadingFunding || !!error" @refresh="refresh" @busy="activationBusy" />
    <RouterLink v-if="purchase?.status === 'completed'" :to="authStore.dashboardRoute" class="btn btn-primary">Continue to dashboard</RouterLink>
    <h2 class="h5 mt-4">Recent USDT deposits</h2>
    <p v-if="historyError" role="status">{{ historyError }}</p>
    <p v-if="!historyError && !deposits.length" class="funding-empty">Waiting for your first deposit.<br /><small>It will appear here once detected. You don’t need to submit a payment receipt.</small></p>
    <div v-for="deposit in deposits" :key="`${deposit.id}:${deposit.log_index}`" class="border rounded p-2 mb-2">
      <p class="mb-1" :title="deposit.amount">{{ usdtDisplay(deposit.amount) }} USDT — {{ deposit.status }} · Confirmations: {{ deposit.confirmations }}</p>
      <p class="text-break mb-1">Transaction: {{ deposit.tx_hash }}</p>
      <p class="mb-0">{{ deposit.credited_at ? 'Credited to your ledger' : 'Not yet credited' }} · {{ deposit.swept_at ? 'Swept to treasury (your balance entitlement is unchanged)' : 'Sweep pending' }}</p>
    </div>
  </AuthCardLayout>
</template>

<style scoped>
:deep(.funding-card) { max-width: 760px; margin: 24px 0; border: 1px solid var(--sg-line-3); border-radius: 24px; background: radial-gradient(ellipse at top right,rgba(0,240,255,.08),transparent 60%),var(--sg-panel); box-shadow: var(--sg-shadow); }
.funding-kicker { color: var(--sg-accent-light); font: 600 11px var(--sg-font-mono); letter-spacing: .14em; margin-bottom: 14px; }
.funding-intro { color: var(--sg-muted); max-width: 530px; }
.funding-total { padding: 22px; border-radius: 16px; border: 1px solid rgba(0,240,255,.22); background: rgba(0,240,255,.05); margin: 24px 0; }
.funding-total > span { color: var(--sg-muted); font-size: 13px; }
.funding-total strong { display:block; font-size:clamp(28px,5vw,40px); font-variant-numeric:tabular-nums; letter-spacing:-.03em; overflow-wrap:anywhere; }
.funding-total small { font-size: 14px; color:var(--sg-accent-light); letter-spacing:0; }
.funding-total p { font-size:12px; margin:4px 0 0; color:var(--sg-muted); }
.funding-network { display:flex; gap:12px; align-items:center; font-weight:700; margin-bottom:16px; }
.funding-network span { font-size:12px; color:var(--sg-accent-light); padding:5px 10px; border:1px solid var(--sg-line-3); border-radius:99px; }
.funding-address-panel { display:grid; grid-template-columns:176px minmax(0,1fr); gap:24px; align-items:center; }
.funding-address-panel img { border-radius:14px; background:white; padding:8px; max-width:100%; height:auto; }
.funding-address-panel p { color:var(--sg-muted); font-size:13px; }
.funding-address { display:block; overflow-wrap:anywhere; user-select:all; font-size:12px; line-height:1.8; }
.funding-safety { color:var(--sg-warn); font-size:12px; margin:18px 0 24px; }
.funding-breakdown { border-top:1px solid var(--sg-line-3); padding-top:16px; }
.funding-breakdown > div { display:flex; justify-content:space-between; gap:14px; padding:7px 0; }
.funding-breakdown dt { color:var(--sg-muted); font-weight:400; }
.funding-breakdown dd { text-align:right; font-variant-numeric:tabular-nums; margin:0; overflow-wrap:anywhere; }
.funding-empty { border:1px dashed var(--sg-line-3); border-radius:14px; padding:20px; color:var(--sg-muted); }
.funding-card button:focus-visible,.funding-card summary:focus-visible { outline:2px solid var(--sg-accent-light); outline-offset:4px; }
@media(max-width:575px) { .funding-address-panel { grid-template-columns:1fr; text-align:center; } .funding-address-panel img { margin:auto; } .funding-breakdown { font-size:13px; } }
</style>
