<script setup>
import { computed } from 'vue'
import { countdownText, remainingSeconds, localDeadline } from '@/utils/withdrawalTime'
const props = defineProps({ deadline: String, now: { type: Number, required: true }, kind: { type: String, default: 'address' }, compact: Boolean })
const remaining = computed(() => remainingSeconds(props.deadline, props.now))
const waiting = computed(() => remaining.value !== null && remaining.value > 0)
const title = computed(() => remaining.value === null ? 'Waiting time unavailable' : waiting.value
  ? props.kind === 'account' ? 'Account security wait' : props.kind === 'verification' ? 'Complete verification in' : 'Address available in'
  : props.kind === 'verification' ? 'Verification time ended' : props.kind === 'account' ? 'Account waiting period ended' : 'Address waiting period ended')
</script>

<template>
  <div class="withdrawal-countdown" :class="{ ready: remaining === 0, compact }">
    <div class="countdown-top" aria-live="polite"><span class="countdown-icon" aria-hidden="true">{{ remaining === 0 ? '✓' : '◷' }}</span><strong>{{ title }}</strong><span class="countdown-state">{{ remaining === null ? 'Check status' : remaining === 0 ? 'Ended' : 'Waiting' }}</span></div>
    <div v-if="waiting" class="countdown-digits" role="timer" aria-live="off">{{ countdownText(remaining) }}</div>
    <p v-if="deadline && remaining !== null" class="countdown-date">{{ kind === 'verification' ? 'Expires' : 'Ends' }} {{ localDeadline(deadline) }} <span>· your local time</span></p>
    <p v-if="remaining === null" class="countdown-help">Check status to load the deadline.</p>
    <p v-else-if="kind === 'verification'" class="countdown-help">{{ waiting ? 'Use the newest email code and a fresh authenticator code.' : 'Check request status before starting again. Do not repeat a submitted withdrawal.' }}</p>
    <p v-else class="countdown-help">{{ waiting ? kind === 'account' ? 'You can request a withdrawal after this wait and your address wait both end.' : 'A longer account security wait may still apply.' : 'Check status to confirm your current eligibility. Other security checks still apply.' }}</p>
  </div>
</template>

<style scoped>
.withdrawal-countdown { border: 1px solid #736138; background: linear-gradient(130deg, #28251b, #111c23); border-radius: 16px; padding: 18px; margin: 12px 0; color: #ecf3f5; }
.countdown-top { display: flex; align-items: center; gap: 9px; font-size: .95rem; }
.countdown-icon { color: #f2ca79; font-size: 1.4rem; }
.countdown-state { margin-left: auto; color: #f2ca79; background: #f2ca7914; padding: 3px 9px; border-radius: 20px; font-size: .72rem; white-space: nowrap; }
.countdown-digits { font-variant-numeric: tabular-nums; font-size: clamp(1.55rem, 6vw, 2.15rem); font-weight: 650; letter-spacing: .025em; margin: 10px 0; }
.countdown-date, .countdown-help { font-size: .82rem; line-height: 1.6; margin: 5px 0 0; color: #bdcbd1; overflow-wrap: anywhere; }
.countdown-date span { color: #a6bac4; }
.ready { border-color: #286353; background: linear-gradient(130deg, #132b26, #111c23); }
.ready .countdown-icon, .ready .countdown-state { color: #79dfb3; }
.compact { padding: 13px 15px; }
.compact .countdown-digits { font-size: 1.4rem; }
</style>
