<script setup>
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'

const state = ref(null)
const busy = ref(false)
const error = ref('')
const now = ref(Date.now())
let timer
let clock
let stopped = false
const verified = computed(() => state.value?.verified === true && state.value?.environment === 'production'
  && Date.parse(state.value.valid_until || '') > now.value)
const labels = { not_linked: 'Connect your Tier account', not_started: 'Not started', pending: 'Pending', in_review: 'Under review', approved: 'Verified', checking: 'Refreshing verification', resubmission_required: 'More information needed', rejected: 'Verification unsuccessful', unavailable: 'Unavailable' }
const statusLabel = computed(() => verified.value ? 'Verified' : state.value?.status === 'approved' ? 'Refreshing verification' : labels[state.value?.status] || 'Checking verification')
const verificationURL = computed(() => {
  try {
    const url = new URL(state.value?.verification_url)
    return ['https:', 'http:'].includes(url.protocol) && url.pathname === '/identity-verification' ? url.href : ''
  } catch { return '' }
})
const telegramLinks = computed(() => verified.value ? (state.value?.telegram_links || []).filter(link => {
  try { return new URL(link).protocol === 'https:' } catch { return false }
}) : [])
async function refresh() {
  if (busy.value || stopped) return
  busy.value = true
  try {
    const { data } = await api.get('/kyc/show')
    if (!stopped) { state.value = data; error.value = '' }
  } catch (err) {
    if (!stopped) { state.value = null; error.value = err?.response?.data?.message || 'Verification could not be confirmed. Please retry.' }
  } finally { if (!stopped) busy.value = false }
}
function visible() { if (document.visibilityState === 'visible') refresh() }
onMounted(() => {
  refresh()
  timer = window.setInterval(visible, 30000)
  clock = window.setInterval(() => { now.value = Date.now() }, 1000)
  document.addEventListener('visibilitychange', visible)
  window.addEventListener('pageshow', visible)
})
onBeforeUnmount(() => {
  stopped = true; window.clearInterval(timer); window.clearInterval(clock)
  document.removeEventListener('visibilitychange', visible); window.removeEventListener('pageshow', visible)
})
</script>

<template>
  <DashboardLayout>
    <section class="card wallet-panel tier-kyc-panel">
      <p class="wallet-eyebrow">YOUR ACCOUNT</p>
      <div class="d-flex flex-wrap align-items-center justify-content-between gap-3 mb-3">
        <div><h1 class="h3 mb-1">Identity verification</h1><p class="text-muted mb-0">Shared securely through your connected SIGNET Tier account.</p></div>
        <span v-if="state" class="badge rounded-pill" :class="verified ? 'bg-success' : 'bg-info text-dark'" role="status">{{ state.environment === 'sandbox' ? 'Sandbox · ' : '' }}{{ statusLabel }}</span>
      </div>
      <p v-if="error" class="alert alert-warning" role="alert">{{ error }}</p>
      <p v-if="!state && !error" role="status">Loading verification status…</p>
      <template v-if="state">
        <div v-if="verified" class="alert alert-success">Your identity is verified. Plans using the same SIGNET identity share this verification. You do not need to submit documents again here.</div>
        <template v-else-if="!state.linked">
          <p>Connect your existing Tier account first. We will then check its identity verification automatically.</p>
          <RouterLink to="/wallet/tier-link" class="btn btn-primary">Connect my Tier account</RouterLink>
        </template>
        <template v-else>
          <p>Complete or review verification in your connected Tier account. Sign in with that Tier account when prompted. Your result will appear here automatically.</p>
          <a v-if="verificationURL" :href="verificationURL" class="btn btn-primary">{{ state.status === 'in_review' ? 'View verification in Tier' : 'Continue verification in Tier' }}</a>
        </template>
        <p v-if="state.environment === 'sandbox'" class="alert alert-warning mt-3">Sandbox result. This does not count as production identity verification.</p>
        <p v-if="state.withdrawal_required && !verified" class="small mt-3">Complete production identity verification before requesting a withdrawal.</p>
        <p class="text-muted small mt-3 mb-0">Manage your verification securely in the connected Tier portal. Its current verification status is shared with your SIGNET app account.</p>
        <div v-if="telegramLinks.length" class="mt-3"><h2 class="h6">Your package communities</h2><a v-for="(link, index) in telegramLinks" :key="`${index}-${link}`" :href="link" class="btn btn-outline-info me-2 mb-2" target="_blank" rel="noopener noreferrer">Join Telegram {{ index + 1 }}</a></div>
      </template>
      <div class="mt-3"><button class="btn btn-outline-info" :disabled="busy" @click="refresh">{{ busy ? 'Checking…' : 'Refresh status' }}</button></div>
    </section>
  </DashboardLayout>
</template>

<style scoped>
.tier-kyc-panel { max-width: 960px; margin: 0 auto; padding: clamp(1rem, 3vw, 2rem); }
.tier-kyc-panel .btn { white-space: normal; }
</style>
