<script setup>
// Ports auth/password_reset_form.blade.php (route('password.reset', $token) /
// route('password.update')). See passwordResetHandler in auth_handler.go.
// As in the original, the email field starts empty, field errors show under
// their inputs, and a successful reset lands on the login page.
import { reactive, ref, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import api from '@/api/client'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'

const props = defineProps({
  token: { type: String, default: '' },
})

const router = useRouter()
// Keep new reset proofs in memory, not request URLs or persistent storage.
const token = ref(new URLSearchParams(window.location.hash.slice(1)).get('token') || props.token)
if (window.location.hash) {
  window.history.replaceState(window.history.state, '', window.location.pathname + window.location.search)
}
const form = reactive({ email: '', password: '', password_confirmation: '' })
onBeforeUnmount(() => {
  token.value = ''
  form.password = ''
  form.password_confirmation = ''
})
const errors = ref({})
const submitting = ref(false)

async function onSubmit() {
  errors.value = {}
  submitting.value = true
  try {
    const { data } = await api.post('/password/reset', {
      token: token.value,
      email: form.email,
      password: form.password,
      password_confirmation: form.password_confirmation,
    })
    if (data.status === 'success') {
      router.push('/login')
    } else if (data.errors) {
      errors.value = data.errors
    }
  } catch (err) {
    errors.value = err?.response?.data?.errors || {}
  } finally {
    form.password = ''
    form.password_confirmation = ''
    submitting.value = false
  }
}
</script>

<template>
  <AuthCardLayout back-text="Back to log in" back-to="/login" back-placement="row">
    <h1 class="h3 mb-4">Reset password</h1>
    <form method="POST" @submit.prevent="onSubmit">
      <input type="hidden" name="token" :value="token" />
      <!-- Form -->
      <div class="mb-4">
        <label for="email">Your Email</label>
        <div class="input-group">
          <input id="email" v-model="form.email" type="email" class="form-control" :class="{ 'is-invalid': errors.email }" name="email" required />
        </div>
        <span v-if="errors.email" class="invalid-feedback" role="alert">
          <strong>{{ errors.email[0] }}</strong>
        </span>
      </div>
      <!-- End of Form -->
      <!-- Form -->
      <div class="form-group mb-4">
        <label for="password">Your Password</label>
        <div class="input-group">
          <span id="basic-addon2" class="input-group-text">
            <svg class="icon icon-xs text-gray-600" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z" clip-rule="evenodd"></path></svg>
          </span>
          <input id="password" v-model="form.password" type="password" class="form-control" :class="{ 'is-invalid': errors.password }" name="password" required />
          <span v-if="errors.password" class="invalid-feedback" role="alert">
            <strong>{{ errors.password[0] }}</strong>
          </span>
        </div>
      </div>
      <!-- End of Form -->
      <!-- Form -->
      <div class="form-group mb-4">
        <label for="confirm_password">Confirm Password</label>
        <div class="input-group">
          <span class="input-group-text">
            <svg class="icon icon-xs text-gray-600" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z" clip-rule="evenodd"></path></svg>
          </span>
          <input id="password_confirmation" v-model="form.password_confirmation" type="password" class="form-control" name="password_confirmation" required />
        </div>
      </div>
      <!-- End of Form -->
      <div class="d-grid">
        <button type="submit" class="btn btn-gray-800" :disabled="submitting">
          Reset password
        </button>
      </div>
    </form>
  </AuthCardLayout>
</template>
