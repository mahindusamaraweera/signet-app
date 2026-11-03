<script setup>
// Successful pending registrations resume their funding/status page.
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import api from '@/api/client'
import { useAuthStore } from '@/store/auth'

const router = useRouter()
const authStore = useAuthStore()

const form = reactive({ email: '', password: '', remember: false })
const errors = ref({})
const flashMessage = ref('')
const submitting = ref(false)
const showPassword = ref(false)

// Hero artwork lives in public/; until it is uploaded, show the brand logo.
const logoSrc = '/resources/site/assets/logo/signet-light.png'
const heroSrc = ref('/resources/site/assets/login/signet-infinity.png')

function onHeroError() {
  heroSrc.value = logoSrc
}

const features = [
  { title: 'Long-Term Staking', text: 'Plans built for the long run', icon: 'bolt' },
  { title: 'Staking Rewards', text: 'Rewards subject to plan terms', icon: 'chart' },
  { title: 'More Liquidity', text: 'Access according to lock-up conditions', icon: 'cycle' },
]

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
  <main class="crown-login">
    <header class="crown-topbar">
      <RouterLink to="/" aria-label="Back to Homepage">
        <img :src="logoSrc" alt="Signet" class="crown-brand" />
      </RouterLink>
    </header>

    <div class="crown-grid">
      <section class="crown-hero" aria-label="Signet Infinity">
        <img :src="heroSrc" alt="Signet Infinity, long-term staking" class="crown-hero-img" @error="onHeroError" />
        <p class="crown-tagline">Stake long <span>•</span> Grow steady <span>•</span> Stay secure</p>
        <ul class="crown-features">
          <li v-for="item in features" :key="item.title">
            <span class="crown-feature-icon" aria-hidden="true">
              <svg v-if="item.icon === 'bolt'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"><path d="M13 2 4 14h7l-1 8 9-12h-7l1-8z" /></svg>
              <svg v-else-if="item.icon === 'chart'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M6 20v-6M12 20V8M18 20V4" /></svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 11a8 8 0 0 0-14.3-4.9L4 8M4 4v4h4M4 13a8 8 0 0 0 14.3 4.9L20 16M20 20v-4h-4" /></svg>
            </span>
            <span>
              <strong>{{ item.title }}</strong>
              <small>{{ item.text }}</small>
            </span>
          </li>
        </ul>
      </section>

      <section class="crown-card">
        <div class="crown-card-head">
          <svg class="crown-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="M12 12c-2-2.7-3.6-4-5.5-4a4 4 0 0 0 0 8c1.9 0 3.5-1.3 5.5-4zm0 0c2 2.7 3.6 4 5.5 4a4 4 0 0 0 0-8c-1.9 0-3.5 1.3-5.5 4z" /></svg>
          <div>
            <p class="crown-eyebrow">Signet Infinity</p>
            <h1>Welcome back</h1>
            <p class="crown-sub">Securely access your SIGNET portfolio.</p>
          </div>
        </div>

        <div v-if="flashMessage" class="alert alert-danger" role="alert">{{ flashMessage }}</div>
        <p v-for="(messages, field) in errors" :key="field" class="text-danger small" role="alert">{{ Array.isArray(messages) ? messages[0] : messages }}</p>

        <form method="POST" @submit.prevent="onSubmit">
          <label for="email" class="crown-label">Email address</label>
          <input id="email" v-model="form.email" type="email" class="crown-input" placeholder="Enter your email" name="email" autocomplete="username" autofocus required />

          <label for="password" class="crown-label">Password</label>
          <div class="crown-password">
            <input id="password" v-model="form.password" :type="showPassword ? 'text' : 'password'" class="crown-input" placeholder="Enter your password" name="password" autocomplete="current-password" required />
            <button type="button" class="crown-eye" :aria-label="showPassword ? 'Hide password' : 'Show password'" :aria-pressed="showPassword" @click="togglePassword">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true">
                <path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z" />
                <circle cx="12" cy="12" r="3" />
                <path v-if="showPassword" d="M4 4l16 16" />
              </svg>
            </button>
          </div>

          <div class="crown-row">
            <label class="crown-check" for="remember">
              <input id="remember" v-model="form.remember" type="checkbox" value="1" name="remember" />
              Remember device
            </label>
            <RouterLink to="/password/reset" class="crown-link">Forgot password?</RouterLink>
          </div>

          <button type="submit" class="crown-submit" :disabled="submitting">
            {{ submitting ? 'Signing in…' : 'Sign In' }}
            <svg v-if="!submitting" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6" /></svg>
          </button>

          <p class="crown-secure">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="5" y="11" width="14" height="10" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" /></svg>
            Secure account access
          </p>
        </form>

        <p class="crown-register">Don't have an account? <RouterLink to="/register" class="crown-link">Create one</RouterLink></p>
      </section>
    </div>
  </main>
</template>

