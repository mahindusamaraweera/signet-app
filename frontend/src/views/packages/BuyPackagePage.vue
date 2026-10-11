<script setup>
// Ports packages/buy-package.blade.php ("Buy Package" — route('buy.package')
// / posts route('buy.packages'), any authenticated). See
// package_handler.go's buyPackageHandler (GET /buy-package: active
// packages only) and buyPackagesHandler (POST /buy-packages).
//
// buyPackagesHandler's success response is {status:'success',
// parent_data:<models.User>} — no separate "done" page endpoint exists to
// re-fetch this from, so per FRONTEND_CONVENTIONS the response is stashed
// (sessionStorage) and the SPA navigates to buy.package.done, which reads
// it back rather than re-fetching.
import { computed, ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'
import FlashAlert from '@/components/shared/FlashAlert.vue'
import { useApiAction } from '@/composables/useApiAction'
import { visiblePackageChoices } from '@/utils/packageDisplay'

const router = useRouter()
const { run } = useApiAction()

const loading = ref(true)
const loadError = ref('')
const packages = ref([])
const selectedPackage = ref('')
const blockchainEnabled = ref(false)
const topupEnabled = ref(false)
const showLowerPackages = ref(false)
const fees = {1:0.10,100:20,500:80,1000:130,2500:280,5000:1030,10000:2030,25000:5030,50000:15030,100000:30030,500000:150030,1000000:400030}
const visiblePackages = computed(() => visiblePackageChoices(packages.value, blockchainEnabled.value, showLowerPackages.value))
const selected = computed(() => packages.value.find(p => String(p.id) === String(selectedPackage.value)))
const submitting = ref(false)
watch(visiblePackages, choices => {
  if (selectedPackage.value && !choices.some(p => String(p.id) === String(selectedPackage.value))) selectedPackage.value = ''
})

async function fetchPackages() {
  loading.value = true
  loadError.value = ''
  try {
    const { data } = await api.get('/buy-package')
    packages.value = data.packages || []
    blockchainEnabled.value = !!data.blockchain_enabled
    topupEnabled.value = !!data.topup_enabled
  } catch (err) {
    loadError.value = err?.response?.data?.message || 'Could not load packages.'
  } finally {
    loading.value = false
  }
}

async function submit() {
  if (submitting.value || loading.value || !selectedPackage.value || (blockchainEnabled.value && !topupEnabled.value)) return
  submitting.value = true
  // No separate "saved" alert — the response drives the very next screen
  // (Upliner Activation), matching the original's direct-redirect flow.
  const { ok, data } = await run(() => api.post('/buy-packages', { package: String(selectedPackage.value) }), {
    showSuccessAlert: false,
  })
  submitting.value = false
  if (ok) {
    if (data.blockchain_enabled) {
      router.push('/wallet/deposits')
      return
    }
    sessionStorage.setItem('signet:buyPackageDone', JSON.stringify(data.parent_data))
    router.push({ name: 'buy.package.done' })
  }
}

onMounted(fetchPackages)
</script>

<template>
  <!-- buy-package.blade.php renders its card inside the app shell (sidebar + topbar). -->
  <DashboardLayout>
    <AuthCardLayout :show-back-link="false">
      <FlashAlert type="danger" :message="loadError" @close="loadError = ''" />
      <form method="POST" @submit.prevent="submit">
        <div class="form-group">
          <label for="package">Choose your package</label>
          <select id="package" v-model="selectedPackage" name="package" class="form-control" :disabled="loading || submitting" required>
            <option value="">Select Package</option>
            <option v-for="pack in visiblePackages" :key="pack.id" :value="pack.id">{{ pack.name }} — {{ pack.price }} USDT</option>
          </select>
        </div>
        <template v-if="blockchainEnabled">
          <label class="mt-2"><input v-model="showLowerPackages" type="checkbox" :disabled="loading || submitting" /> Show lower / test packages</label>
          <p class="mt-3">Top-ups extend your income cap only. You receive no new SIGNET allocation, bonus or vesting.</p>
          <p v-if="selected && fees[selected.price] != null">Package: {{ selected.price }} USDT. Service fee: {{ fees[selected.price] }} USDT. Required deposit: {{ Number(selected.price) + fees[selected.price] }} USDT.</p>
          <p v-if="!topupEnabled">Automatic top-ups are not enabled yet.</p>
        </template>
        <button type="submit" class="btn btn-primary mt-4" :disabled="submitting || loading || (blockchainEnabled && !topupEnabled)">Next</button>
      </form>
      <div class="form-group"></div>
    </AuthCardLayout>
  </DashboardLayout>
</template>
