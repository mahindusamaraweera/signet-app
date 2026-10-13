<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import api from '@/api/client'
const cap = ref(null)
const stale = ref(false)
let timer
let stopped = false
async function refresh() {
  try { const { data } = await api.get('/blockchain/income-cap'); if (!stopped) { cap.value = data; stale.value = false } }
  catch { if (!stopped) stale.value = true }
  finally { if (!stopped) timer = setTimeout(refresh, 60000) }
}
onMounted(refresh)
onUnmounted(() => { stopped = true; clearTimeout(timer) })
</script>
<template>
  <aside v-if="cap && (cap.cap_reached || cap.diversions?.length)" class="alert alert-warning" aria-label="Income cap alert">
    <p v-if="stale" class="mb-1">Latest cap status is temporarily unavailable. Showing the last confirmed records.</p>
    <p v-if="cap.cap_reached" class="mb-1">Your income cap has been reached. Until an eligible top-up extends it, capped team commissions go to the company.</p>
    <p class="mb-1">Income cap: {{ cap.cap_usd }} USD · Income balance: {{ cap.income_balance_usd }} USD · Remaining capacity: {{ cap.remaining_usd }} USD.</p>
    <details v-if="cap.diversions?.length"><summary>Recent commissions redirected to company</summary><p v-for="notice in cap.diversions" :key="notice.id" class="mb-1">{{ notice.created_at }} — {{ notice.commission_kind }}: {{ notice.commission_usd }} USD commission exceeded your available cap and was assigned to the company.</p></details>
    <p class="small mb-0">These are the existing USD income-cap records, not your SIGNET wallet balance.
      <RouterLink v-if="cap.cap_reached && cap.topup_enabled && !stale" to="/buy-package">Review top-up packages</RouterLink>
      <span v-else-if="!stale && !cap.topup_enabled">Top-up purchasing is not enabled yet.</span>
    </p>
  </aside>
</template>
