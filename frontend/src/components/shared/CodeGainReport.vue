<script setup>
// Shared body of leaders/index.blade.php ("Leaders Gain") and
// executives/index.blade.php ("Executives Gain"), which are identical apart
// from the Leader/Executive wording and the leader_id/executive_id filter.
// See leaderexecutive_handler.go's leaderExecCodeGainQuery.
//
// The original filter is a GET form and its pagination uses plain links
// without withQueryString(), so both drive the URL query here.
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Swal from 'sweetalert2'
import api from '@/api/client'
import Paginator from '@/components/shared/Paginator.vue'
import FlashAlert from '@/components/shared/FlashAlert.vue'
import { numberFormat } from '@/utils/format'

const props = defineProps({
  kind: { type: String, required: true }, // 'leader' | 'executive'
})

const Label = props.kind === 'leader' ? 'Leader' : 'Executive'
const Labels = props.kind === 'leader' ? 'Leaders' : 'Executives'
const endpoint = props.kind === 'leader' ? '/leaders/gain' : '/executives/gain'
const idParam = props.kind === 'leader' ? 'leader_id' : 'executive_id'
const listKey = props.kind === 'leader' ? 'leaders' : 'executives'
const optionsKey = props.kind === 'leader' ? 'leader_options' : 'executive_options'

const route = useRoute()
const router = useRouter()

const loadError = ref('')
const filterId = ref('')
const fromDate = ref('')
const toDate = ref('')
const options = ref([])
const users = ref(null)

async function fetchData() {
  loadError.value = ''
  const q = route.query
  filterId.value = q[idParam] || ''
  try {
    const params = {}
    for (const key of ['page', idParam, 'from', 'to']) if (q[key]) params[key] = q[key]
    const { data } = await api.get(endpoint, { params })
    fromDate.value = data.from
    toDate.value = data.to
    options.value = data[optionsKey] || []
    users.value = data[listKey]
  } catch (err) {
    loadError.value = err?.response?.data?.message || `Could not load ${Labels.toLowerCase()} gain.`
  }
}

watch(() => route.query, fetchData, { immediate: true })

function search() {
  router.push({ path: route.path, query: { [idParam]: filterId.value, from: fromDate.value, to: toDate.value } })
}

function goToPage(page) {
  router.push({ path: route.path, query: { ...route.query, page } })
}

function copyBinanceId(binanceId) {
  navigator.clipboard.writeText(binanceId).then(() => {
    Swal.fire({
      toast: true,
      position: 'top-end',
      icon: 'success',
      title: 'Binance ID copied!',
      showConfirmButton: false,
      timer: 1500,
    })
  })
}
</script>

<template>
  <FlashAlert type="danger" :message="loadError" @close="loadError = ''" />

  <div class="py-4">
    <div class="d-flex justify-content-between w-100 flex-wrap">
      <div class="mb-3 mb-lg-0">
        <h1 class="h4">{{ Labels }} Gain</h1>
      </div>
    </div>
  </div>

  <div class="card border-0 shadow mb-4">
    <div class="card-body">
      <!-- Date Range Filter -->
      <form method="GET" @submit.prevent="search">
        <div class="row">
          <div class="col-md-3">
            <label>{{ Label }}</label>
            <select v-model="filterId" :name="idParam" class="form-control">
              <option value="">All {{ Labels }}</option>
              <option v-for="option in options" :key="option.id" :value="String(option.id)">
                {{ option.name }}
              </option>
            </select>
          </div>

          <div class="col-md-3">
            <label>From</label>
            <input v-model="fromDate" type="date" name="from" class="form-control" />
          </div>

          <div class="col-md-3">
            <label>To</label>
            <input v-model="toDate" type="date" name="to" class="form-control" />
          </div>

          <div class="col-md-3">
            <label>&nbsp;</label>
            <button class="btn btn-primary d-block w-100">
              Search
            </button>
          </div>
        </div>
      </form>

      <div class="table-responsive mt-3">
        <table class="table align-items-center table-flush">
          <thead class="thead-light">
            <tr>
              <th class="border-bottom" scope="col">{{ Label }}</th>
              <th class="border-bottom" scope="col">SIGNET ID</th>
              <th class="border-bottom" scope="col">Binance ID</th>
              <th class="border-bottom" scope="col">Total Package Value</th>
              <th class="border-bottom" scope="col">5% Gain</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in users?.data" :key="user.id">
              <td>{{ user.name }}</td>
              <td>{{ 'SIG-00' + user.id }}</td>
              <td>
                <div class="d-flex align-items-center gap-2">
                  <span>{{ user.binance_pay_id }}</span>
                  <button
                    v-if="user.binance_pay_id"
                    type="button"
                    class="btn btn-sm btn-outline-secondary"
                    title="Copy Binance ID"
                    @click="copyBinanceId(user.binance_pay_id)"
                  >
                    <i class="fas fa-copy"></i>
                  </button>
                </div>
              </td>
              <td>${{ numberFormat(user.total_package, 2) }}</td>
              <td>${{ numberFormat(user.total_package * 0.05, 2) }}</td>
            </tr>
          </tbody>
        </table>

        <div class="d-flex justify-content-center">
          <Paginator :pagination="users" @change="goToPage" />
        </div>
      </div>
    </div>
  </div>
</template>
