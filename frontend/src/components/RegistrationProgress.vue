<script setup>
defineProps({ step: { type: Number, required: true } })
const stages = ['Account', 'Verify email', 'Package', 'Deposit']
</script>
<template>
  <ol class="registration-progress" aria-label="Registration progress">
    <li v-for="(label,index) in stages" :key="label" :class="{ current: step === index + 1, previous: step > index + 1 }" :aria-current="step === index + 1 ? 'step' : null">
      <span aria-hidden="true">{{ index + 1 }}</span><small>{{ label }}</small>
    </li>
  </ol>
</template>
<style scoped>
.registration-progress { display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;list-style:none;padding:0;margin:0 0 28px; }
.registration-progress li { border-top:2px solid var(--sg-line-3);padding-top:12px;color:var(--sg-muted);display:flex;gap:7px;align-items:center; }
.registration-progress span { font:11px var(--sg-font-mono); }
.registration-progress small { font-size:11px; }
.registration-progress .current { border-color:var(--sg-accent);color:var(--sg-accent-light); }
.registration-progress .previous { border-color:var(--sg-muted); }
@media(max-width:400px) { .registration-progress li { align-items:flex-start;flex-direction:column;gap:4px; } }
</style>
