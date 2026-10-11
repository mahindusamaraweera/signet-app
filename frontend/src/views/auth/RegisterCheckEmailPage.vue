<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'
import RegistrationProgress from '@/components/RegistrationProgress.vue'

const route = useRoute()
const restart = computed(() => ({ name: 'register.step1', query: typeof route.query.ref === 'string' ? { ref: route.query.ref } : {} }))
</script>

<template>
  <AuthCardLayout back-text="Back to login" back-to="/login" card-class="email-check-card p-4 p-lg-5 w-100">
    <RegistrationProgress :step="2" />
    <div class="email-check-icon" aria-hidden="true">
      <svg viewBox="0 0 24 24" width="34" height="34" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="5" width="18" height="14" rx="3" /><path d="m4 7 8 6 8-6" /></svg>
    </div>
    <p class="email-check-eyebrow">SIGNET · EMAIL VERIFICATION</p>
    <h1 tabindex="-1" class="h2">Check your email</h1>
    <p class="email-check-lead">Open the verification email to continue your registration.</p>
    <ol class="email-check-steps">
      <li><span>01</span><div><strong>Find your verification email</strong><p>Check your inbox, spam or junk folder.</p></div></li>
      <li><span>02</span><div><strong>Follow the verification link</strong><p>Confirm your email on the page that opens.</p></div></li>
      <li><span>03</span><div><strong>Select and fund your package</strong><p>After verification, choose your package and receive your personal USDT deposit address.</p></div></li>
    </ol>
    <div class="email-check-note">Your account is created only after verification. No deposit or payment is needed yet.</div>
    <details class="email-check-help">
      <summary>Can’t find the email?</summary>
      <p>Allow a few minutes and check spam. If you entered the wrong address or need a new link, return to registration and submit your details again. Resend limits still apply.</p>
    </details>
    <RouterLink :to="restart" class="btn btn-outline-primary w-100 mt-3">Return to registration</RouterLink>
    <p class="small text-center mt-3 mb-0">Already verified? <RouterLink to="/login">Sign in</RouterLink></p>
  </AuthCardLayout>
</template>

<style scoped>
.email-check-card { max-width:560px; margin:24px 0; border:1px solid var(--sg-line-3); border-radius:24px; background:radial-gradient(ellipse at top,rgba(0,240,255,.08),transparent 65%),var(--sg-panel); box-shadow:var(--sg-shadow); }
.email-check-icon { display:grid; place-items:center; width:72px; height:72px; border-radius:20px; border:1px solid rgba(0,240,255,.25); color:var(--sg-accent-light); background:rgba(0,240,255,.06); margin-bottom:24px; }
.email-check-eyebrow { font:600 10px var(--sg-font-mono); letter-spacing:.14em; color:var(--sg-accent-light); }
.email-check-lead { color:var(--sg-muted); line-height:1.7; }
.email-check-steps { list-style:none; padding:0; margin:28px 0; }
.email-check-steps li { display:flex; gap:16px; margin-bottom:22px; }
.email-check-steps li > span { flex-shrink:0; display:grid; place-items:center; width:32px; height:32px; font:12px var(--sg-font-mono); border-radius:10px; border:1px solid var(--sg-line-3); color:var(--sg-accent-light); }
.email-check-steps strong { font-size:14px; }
.email-check-steps p { color:var(--sg-muted); font-size:13px; margin:5px 0 0; line-height:1.6; }
.email-check-note { padding:16px; border:1px solid rgba(0,240,255,.2); border-radius:14px; font-size:13px; color:var(--sg-accent-light); }
.email-check-help { margin-top:24px; font-size:13px; }
.email-check-help summary { cursor:pointer; padding:8px 0; }
.email-check-help p { color:var(--sg-muted); line-height:1.7; }
.email-check-card :is(a,summary):focus-visible { outline:2px solid var(--sg-accent-light); outline-offset:4px; }
</style>
