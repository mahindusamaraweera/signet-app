<script setup>
// Ports company/roc_income.blade.php (route('company.roc')). See
// roc_handler.go's rocIncomeHandler/rocUpdateStatusHandler.
//
// The summary row is two tiles — "Per Week Total" (with "5% Week Total")
// and "Total Paying ROC"; the "Balance Forward" tile is commented out in
// the original. The tiles' icons are Phosphor classes the original never
// loads, so they render as empty circles there too. The week filter is a
// GET form, so it drives the URL query.
import { ref, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import Paginator from '@/components/shared/Paginator.vue'
import { useToast } from '@/composables/useToast'
import { nullStr, numberFormat } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const { toastSuccess, toastError } = useToast()

const jobs = ref([])
const selectedJobId = ref('')
const weeklySummary = ref(null)
const logs = ref(null)
const loadError = ref('')

const dateOnly = (v) => (v ? String(v).slice(0, 10) : '')

async function fetchRoc() {
  loadError.value = ''
  const q = route.query
  try {
    const params = {}
    for (const key of ['page', 'job_id']) if (q[key]) params[key] = q[key]
    const { data } = await api.get('/roc', { params })
    if (data.status === 'error') {
      loadError.value = data.message || 'No ROC job found.'
      return
    }
    jobs.value = data.jobs || []
    selectedJobId.value = data.selected_job_id || ''
    weeklySummary.value = data.weekly_summary || null
    logs.value = data.roc_income_logs
  } catch (err) {
    loadError.value = err?.response?.data?.message || 'Could not load ROC income.'
  }
}

watch(() => route.query, fetchRoc, { immediate: true })

function applyFilter() {
  router.push({ path: route.path, query: { job_id: selectedJobId.value } })
}

function goToPage(page) {
  router.push({ path: route.path, query: { ...route.query, page } })
}

const perWeekTotal = computed(() => Number(weeklySummary.value?.per_week_total || 0))
const totalPayingRoc = computed(() => Number(weeklySummary.value?.total_amount || 0) - Number(weeklySummary.value?.balance_forward || 0))

const ucfirst = (s) => (s ? s.charAt(0).toUpperCase() + s.slice(1) : '')

async function onToggleStatus(row, event) {
  const checkbox = event.target
  const status = checkbox.checked ? 'paid' : 'pending'
  try {
    const { data } = await api.post('/roc/status-update', { id: row.id, status })
    if (data.success) {
      row.status = status
      toastSuccess(`Status changed to ${ucfirst(status)}`)
    } else {
      toastError('Failed to update status')
      checkbox.checked = !checkbox.checked // revert toggle
    }
  } catch {
    toastError('Server error. Try again.')
    checkbox.checked = !checkbox.checked // revert toggle
  }
}
</script>

<template>
  <DashboardLayout>
    <div v-if="loadError" class="alert alert-danger">{{ loadError }}</div>

    <div class="py-4">
      <div class="d-flex justify-content-between w-100 flex-wrap">
        <div class="mb-3 mb-lg-0">
          <h1 class="h4">ROC Income</h1>
        </div>
      </div>
    </div>

    <div class="card border-0 shadow mb-4">
      <div class="card-body">
        <!-- Search Form -->
        <form method="GET" class="mb-3" @submit.prevent="applyFilter">
          <div class="input-group">
            <select v-model="selectedJobId" name="job_id" class="form-select">
              <option value="">-- Select Week --</option>
              <option v-for="job in jobs" :key="job.job_id" :value="job.job_id">
                {{ dateOnly(job.week_start) }} to {{ dateOnly(job.week_end) }}
              </option>
            </select>
            <button type="submit" class="btn btn-primary">Filter</button>
            <RouterLink v-if="route.query.job_id" to="/roc" class="btn btn-outline-secondary">Clear</RouterLink>
          </div>
        </form>

        <div class="row g-3 mb-4">
          <div class="col-12 col-md-6">
            <div class="p-3 rounded-4 border bg-light h-100">
              <div class="d-flex align-items-center justify-content-between">
                <div>
                  <div class="text-muted small fw-semibold">Per Week Total</div>
                  <div class="fs-4 fw-bold mt-1">USDT {{ numberFormat(perWeekTotal, 2) }}</div>
                </div>
                <div>
                  <div class="text-muted small fw-semibold">5% Week Total</div>
                  <div class="fs-4 fw-bold mt-1">USDT {{ numberFormat(perWeekTotal * 0.05, 2) }}</div>
                </div>
                <div class="rounded-circle bg-white border d-flex align-items-center justify-content-center" style="width: 46px; height: 46px">
                  <i class="ph-bold ph-coins fs-4 text-primary"></i>
                </div>
              </div>
              <div class="mt-2 small text-muted">Total for selected week</div>
            </div>
          </div>
          <div class="col-12 col-md-6">
            <div class="p-3 rounded-4 border bg-light h-100">
              <div class="d-flex align-items-center justify-content-between">
                <div>
                  <div class="text-muted small fw-semibold">Total Paying ROC</div>
                  <div class="fs-4 fw-bold mt-1">
                    USDT {{ numberFormat(totalPayingRoc, 2) }}
                  </div>
                </div>
                <div class="rounded-circle bg-white border d-flex align-items-center justify-content-center" style="width: 46px; height: 46px">
                  <i class="ph-bold ph-trend-down fs-4 text-danger"></i>
                </div>
              </div>
              <div class="mt-2 small text-muted">Selected Week Total Paying</div>
            </div>
          </div>
        </div>

        <div class="table-responsive">
          <table class="table align-items-center table-flush">
            <thead class="thead-light">
              <tr>
                <th class="border-bottom" scope="col">User name</th>
                <th class="border-bottom" scope="col">SIGNET ID</th>
                <th class="border-bottom" scope="col">Binance ID</th>
                <th class="border-bottom" scope="col">WhatsApp</th>
                <th class="border-bottom" scope="col">Earnings</th>
                <th class="border-bottom" scope="col">Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in logs?.data" :key="row.id">
                <td>{{ nullStr(row.user_name) }}</td>
                <td>{{ 'SIG-00' + row.user_id }}</td>
                <td>{{ nullStr(row.binance_pay_id) }}</td>
                <td>{{ nullStr(row.whatsapp_number) }}</td>
                <td>{{ numberFormat(row.amount, 2) }}</td>
                <td>
                  <div class="form-check form-switch">
                    <input type="checkbox" class="form-check-input toggle-status" :checked="row.status == 'paid'" @change="onToggleStatus(row, $event)" />
                    <label class="form-check-label">
                      {{ ucfirst(row.status) }}
                    </label>
                  </div>
                </td>
              </tr>
              <tr v-if="logs && !logs.data?.length">
                <td colspan="4" class="text-center text-muted">No users found</td>
              </tr>
            </tbody>
          </table>
          <div class="d-flex justify-content-center">
            <Paginator :pagination="logs" @change="goToPage" />
          </div>
        </div>
      </div>
    </div>
  </DashboardLayout>
</template>
