<script setup>
import { watch, onUnmounted } from 'vue'
import Swal from 'sweetalert2'
import { partialDepositReminder } from '@/utils/partialDepositReminder'

const props = defineProps({ purchase: { type: Object, default: null } })
const shown = new Set()
let busy = false
let stopped = false

watch(() => props.purchase, async (purchase) => {
  const notice = partialDepositReminder(purchase)
  if (stopped || busy || !notice || shown.has(notice.key)) return
  busy = true
  shown.add(notice.key)
  try {
    await Swal.fire({
      icon: 'info',
      title: notice.topup ? 'Complete your top-up' : 'Complete your package deposit',
      text: `Your deposit has been recorded and your account is protected. Deposit another ${notice.remaining} USDT to ${notice.topup ? 'complete your top-up' : 'activate your package automatically'}.`,
      confirmButtonText: 'Understood',
    })
  } finally { busy = false }
}, { immediate: true })

onUnmounted(() => {
  stopped = true
  if (busy) Swal.close()
})
</script>

<template><!-- Reminder uses an accessible modal and never submits a payment. --></template>
