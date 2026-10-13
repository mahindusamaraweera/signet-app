<script setup>
// Ports the "Mining Community Staking Token" card + its inline "Mining
// Dashboard Script" from admin/agent/user dashboard.blade.php, keeping the
// original's element ids (signet-dashboard's miner scene and signet-ui.css
// key off #miningToken, #statusBadge, #progressBar, ...) and its exact
// update behaviour:
//  - loadData(): one GET /mining/user/{id}; total_token and daily_mining
//    must be truthy or the load counts as a failure ("Disconnected").
//  - every 1s, while status is 'active', mining_token grows by
//    daily_mining/86400 and the card is redrawn; reaching total_token flips
//    status to 'inactive' and fires the "Mining Complete!" alert.
//  - every 5s, mining_token + status are re-synced from the server
//    (without redrawing — the next local tick does that).
// The card is only redrawn from updateUI(), so what's on screen can lag the
// in-memory numbers exactly as it does in the original.
import { reactive, ref, onMounted, onBeforeUnmount } from 'vue'
import Swal from 'sweetalert2'
import api from '@/api/client'

const props = defineProps({
  userId: { type: [Number, String], required: true },
  allUsersCount: { type: Number, default: 0 },
  // admin: 'col-lg-12'; agent/user: 'col-lg-12 mb-4'
  colClass: { type: String, default: 'col-lg-12 mb-4' },
  // agent's markup starts Total/Daily at "0"; admin/user at "0.00000000"
  initialCount: { type: String, default: '0.00000000' },
  // agent's script parses total/daily with parseInt; admin/user parseFloat
  parseCounts: { type: Function, default: parseFloat },
})

const ui = reactive({
  connection: 'connecting', // 'connecting' | 'connected' | 'disconnected'
  miningToken: '0.00000000',
  totalToken: props.initialCount,
  dailyMining: props.initialCount,
  progressPercent: '0%',
  progressWidth: '0%',
  miningRate: '0.00000000',
  status: 'Loading...',
  statusClass: 'badge bg-secondary',
  lastUpdate: 'Never',
})

const miningData = { mining_token: 0, total_token: 0, daily_mining: 0, status: 'inactive' }
const loading = ref(false)
let stopped = false
let localUpdateInterval = null
let serverSyncInterval = null

function debugLog(message, data = null) {
  console.log(`[Mining] ${message}`, data || '')
}

async function fetchMining() {
  const { data } = await api.get(`/mining/user/${props.userId}`)
  return data
}

async function loadData() {
  if (stopped || loading.value) return
  if (!props.userId) {
    debugLog('Error: No user ID')
    return
  }
  loading.value = true
  debugLog('Loading mining data for user ' + props.userId)
  try {
    const result = await fetchMining()
    if (stopped) return
    debugLog('User data response', result)
    if (result.success && result.data) {
      if (!result.data.total_token || !result.data.daily_mining) {
        throw new Error('Backend is missing required fields: total_token and daily_mining. Please update your controller.')
      }
      miningData.mining_token = parseFloat(result.data.mining_token) || 0
      miningData.total_token = props.parseCounts(result.data.total_token) || 0
      miningData.daily_mining = props.parseCounts(result.data.daily_mining) || 0
      miningData.status = result.data.status || 'inactive'
      debugLog('Mining data loaded successfully')
      ui.connection = 'connected'
      updateUI()
      startLocalMining()
      startServerSync()
    } else {
      throw new Error(result.error || 'No mining data found for this user')
    }
  } catch (error) {
    if (stopped) return
    console.error('Error loading data:', error)
    debugLog('Error: ' + error.message)
    ui.connection = 'disconnected'
    ui.status = 'Unavailable'
    ui.statusClass = 'badge bg-warning'
    ui.miningToken = ui.totalToken = ui.dailyMining = ui.miningRate = ui.progressPercent = '—'
  } finally {
    if (!stopped) loading.value = false
  }
}

function updateUI() {
  ui.miningToken = parseFloat(miningData.mining_token).toFixed(8)
  ui.totalToken = String(parseInt(miningData.total_token))
  ui.dailyMining = String(parseInt(miningData.daily_mining))
  const progress = miningData.total_token > 0 ? (miningData.mining_token / miningData.total_token) * 100 : 0
  ui.progressPercent = progress.toFixed(2) + '%'
  ui.progressWidth = Math.min(progress, 100) + '%'
  ui.miningRate = (miningData.daily_mining / 86400).toFixed(8)
  ui.status = String(miningData.status).toUpperCase()
  ui.statusClass = `badge ${miningData.status === 'active' ? 'bg-success' : 'bg-danger'}`
  ui.lastUpdate = new Date().toLocaleTimeString()
}

