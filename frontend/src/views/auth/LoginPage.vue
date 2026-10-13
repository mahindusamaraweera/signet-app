<script setup>
// Successful pending registrations resume their funding/status page.
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import api from '@/api/client'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'
import { useAuthStore } from '@/store/auth'

const router = useRouter()
const authStore = useAuthStore()

const form = reactive({ email: '', password: '', remember: false })
const errors = ref({})
const flashMessage = ref('')
const submitting = ref(false)
const showPassword = ref(false)

function togglePassword() {
  showPassword.value = !showPassword.value
}

async function onSubmit() {
  errors.value = {}
  flashMessage.value = ''
  submitting.value = true
  try {
    const { data } = await api.post('/login', { email: form.email, password: form.password })
    // The session cookie is issued as soon as credentials check out, even
    // when the response below is `status:'error'` (no package / pending
    // package) — so refresh the shared auth state regardless of status.
    await authStore.bootstrap()
    if (data.status === 'success') {
      router.push(data.redirect || authStore.landingRoute)
    } else {
      flashMessage.value = data.message || 'Invalid credentials'
    }
  } catch (err) {
    if (err?.response?.status === 422) {
      errors.value = err.response.data.errors || {}
    } else {
      flashMessage.value = err?.response?.data?.message || 'Something went wrong. Please try again.'
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthCardLayout signin-background card-class="bg-white shadow border-0 rounded border-light p-4 p-lg-5 w-100 fmxw-500">
    <div class="text-center text-md-center mb-4 mt-md-0">
      <img class="navbar-brand-dark mb-3" src="/resources/site/assets/logo/signet-light.png" alt="Signet" style="height: 80px !important" />
      <h1 class="mb-0 h3">Sign in to Signetint platform</h1>
    </div>
    <div v-if="flashMessage" class="alert alert-danger">{{ flashMessage }}</div>
    <form class="mt-4" method="POST" @submit.prevent="onSubmit">
      <!-- Form -->
      <div class="form-group mb-4">
        <label for="email">Your Email</label>
        <div class="input-group">
          <span id="basic-addon1" class="input-group-text">
            <svg class="icon icon-xs text-gray-600" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path d="M2.003 5.884L10 9.882l7.997-3.998A2 2 0 0016 4H4a2 2 0 00-1.997 1.884z"></path><path d="M18 8.118l-8 4-8-4V14a2 2 0 002 2h12a2 2 0 002-2V8.118z"></path></svg>
          </span>
          <input id="email" v-model="form.email" type="email" class="form-control" placeholder="example@company.com" name="email" autofocus required />
        </div>
      </div>
      <!-- End of Form -->
      <div class="form-group">
        <!-- Form -->
        <div class="form-group mb-4">
          <label for="password">Your Password</label>
          <div class="input-group">
            <span id="basic-addon2" class="input-group-text">
              <svg class="icon icon-xs text-gray-600" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z" clip-rule="evenodd"></path></svg>
            </span>
            <input id="password" v-model="form.password" :type="showPassword ? 'text' : 'password'" placeholder="Password" class="form-control" name="password" required />
            <span class="input-group-text" style="cursor: pointer" @click="togglePassword">
              <svg id="eyeIcon" class="icon icon-xs text-gray-600" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg">
                <path id="eyeOpen" :style="{ display: showPassword ? 'none' : 'block' }" d="M10 3C5 3 1 10 1 10s4 7 9 7 9-7 9-7-4-7-9-7zm0 12a5 5 0 110-10 5 5 0 010 10z"></path>
                <path id="eyeClosed" :style="{ display: showPassword ? 'block' : 'none' }" d="M10 3C5 3 1 10 1 10s4 7 9 7 9-7 9-7-4-7-9-7zm0 12a5 5 0 110-10 5 5 0 010 10zm5.707-8.707l-1.414-1.414L10 9.586 6.707 6.293 5.293 7.707 8.586 11l-3.293 3.293 1.414 1.414L10 12.414l3.293 3.293 1.414-1.414L11.414 11l3.293-3.293z"></path>
              </svg>
            </span>
          </div>
        </div>
        <!-- End of Form -->
        <div class="d-flex justify-content-between align-items-top mb-4">
          <div class="form-check">
            <input id="remember" v-model="form.remember" class="form-check-input" type="checkbox" value="1" name="remember" />
            <label class="form-check-label mb-0" for="remember">
              Remember me
            </label>
          </div>
          <div><RouterLink to="/password/reset" class="small text-right">Lost password?</RouterLink></div>
        </div>
      </div>
      <div class="d-grid">
        <button type="submit" class="btn btn-gray-800" :disabled="submitting">Sign in</button>
      </div>
    </form>
  </AuthCardLayout>
</template>
