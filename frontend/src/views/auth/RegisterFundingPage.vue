<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import api from '@/api/client'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'
import QRCode from 'qrcode'
import PartialDepositReminder from '@/components/PartialDepositReminder.vue'

const wallet = ref(null)
const purchase = ref(null)
const error = ref('')
const qr = ref('')
const deposits = ref([])
const historyError = ref('')
let timer
let stopped = false
async function refresh() {
  try {
    const [w, p] = await Promise.all([api.post('/blockchain/wallet'), api.get('/blockchain/purchase')])
    if (stopped) return
    wallet.value = w.data.wallet
    purchase.value = p.data.purchase
    error.value = ''
    qr.value = wallet.value?.address ? await QRCode.toDataURL(wallet.value.address) : ''
    try {
      const { data } = await api.get('/blockchain/deposits')
      if (!stopped) { deposits.value = data.deposits || []; historyError.value = '' }
    } catch { if (!stopped) historyError.value = 'Deposit history is temporarily unavailable.' }
  } catch (err) {
    if (!stopped) error.value = err?.response?.data?.message || 'Funding service unavailable. Your account is preserved; please retry.'
  } finally {
    if (!stopped && purchase.value?.status !== 'completed') timer = setTimeout(refresh, 15000)
  }
}
onMounted(refresh)
onUnmounted(() => { stopped = true; clearTimeout(timer) })
</script>

<template>
  <AuthCardLayout back-text="Back to login" back-to="/login">
    <PartialDepositReminder :purchase="purchase" />
    <h1 class="h3 mb-3">Fund your package</h1>
    <p v-if="error" class="alert alert-warning" role="alert">{{ error }}</p>
    <template v-if="wallet">
      <p>Send USDT on BSC (chain {{ wallet.chain_id }}) to your personal deposit address.</p>
      <p v-if="wallet.provisioning_status !== 'ready'">Your deposit address is being prepared.</p>
      <template v-else>
        <img v-if="qr" :src="qr" alt="Deposit address QR code" width="200" height="200" />
        <p class="text-break"><code>{{ wallet.address }}</code></p>
      </template>
    </template>
    <template v-if="purchase">
      <p>Package price: {{ purchase.package_price }} USDT</p>
      <p v-if="purchase.service_fee_usdt">Service fee: {{ purchase.service_fee_usdt }} USDT</p>
      <p v-if="purchase.required_usdt">Total required deposit: {{ purchase.required_usdt }} USDT</p>
      <p>Available USDT: {{ purchase.available_usdt }}</p>
      <div v-if="purchase.status === 'awaiting_deposit'" class="alert alert-info" role="status">
        Deposit another {{ purchase.remaining_usdt }} USDT to complete automatic package activation.
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
    <RouterLink v-if="purchase?.status === 'completed'" to="/login" class="btn btn-primary">Continue to login</RouterLink>
    <h2 class="h5 mt-4">Recent USDT deposits</h2>
    <p v-if="historyError" role="status">{{ historyError }}</p>
    <p v-if="!historyError && !deposits.length">No deposits detected yet. A payment appears here after the blockchain scanner detects it.</p>
    <div v-for="deposit in deposits" :key="`${deposit.id}:${deposit.log_index}`" class="border rounded p-2 mb-2">
      <p class="mb-1">{{ deposit.amount }} USDT — {{ deposit.status }} · Confirmations: {{ deposit.confirmations }}</p>
      <p class="text-break mb-1">Transaction: {{ deposit.tx_hash }}</p>
      <p class="mb-0">{{ deposit.credited_at ? 'Credited to your ledger' : 'Not yet credited' }} · {{ deposit.swept_at ? 'Swept to treasury (your balance entitlement is unchanged)' : 'Sweep pending' }}</p>
    </div>
  </AuthCardLayout>
</template>