function startLocalMining() {
  if (localUpdateInterval) clearInterval(localUpdateInterval)
  debugLog('Starting local mining updates')
  localUpdateInterval = setInterval(() => {
    if (miningData.status === 'active' && miningData.daily_mining > 0) {
      const miningPerSecond = miningData.daily_mining / 86400
      miningData.mining_token = parseFloat(miningData.mining_token) + miningPerSecond
      if (miningData.mining_token >= miningData.total_token) {
        miningData.mining_token = miningData.total_token
        miningData.status = 'inactive'
        debugLog('Mining completed!')
        Swal.fire({
          icon: 'success',
          title: 'Mining Complete!',
          text: 'You have reached your total token limit.',
          timer: 3000,
        })
      }
      updateUI()
    }
  }, 1000)
}

function startServerSync() {
  if (serverSyncInterval) clearInterval(serverSyncInterval)
  debugLog('Starting server sync (every 5 seconds)')
  serverSyncInterval = setInterval(async () => {
    try {
      const result = await fetchMining()
      if (stopped) return
      if (result.success && result.data) {
        miningData.mining_token = parseFloat(result.data.mining_token)
        miningData.status = result.data.status
        ui.connection = 'connected'
        debugLog('Synced with server')
      } else {
        throw new Error('Sync failed')
      }
    } catch (error) {
      if (stopped) return
      console.error('Server sync error:', error)
      ui.connection = 'disconnected'
      debugLog('Sync error: ' + error.message)
    }
  }, 5000)
}

onMounted(() => {
  debugLog('Initializing mining dashboard')
  if (props.userId) {
    loadData()
  } else {
    debugLog('Error: User not authenticated')
    Swal.fire({ icon: 'error', title: 'Authentication Error', text: 'Please login to view mining dashboard' })
  }
})

onBeforeUnmount(() => {
  stopped = true
  if (localUpdateInterval) clearInterval(localUpdateInterval)
  if (serverSyncInterval) clearInterval(serverSyncInterval)
})
</script>

<template>
  <div :class="colClass">
    <div class="card shadow-lg">
      <div class="card-header bg-primary text-white">
        <div class="d-flex justify-content-between align-items-center">
          <h3 class="mb-0">⛏️ Mining Community Staking Token - All Users Count({{ allUsersCount }})</h3>
          <span v-if="ui.connection === 'connecting'" id="connectionBadge" class="badge bg-light text-dark">
            <i class="bi bi-circle-fill text-secondary"></i> Connecting...
          </span>
          <span v-else-if="ui.connection === 'connected'" id="connectionBadge" class="badge bg-success">
            <i class="bi bi-circle-fill text-success"></i> Connected
          </span>
          <span v-else id="connectionBadge" class="badge bg-danger">
            <i class="bi bi-circle-fill text-danger"></i> Disconnected
          </span>
        </div>
      </div>
      <div class="card-body">
        <p v-if="ui.connection === 'disconnected'" class="alert alert-warning small" role="status">Mining data is temporarily unavailable. Any previous values below are the last loaded estimate, not a confirmed wallet balance. <button type="button" class="btn btn-sm btn-outline-primary ms-2" :disabled="loading" @click="loadData">{{ loading ? 'Retrying…' : 'Retry loading' }}</button></p>
        <!-- Mining Stats -->
        <div class="row text-center mb-4">
          <div class="col-md-3">
            <h6 class="text-muted">Mining Token</h6>
            <h3 id="miningToken" class="text-success">{{ ui.miningToken }}</h3>
          </div>
          <div class="col-md-3">
            <h6 class="text-muted">Total Token</h6>
            <h3 id="totalToken" class="text-info">{{ ui.totalToken }}</h3>
          </div>
          <div class="col-md-3">
            <h6 class="text-muted">Daily Mining</h6>
            <h3 id="dailyMining" class="text-primary">{{ ui.dailyMining }}</h3>
          </div>
          <div class="col-md-3">
            <h6 class="text-muted">Status</h6>
            <h3>
              <span id="statusBadge" :class="ui.statusClass">{{ ui.status }}</span>
            </h3>
          </div>
        </div>

        <!-- Progress Bar -->
        <div class="mb-3">
          <div class="d-flex justify-content-between mb-2">
            <span>Progress</span>
            <span id="progressPercent">{{ ui.progressPercent }}</span>
          </div>
          <div class="progress" style="height: 25px">
            <div
              id="progressBar"
              class="progress-bar progress-bar-striped progress-bar-animated bg-success"
              role="progressbar"
              :style="{ width: ui.progressWidth }"
            ></div>
          </div>
        </div>

        <!-- Mining Rate -->
        <div class="text-center mb-3">
          <span class="badge bg-info fs-6">
            <i class="bi bi-speedometer2"></i>
            <span id="miningRate">{{ ui.miningRate }}</span> tokens/second
          </span>
        </div>

        <div class="text-center text-muted small">
          Last updated: <span id="lastUpdate">{{ ui.lastUpdate }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
