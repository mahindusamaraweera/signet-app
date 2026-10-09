<script setup>
// Ports company/dashboard.blade.php (route('company.dashboard')). See
// dashboard_handler.go's companyDashboardHandler for the response shape.
//
// The original's date filter is a plain GET form and its paginator links
// are plain ?page=N links (which drop the filter, as Laravel's links() does
// without withQueryString()), so both drive the URL query here and the page
// re-fetches from it — reload/back behave the same way.
import { ref, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import Paginator from '@/components/shared/Paginator.vue'
import FlashAlert from '@/components/shared/FlashAlert.vue'
import { useAuthStore } from '@/store/auth'
import { useDashboardMetaStore } from '@/store/dashboardMeta'
import { useSignetDashboard } from '@/composables/useSignetDashboard'
import { numberFormat } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const dashboardMeta = useDashboardMetaStore()

const resp = ref(null)
const loadError = ref('')
const fromDate = ref('')
const toDate = ref('')
const selectedUserId = ref('')

useSignetDashboard(resp)

async function fetchDashboard() {
  loadError.value = ''
  const q = route.query
  fromDate.value = q.from_date || ''
  toDate.value = q.to_date || ''
  try {
    const params = {}
    if (q.page) params.page = q.page
    if (q.from_date) params.from_date = q.from_date
    if (q.to_date) params.to_date = q.to_date
    const { data } = await api.get('/company/dashboard', { params })
    resp.value = data
    dashboardMeta.setNewActivations(data.new_activations)
  } catch (err) {
    loadError.value = err?.response?.data?.message || 'Could not load dashboard data.'
  }
}

watch(() => route.query, fetchDashboard, { immediate: true })

function applyFilter() {
  const query = {}
  query.from_date = fromDate.value
  query.to_date = toDate.value
  router.push({ path: route.path, query })
}

function goToPage(page) {
  router.push({ path: route.path, query: { ...route.query, page } })
}

function redirectToTokens() {
  if (selectedUserId.value) {
    router.push('/view-tokens/' + selectedUserId.value)
  } else {
    window.alert('Please select a user before proceeding.')
  }
}

const packageWiseCounts = computed(() => resp.value?.package_wise_counts || [])
const grandTotal = computed(() => Number(resp.value?.grand_total || 0))
const totalActivations = computed(() => packageWiseCounts.value.reduce((sum, r) => sum + Number(r.active_count || 0), 0))

function percentage(pkg) {
  return grandTotal.value > 0 ? (pkg.total_value / grandTotal.value) * 100 : 0
}
</script>

<template>
  <DashboardLayout>
    <FlashAlert type="danger" :message="loadError" @close="loadError = ''" />
    <template v-if="resp">
      <div class="row mt-4">
        <div class="col-12 mb-4">
          <div class="card bg-yellow-100 border-0 shadow">
            <div class="card-header d-sm-flex flex-row align-items-center flex-0">
              <div class="d-block mb-3 mb-sm-0">
                <div class="fs-5 fw-normal mb-2"></div>
                <h2 class="fs-3 fw-extrabold">Welcome, <span class="text-capitalize">{{ authStore.user?.role }}</span> Head - All Users Count({{ resp.all_users }})</h2>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="row">
        <div class="col-18 col-xl-8">
          <div class="row">
            <div class="col-12 mb-4">
              <div class="card border-0 shadow">
                <div class="card-header">
                  <div class="row align-items-center">
                    <div class="col">
                      <h2 class="fs-5 fw-bold mb-0">All Tokens</h2>
                    </div>
                    <div class="col text-end">
                      <a href="#" class="btn btn-sm btn-primary" @click.prevent>See all</a>
                    </div>
                  </div>
                </div>
                <div class="table-responsive">
                  <table class="table align-items-center table-flush">
                    <thead class="thead-light">
                      <tr>
                        <th class="border-bottom" scope="col">User name</th>
                        <th class="border-bottom" scope="col">Total Tokens</th>
                        <th class="border-bottom" scope="col">Active Tokens</th>
                        <th class="border-bottom" scope="col">Used Tokens</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="token in resp.token_counts?.data" :key="token.user_id">
                        <td>{{ token.user_name || 'Unknown User' }}</td>
                        <td>{{ Number(token.active_count || 0) + Number(token.deactive_count || 0) }} USDT</td>
                        <td><span class="badge bg-success">{{ token.active_count }} USDT</span></td>
                        <td><span class="badge bg-danger">{{ token.deactive_count }} USDT</span></td>
                      </tr>
                    </tbody>
                  </table>
                  <div class="d-flex justify-content-center">
                    <Paginator :pagination="resp.token_counts" @change="goToPage" />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div class="col-12 col-xl-4">
          <div class="card border-0 shadow-lg rounded-4 mb-4">
            <div class="card-header bg-gradient-primary text-white rounded-top-4">
              <h5 class="fw-bold mb-0">Generate Token</h5>
            </div>
            <div class="card-body">
              <label class="small fw-semibold mb-1">Select User</label>
              <select id="user_id" v-model="selectedUserId" name="user_id" class="form-select mb-3">
                <option value="">Select User</option>
                <option v-for="id in resp.user_ids" :key="id" :value="id">
                  {{ id == 1 ? 'Company Head' : 'SIG-00' + id }}
                </option>
              </select>
              <button type="button" class="btn btn-primary w-100 fw-semibold" @click="redirectToTokens">
                View Tokens
              </button>
            </div>
          </div>
        </div>
      </div>
      <div class="row">
        <div class="col-12">
          <div class="card border-0 shadow-sm rounded-4 mb-4 bg-light">
            <div class="card-body py-3">
              <form method="GET" class="row align-items-end g-2" @submit.prevent="applyFilter">
                <div class="col-md-4">
                  <label class="small fw-semibold text-muted mb-1">From Date</label>
                  <input v-model="fromDate" type="date" name="from_date" class="form-control" />
                </div>
                <div class="col-md-4">
                  <label class="small fw-semibold text-muted mb-1">To Date</label>
                  <input v-model="toDate" type="date" name="to_date" class="form-control" />
                </div>
                <div class="col-md-4 d-flex gap-2">
                  <button type="submit" class="btn btn-success w-100">
                    Apply Filter
                  </button>
                  <RouterLink to="/company/dashboard" class="btn btn-outline-dark w-100">
                    Reset
                  </RouterLink>
                </div>
              </form>
            </div>
          </div>

          <div class="row mb-4">
            <div class="col-md-4">
              <div class="card border-0 shadow-sm rounded-4 text-center p-3">
                <small class="text-muted">Total Activations</small>
                <h3 class="fw-bold text-primary mb-0">
                  {{ numberFormat(totalActivations) }}
                </h3>
              </div>
            </div>
            <div class="col-md-4">
              <div class="card border-0 shadow-sm rounded-4 text-center p-3">
                <small class="text-muted">Total Value</small>
                <h3 class="fw-bold text-success mb-0">
                  {{ numberFormat(grandTotal) }} USDT
                </h3>
              </div>
            </div>
            <div class="col-md-4">
              <div class="card border-0 shadow-sm rounded-4 text-center p-3">
                <small class="text-muted">Packages Activated</small>
                <h3 class="fw-bold text-dark mb-0">
                  {{ packageWiseCounts.length }}
                </h3>
              </div>
            </div>
          </div>

          <div class="card border-0 shadow rounded-4">
            <div class="card-header border-0 pb-0 d-flex justify-content-between align-items-center">
              <h6 class="fw-bold text-dark">Package Activation Summary</h6>
              <span v-if="grandTotal > 0" class="badge bg-success fs-6">
                {{ numberFormat(grandTotal) }} USDT
              </span>
            </div>
            <div class="card-body pt-3">
              <div v-for="pkg in packageWiseCounts" :key="pkg.package_id" class="card border-0 shadow-sm rounded-4 mb-3">
                <div class="card-body">
                  <div class="d-flex justify-content-between align-items-center mb-2">
                    <div>
                      <span class="badge bg-primary fs-6 px-3 py-2">
                        {{ pkg.name || 'Unknown Package' }}
                      </span>
                    </div>
                    <div class="text-end">
                      <div class="fw-bold text-success">
                        {{ numberFormat(pkg.total_value) }} USDT
                      </div>
                      <small class="text-muted">
                        {{ pkg.active_count }} Activations
                      </small>
                    </div>
                  </div>
                  <div class="progress rounded-pill" style="height: 8px">
                    <div class="progress-bar bg-success" :style="{ width: percentage(pkg) + '%' }"></div>
                  </div>
                </div>
              </div>
              <div v-if="!packageWiseCounts.length" class="text-center text-muted py-4">
                No activations found
              </div>
            </div>
          </div>

          <div v-if="grandTotal > 0" class="card border-0 shadow-sm bg-dark text-white mt-4 rounded-4">
            <div class="card-body d-flex justify-content-between align-items-center">
              <h6 class="mb-0 fw-bold">
                Total Activation Revenue
              </h6>
              <h4 class="mb-0 fw-bold text-success">
                {{ numberFormat(grandTotal) }} USDT
              </h4>
            </div>
          </div>
        </div>
      </div>
    </template>
  </DashboardLayout>
</template>
