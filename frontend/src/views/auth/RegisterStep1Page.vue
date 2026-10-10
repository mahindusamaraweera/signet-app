<script setup>
// Ports auth/register_step1.blade.php — on-screen labelled "Step 1 of 2"
// (route('register.step1') / posts to register.processStep1).
// See registerReferralHandler / registerStep1Handler in auth_handler.go.
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Swal from 'sweetalert2'
import api from '@/api/client'
import FlashAlert from '@/components/shared/FlashAlert.vue'

const route = useRoute()
const router = useRouter()

const loading = ref(true)
const loadError = ref('')
const countries = ref([])
const leaders = ref([])
const submitting = ref(false)
const errors = ref({})
const formError = ref('')
const verificationNotice = ref('')
const blockchainEnabled = ref(false)

const form = reactive({
  referral_code: '',
  leader_code: '',
  executive_code: '',
  country: '',
  name: '',
  email: '',
  country_code: '+1',
  whatsapp_number: '',
  binance_pay_id: '',
  password: '',
  password_confirmation: '',
})

// Exact dial-code list transcribed from register_step1.blade.php (country-name order, duplicate codes preserved verbatim).
const dialCodes = [
  { code: "+1", label: "+1 (USA, Canada)" },
  { code: "+93", label: "+93 (Afghanistan)" },
  { code: "+355", label: "+355 (Albania)" },
  { code: "+213", label: "+213 (Algeria)" },
  { code: "+376", label: "+376 (Andorra)" },
  { code: "+244", label: "+244 (Angola)" },
  { code: "+1264", label: "+1264 (Anguilla)" },
  { code: "+1268", label: "+1268 (Antigua and Barbuda)" },
  { code: "+54", label: "+54 (Argentina)" },
  { code: "+374", label: "+374 (Armenia)" },
  { code: "+61", label: "+61 (Australia)" },
  { code: "+43", label: "+43 (Austria)" },
  { code: "+994", label: "+994 (Azerbaijan)" },
  { code: "+1242", label: "+1242 (Bahamas)" },
  { code: "+973", label: "+973 (Bahrain)" },
  { code: "+880", label: "+880 (Bangladesh)" },
  { code: "+1246", label: "+1246 (Barbados)" },
  { code: "+375", label: "+375 (Belarus)" },
  { code: "+32", label: "+32 (Belgium)" },
  { code: "+501", label: "+501 (Belize)" },
  { code: "+229", label: "+229 (Benin)" },
  { code: "+975", label: "+975 (Bhutan)" },
  { code: "+591", label: "+591 (Bolivia)" },
  { code: "+387", label: "+387 (Bosnia and Herzegovina)" },
  { code: "+267", label: "+267 (Botswana)" },
  { code: "+55", label: "+55 (Brazil)" },
  { code: "+1284", label: "+1284 (British Virgin Islands)" },
  { code: "+673", label: "+673 (Brunei)" },
  { code: "+359", label: "+359 (Bulgaria)" },
  { code: "+226", label: "+226 (Burkina Faso)" },
  { code: "+257", label: "+257 (Burundi)" },
  { code: "+855", label: "+855 (Cambodia)" },
  { code: "+237", label: "+237 (Cameroon)" },
  { code: "+1", label: "+1 (Canada)" },
  { code: "+238", label: "+238 (Cape Verde)" },
  { code: "+1345", label: "+1345 (Cayman Islands)" },
  { code: "+56", label: "+56 (Chile)" },
  { code: "+86", label: "+86 (China)" },
  { code: "+57", label: "+57 (Colombia)" },
  { code: "+269", label: "+269 (Comoros)" },
  { code: "+242", label: "+242 (Congo, Republic of the)" },
  { code: "+243", label: "+243 (Congo, Democratic Republic of the)" },
  { code: "+682", label: "+682 (Cook Islands)" },
  { code: "+506", label: "+506 (Costa Rica)" },
  { code: "+225", label: "+225 (Côte d'Ivoire)" },
  { code: "+385", label: "+385 (Croatia)" },
  { code: "+53", label: "+53 (Cuba)" },
  { code: "+599", label: "+599 (Curacao)" },
  { code: "+357", label: "+357 (Cyprus)" },
  { code: "+420", label: "+420 (Czech Republic)" },
  { code: "+45", label: "+45 (Denmark)" },
  { code: "+253", label: "+253 (Djibouti)" },
  { code: "+1767", label: "+1767 (Dominica)" },
  { code: "+1849", label: "+1849 (Dominican Republic)" },
  { code: "+593", label: "+593 (Ecuador)" },
  { code: "+20", label: "+20 (Egypt)" },
  { code: "+503", label: "+503 (El Salvador)" },
  { code: "+240", label: "+240 (Equatorial Guinea)" },
  { code: "+291", label: "+291 (Eritrea)" },
  { code: "+372", label: "+372 (Estonia)" },
  { code: "+251", label: "+251 (Ethiopia)" },
  { code: "+500", label: "+500 (Falkland Islands)" },
  { code: "+298", label: "+298 (Faroe Islands)" },
  { code: "+679", label: "+679 (Fiji)" },
  { code: "+358", label: "+358 (Finland)" },
  { code: "+33", label: "+33 (France)" },
  { code: "+594", label: "+594 (French Guiana)" },
  { code: "+689", label: "+689 (French Polynesia)" },
  { code: "+241", label: "+241 (Gabon)" },
  { code: "+220", label: "+220 (Gambia)" },
  { code: "+995", label: "+995 (Georgia)" },
  { code: "+49", label: "+49 (Germany)" },
  { code: "+233", label: "+233 (Ghana)" },
  { code: "+350", label: "+350 (Gibraltar)" },
  { code: "+30", label: "+30 (Greece)" },
  { code: "+299", label: "+299 (Greenland)" },
  { code: "+1473", label: "+1473 (Grenada)" },
  { code: "+590", label: "+590 (Guadeloupe)" },
  { code: "+502", label: "+502 (Guatemala)" },
  { code: "+44", label: "+44 (Guernsey)" },
  { code: "+224", label: "+224 (Guinea)" },
  { code: "+245", label: "+245 (Guinea-Bissau)" },
  { code: "+595", label: "+595 (Guyana)" },
  { code: "+509", label: "+509 (Haiti)" },
  { code: "+504", label: "+504 (Honduras)" },
  { code: "+852", label: "+852 (Hong Kong)" },
  { code: "+36", label: "+36 (Hungary)" },
  { code: "+354", label: "+354 (Iceland)" },
  { code: "+91", label: "+91 (India)" },
  { code: "+62", label: "+62 (Indonesia)" },
  { code: "+98", label: "+98 (Iran)" },
  { code: "+964", label: "+964 (Iraq)" },
  { code: "+353", label: "+353 (Ireland)" },
  { code: "+972", label: "+972 (Israel)" },
  { code: "+39", label: "+39 (Italy)" },
  { code: "+1", label: "+1 (Jamaica)" },
  { code: "+81", label: "+81 (Japan)" },
  { code: "+44", label: "+44 (Jersey)" },
  { code: "+962", label: "+962 (Jordan)" },
  { code: "+7", label: "+7 (Kazakhstan)" },
  { code: "+254", label: "+254 (Kenya)" },
  { code: "+686", label: "+686 (Kiribati)" },
  { code: "+965", label: "+965 (Kuwait)" },
  { code: "+996", label: "+996 (Kyrgyzstan)" },
  { code: "+856", label: "+856 (Laos)" },
  { code: "+371", label: "+371 (Latvia)" },
  { code: "+961", label: "+961 (Lebanon)" },
  { code: "+266", label: "+266 (Lesotho)" },
  { code: "+231", label: "+231 (Liberia)" },
  { code: "+218", label: "+218 (Libya)" },
  { code: "+423", label: "+423 (Liechtenstein)" },
  { code: "+370", label: "+370 (Lithuania)" },
  { code: "+352", label: "+352 (Luxembourg)" },
  { code: "+853", label: "+853 (Macau)" },
  { code: "+389", label: "+389 (Macedonia)" },
  { code: "+261", label: "+261 (Madagascar)" },
  { code: "+265", label: "+265 (Malawi)" },
  { code: "+60", label: "+60 (Malaysia)" },
  { code: "+960", label: "+960 (Maldives)" },
  { code: "+223", label: "+223 (Mali)" },
  { code: "+356", label: "+356 (Malta)" },
  { code: "+692", label: "+692 (Marshall Islands)" },
  { code: "+596", label: "+596 (Martinique)" },
  { code: "+222", label: "+222 (Mauritania)" },
  { code: "+230", label: "+230 (Mauritius)" },
  { code: "+262", label: "+262 (Mayotte)" },
  { code: "+52", label: "+52 (Mexico)" },
  { code: "+691", label: "+691 (Micronesia)" },
  { code: "+373", label: "+373 (Moldova)" },
  { code: "+377", label: "+377 (Monaco)" },
  { code: "+976", label: "+976 (Mongolia)" },
  { code: "+382", label: "+382 (Montenegro)" },
  { code: "+1664", label: "+1664 (Montserrat)" },
  { code: "+212", label: "+212 (Morocco)" },
  { code: "+258", label: "+258 (Mozambique)" },
  { code: "+95", label: "+95 (Myanmar)" },
  { code: "+264", label: "+264 (Namibia)" },
  { code: "+674", label: "+674 (Nauru)" },
  { code: "+977", label: "+977 (Nepal)" },
  { code: "+31", label: "+31 (Netherlands)" },
  { code: "+687", label: "+687 (New Caledonia)" },
  { code: "+64", label: "+64 (New Zealand)" },
  { code: "+505", label: "+505 (Nicaragua)" },
  { code: "+227", label: "+227 (Niger)" },
  { code: "+234", label: "+234 (Nigeria)" },
  { code: "+683", label: "+683 (Niue)" },
  { code: "+850", label: "+850 (North Korea)" },
  { code: "+47", label: "+47 (Norway)" },
  { code: "+968", label: "+968 (Oman)" },
  { code: "+92", label: "+92 (Pakistan)" },
  { code: "+680", label: "+680 (Palau)" },
  { code: "+970", label: "+970 (Palestinian Territories)" },
  { code: "+507", label: "+507 (Panama)" },
  { code: "+675", label: "+675 (Papua New Guinea)" },
  { code: "+595", label: "+595 (Paraguay)" },
  { code: "+51", label: "+51 (Peru)" },
  { code: "+63", label: "+63 (Philippines)" },
  { code: "+48", label: "+48 (Poland)" },
  { code: "+351", label: "+351 (Portugal)" },
  { code: "+1", label: "+1 (Puerto Rico)" },
  { code: "+974", label: "+974 (Qatar)" },
  { code: "+40", label: "+40 (Romania)" },
  { code: "+7", label: "+7 (Russia)" },
  { code: "+250", label: "+250 (Rwanda)" },
  { code: "+590", label: "+590 (Saint Barthelemy)" },
  { code: "+290", label: "+290 (Saint Helena)" },
  { code: "+1", label: "+1 (Saint Kitts and Nevis)" },
  { code: "+508", label: "+508 (Saint Pierre and Miquelon)" },
  { code: "+1", label: "+1 (Saint Vincent and the Grenadines)" },
  { code: "+221", label: "+221 (Senegal)" },
  { code: "+381", label: "+381 (Serbia)" },
  { code: "+248", label: "+248 (Seychelles)" },
  { code: "+232", label: "+232 (Sierra Leone)" },
  { code: "+65", label: "+65 (Singapore)" },
  { code: "+1", label: "+1 (Sint Maarten)" },
  { code: "+421", label: "+421 (Slovakia)" },
  { code: "+386", label: "+386 (Slovenia)" },
  { code: "+677", label: "+677 (Solomon Islands)" },
  { code: "+252", label: "+252 (Somalia)" },
  { code: "+27", label: "+27 (South Africa)" },
  { code: "+82", label: "+82 (South Korea)" },
  { code: "+34", label: "+34 (Spain)" },
  { code: "+94", label: "+94 (Sri Lanka)" },
  { code: "+249", label: "+249 (Sudan)" },
  { code: "+597", label: "+597 (Suriname)" },
  { code: "+47", label: "+47 (Svalbard and Jan Mayen)" },
  { code: "+268", label: "+268 (Swaziland)" },
  { code: "+46", label: "+46 (Sweden)" },
  { code: "+41", label: "+41 (Switzerland)" },
  { code: "+963", label: "+963 (Syria)" },
  { code: "+886", label: "+886 (Taiwan)" },
  { code: "+992", label: "+992 (Tajikistan)" },
  { code: "+255", label: "+255 (Tanzania)" },
  { code: "+66", label: "+66 (Thailand)" },
  { code: "+670", label: "+670 (Timor-Leste)" },
  { code: "+228", label: "+228 (Togo)" },
  { code: "+690", label: "+690 (Tokelau)" },
  { code: "+676", label: "+676 (Tonga)" },
  { code: "+1", label: "+1 (Trinidad and Tobago)" },
  { code: "+216", label: "+216 (Tunisia)" },
  { code: "+90", label: "+90 (Turkey)" },
  { code: "+993", label: "+993 (Turkmenistan)" },
  { code: "+1", label: "+1 (Turks and Caicos Islands)" },
  { code: "+688", label: "+688 (Tuvalu)" },
  { code: "+256", label: "+256 (Uganda)" },
  { code: "+380", label: "+380 (Ukraine)" },
  { code: "+971", label: "+971 (United Arab Emirates)" },
  { code: "+44", label: "+44 (United Kingdom)" },
  { code: "+1", label: "+1 (United States)" },
  { code: "+598", label: "+598 (Uruguay)" },
  { code: "+998", label: "+998 (Uzbekistan)" },
  { code: "+678", label: "+678 (Vanuatu)" },
  { code: "+58", label: "+58 (Venezuela)" },
  { code: "+84", label: "+84 (Vietnam)" },
  { code: "+1284", label: "+1284 (Virgin Islands)" },
  { code: "+967", label: "+967 (Yemen)" },
  { code: "+260", label: "+260 (Zambia)" },
  { code: "+263", label: "+263 (Zimbabwe)" },
]

