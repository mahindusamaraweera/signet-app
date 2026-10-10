<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { vestingProgress } from '@/utils/vestingProgress'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import { useAuthStore } from '@/store/auth'

const props = defineProps({ admin: { type: Boolean, default: false } })
const auth = useAuthStore()
const userId = ref('')
const schedules = ref([])
const error = ref('')
const message = ref('')
const busy = ref(false)
const drafts = ref({})
const pending = ref({})
const switchPreviews = ref({})
const switchRequests = ref({})
const switchConsent = ref({})
const elapsedSinceFetch = ref(Infinity)
let fetchedAt = null
let tickTimer
let refreshTimer
const endpoint = computed(() => props.admin ? `/blockchain/users/${userId.value}/vesting` : '/blockchain/vesting')

async function load() {
  if (props.admin && !/^[1-9][0-9]*$/.test(userId.value)) { error.value = 'Enter the numeric user ID.'; return }
  busy.value = true
  error.value = ''
  try {
    const { data } = await api.get(endpoint.value)
    schedules.value = data.schedules || []
    fetchedAt = performance.now()
    elapsedSinceFetch.value = 0
    for (const s of schedules.value) drafts.value[s.id] ||= { direction: 'subtract', days: 1, reason: '' }
  } catch (err) { error.value = err?.response?.data?.message || 'Could not load vesting.' }
  finally { busy.value = false }
}

async function adjust(s) {
  const draft = drafts.value[s.id]
  if (!draft.reason.trim() || Number(draft.days) < 1) { error.value = 'Enter days and the reason for this adjustment.'; return }
  if (!pending.value[s.id]) pending.value[s.id] = {
    schedule_id: s.id, adjustment_id: crypto.randomUUID(), direction: draft.direction,
    days: Number(draft.days), reason: draft.reason.trim(), expected_revision: Number(s.external_revision),
  }
  busy.value = true
  error.value = ''
  try {
    await api.post(`${endpoint.value}/adjust`, pending.value[s.id])
    delete pending.value[s.id]
    message.value = 'Vesting time adjusted. Previously released amounts remain unchanged.'
    await load()
  } catch (err) {
    if (err?.response?.status === 422) {
      delete pending.value[s.id]
      await load()
    }
    error.value = err?.response?.data?.message || 'Adjustment was not confirmed. Retry the same request.'
  }
  finally { busy.value = false }
}

async function previewSwitch(s, mode = 'existing') {
  busy.value = true
  error.value = ''
  try {
    const { data } = await api.post('/blockchain/vesting/switch/preview', { schedule_id: s.id, mode })
    switchPreviews.value[s.id] = data.preview
    switchConsent.value[s.id] = false
  } catch (err) { error.value = err?.response?.data?.message || 'Could not preview the switch.' }
  finally { busy.value = false }
}

async function commitSwitch(s) {
  if (!switchRequests.value[s.id]) {
    if (!switchConsent.value[s.id] || !switchPreviews.value[s.id]) return
    switchRequests.value[s.id] = {
      schedule_id: s.id, request_id: crypto.randomUUID(), consent: true,
      confirmation_token: switchPreviews.value[s.id].confirmation_token,
    }
  }
  busy.value = true
  error.value = ''
  try {
    await api.post('/blockchain/vesting/switch/commit', switchRequests.value[s.id])
    delete switchRequests.value[s.id]
    delete switchPreviews.value[s.id]
    message.value = 'Vesting plan updated. Previously released tokens are unchanged.'
    await load()
  } catch (err) {
    if (err?.response?.status === 422) {
      delete switchRequests.value[s.id]
      delete switchPreviews.value[s.id]
      await load()
    }
    error.value = err?.response?.data?.message || 'Switch not confirmed. Retry the same request.'
  } finally { busy.value = false }
}
onMounted(() => {
  if (!props.admin) load()
  tickTimer = setInterval(() => {
    if (fetchedAt !== null) elapsedSinceFetch.value = performance.now() - fetchedAt
  }, 1000)
  refreshTimer = setInterval(() => {
    if (!props.admin && !busy.value && !Object.keys(switchRequests.value).length) load()
  }, 60000)
})
onUnmounted(() => { clearInterval(tickTimer); clearInterval(refreshTimer) })
</script>

