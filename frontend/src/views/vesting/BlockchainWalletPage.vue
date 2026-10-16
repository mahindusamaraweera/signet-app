<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import QRCode from 'qrcode'
import AssetAmount from '@/components/AssetAmount.vue'
import { usdtDeficitDisplay } from '@/utils/usdtDisplay'
import { activationAction } from '@/utils/purchaseActivation'
import PurchaseActivation from '@/components/PurchaseActivation.vue'
import PartialDepositReminder from '@/components/PartialDepositReminder.vue'

const wallet = ref(null)
const qr = ref('')
const copyMessage = ref('')
async function copyAddress() {
  try {
    await navigator.clipboard.writeText(wallet.value.address)
    copyMessage.value = 'Address copied'
  } catch { copyMessage.value = 'Select the address below and copy it from your browser.' }
}
function statusLabel(value) { return String(value || 'Pending').replaceAll('_', ' ') }
const deposits = ref([])
const historyLoaded = ref(false)
const withdrawalsLoaded = ref(false)
const nextCursor = ref(null)
const error = ref('')
const historyError = ref('')
const busy = ref(false)
const activating = ref(false)
const historyBusy = ref(false)
const withdrawals = ref([])
const withdrawalCursor = ref(null)
const withdrawalError = ref('')
const withdrawalBusy = ref(false)
const purchase = ref(null)
const purchaseError = ref('')
const fundingReady = computed(() => activationAction(purchase.value, wallet.value)?.ready && !error.value && !purchaseError.value)
let stopped = false
let timer

async function history(older = false) {
  if (historyBusy.value) return
  historyBusy.value = true
  try {
    const { data } = await api.get('/blockchain/deposits', { params: older ? { before_id: nextCursor.value } : {} })
    if (stopped) return
    historyLoaded.value = true
    deposits.value = older ? [...deposits.value, ...(data.deposits || [])] : data.deposits || []
    nextCursor.value = data.next_before_id
    historyError.value = ''
  } catch (err) { if (!stopped) historyError.value = err?.response?.data?.message || 'Could not load deposit history.' }
  finally { historyBusy.value = false }
}

async function withdrawalHistory(older = false) {
  if (withdrawalBusy.value || stopped) return
  withdrawalBusy.value = true
  try {
    const { data } = await api.get('/blockchain/withdrawals', { params: older ? { before_id: withdrawalCursor.value } : {} })
    if (stopped) return
    withdrawalsLoaded.value = true
    withdrawals.value = older ? [...withdrawals.value, ...(data.withdrawals || [])] : data.withdrawals || []
    withdrawalCursor.value = data.next_before_id
    withdrawalError.value = ''
  } catch (err) {
    if (!stopped) withdrawalError.value = err?.response?.data?.message || 'Withdrawal history is temporarily unavailable. Any records below are the last loaded snapshot.'
  } finally { withdrawalBusy.value = false }
}

async function refresh() {
  if (stopped || busy.value || activating.value) return
  busy.value = true
  try {
    const { data } = await api.post('/blockchain/wallet')
    if (stopped) return
    wallet.value = data.wallet
    qr.value = wallet.value?.provisioning_status === 'ready' ? await QRCode.toDataURL(wallet.value.address) : ''
    error.value = ''
    try {
      const response = await api.get('/blockchain/purchase')
      if (!stopped) { purchase.value = response.data.purchase; purchaseError.value = '' }
    } catch (err) {
      if (!stopped) {
        if (err?.response?.status === 404) { purchase.value = null; purchaseError.value = '' }
        else purchaseError.value = 'Purchase status is temporarily unavailable. Any status below is the last confirmed snapshot.'
      }
    }
    await Promise.all([history(), withdrawalHistory()])
  } catch (err) { error.value = err?.response?.data?.message || 'Could not load your deposit wallet.' }
  finally {
    busy.value = false
    clearTimeout(timer)
    if (!stopped) timer = setTimeout(refresh, purchase.value && purchase.value.status !== 'completed' ? 15000 : 60000)
  }
}
onMounted(refresh)
onUnmounted(() => { stopped = true; clearTimeout(timer) })
</script>

