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
import { usdtDisplay } from '@/utils/usdtDisplay'
import RegistrationProgress from '@/components/RegistrationProgress.vue'

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
  <AuthCardLayout back-text="Back to login" back-to="/login" card-class="package-selection-card p-4 p-lg-5 w-100">
    <RegistrationProgress :step="3" />
    <h1 class="h3 mb-2">Choose your package</h1>
    <p class="text-muted mb-4">Select a package that suits you. Review the deposit total before continuing.</p>
    <FlashAlert type="danger" :message="loadError || formError" @close="formError = ''" />
    <form method="POST" @submit.prevent="onSubmit">
      <fieldset class="package-choices" :disabled="loading || submitting || !!loadError">
        <legend class="h6">Available packages</legend>
        <input type="hidden" name="newUserID" :value="id" />
        <p v-if="loading" role="status">Loading available packages…</p>
        <p v-else-if="!visiblePackages.length && !loadError" role="status">No packages are currently available. Please check again later.</p>
        <div class="package-grid">
          <label v-for="pack in visiblePackages" :key="pack.id" class="package-choice" :class="{ selected: selectedPackage === String(pack.id) }">
            <input v-model="selectedPackage" type="radio" name="package" :value="String(pack.id)" required />
            <span><small>{{ pack.name }}</small><strong>{{ usdtDisplay(pack.price) }} <em>USDT</em></strong></span>
          </label>
        </div>
      </fieldset>
      <p v-if="errors.package" class="text-danger mt-2" role="alert">{{ errors.package[0] }}</p>
      <label v-if="blockchainEnabled" class="small text-muted mt-3"><input v-model="showLowerPackages" type="checkbox" :disabled="loading || submitting" /> Show lower / test packages</label>
      <section v-if="blockchainEnabled && selected && fees[selected.price] != null" class="package-summary mt-4" aria-label="Selected package cost">
        <h2 class="h6">Your deposit summary</h2>
        <dl><div><dt>Package</dt><dd>{{ usdtDisplay(selected.price) }} USDT</dd></div><div><dt>Service fee</dt><dd>{{ usdtDisplay(fees[selected.price]) }} USDT</dd></div>
        <div class="package-summary-total"><dt>Total to deposit</dt><dd>{{ usdtDisplay(Number(selected.price) + fees[selected.price]) }} USDT</dd></div></dl>
        <p class="small mb-0 text-muted">Only the package amount is swapped to SIGNET. The service fee is paid separately. You’ll receive your personal deposit address on the next screen.</p>
      </section>
      <button type="submit" class="btn btn-primary w-100 mt-4" :disabled="loading || submitting || !!loadError || !selectedPackage">{{ submitting ? 'Preparing next step…' : blockchainEnabled ? 'Continue to funding' : 'Continue' }}</button>
    </form>
    <div class="form-group"></div>
  </AuthCardLayout>
</template>

<style scoped>
.package-selection-card { max-width:760px;margin:24px 0;border:1px solid var(--sg-line-3);border-radius:24px;background:var(--sg-panel);box-shadow:var(--sg-shadow); }
.package-choices { margin:0;padding:0;border:0; }
.package-grid { display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px; }
.package-choice { display:flex;align-items:center;gap:12px;padding:18px;border:1px solid var(--sg-line-3);border-radius:14px;cursor:pointer;background:var(--sg-glass); }
.package-choice.selected { border-color:var(--sg-accent);background:rgba(0,240,255,.07); }
.package-choice input { accent-color:var(--sg-accent);flex-shrink:0; }
.package-choice small { display:block;color:var(--sg-muted);margin-bottom:6px;overflow-wrap:anywhere; }
.package-choice strong { font-size:20px;font-variant-numeric:tabular-nums;overflow-wrap:anywhere; }
.package-choice em { font-style:normal;font-size:11px;color:var(--sg-accent-light); }
.package-summary { padding:22px;background:rgba(0,240,255,.04);border:1px solid var(--sg-line-3);border-radius:16px; }
.package-summary dl { margin:16px 0; }.package-summary dl>div { display:flex;justify-content:space-between;gap:12px;margin:10px 0; }
.package-summary dt { font-weight:400;color:var(--sg-muted); }.package-summary dd { margin:0;text-align:right;font-variant-numeric:tabular-nums; }
.package-summary-total { border-top:1px solid var(--sg-line-3);padding-top:14px;font-weight:700; }
.package-choice:focus-within { outline:2px solid var(--sg-accent-light);outline-offset:3px; }
@media(max-width:575px) { .package-grid { grid-template-columns:1fr; } }
</style>
