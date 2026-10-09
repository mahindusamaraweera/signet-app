<script setup>
// Ports salaries/index.blade.php ("Salaries"). See salary_handler.go's
// salariesIndexHandler/salariesSearchUsersHandler/salariesStoreHandler.
//
// The "Add Salary" user picker is Select2 with the original's AJAX search
// (salaries.searchUsers → {id, text} results). The original's
// "Actions"/delete column is commented out, so no delete button renders.
// The date filter is a GET form, so it drives the URL query.
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Swal from 'sweetalert2'
import $ from 'jquery'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import Paginator from '@/components/shared/Paginator.vue'
import { useBsModal } from '@/composables/useBsModal'
import { useSelect2 } from '@/composables/useSelect2'
import { nullStr, numberFormat } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const addModal = useBsModal()

const loadError = ref('')
const dateFrom = ref('')
const dateTo = ref('')
const salaries = ref(null)
const totalAmount = ref(0)

async function fetchData() {
  loadError.value = ''
  const q = route.query
  dateFrom.value = q.date_from || ''
  dateTo.value = q.date_to || ''
  try {
    const params = {}
    for (const key of ['page', 'date_from', 'date_to']) if (q[key]) params[key] = q[key]
    const { data } = await api.get('/salaries', { params })
    salaries.value = data.salaries
    totalAmount.value = data.total_amount
  } catch (err) {
    loadError.value = err?.response?.data?.message || 'Could not load salaries.'
  }
}

watch(() => route.query, fetchData, { immediate: true })

function applyFilter() {
  router.push({ path: route.path, query: { date_from: dateFrom.value, date_to: dateTo.value } })
}

function goToPage(page) {
  router.push({ path: route.path, query: { ...route.query, page } })
}

// --- Add Salary modal ---
const userSelect = ref(null)
const form = ref({ user_id: '', amount: '', salary_date: '', remarks: '' })

const select2 = useSelect2(
  userSelect,
  () => ({
    dropdownParent: $(addModal.el.value),
    placeholder: 'Search by name, Signet ID, or WhatsApp',
    minimumInputLength: 1,
    ajax: {
      url: '/api/v1/salaries/search-users',
      dataType: 'json',
      delay: 250,
      xhrFields: { withCredentials: true },
      data: (params) => ({ term: params.term }),
      processResults: (data) => ({ results: data }),
    },
  }),
  (value) => {
    form.value.user_id = value || ''
  },
)

// Reset form + select2 when the modal closes.
function onHidden() {
  form.value = { user_id: '', amount: '', salary_date: '', remarks: '' }
  select2.set(null)
}
onMounted(() => addModal.el.value?.addEventListener('hidden.bs.modal', onHidden))
onBeforeUnmount(() => addModal.el.value?.removeEventListener('hidden.bs.modal', onHidden))

async function submitAdd() {
  try {
    const { data } = await api.post('/salaries', {
      user_id: Number(form.value.user_id) || 0,
      amount: Number(form.value.amount) || 0,
      salary_date: form.value.salary_date,
      remarks: form.value.remarks,
    })
    if (data.success) {
      Swal.fire('Saved!', data.message ?? 'Salary added successfully', 'success').then(() => window.location.reload())
    } else {
      Swal.fire('Error', data.message ?? 'Could not save salary', 'error')
    }
  } catch (err) {
    const message = err?.response?.data?.message
    if (message) Swal.fire('Error', message, 'error')
    else Swal.fire('Error', 'Something went wrong', 'error')
  }
}
</script>

<template>
  <DashboardLayout>
    <div v-if="loadError" class="alert alert-danger">{{ loadError }}</div>

    <div class="py-4">
      <div class="d-flex justify-content-between w-100 flex-wrap">
        <div class="mb-3 mb-lg-0">
          <h1 class="h4">Salaries</h1>
        </div>
        <div>
          <button class="btn btn-primary" @click="addModal.show()">
            <i class="bi bi-plus-lg me-1"></i> Add Salary
          </button>
        </div>
      </div>
    </div>

    <div class="card border-0 shadow mb-4">
      <div class="card-body">
        <!-- Date Range Filter -->
        <form method="GET" class="row g-2 align-items-end mb-3" @submit.prevent="applyFilter">
          <div class="col-auto">
            <label class="form-label mb-1 small text-muted">From</label>
            <input v-model="dateFrom" type="date" name="date_from" class="form-control" />
          </div>
          <div class="col-auto">
            <label class="form-label mb-1 small text-muted">To</label>
            <input v-model="dateTo" type="date" name="date_to" class="form-control" />
          </div>
          <div class="col-auto">
            <button class="btn btn-primary" type="submit">Filter</button>
            <RouterLink v-if="route.query.date_from || route.query.date_to" to="/salaries" class="btn btn-outline-secondary">Clear</RouterLink>
          </div>
          <div class="col-auto ms-auto">
            <span class="text-muted small">Total in range:</span>
            <span class="fw-bold">{{ numberFormat(totalAmount, 2) }}</span>
          </div>
        </form>

        <div class="table-responsive">
          <table class="table align-items-center table-flush">
            <thead class="thead-light">
              <tr>
                <th class="border-bottom" scope="col">User</th>
                <th class="border-bottom" scope="col">SIGNET ID</th>
                <th class="border-bottom" scope="col">Amount</th>
                <th class="border-bottom" scope="col">Date</th>
                <th class="border-bottom" scope="col">Remarks</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="salary in salaries?.data" :key="salary.id">
                <td>{{ nullStr(salary.user_name) || 'N/A' }}</td>
                <td>{{ 'SIG-00' + salary.user_id }}</td>
                <td>{{ numberFormat(salary.amount, 2) }}</td>
                <td>{{ String(salary.salary_date || '').slice(0, 10) }}</td>
                <td>{{ nullStr(salary.remarks) || '-' }}</td>
              </tr>
              <tr v-if="salaries && !salaries.data?.length">
                <td colspan="6" class="text-center text-muted">No salary records found</td>
              </tr>
            </tbody>
          </table>

          <div class="d-flex justify-content-center">
            <Paginator :pagination="salaries" @change="goToPage" />
          </div>
        </div>
      </div>
    </div>

    <!-- Add Salary Modal -->
    <div id="addSalaryModal" :ref="addModal.el" class="modal fade" tabindex="-1">
      <div class="modal-dialog">
        <div class="modal-content">
          <form id="addSalaryForm" @submit.prevent="submitAdd">
            <div class="modal-header">
              <h5 class="modal-title">Add Salary</h5>
              <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
            </div>
            <div class="modal-body">
              <div class="mb-3">
                <label class="form-label">User</label>
                <select id="salary_user_id" ref="userSelect" name="user_id" class="form-control" style="width: 100%" required></select>
              </div>
              <div class="mb-3">
                <label class="form-label">Amount</label>
                <input id="salary_amount" v-model="form.amount" type="number" step="0.01" min="0" name="amount" class="form-control" required />
              </div>
              <div class="mb-3">
                <label class="form-label">Date</label>
                <input id="salary_date" v-model="form.salary_date" type="date" name="salary_date" class="form-control" required />
              </div>
              <div class="mb-3">
                <label class="form-label">Remarks (optional)</label>
                <input id="salary_remarks" v-model="form.remarks" type="text" name="remarks" class="form-control" />
              </div>
            </div>
            <div class="modal-footer">
              <button class="btn btn-secondary" data-bs-dismiss="modal" type="button">Close</button>
              <button type="submit" class="btn btn-primary">Save</button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </DashboardLayout>
</template>