<style scoped>
.crown-login {
  /* Signet theme palette (assets/theme/signet-ui.css), same as the old login. */
  --accent: var(--sg-accent, #00F0FF);
  --accent-soft: var(--sg-accent-light, #8FE9F5);
  --accent-rgb: 0, 240, 255;
  --ink: var(--sg-fg, #EAF6F8);
  --muted: var(--sg-muted, #9DB2BE);
  --field: var(--sg-input, rgba(4, 7, 10, 0.62));
  --line: var(--sg-line-3, rgba(255, 255, 255, 0.14));
  min-height: 100vh;
  min-height: 100svh;
  padding: 24px 16px 40px;
  color: var(--ink);
  /* Transparent so the theme's grid + top glow on <body> shows through. */
  background: radial-gradient(ellipse 55% 40% at 50% 55%, rgba(0, 240, 255, 0.07), transparent 70%);
}
.crown-topbar { max-width: 1280px; margin: 0 auto 8px; }
.crown-brand { height: 40px; width: auto; }

.crown-grid {
  max-width: 1280px;
  margin: 0 auto;
  display: grid;
  grid-template-columns: 1fr;
  gap: 32px;
  align-items: center;
}
@media (min-width: 992px) {
  .crown-grid { grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr); gap: 48px; min-height: calc(100svh - 120px); }
}

/* Hero */
.crown-hero { text-align: center; }
.crown-hero-img { width: 100%; max-width: 560px; height: auto; filter: drop-shadow(0 20px 40px rgba(0, 0, 0, 0.6)); }

.crown-tagline { margin: 0 0 24px; color: var(--accent); text-transform: uppercase; font-weight: 700; font-size: 0.8rem; letter-spacing: 0.05em; }
.crown-tagline span { margin: 0 6px; }
.crown-features { list-style: none; padding: 0; margin: 0 auto; max-width: 420px; display: grid; gap: 14px; text-align: left; }
.crown-features li { display: flex; align-items: center; gap: 16px; }
.crown-features strong { display: block; font-size: 0.95rem; }
.crown-features small { color: var(--muted); }
.crown-feature-icon {
  flex: 0 0 48px; height: 48px; display: grid; place-items: center;
  border: 1px solid rgba(var(--accent-rgb), 0.6); border-radius: 10px; color: var(--accent);
  background: rgba(var(--accent-rgb), 0.06);
}
.crown-feature-icon svg { width: 22px; height: 22px; }
@media (max-width: 991.98px) {
  .crown-hero-img { max-width: 320px; }
  .crown-features { display: none; }
}

/* Card */
.crown-card {
  width: 100%; max-width: 440px; margin: 0 auto; padding: 32px 28px;
  border: 1px solid rgba(var(--accent-rgb), 0.55); border-radius: 18px;
  background: var(--sg-card-bg), rgba(8, 13, 18, 0.9);
  box-shadow: 0 0 40px rgba(var(--accent-rgb), 0.08), 0 24px 60px rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(6px);
}
.crown-card-head { display: flex; gap: 14px; margin-bottom: 24px; }
.crown-icon { width: 30px; height: 30px; color: var(--accent); flex: 0 0 auto; margin-top: 2px; }
.crown-eyebrow { margin: 0 0 6px; color: var(--accent); font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; }
.crown-card h1 { margin: 0 0 8px; font-size: 1.75rem; font-weight: 600; color: #fff; }
.crown-sub { margin: 0; color: var(--muted); font-size: 0.85rem; }

.crown-label { display: block; margin: 0 0 8px; font-size: 0.9rem; font-weight: 500; color: var(--ink); }
.crown-input {
  width: 100%; height: 48px; padding: 0 14px; margin-bottom: 20px;
  border: 1px solid var(--line); border-radius: 10px; background: var(--field); color: var(--ink);
  font-size: 0.9rem; outline: none; transition: border-color 0.15s, box-shadow 0.15s;
}
.crown-input::placeholder { color: #6b7587; }
.crown-input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px rgba(var(--accent-rgb), 0.18); }
.crown-password { position: relative; }
.crown-password .crown-input { padding-right: 48px; }
.crown-eye {
  position: absolute; top: 0; right: 0; width: 48px; height: 48px;
  display: grid; place-items: center; border: 0; background: none; color: var(--muted); cursor: pointer;
}
.crown-eye svg { width: 18px; height: 18px; }
.crown-eye:hover { color: var(--ink); }

.crown-row { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin: 4px 0 24px; flex-wrap: wrap; }
.crown-check { display: flex; align-items: center; gap: 10px; margin: 0; font-size: 0.85rem; color: var(--ink); cursor: pointer; }
.crown-check input { width: 18px; height: 18px; accent-color: var(--accent); }
.crown-link { color: var(--accent); font-weight: 600; font-size: 0.85rem; text-decoration: none; }
.crown-link:hover { color: var(--accent-soft); text-decoration: underline; }

.crown-submit {
  width: 100%; height: 50px; display: flex; align-items: center; justify-content: center; gap: 8px;
  border: 0; border-radius: 10px; font-weight: 700; color: #04121f;
  background: var(--sg-grad, linear-gradient(100deg, #00F0FF, #0A7CFF));
  box-shadow: 0 8px 24px rgba(var(--accent-rgb), 0.25); cursor: pointer; transition: filter 0.15s;
}
.crown-submit svg { width: 18px; height: 18px; }
.crown-submit:hover:not(:disabled) { filter: brightness(1.06); }
.crown-submit:disabled { opacity: 0.7; cursor: wait; }

.crown-secure { display: flex; align-items: center; justify-content: center; gap: 6px; margin: 14px 0 0; color: var(--muted); font-size: 0.75rem; }
.crown-secure svg { width: 13px; height: 13px; }
.crown-register { margin: 24px 0 0; padding-top: 20px; border-top: 1px solid var(--line); text-align: center; color: var(--muted); font-size: 0.85rem; }
</style>
