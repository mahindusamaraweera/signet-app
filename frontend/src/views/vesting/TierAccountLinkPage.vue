<script setup>
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'

const route = useRoute()
const linked = ref(false)
const tierId = ref('')
const busy = ref(false)
const error = ref('')
let callbackProof = null
async function status() {
  const { data } = await api.get('/blockchain/tier-link')
  linked.value = data.linked === true
  tierId.value = data.tier_user_id || ''
}
async function begin() {
  busy.value = true; error.value = ''
  try {
    const { data } = await api.post('/blockchain/tier-link/begin')
    window.location.assign(data.authorization_url)
  } catch (err) { error.value = err?.response?.data?.message || 'Could not start account linking.'; busy.value = false }
}
async function complete() {
  busy.value = true; error.value = ''
  try {
    await status()
    if (!linked.value) {
      if (!callbackProof) return
      await api.post('/blockchain/tier-link/complete', callbackProof)
      callbackProof = null
      await status()
    }
  } catch (err) { error.value = err?.response?.data?.message || err.message || 'Account linking was not confirmed.' }
  finally { busy.value = false }
}
onMounted(async () => {
  const code = route.query.code
  const state = route.query.state
  if (typeof code === 'string' && typeof state === 'string') {
    callbackProof = { code, state }
    window.history.replaceState(window.history.state, '', route.path)
  }
  await complete()
})
</script>

<template>
  <DashboardLayout>
    <section class="card wallet-panel tier-link-panel"><p class="wallet-eyebrow">ACCOUNT CONNECTION</p>
    <h1 class="h3">Connect my Tier account</h1>
    <p>Sign in to your existing Tier account and confirm ownership. This link verifies qualifying investments; it does not merge accounts, wallets or referral teams.</p>
    <p v-if="error" class="alert alert-warning" role="alert">{{ error }}</p>
    <p v-if="linked" class="alert alert-success" role="status">Tier account #{{ tierId }} is linked. Your existing balances and vesting have not changed.</p>
    <template v-else>
      <button v-if="callbackProof" class="btn btn-primary" :disabled="busy" @click="complete">Retry link confirmation</button>
      <button v-else class="btn btn-primary" :disabled="busy" @click="begin">{{ busy ? 'Checking…' : 'Connect my Tier account' }}</button>
    </template>
    <RouterLink to="/wallet/vesting" class="btn btn-link">Back to vesting</RouterLink>
    </section>
  </DashboardLayout>
</template>
