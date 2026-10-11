<script setup>
// Ports auth/register_step2.blade.php — on-screen labelled "Step 2: Select a
// Package" (route('register.step2', $id) / posts to register.processStep2).
// See registerStep2FormHandler / registerStep2SubmitHandler in auth_handler.go.
//
// The POST response's `parent` object (Binance ID / WhatsApp / vacation flag)
// is only ever returned here, in-band — there's no separate GET to refetch it
// — so it's stashed in sessionStorage for the Step 3 "wait for upliner" page
// to read after navigation.
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '@/api/client'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'
import FlashAlert from '@/components/shared/FlashAlert.vue'
import { visiblePackageChoices } from '@/utils/packageDisplay'

const props = defineProps({
  id: { type: [String, Number], required: true },
})

const router = useRouter()

const loading = ref(true)
const loadError = ref('')
const packages = ref([])
const showLowerPackages = ref(false)
const blockchainEnabled = ref(false)
const fees = {1:0.10,100:20,500:80,1000:130,2500:280,5000:1030,10000:2030,25000:5030,50000:15030,100000:30030,500000:150030,1000000:400030}
const visiblePackages = computed(() => visiblePackageChoices(packages.value, blockchainEnabled.value, showLowerPackages.value))
const selected = computed(() => packages.value.find(p => String(p.id) === selectedPackage.value))
const selectedPackage = ref('')
const errors = ref({})
const formError = ref('')
const submitting = ref(false)
watch(visiblePackages, choices => {
  if (selectedPackage.value && !choices.some(p => String(p.id) === String(selectedPackage.value))) selectedPackage.value = ''
})

onMounted(async () => {
  try {
    const { data } = await api.get(`/register/step2/${props.id}`)
    packages.value = data.packages || []
    blockchainEnabled.value = !!data.blockchain_enabled
  } catch (err) {
    loadError.value = err?.response?.data?.message || 'Could not load packages.'
  } finally {
    loading.value = false
  }
})

async function onSubmit() {
  if (loading.value || submitting.value || !selectedPackage.value) return
  errors.value = {}
  formError.value = ''
  submitting.value = true
  try {
    const { data } = await api.post('/register/step2', {
      package: selectedPackage.value,
      newUserID: String(props.id),
    })
    if (data.status === 'success') {
	  if (data.blockchain_enabled) {
	    router.push({ name: 'register.funding', params: { id: data.user_id } })
	    return
	  }
      try {
        sessionStorage.setItem(`signet_register_parent_${props.id}`, JSON.stringify(data.parent))
      } catch {
        // sessionStorage unavailable — Step 3 will just render without parent details.
      }
      router.push({ name: 'register.step3.wait', params: { id: data.user_id } })
    } else {
      formError.value = data.message || 'Something went wrong. Please try again.'
    }
  } catch (err) {
    if (err?.response?.status === 422) {
      errors.value = err.response.data.errors || {}
    } else {
      formError.value = err?.response?.data?.message || 'Something went wrong. Please try again.'
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthCardLayout back-text="Back to log in" back-to="/login" back-placement="row">
    <h1 class="h3 mb-4">Step 2: Select a Package</h1>
    <FlashAlert type="danger" :message="loadError || formError" @close="formError = ''" />
    <form method="POST" @submit.prevent="onSubmit">
      <div class="form-group">
        <label for="package">Choose your package</label>
        <input type="hidden" name="newUserID" :value="id" />
        <select id="package" v-model="selectedPackage" name="package" class="form-control" :disabled="loading || submitting" required>
          <option value="">Select Package</option>
          <option v-for="pack in visiblePackages" :key="pack.id" :value="String(pack.id)">{{ pack.name }} — {{ pack.price }} USDT</option>
        </select>
      </div>
      <label v-if="blockchainEnabled" class="mt-2"><input v-model="showLowerPackages" type="checkbox" :disabled="loading || submitting" /> Show lower / test packages</label>
      <p v-if="blockchainEnabled && selected && fees[selected.price] != null" class="mt-3">Package: {{ selected.price }} USDT. Service fee: {{ fees[selected.price] }} USDT. Required deposit: {{ Number(selected.price) + fees[selected.price] }} USDT. Only the package amount is swapped to SIGNET.</p>
      <button type="submit" class="btn btn-primary mt-4" :disabled="loading || submitting || !selectedPackage">Next</button>
    </form>
    <div class="form-group"></div>
  </AuthCardLayout>
</template>
