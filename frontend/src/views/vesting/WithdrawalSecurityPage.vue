<script setup>
import { onMounted, onBeforeUnmount, ref } from 'vue'
import QRCode from 'qrcode'
import WithdrawalCountdown from '@/components/WithdrawalCountdown.vue'
import { useWithdrawalClock } from '@/composables/useWithdrawalClock'
import { localDeadline } from '@/utils/withdrawalTime'
import { pastedAuthenticatorCode } from '@/utils/authenticatorCode'
const { now, sync } = useWithdrawalClock()
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'

const status = ref(null)
const busy = ref(false)
const error = ref('')
const message = ref('')
const password = ref('')
const emailCode = ref('')
const authCode = ref('')
const challenge = ref('')
const setup = ref(null)
const qr = ref('')
const address = ref('')
const addressChallenge = ref('')
const challengedAddress = ref('')
const addressEmailCode = ref('')
const addressAuthCode = ref('')
const addresses = ref([])
const revokeTarget = ref(null)
const revokeChallenge = ref('')
const revokeEmailCode = ref('')
const revokeAuthCode = ref('')
let stopped = false

async function refresh() {
  const { data } = await api.get('/blockchain/security')
  if (!stopped) { status.value = data; sync(data.server_time) }
  const { data: addressData } = await api.get('/blockchain/security/addresses')
  if (!stopped) addresses.value = addressData.addresses || []
}

async function action(kind) {
  if (busy.value || stopped) return
  busy.value = true
  error.value = ''
  message.value = ''
  const currentPassword = password.value
  password.value = ''
  try {
    if (kind === 'send') {
      const { data } = await api.post('/blockchain/security/email', { password: currentPassword })
      if (stopped) return
      challenge.value = data.challenge_id
      emailCode.value = ''
      setup.value = null
      qr.value = ''
      message.value = data.message
    } else if (kind === 'setup') {
      const { data } = await api.post('/blockchain/security/authenticator', { password: currentPassword, challenge_id: challenge.value, email_code: emailCode.value })
      if (stopped) return
      challenge.value = ''
      emailCode.value = ''
      setup.value = data
      const image = data.otpauth_url ? await QRCode.toDataURL(data.otpauth_url) : ''
      if (stopped) return
      qr.value = image
      await refresh()
    } else if (kind === 'confirm') {
      const { data } = await api.post('/blockchain/security/authenticator/confirm', { password: currentPassword, code: authCode.value })
      if (stopped) return
      authCode.value = ''
      setup.value = null
      qr.value = ''
      message.value = data.message
      await refresh()
    } else if (kind === 'address-send') {
      addressChallenge.value = ''
      challengedAddress.value = ''
      addressEmailCode.value = addressAuthCode.value = ''
      const destination = address.value
      const { data } = await api.post('/blockchain/security/address', { password: currentPassword, address: destination })
      if (stopped) return
      addressChallenge.value = data.challenge_id
      challengedAddress.value = destination
      message.value = data.message
    } else if (kind === 'address-confirm') {
      const payload = { password: currentPassword, address: challengedAddress.value, challenge_id: addressChallenge.value, email_code: addressEmailCode.value, code: addressAuthCode.value }
      addressEmailCode.value = addressAuthCode.value = ''
      const { data } = await api.post('/blockchain/security/address/confirm', payload)
      if (stopped) return
      addressChallenge.value = challengedAddress.value = ''
      message.value = data.message
      await refresh()
    } else if (kind === 'revoke-send') {
      revokeChallenge.value = revokeEmailCode.value = revokeAuthCode.value = ''
      const { data } = await api.post('/blockchain/security/address/revoke', { password: currentPassword, address: revokeTarget.value.id })
      if (stopped) return
      revokeChallenge.value = data.challenge_id
      message.value = data.message
    } else if (kind === 'revoke-confirm') {
      const payload = { password: currentPassword, address: revokeTarget.value.id, challenge_id: revokeChallenge.value, email_code: revokeEmailCode.value, code: revokeAuthCode.value }
      revokeEmailCode.value = revokeAuthCode.value = ''
      const { data } = await api.post('/blockchain/security/address/revoke/confirm', payload)
      if (stopped) return
      revokeTarget.value = null
      revokeChallenge.value = ''
      message.value = data.message
      await refresh()
    } else { await refresh() }
  } catch (err) {
    if (kind === 'confirm' && !stopped) {
      try {
        await refresh()
        if (status.value?.authenticator_confirmed) {
          setup.value = null
          qr.value = authCode.value = ''
          message.value = 'Authenticator is confirmed.'
          return
        }
      } catch { /* Preserve the original error if status refresh also fails. */ }
    }
    if (!stopped) error.value = err?.response?.data?.message || 'Security setup temporarily unavailable. Please try again.'
  } finally { if (!stopped) busy.value = false }
}