onMounted(async () => {
  form.referral_code = route.query.ref || ''
  try {
    const { data } = await api.get('/register/referral', { params: { ref: form.referral_code } })
    if (data.status === 'verification_required') {
      form.password = ''
      form.password_confirmation = ''
      await Swal.fire({ icon: 'info', title: 'Verify your email', text: data.message || 'Check your inbox for the verification link. Your account is created only after verification.' })
    } else if (data.status === 'success') {
      blockchainEnabled.value = !!data.blockchain_enabled
      countries.value = data.countries || []
      leaders.value = data.leaders || []
    } else {
      loadError.value = data.message || 'Invalid referral code'
    }
  } catch (err) {
    loadError.value = err?.response?.data?.message || 'Invalid referral code'
  } finally {
    loading.value = false
  }
})

function checkDuplicateSelection(changed) {
  if (form.leader_code && form.executive_code && form.leader_code === form.executive_code) {
    Swal.fire({
      icon: 'warning',
      title: 'Invalid Selection',
      text: 'Manager and Executive Manager cannot be the same user.',
    })
    if (changed === 'leader') form.leader_code = ''
    else form.executive_code = ''
  }
}

async function onSubmit() {
  errors.value = {}
  formError.value = ''
  submitting.value = true
  try {
    const { data } = await api.post('/register/step1', { ...form })
    if (data.status === 'verification_required') {
      verificationNotice.value = 'Check your inbox and spam folder for the verification link. Your account is created only after you verify your email. Then you can select a package and fund your personal wallet.'
      form.password = ''
      form.password_confirmation = ''
    } else if (data.status === 'success') {
      router.push({ name: 'register.step2', params: { id: data.user_id } })
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
  <main>
    <section class="min-vh-100 py-5 d-flex align-items-center" style="background: linear-gradient(180deg, #f7f9fc 0%, #eef1f8 100%)">
      <div class="container">
        <div class="row justify-content-center">
          <div class="col-12 col-lg-8 col-xl-7">
            <div class="mb-3">
              <RouterLink to="/login" class="d-inline-flex align-items-center text-decoration-none text-secondary small fw-medium">
                <svg class="me-1" width="16" height="16" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg">
                  <path fill-rule="evenodd" d="M7.707 14.707a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 1.414L5.414 9H17a1 1 0 110 2H5.414l2.293 2.293a1 1 0 010 1.414z" clip-rule="evenodd"></path>
                </svg>
                Back to log in
              </RouterLink>
            </div>

            <div v-if="loading" class="card border-0 shadow-lg rounded-4 overflow-hidden p-5 text-center text-muted">Loading&hellip;</div>

            <div v-else-if="loadError" class="card border-0 shadow-lg rounded-4 overflow-hidden p-4">
              <FlashAlert type="danger" :message="loadError" />
              <RouterLink to="/login" class="btn btn-primary mt-2">Go to Login</RouterLink>
            </div>

            <div v-else class="card border-0 shadow-lg rounded-4 overflow-hidden">
              <div class="px-4 px-md-5 pt-4 pt-md-5">
                <div class="d-flex align-items-center justify-content-between mb-2">
                  <span class="badge rounded-pill text-bg-primary bg-opacity-10 text-primary fw-semibold px-3 py-2">Step 1 of 2</span>
                  <span class="text-muted small">Account details</span>
                </div>
                <h1 class="h3 fw-bold mb-1">Create your account</h1>
                <p class="text-muted mb-4">Account details → verify email → select package → fund your wallet.</p>
                <div class="progress mb-4" style="height: 6px; border-radius: 999px">
                  <div class="progress-bar bg-primary" role="progressbar" style="width: 50%; border-radius: 999px"></div>
                </div>
              </div>

              <form method="POST" class="px-4 px-md-5 pb-4 pb-md-5" @submit.prevent="onSubmit">
                <FlashAlert type="danger" :message="formError" @close="formError = ''" />
                <div v-if="verificationNotice" class="alert alert-info" role="status">
                  <h2 class="h6">Verify your email to continue</h2>
                  <p class="mb-0">{{ verificationNotice }}</p>
                </div>

                <div class="bg-light rounded-3 mb-4">
                  <h2 class="h6 fw-bold text-uppercase text-muted mb-3" style="letter-spacing: .05em; font-size: .75rem">Referral &amp; management</h2>
                  <div class="row g-3">
                    <div class="col-12">
                      <label for="referral_code" class="form-label small fw-semibold">Referral Code</label>
                      <input id="referral_code" type="text" class="form-control bg-white" :value="form.referral_code" readonly />
                    </div>
                    <div class="col-12 col-md-6">
                      <label for="leader_code" class="form-label small fw-semibold">Manager Code</label>
                      <select id="leader_code" v-model="form.leader_code" class="form-select bg-white" :class="{ 'is-invalid': errors.leader_code }" @change="checkDuplicateSelection('leader')">
                        <option value="">Select Manager</option>
                        <option v-for="l in leaders" :key="l.id" :value="String(l.id)">{{ l.signet_id }} - {{ l.name }}</option>
                      </select>
                      <div v-if="errors.leader_code" class="invalid-feedback">{{ errors.leader_code[0] }}</div>
                    </div>
                    <div class="col-12 col-md-6">
                      <label for="executive_code" class="form-label small fw-semibold">Executive Code</label>
                      <select id="executive_code" v-model="form.executive_code" class="form-select bg-white" :class="{ 'is-invalid': errors.executive_code }" @change="checkDuplicateSelection('executive')">
                        <option value="">Select Executive</option>
                        <option v-for="l in leaders" :key="l.id" :value="String(l.id)">{{ l.signet_id }} - {{ l.name }}</option>
                      </select>
                      <div v-if="errors.executive_code" class="invalid-feedback">{{ errors.executive_code[0] }}</div>
                    </div>
                  </div>
                </div>

                <div class="mb-4">
                  <h2 class="h6 fw-bold text-uppercase text-muted mb-3" style="letter-spacing: .05em; font-size: .75rem">Personal details</h2>
                  <div class="row g-3">
                    <div class="col-12">
                      <label for="country_id" class="form-label small fw-semibold">Country</label>
                      <select id="country_id" v-model="form.country" class="form-select" :class="{ 'is-invalid': errors.country }" required>
                        <option value="">Select Country</option>
                        <option v-for="c in countries" :key="c.id" :value="String(c.id)">{{ c.code }} - {{ c.name }}</option>
                      </select>
                      <div v-if="errors.country" class="invalid-feedback">{{ errors.country[0] }}</div>
                    </div>
                    <div class="col-12">
                      <label for="name" class="form-label small fw-semibold">Full Name</label>
                      <input id="name" v-model="form.name" type="text" class="form-control" :class="{ 'is-invalid': errors.name }" required placeholder="Jane Doe" />
                      <div v-if="errors.name" class="invalid-feedback">{{ errors.name[0] }}</div>
                    </div>
                    <div class="col-12">
                      <label for="email" class="form-label small fw-semibold">Email Address</label>
                      <input id="email" v-model="form.email" type="email" class="form-control" :class="{ 'is-invalid': errors.email }" required placeholder="jane@example.com" />
                      <div v-if="errors.email" class="invalid-feedback">{{ errors.email[0] }}</div>
                    </div>
                    <div class="col-12">
                      <label for="whatsapp_number" class="form-label small fw-semibold">WhatsApp Number</label>
                      <div class="input-group">
                        <select v-model="form.country_code" class="form-select" :class="{ 'is-invalid': errors.country_code }" style="max-width: 150px" required>
                          <option v-for="(d, i) in dialCodes" :key="i" :value="d.code">{{ d.label }}</option>
                        </select>
                        <input
                          id="whatsapp_number"
                          v-model="form.whatsapp_number"
                          type="text"
                          class="form-control"
                          :class="{ 'is-invalid': errors.whatsapp_number }"
                          required
                          pattern="^\d{9}$"
                          title="Please enter exactly 9 digits (0-9)"
                          placeholder="712345678"
                        />
                        <div v-if="errors.whatsapp_number" class="invalid-feedback">{{ errors.whatsapp_number[0] }}</div>
                      </div>
                      <div class="form-text">Enter 9 digits without the leading zero.</div>
                    </div>
                  </div>
                </div>

                <div class="mb-4">
                  <h2 class="h6 fw-bold text-uppercase text-muted mb-3" style="letter-spacing: .05em; font-size: .75rem">Payment &amp; security</h2>
                  <div class="row g-3">
                    <div v-if="!blockchainEnabled" class="col-12">
                      <label for="binance_pay_id" class="form-label small fw-semibold">Binance Pay ID</label>
                      <input id="binance_pay_id" v-model="form.binance_pay_id" type="text" class="form-control" :class="{ 'is-invalid': errors.binance_pay_id }" required />
                      <div v-if="errors.binance_pay_id" class="invalid-feedback">{{ errors.binance_pay_id[0] }}</div>
                    </div>
                    <div class="col-12 col-md-6">
                      <label for="password" class="form-label small fw-semibold">Password</label>
                      <input id="password" v-model="form.password" type="password" class="form-control" :class="{ 'is-invalid': errors.password }" required />
                      <div v-if="errors.password" class="invalid-feedback">{{ errors.password[0] }}</div>
                    </div>
                    <div class="col-12 col-md-6">
                      <label for="password_confirmation" class="form-label small fw-semibold">Confirm Password</label>
                      <input id="password_confirmation" v-model="form.password_confirmation" type="password" class="form-control" required />
                    </div>
                  </div>
                </div>

                <button type="submit" class="btn btn-primary btn-lg w-100 rounded-3 fw-semibold py-3" :disabled="submitting">
                  {{ submitting ? 'Submitting…' : 'Send verification email' }}
                </button>
              </form>
            </div>

            <p class="text-center text-muted small mt-4 mb-0">
              Already have an account? <RouterLink to="/login" class="fw-semibold text-decoration-none">Sign in</RouterLink>
            </p>
          </div>
        </div>
      </div>
    </section>
  </main>
</template>