<template>
  <DashboardLayout>
    <PartialDepositReminder :purchase="fundingReady ? null : purchase" />
    <div class="page-heading">
      <div><p class="wallet-eyebrow">DEPOSIT & ACTIVITY</p><h1>My deposit wallet</h1><p>Fund your package and track your confirmed balances.</p></div>
      <button class="btn btn-outline-primary" :disabled="busy || activating || historyBusy" @click="refresh">{{ busy ? 'Refreshing…' : 'Refresh wallet' }}</button>
    </div>
    <p v-if="error" class="alert alert-warning" role="alert">{{ error }}</p>
    <p v-if="busy && !wallet" class="wallet-empty" role="status">Loading your wallet…</p>
    <div v-if="wallet" class="balance-grid">
      <section class="balance-tile balance-tile-accent"><span>Available USDT</span><strong><AssetAmount :value="wallet.available" asset="USDT" /></strong><small>Confirmed funds for your package</small></section>
      <section class="balance-tile"><span>Available SIGNET</span><strong><AssetAmount :value="wallet.balances?.SIGNET?.available" asset="SIGNET" /></strong><small>Settled credits in your wallet</small></section>
      <section class="balance-tile"><span>Locked SIGNET</span><strong><AssetAmount :value="wallet.balances?.SIGNET?.locked" asset="SIGNET" /></strong><small>Check Tier eligibility and releases in <RouterLink to="/wallet/vesting">vesting</RouterLink></small></section>
    </div>
    <div class="deposit-workspace">
    <section v-if="wallet" class="card wallet-panel">
      <div class="panel-heading"><h2>Deposit USDT</h2><span class="wallet-network-note">BSC · Chain {{ wallet.chain_id }}</span></div>
      <template v-if="wallet.provisioning_status === 'ready'">
        <div class="deposit-address-layout">
          <img v-if="qr" :src="qr" alt="Personal deposit address QR code" width="176" height="176" />
          <div class="deposit-address-details"><h3>Your personal address</h3><p>Scan with your wallet or copy the address below.</p><code class="address-box">{{ wallet.address }}</code><button class="btn btn-outline-primary mt-3" @click="copyAddress">Copy address</button><p v-if="copyMessage" class="small mt-2 mb-0" role="status">{{ copyMessage }}</p></div>
        </div>
        <div class="network-guidance"><strong>Send USDT on BSC only</strong><p>Funds appear after blockchain confirmation. Select a package before funding; unassigned deposits stay in your wallet.</p></div>
      </template>
      <p v-else class="wallet-empty" role="status">Your deposit address is being prepared. Refresh to check again.</p>
      <details v-if="wallet.balances" class="ledger-details"><summary>View all ledger balances</summary><div class="table-responsive"><table class="table"><caption class="small">Only available settled credits are spendable. Withdrawal security and processing requirements apply.</caption><thead><tr><th>Asset</th><th>Available</th><th>Locked</th><th>Pending withdrawal</th></tr></thead><tbody><tr v-for="asset in ['USDT', 'SIGNET']" :key="asset"><th>{{ asset }}</th><td><AssetAmount :value="wallet.balances[asset]?.available" :asset="asset" /></td><td><AssetAmount :value="wallet.balances[asset]?.locked" :asset="asset" /></td><td><AssetAmount :value="wallet.balances[asset]?.pending_withdrawal" :asset="asset" /></td></tr></tbody></table></div></details>
    </section>
    <section v-if="wallet && !busy && !error && !purchase && !purchaseError" class="card wallet-panel"><p class="wallet-eyebrow">PACKAGE FUNDING</p><h2>No pending package</h2><p>Your wallet keeps any unassigned funds. Choose a top-up to extend your income cap.</p><RouterLink to="/buy-package" class="btn btn-primary">Choose a package</RouterLink></section>
    <p v-if="purchaseError" class="alert alert-warning" role="status">{{ purchaseError }}</p>
    <section v-if="purchase" class="card wallet-panel purchase-panel">
      <p class="wallet-eyebrow">PACKAGE FUNDING</p>
      <h2>{{ purchase.purchase_kind === 'topup' ? 'Top-up' : 'Package purchase' }}</h2>
      <dl class="purchase-breakdown"><div><dt>Selected package</dt><dd><AssetAmount :value="purchase.package_price" asset="USDT" /></dd></div><div v-if="purchase.service_fee_usdt"><dt>Service fee</dt><dd><AssetAmount :value="purchase.service_fee_usdt" asset="USDT" /></dd></div><div v-if="purchase.required_usdt" class="purchase-total"><dt>Total required</dt><dd><AssetAmount :value="purchase.required_usdt" asset="USDT" /></dd></div></dl>
      <p v-if="purchase.placement_required" class="alert alert-warning" role="status">Your deposit wallet and funds remain preserved. A new available placement is required before package activation; depositing more does not restore the old placement.</p>
      <p v-else-if="purchase.status === 'awaiting_deposit' && fundingReady" role="status">Your confirmed balance covers this package and its service fee. Activate below or wait for automatic activation. No additional deposit is needed.</p>
      <div v-else-if="purchase.status === 'awaiting_deposit'" role="status">Remaining deposit: <strong :title="purchase.remaining_usdt">{{ usdtDeficitDisplay(purchase.remaining_usdt) }} USDT</strong>. Any confirmed partial deposit preserves your account.<details class="small mt-2"><summary>Exact remaining amount</summary>{{ purchase.remaining_usdt }} USDT</details></div>
      <p v-else-if="purchase.status === 'awaiting_reopening' && purchase.reopening_status === 'review_required'" class="alert alert-warning" role="status">Replacement confirmation requires support review. Funds remain preserved and package funding is blocked. Do not submit another replacement or deposit again to resolve this state.</p>
      <p v-else-if="purchase.status === 'awaiting_reopening'" role="status">Your replacement placement is awaiting custody confirmation. Funds remain preserved; package funding will remain blocked until confirmation. Do not submit another replacement.</p>
      <p v-else-if="purchase.status === 'awaiting_commission_settlement'" role="status">Funding is confirmed. Team commissions and company USDT settlement are processing. Your cap will extend after settlement; do not deposit again for this purchase.</p>
      <p v-else-if="purchase.status === 'completed'" role="status">{{ purchase.purchase_kind === 'topup' ? 'Top-up complete: your income cap has increased. No new purchaser bonus or vesting was created.' : 'Package active: purchased SIGNET and bonus are locked in your custodial wallet. A qualifying Tier plan is required to start vesting.' }}</p>
      <p v-else role="status">Purchase status: {{ purchase.status }}. Funds and financial records remain preserved.</p>
      <PurchaseActivation :purchase="purchase" :wallet="wallet" :disabled="busy || !!error || !!purchaseError" @refresh="refresh" @busy="activating = $event" />
    </section>
    </div>
    <section class="card wallet-panel activity-panel">
    <div class="panel-heading"><h2>Deposit history</h2><span class="small text-muted">{{ historyLoaded ? `${deposits.length} loaded` : 'Not loaded' }}</span></div>
    <p v-if="historyError" class="alert alert-warning" role="alert">{{ historyError }}</p>
    <p v-if="historyLoaded && !historyBusy && !historyError && !deposits.length" class="wallet-empty">No deposits yet. Your confirmed transactions will appear here.</p>
    <p v-if="!historyLoaded && !historyBusy" class="wallet-empty" role="status">Deposit history has not loaded. This does not indicate a zero balance.</p>
    <section v-for="deposit in deposits" :key="deposit.id" class="activity-record">
      <div class="activity-record-heading"><strong><AssetAmount :value="deposit.amount" asset="USDT" /></strong><span class="wallet-status" :class="{ settled: ['credited', 'confirmed', 'swept'].includes(deposit.status) }">{{ statusLabel(deposit.status) }}</span></div><p class="small text-muted">{{ deposit.confirmations }} confirmations</p>
      <p class="text-break">Transaction: {{ deposit.tx_hash }} · Log {{ deposit.log_index }}</p>
      <p class="mb-0">{{ deposit.swept_at ? 'Swept to treasury; your ledger entitlement is retained.' : 'Not yet swept.' }}</p>
    </section>
    <button v-if="nextCursor" class="btn btn-secondary" :disabled="historyBusy || busy" @click="history(true)">Load older deposits</button>
    </section>
    <section class="card wallet-panel activity-panel">
    <div class="panel-heading"><h2>Withdrawal history</h2><RouterLink to="/wallet/withdraw">Withdraw SIGNET →</RouterLink></div>
    <p class="small text-muted">SIGNET withdrawals pay 97% to the user. The deducted 3% is converted to USDT for the fee address when fee processing is enabled; its confirmation is shown separately below.</p>
    <p v-if="withdrawalError" class="alert alert-warning" role="alert">{{ withdrawalError }}</p>
    <p v-if="withdrawalsLoaded && !withdrawalBusy && !withdrawalError && !withdrawals.length" class="wallet-empty">No withdrawals yet. Track your payouts here once requested.</p>
    <section v-for="withdrawal in withdrawals" :key="withdrawal.id" class="activity-record">
      <div class="activity-record-heading"><strong><AssetAmount :value="withdrawal.net_amount" :asset="withdrawal.asset" /></strong><span class="wallet-status" :class="{ settled: withdrawal.status === 'confirmed' }">{{ statusLabel(withdrawal.status) }}</span></div><p class="small">Gross <AssetAmount :value="withdrawal.amount" :asset="withdrawal.asset" /> · Fee <AssetAmount :value="withdrawal.fee" :asset="withdrawal.asset" /></p>
      <p class="text-break">Destination: {{ withdrawal.to_address }}</p>
      <p class="text-break mb-0">{{ withdrawal.tx_hash ? `Transaction: ${withdrawal.tx_hash}` : 'No transaction hash recorded yet.' }}</p>
      <div v-if="withdrawal.fee_payment" class="small mt-2">
        <p class="mb-1">Fee conversion: {{ statusLabel(withdrawal.fee_payment.status) }} · <AssetAmount :value="withdrawal.fee_payment.signet_amount" asset="SIGNET" /> · <span v-if="withdrawal.fee_payment.received_usdt == null">USDT receipt not confirmed</span><span v-else><AssetAmount :value="withdrawal.fee_payment.received_usdt" asset="USDT" /> received</span></p>
        <p class="text-break mb-1">Fee destination: {{ withdrawal.fee_payment.to_address }}</p>
        <p class="text-break mb-0">{{ withdrawal.fee_payment.tx_hash ? `Fee transaction: ${withdrawal.fee_payment.tx_hash}` : 'Fee transaction not submitted yet.' }}</p>
      </div>
      <p v-else class="small mt-2 mb-0">No separate fee-conversion record is available for this withdrawal.</p>
    </section>
    <button v-if="withdrawalCursor" class="btn btn-secondary" :disabled="withdrawalBusy || busy" @click="withdrawalHistory(true)">Load older withdrawals</button>
    </section>
    <RouterLink to="/wallet/vesting" class="btn btn-link">View SIGNET vesting</RouterLink>
    <RouterLink to="/wallet/tier-link" class="btn btn-link">Connect my Tier account</RouterLink>
  </DashboardLayout>
</template>
