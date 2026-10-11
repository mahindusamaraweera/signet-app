<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '@/api/client'

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
    error.value = err?.response?.data?.message || 'Verification temporarily unavailable. Please try again.'
  } finally { busy.value = false }
}
</script>

<template>
  <main class="container py-5" style="max-width: 640px">
    <section class="card p-4">
      <h1 class="h4">Verify your email</h1>
      <p>Confirm your email to create your account and continue to package selection.</p>
      <p v-if="error" class="alert alert-warning" role="alert">{{ error }}</p>
      <button class="btn btn-primary" :disabled="!valid || busy" @click="verify">{{ busy ? 'Verifying…' : 'Verify email and continue' }}</button>
      <RouterLink class="mt-3" :to="{ name: 'register.step1' }">Return to registration</RouterLink>
    </section>
  </main>
</template>
