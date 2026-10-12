<script setup>
import { computed, ref, watch } from 'vue'
import api from '@/api/client'
import { activationAction, hasUSDTShortfall } from '@/utils/purchaseActivation'
import { usdtDeficitDisplay } from '@/utils/usdtDisplay'

const props = defineProps({ purchase: Object, wallet: Object, disabled: Boolean })
const emit = defineEmits(['refresh', 'busy'])
const busy = ref(false)
const message = ref('')
const failed = ref(false)
const action = computed(() => activationAction(props.purchase, props.wallet))
const explanation = computed(() => {
  if (props.purchase?.status !== 'awaiting_deposit') return 'Your purchase is processing. Checking again resumes the same purchase and its swap and activation steps; do not send another payment for it.'
  const completion = props.purchase.purchase_kind === 'topup'
    ? 'Your income cap increases after swap and settlement confirmation.'
    : 'Your package becomes active after the swap and SIGNET allocation are confirmed.'
  return `Automatic activation can continue in the background. This button checks your confirmed USDT against the selected package price plus service fee, then starts or resumes the package amount’s swap to SIGNET. ${completion}`
})
watch(() => `${props.purchase?.id}:${props.purchase?.status}`, () => { message.value = ''; failed.value = false })
async function activate() {
  if (busy.value || props.disabled || !action.value) return
  const id = String(props.purchase.id)
  busy.value = true
  emit('busy', true)
  message.value = ''
  failed.value = false
  try {
    const { data } = await api.post('/blockchain/purchase/sync', { purchase_id: id })
    if (String(props.purchase?.id) !== id) return
    message.value = data.purchase?.status === 'awaiting_deposit' && hasUSDTShortfall(data.purchase.remaining_usdt)
      ? `Balance checked. ${usdtDeficitDisplay(data.purchase.remaining_usdt)} USDT still needed, including the service fee. Wait for any recent deposit to confirm before sending more.`
      : data.message
  } catch (err) {
    failed.value = true
    message.value = err?.response?.status === 429
      ? 'Please wait a minute before checking again. Automatic activation can continue in the meantime.'
      : err?.response?.data?.message || 'The check could not finish. Refresh your status before retrying; your recorded deposit is preserved.'
  } finally {
    busy.value = false
    emit('busy', false)
    emit('refresh')
  }
}
</script>

<template>
  <div v-if="action || message" class="activation-action my-3">
    <template v-if="action">
      <p class="small text-muted mb-2">{{ explanation }}</p>
      <button type="button" class="btn btn-primary" :disabled="busy || disabled" @click="activate">{{ busy ? 'Checking your purchase…' : action.label }}</button>
    </template>
    <p v-if="message" class="mt-2 mb-0" :class="{ 'text-warning': failed }" role="status">{{ message }}</p>
  </div>
</template>