<template>
  <DashboardLayout>
    <h1 class="h3">{{ admin ? 'Manage user vesting' : 'My SIGNET vesting' }}</h1>
    <p v-if="error" role="alert" class="alert alert-warning">{{ error }}</p>
    <p v-if="message" role="status" class="alert alert-success">{{ message }}</p>
    <form v-if="admin && (auth.isAdmin || auth.isCompany)" @submit.prevent="load" class="mb-4">
      <label for="vesting-user">User ID</label>
      <input id="vesting-user" v-model="userId" class="form-control" inputmode="numeric" required :disabled="Object.keys(pending).length > 0" />
      <button class="btn btn-primary mt-2" :disabled="busy">Load vesting</button>
    </form>
    <p v-if="!busy && !schedules.length">No vesting schedules loaded.</p>
    <section v-for="s in schedules" :key="s.id" class="card p-3 mb-3">
      <h2 class="h5">Vesting #{{ s.id }} — {{ s.status }}</h2>
      <p>Total: {{ s.total_amount }} SIGNET · Released: {{ s.released_amount }} SIGNET</p>
      <div v-if="s.progress_estimate" class="mb-3">
        <p class="mb-1">Estimated vesting progress</p>
        <output class="vesting-odometer" aria-live="off">{{ vestingProgress(s, elapsedSinceFetch) ?? 'Refresh to see current estimate' }}<span v-if="vestingProgress(s, elapsedSinceFetch) !== null"> SIGNET</span></output>
        <p class="small text-muted">Display estimate only, updated each second. Tokens settle daily; this number is not a spendable wallet balance.</p>
      </div>
      <p>Start: {{ s.starts_at }} · Lock ends: {{ s.cliff_ends_at || 'No cliff' }} · Completes: {{ s.ends_at }}</p>
      <div v-if="!admin && (s.existing_user_switch_available || s.tier_switch_review_available || switchRequests[s.id])" class="mb-3">
        <p>You may keep your current method. Option 1 releases over 10,000 days. Option 2 waits 2,000 days from purchase, then releases over 7,500 days. Existing-user switches spread the remaining allocation over 10,000 days.</p>
        <button v-if="s.existing_user_switch_available && !switchPreviews[s.id] && !switchRequests[s.id]" class="btn btn-outline-primary" :disabled="busy" @click="previewSwitch(s)">Review 10,000-day switch</button>
        <button v-if="s.tier_switch_review_available && !switchPreviews[s.id] && !switchRequests[s.id]" class="btn btn-outline-primary" :disabled="busy" @click="previewSwitch(s, 'tier')">Check linked Tier eligibility</button>
        <div v-if="switchPreviews[s.id]">
          <p v-if="switchPreviews[s.id].option === 'option_2'">Option 2: release starts after the 2,000-day wait measured from your package purchase date.</p>
          <p v-if="switchPreviews[s.id].option === 'option_1'">Option 1: your combined qualifying 3,000-day Tier investments match this package.</p>
          <p>Remaining: {{ switchPreviews[s.id].remaining_amount }} SIGNET · Daily: {{ switchPreviews[s.id].daily_release_amount }} SIGNET</p>
          <p>Starts: {{ switchPreviews[s.id].starts_at }} · Ends: {{ switchPreviews[s.id].ends_at }}. Preview expires in five minutes.</p>
          <label><input v-model="switchConsent[s.id]" type="checkbox" :disabled="busy || !!switchRequests[s.id]" /> I accept these dates and daily release terms. Previously released tokens stay unchanged.</label>
        </div>
        <button v-if="switchPreviews[s.id] || switchRequests[s.id]" class="btn btn-primary mt-2" :disabled="busy || (!switchConsent[s.id] && !switchRequests[s.id])" @click="commitSwitch(s)">{{ switchRequests[s.id] ? 'Retry switch confirmation' : 'Confirm switch' }}</button>
      </div>
      <form v-if="admin && drafts[s.id] && ['active', 'pending'].includes(s.status)" @submit.prevent="adjust(s)">
        <fieldset :disabled="busy || !!pending[s.id]">
          <label :for="`direction-${s.id}`">Adjust remaining time</label>
          <select :id="`direction-${s.id}`" v-model="drafts[s.id].direction" class="form-select"><option value="subtract">Shorten</option><option value="add">Extend</option></select>
          <label :for="`days-${s.id}`">Days</label>
          <input :id="`days-${s.id}`" v-model="drafts[s.id].days" class="form-control" type="number" min="1" max="36500" required />
          <label :for="`reason-${s.id}`">Performance reason</label>
          <textarea :id="`reason-${s.id}`" v-model="drafts[s.id].reason" class="form-control" minlength="3" maxlength="2000" required />
        </fieldset>
        <button class="btn btn-primary mt-2" :disabled="busy">{{ pending[s.id] ? 'Retry adjustment' : 'Apply adjustment' }}</button>
      </form>
      <details v-if="s.adjustments?.length" class="mt-3">
        <summary>Adjustment history</summary>
        <p v-for="(a, i) in s.adjustments" :key="i">{{ a.created_at }} · Admin {{ a.admin_id }} · {{ a.direction }} {{ a.days }} days · {{ a.reason }}</p>
      </details>
    </section>
  </DashboardLayout>
</template>

<style scoped>
.vesting-odometer { font-variant-numeric: tabular-nums; font-family: ui-monospace, monospace; font-size: 1.5rem; }
</style>