onMounted(() => action('refresh'))
onBeforeUnmount(() => {
  stopped = true
  password.value = emailCode.value = authCode.value = challenge.value = qr.value = ''
  setup.value = null
  address.value = addressChallenge.value = challengedAddress.value = addressEmailCode.value = addressAuthCode.value = ''
  revokeTarget.value = null
  revokeChallenge.value = revokeEmailCode.value = revokeAuthCode.value = ''
})
</script>

<template>
  <DashboardLayout>
  <div class="wallet-form-page">
    <h1 class="h4">Withdrawal security</h1>
    <p class="alert alert-info">Complete email verification, authenticator setup and withdrawal-address verification before requesting a payout. Address waiting periods and security holds still apply. Processing depends on the platform’s enabled services. Device verification and KYC are not required for this flow.</p>
    <p v-if="error" class="alert alert-warning" role="alert">{{ error }}</p>
    <p v-if="message" class="alert alert-success" role="status">{{ message }}</p>
    <section v-if="status" class="card p-3 mb-3">
      <h2 class="h5">Your security checklist</h2><div class="security-checklist"><div><span>Email verification</span><strong :class="status.email_verified ? 'text-success' : 'text-warning'">{{ status.email_verified ? 'Verified' : 'Required' }}</strong></div><div><span>Authenticator</span><strong :class="status.authenticator_confirmed ? 'text-success' : 'text-warning'">{{ status.authenticator_confirmed ? 'Confirmed' : 'Setup required' }}</strong></div></div>
      <p v-if="status.withdrawals_frozen" class="text-danger">Withdrawals are frozen. Contact support.</p>
      <WithdrawalCountdown v-if="status.hold_until" :deadline="status.hold_until" :now="now" kind="account" />
      <p v-if="status.active_withdrawal_id" role="status" class="alert alert-info">An existing withdrawal is processing or needs review. Wait for completion or confirmed cancellation before requesting another.</p>
      <WithdrawalCountdown v-if="status.withdrawal_cooldown_until" :deadline="status.withdrawal_cooldown_until" :now="now" kind="cooldown" compact />
      <button class="btn btn-outline-secondary" :disabled="busy" @click="action('refresh')">Refresh status</button>
    </section>
    <section v-if="status && !status.authenticator_confirmed && !status.withdrawals_frozen" class="card p-3">
      <h2 class="h5">Set up your authenticator</h2>
      <p>A fresh email code is required even if your registration email was already verified. Re-enter your current password for each step.</p>
      <label for="security-password">Current password</label>
      <input id="security-password" v-model="password" type="password" autocomplete="current-password" class="form-control mb-3" :disabled="busy" />
      <button class="btn btn-secondary mb-3" :disabled="busy || !password" @click="action('send')">Send email code</button>
      <template v-if="challenge">
        <label for="security-email-code">Email verification code</label>
        <input id="security-email-code" v-model="emailCode" inputmode="numeric" autocomplete="one-time-code" maxlength="6" class="form-control mb-3" :disabled="busy" />
        <button class="btn btn-primary" :disabled="busy || !password || !/^\d{6}$/.test(emailCode)" @click="action('setup')">Verify email and start setup</button>
      </template>
      <template v-if="setup">
        <p class="mt-3">{{ setup.existing_authenticator ? 'Use your existing authenticator to confirm ownership. Its secret is not displayed.' : 'Scan this QR code in Google Authenticator, then enter its code below. Keep the secret private.' }}</p>
        <img v-if="qr" :src="qr" alt="Authenticator setup QR code" width="220" height="220" />
        <p v-if="setup.secret" class="text-break">Manual setup secret: {{ setup.secret }}</p>
        <p>Setup expires at {{ localDeadline(setup.expires_at) }} (your local time).</p>
        <label for="security-auth-code">Authenticator code</label>
        <input id="security-auth-code" v-model="authCode" inputmode="numeric" autocomplete="off" maxlength="6" class="form-control mb-3" :disabled="busy" @paste.prevent="authCode = pastedAuthenticatorCode($event.clipboardData.getData('text'))" />
        <p class="small">Use the authenticator entry from this setup. Keep this QR open while retrying a code.</p>
        <button class="btn btn-primary" :disabled="busy || !password || !/^\d{6}$/.test(authCode)" @click="action('confirm')">Confirm authenticator</button>
      </template>
    </section>
    <section v-if="status?.email_verified && status?.authenticator_confirmed && !status?.withdrawals_frozen" class="card p-3 mt-3">
      <h2 class="h5">Verify a SIGNET withdrawal address</h2>
      <p>Use a SIGNET-compatible address on BSC. Confirmation requires a fresh email code and authenticator code. The address becomes eligible 24 hours after confirmation; any longer account security hold still applies.</p>
      <label for="withdrawal-address">BSC withdrawal address</label>
      <input id="withdrawal-address" v-model="address" class="form-control mb-3" autocomplete="off" spellcheck="false" maxlength="42" :disabled="busy || !!addressChallenge" />
      <label for="address-password">Current password (re-enter for each step)</label>
      <input id="address-password" v-model="password" type="password" autocomplete="current-password" class="form-control mb-3" :disabled="busy" />
      <button class="btn btn-secondary" :disabled="busy || !password || !/^0x[0-9a-fA-F]{40}$/.test(address)" @click="action('address-send')">Send address verification code</button>
      <template v-if="addressChallenge">
        <p class="mt-3 text-break">Confirm this address: {{ challengedAddress }}</p>
        <label for="address-email-code">Email code</label>
        <input id="address-email-code" v-model="addressEmailCode" inputmode="numeric" autocomplete="one-time-code" maxlength="6" class="form-control mb-3" :disabled="busy" />
        <label for="address-auth-code">Authenticator code</label>
        <input id="address-auth-code" v-model="addressAuthCode" inputmode="numeric" autocomplete="off" maxlength="6" class="form-control mb-3" :disabled="busy" />
        <button class="btn btn-primary" :disabled="busy || !password || !/^\d{6}$/.test(addressEmailCode) || !/^\d{6}$/.test(addressAuthCode)" @click="action('address-confirm')">Verify address</button>
        <button class="btn btn-outline-secondary ms-2" :disabled="busy" @click="addressChallenge = challengedAddress = addressEmailCode = addressAuthCode = ''">Cancel</button>
      </template>
    </section>
    <section v-if="addresses.length" class="card p-3 mt-3">
      <h2 class="h5">Verified withdrawal addresses</h2>
      <div v-for="saved in addresses" :key="saved.id" class="border-bottom py-2">
        <p class="text-break mb-1">{{ saved.asset }} · {{ saved.network }} · {{ saved.address }}</p>
        <p v-if="saved.revoked_at" class="text-danger mb-1">Revoked at {{ localDeadline(saved.revoked_at) }} (your local time)</p>
        <WithdrawalCountdown v-else :deadline="saved.eligible_at" :now="now" compact />
        <button v-if="!saved.revoked_at && !revokeTarget && status?.authenticator_confirmed && !status?.withdrawals_frozen" class="btn btn-outline-danger" :disabled="busy" @click="revokeTarget = saved; password = ''">Revoke address</button>
      </div>
      <div v-if="revokeTarget" class="mt-3">
        <p class="text-break">Disable this destination: {{ revokeTarget.address }}. This does not cancel transactions already broadcast.</p>
        <label for="revoke-password">Current password (re-enter for each step)</label>
        <input id="revoke-password" v-model="password" type="password" autocomplete="current-password" class="form-control mb-3" :disabled="busy" />
        <button class="btn btn-secondary" :disabled="busy || !password" @click="action('revoke-send')">Send revocation email code</button>
        <template v-if="revokeChallenge">
          <label for="revoke-email-code">Email code</label>
          <input id="revoke-email-code" v-model="revokeEmailCode" inputmode="numeric" autocomplete="one-time-code" maxlength="6" class="form-control mb-3" :disabled="busy" />
          <label for="revoke-auth-code">Authenticator code</label>
          <input id="revoke-auth-code" v-model="revokeAuthCode" inputmode="numeric" autocomplete="off" maxlength="6" class="form-control mb-3" :disabled="busy" />
          <button class="btn btn-danger" :disabled="busy || !password || !/^\d{6}$/.test(revokeEmailCode) || !/^\d{6}$/.test(revokeAuthCode)" @click="action('revoke-confirm')">Confirm revocation</button>
        </template>
        <button class="btn btn-outline-secondary ms-2" :disabled="busy" @click="revokeTarget = null; revokeChallenge = revokeEmailCode = revokeAuthCode = ''; password = ''">Cancel</button>
      </div>
    </section>
  </div>
  </DashboardLayout>
</template>
