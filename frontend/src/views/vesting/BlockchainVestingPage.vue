<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { vestingProgress } from '@/utils/vestingProgress'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import AssetAmount from '@/components/AssetAmount.vue'
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
    const { data } = await api.post('/blockchain/vesting/switch/commit', switchRequests.value[s.id])
    delete switchRequests.value[s.id]
    delete switchPreviews.value[s.id]
    const state = data.schedule?.status || data.status
    message.value = ['recovery_pending', 'recovered'].includes(state)
      ? 'Your earnings reached the allocation threshold. Vesting did not start; the remaining allocation is returning to treasury.'
      : 'Vesting plan updated. Previously released tokens are unchanged.'
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
    <p v-if="!busy && !error && !schedules.length" class="wallet-empty">No allocations yet. Your purchased SIGNET and bonus will appear after package activation.</p>
    <section v-for="s in schedules" :key="s.id" class="card p-3 mb-3">
      <h2 class="h5">Allocation #{{ s.id }} — {{ s.status === 'awaiting_tier' ? 'Waiting for qualifying Tier plan' : s.status === 'recovery_pending' ? 'Returning to treasury' : s.status === 'recovered' ? 'Returned to treasury' : s.status }}</h2>
      <div v-if="s.tier_connection_required" class="alert alert-info" role="status">Your purchased SIGNET and bonus remain locked in your custodial wallet. No vesting accrues until you connect a qualifying Tier account and confirm Option 1 or 2. Team commissions remain subject to your income cap. <RouterLink v-if="!admin" to="/wallet/tier-link">Connect Tier account</RouterLink></div>
      <p v-if="['recovery_pending', 'recovered'].includes(s.status)" class="alert alert-info" role="status">Your cumulative settled earnings reached this allocation’s token amount. The remaining locked allocation {{ s.status === 'recovered' ? 'has returned' : 'is returning' }} to treasury. Your earned available tokens and previous withdrawals remain yours.</p>
      <p v-if="s.treasury_return_tx_hash" class="text-break small">Treasury return transaction: {{ s.treasury_return_tx_hash }}</p>
      <div class="vesting-summary"><div><span>Total allocation</span><strong><AssetAmount :value="s.total_amount" asset="SIGNET" /></strong></div><div><span>Released to wallet</span><strong><AssetAmount :value="s.released_amount" asset="SIGNET" /></strong></div></div>
      <div v-if="s.progress_estimate" class="mb-3">
        <p class="mb-1">Estimated vesting progress</p>
        <output class="vesting-odometer" aria-live="off">{{ vestingProgress(s, elapsedSinceFetch) ?? 'Refresh to see current estimate' }}<span v-if="vestingProgress(s, elapsedSinceFetch) !== null"> SIGNET</span></output>
        <p class="small text-muted">Display estimate only, updated each second. Tokens settle daily; this number is not a spendable wallet balance.</p>
      </div>
      <p v-if="s.recovery_threshold_signet" class="small">Settled commission and reward earnings: <AssetAmount :value="s.qualifying_earned_signet" asset="SIGNET" /> · Recovery threshold: <AssetAmount :value="s.recovery_threshold_signet" asset="SIGNET" />. Withdrawals do not reset earnings; deposits, purchases, bonuses, transfers and vesting releases do not count.</p>
      <dl v-if="!['awaiting_tier', 'recovery_pending', 'recovered'].includes(s.status)" class="vesting-dates"><div><dt>Start date</dt><dd>{{ s.starts_at }}</dd></div><div><dt>Lock ends</dt><dd>{{ s.cliff_ends_at || 'No cliff' }}</dd></div><div><dt>Completion</dt><dd>{{ s.ends_at }}</dd></div></dl>
      <div v-if="!admin && (s.existing_user_switch_available || s.tier_switch_review_available || switchRequests[s.id])" class="mb-3">
        <p v-if="s.tier_switch_review_available">Connect your Tier account to check available vesting options. Option 1 releases over 10,000 days from activation. Option 2 waits 2,000 days from your package purchase, then releases over 7,500 days. Qualifying investments include eligible custom stakes and exclude bonuses and earnings.</p>
        <p v-else>You may keep your current historical method or spread the remaining allocation over 10,000 days.</p>
        <button v-if="s.existing_user_switch_available && !switchPreviews[s.id] && !switchRequests[s.id]" class="btn btn-outline-primary" :disabled="busy" @click="previewSwitch(s)">Review 10,000-day switch</button>
        <button v-if="s.tier_switch_review_available && !switchPreviews[s.id] && !switchRequests[s.id]" class="btn btn-outline-primary" :disabled="busy" @click="previewSwitch(s, 'tier')">Check linked Tier eligibility</button>
        <div v-if="switchPreviews[s.id]">
          <dl v-if="switchPreviews[s.id].eligibility" class="vesting-dates">
            <div><dt>Package price (excluding fee)</dt><dd><AssetAmount :value="switchPreviews[s.id].eligibility.package_usd" asset="USDT" /></dd></div>
            <div><dt>Qualifying Tier investment</dt><dd><AssetAmount :value="switchPreviews[s.id].eligibility.qualifying_usd" asset="USDT" /></dd></div>
            <div><dt>Investment with 3,000-day duration</dt><dd><AssetAmount :value="switchPreviews[s.id].eligibility.qualifying_3000_day_usd" asset="USDT" /></dd></div>
          </dl>
          <p v-if="switchPreviews[s.id].eligibility" class="small text-muted">Verified Tier purchase IDs: {{ switchPreviews[s.id].eligibility.purchase_ids?.join(', ') || 'None' }}. Amounts include custom stakes at their saved purchase-time values and exclude bonuses, earnings and refunded purchases.</p>
          <details v-if="switchPreviews[s.id].eligibility" class="mb-3">
            <summary>Exact investment totals</summary>
            <p class="text-break small">Package price: {{ switchPreviews[s.id].eligibility.package_usd }} USDT<br />Qualifying Tier investment: {{ switchPreviews[s.id].eligibility.qualifying_usd }} USDT<br />Investment with 3,000-day duration: {{ switchPreviews[s.id].eligibility.qualifying_3000_day_usd }} USDT</p>
          </details>
          <p v-if="switchPreviews[s.id].option === 'option_2'">Option 2 is available. Release starts after the 2,000-day wait measured from your package purchase date.</p>
          <p v-if="switchPreviews[s.id].option === 'option_1'">Option 1 is available. Start vesting on confirmation, with daily releases over 10,000 days.</p>
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
