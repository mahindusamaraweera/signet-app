<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import QRCode from 'qrcode'

const wallet = ref(null)
const qr = ref('')
const deposits = ref([])
const nextCursor = ref(null)
const error = ref('')
const historyError = ref('')
const busy = ref(false)
const historyBusy = ref(false)
const withdrawals = ref([])
const withdrawalCursor = ref(null)
const withdrawalError = ref('')
const withdrawalBusy = ref(false)
const purchase = ref(null)
const purchaseError = ref('')
let stopped = false
let timer

async function history(older = false) {
  if (historyBusy.value) return
  historyBusy.value = true
  try {
    const { data } = await api.get('/blockchain/deposits', { params: older ? { before_id: nextCursor.value } : {} })
    if (stopped) return
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
    withdrawals.value = older ? [...withdrawals.value, ...(data.withdrawals || [])] : data.withdrawals || []
    withdrawalCursor.value = data.next_before_id
    withdrawalError.value = ''
  } catch (err) {
    if (!stopped) withdrawalError.value = err?.response?.data?.message || 'Withdrawal history is temporarily unavailable. Any records below are the last loaded snapshot.'
  } finally { withdrawalBusy.value = false }
}

async function refresh() {
  if (busy.value) return
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
    <h1 class="h3">My deposit wallet</h1>
    <p>This wallet belongs to your existing account. Provisioning it does not buy a package, restart vesting or change your income cap.</p>
    <p v-if="error" class="alert alert-warning" role="alert">{{ error }}</p>
    <button class="btn btn-primary mb-3" :disabled="busy || historyBusy" @click="refresh">{{ busy ? 'Loading…' : 'Refresh wallet' }}</button>
    <section v-if="wallet" class="card p-3 mb-3">
      <h2 class="h5">USDT on BSC — chain {{ wallet.chain_id }}</h2>
      <p>Confirmed available USDT: {{ wallet.available }}</p>
      <div v-if="wallet.balances" class="mb-3">
        <h3 class="h6">Confirmed ledger balances</h3>
        <p v-for="asset in ['USDT', 'SIGNET']" :key="asset" class="mb-1">{{ asset }} — Available: {{ wallet.balances[asset].available }} · Locked: {{ wallet.balances[asset].locked }} · Pending withdrawal: {{ wallet.balances[asset].pending_withdrawal }}</p>
        <p class="small text-muted">Available SIGNET includes only settled ledger credits. Locked vesting and the per-second progress estimate are not spendable. Withdrawal availability depends on security setup and enabled processing.</p>
      </div>
      <template v-if="wallet.provisioning_status === 'ready'">
        <img v-if="qr" :src="qr" alt="Personal deposit address QR code" width="200" height="200" />
        <p class="text-break mt-2"><code>{{ wallet.address }}</code></p>
        <p>Send only USDT using the configured BSC network. Deposits require blockchain confirmation before credit.</p>
        <p>Select a top-up package before funding it. Unassigned deposits remain in your wallet and do not automatically extend your income cap.</p>
      </template>
      <p v-else role="status">Your address is being prepared. Refresh to check again.</p>
    </section>
    <p v-if="purchaseError" class="alert alert-warning" role="status">{{ purchaseError }}</p>
    <section v-if="purchase" class="card p-3 mb-3">
      <h2 class="h5">{{ purchase.purchase_kind === 'topup' ? 'Top-up' : 'Package purchase' }}</h2>
      <p>Package: {{ purchase.package_price }} USDT</p>
      <p v-if="purchase.service_fee_usdt">Service fee: {{ purchase.service_fee_usdt }} USDT</p>
      <p v-if="purchase.required_usdt">Required deposit: {{ purchase.required_usdt }} USDT</p>
      <p v-if="purchase.placement_required" class="alert alert-warning" role="status">Your deposit wallet and funds remain preserved. A new available placement is required before package activation; depositing more does not restore the old placement.</p>
      <p v-else-if="purchase.status === 'awaiting_deposit'" role="status">Deposit another {{ purchase.remaining_usdt }} USDT. Any confirmed partial deposit preserves your account.</p>
      <p v-else-if="purchase.status === 'awaiting_reopening' && purchase.reopening_status === 'review_required'" class="alert alert-warning" role="status">Replacement confirmation requires support review. Funds remain preserved and package funding is blocked. Do not submit another replacement or deposit again to resolve this state.</p>
      <p v-else-if="purchase.status === 'awaiting_reopening'" role="status">Your replacement placement is awaiting custody confirmation. Funds remain preserved; package funding will remain blocked until confirmation. Do not submit another replacement.</p>
      <p v-else-if="purchase.status === 'awaiting_commission_settlement'" role="status">Funding is confirmed. Team commissions and company USDT settlement are processing. Your cap will extend after settlement; do not deposit again for this purchase.</p>
      <p v-else-if="purchase.status === 'completed'" role="status">{{ purchase.purchase_kind === 'topup' ? 'Top-up complete: your income cap has increased. No new purchaser bonus or vesting was created.' : 'Package active: your first-purchase allocation is managed in your vesting wallet.' }}</p>
      <p v-else role="status">Purchase status: {{ purchase.status }}. Funds and financial records remain preserved.</p>
    </section>
    <h2 class="h5">Deposit history</h2>
    <p v-if="historyError" class="alert alert-warning" role="alert">{{ historyError }}</p>
    <p v-if="!historyBusy && !deposits.length">No deposit records loaded.</p>
    <section v-for="deposit in deposits" :key="deposit.id" class="border rounded p-2 mb-2">
      <p>{{ deposit.amount }} USDT · {{ deposit.status }} · {{ deposit.confirmations }} confirmations</p>
      <p class="text-break">Transaction: {{ deposit.tx_hash }} · Log {{ deposit.log_index }}</p>
      <p class="mb-0">{{ deposit.swept_at ? 'Swept to treasury; your ledger entitlement is retained.' : 'Not yet swept.' }}</p>
    </section>
    <button v-if="nextCursor" class="btn btn-secondary" :disabled="historyBusy || busy" @click="history(true)">Load older deposits</button>
    <h2 class="h5 mt-4">Withdrawal history</h2>
    <p class="small text-muted">SIGNET withdrawals pay 97% to the user. The deducted 3% is converted to USDT for the fee address when fee processing is enabled; its confirmation is shown separately below.</p>
    <p v-if="withdrawalError" class="alert alert-warning" role="alert">{{ withdrawalError }}</p>
    <p v-if="!withdrawalBusy && !withdrawalError && !withdrawals.length">No withdrawal records loaded.</p>
    <section v-for="withdrawal in withdrawals" :key="withdrawal.id" class="border rounded p-2 mb-2">
      <p>{{ withdrawal.amount }} {{ withdrawal.asset }} · Fee {{ withdrawal.fee }} · Net {{ withdrawal.net_amount }} · {{ withdrawal.status }}</p>
      <p class="text-break">Destination: {{ withdrawal.to_address }}</p>
      <p class="text-break mb-0">{{ withdrawal.tx_hash ? `Transaction: ${withdrawal.tx_hash}` : 'No transaction hash recorded yet.' }}</p>
      <div v-if="withdrawal.fee_payment" class="small mt-2">
        <p class="mb-1">Fee conversion: {{ withdrawal.fee_payment.status }} · {{ withdrawal.fee_payment.signet_amount }} SIGNET · {{ withdrawal.fee_payment.received_usdt == null ? 'USDT receipt not confirmed' : `${withdrawal.fee_payment.received_usdt} USDT received` }}</p>
        <p class="text-break mb-1">Fee destination: {{ withdrawal.fee_payment.to_address }}</p>
        <p class="text-break mb-0">{{ withdrawal.fee_payment.tx_hash ? `Fee transaction: ${withdrawal.fee_payment.tx_hash}` : 'Fee transaction not submitted yet.' }}</p>
      </div>
      <p v-else class="small mt-2 mb-0">No separate fee-conversion record is available for this withdrawal.</p>
    </section>
    <button v-if="withdrawalCursor" class="btn btn-secondary" :disabled="withdrawalBusy || busy" @click="withdrawalHistory(true)">Load older withdrawals</button>
    <RouterLink to="/wallet/vesting" class="btn btn-link">View SIGNET vesting</RouterLink>
    <RouterLink to="/wallet/tier-link" class="btn btn-link">Connect my Tier account</RouterLink>
  </DashboardLayout>
</template>
