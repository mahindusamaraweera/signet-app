<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '@/api/client'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'
import RegistrationProgress from '@/components/RegistrationProgress.vue'

const route = useRoute()
const router = useRouter()
const values = new URLSearchParams(route.hash.replace(/^#/, ''))
const id = values.get('id') || ''
let token = values.get('token') || ''
const valid = /^[a-f0-9]{64}$/.test(id) && /^[a-f0-9]{64}$/.test(token)
const busy = ref(false)
const error = ref(valid ? '' : 'This verification link is incomplete. Please register again to receive a new link.')
// Keep credentials in component memory only, not history, storage or queries.
window.history.replaceState(window.history.state, '', window.location.pathname + window.location.search)

async function verify() {
  if (!valid || busy.value || !token) return
  busy.value = true
  error.value = ''
  try {
    const { data } = await api.post('/register/verify-email', { id, token })
    token = ''
    await router.replace({ name: 'register.step2', params: { id: data.user_id } })
  } catch (err) {
    if (err?.response?.status === 422) token = ''
    error.value = err?.response?.data?.message || 'Verification temporarily unavailable. Please try again.'
  } finally { busy.value = false }
}
onMounted(verify)
</script>

<template>
  <AuthCardLayout back-text="Back to login" back-to="/login">
    <RegistrationProgress :step="2" />
    <div class="verification-orb" aria-hidden="true">{{ error ? '!' : '✓' }}</div>
      <p class="small text-info">SIGNET · EMAIL VERIFICATION</p>
      <h1 class="h3">{{ busy ? 'Verifying your email…' : error ? 'Let’s get you back on track' : 'Email confirmed' }}</h1>
      <p class="text-muted" role="status">{{ busy ? 'Please keep this page open. We’re confirming your link securely.' : error ? 'If you have already verified this email, sign in to continue. Otherwise, request a new link through registration.' : 'Taking you to package selection…' }}</p>
      <p v-if="error" class="alert alert-warning" role="alert">{{ error }}</p>
      <button v-if="error && valid && token" class="btn btn-primary w-100" :disabled="busy" @click="verify">Try again</button>
      <RouterLink v-if="error" class="d-block mt-3" :to="{ name: 'register.step1' }">Return to registration</RouterLink>
      <RouterLink v-if="error" class="d-block mt-3" to="/login">Sign in</RouterLink>
  </AuthCardLayout>
</template>

<style scoped>
.verification-orb { display:grid;place-items:center;width:64px;height:64px;border-radius:18px;background:rgba(0,240,255,.08);border:1px solid var(--sg-line-3);color:var(--sg-accent-light);font-size:28px;margin-bottom:24px; }
</style>
