<script setup>
// "4× Income Cap" tile on the Admin/Agent/User dashboards (replaces the old
// "My Wallet" tile). `cap` is the dashboard's total_value — sum of active
// package price × 4 (see dashMyWallet) — and `earned` is wallet_balance,
// so remaining is the same figure the old tile showed.
import { computed } from 'vue'

const props = defineProps({
  colClass: { type: String, default: 'col-12 col-sm-12 col-xl-4 mb-4' },
  earned: { type: [String, Number], default: 0 },
  cap: { type: [String, Number], default: 0 },
})

const earnedNum = computed(() => Number(props.earned) || 0)
const capNum = computed(() => Number(props.cap) || 0)
const remaining = computed(() => Math.max(capNum.value - earnedNum.value, 0))
const percent = computed(() => (capNum.value > 0 ? Math.min((earnedNum.value / capNum.value) * 100, 100) : 0))

const fmt = (n) => n.toLocaleString('en-US', { minimumFractionDigits: 8, maximumFractionDigits: 8 })
</script>

<template>
  <div :class="colClass">
    <div class="card border-0 shadow income-cap-card">
      <div class="card-body">
        <div class="income-cap-head">
          <h2 class="h5 mb-0">4× Income Limit</h2>
          <span class="income-cap-percent">{{ percent.toFixed(1) }}%</span>
        </div>
        <div class="income-cap-track" role="progressbar" :aria-valuenow="percent.toFixed(1)" aria-valuemin="0" aria-valuemax="100" aria-label="Income cap used">
          <div class="income-cap-fill" :style="{ width: percent + '%' }"></div>
        </div>
        <div class="income-cap-foot">
          <span>{{ fmt(earnedNum) }} / {{ fmt(capNum) }}</span>
          <span>{{ fmt(remaining) }} remaining</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.income-cap-card { height: 100%; }
.income-cap-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 18px; }
.income-cap-percent { font-size: 1.05rem; font-weight: 600; font-variant-numeric: tabular-nums; }
.income-cap-track { height: 12px; border-radius: 99px; background: var(--sg-line-2); overflow: hidden; }
.income-cap-fill { height: 100%; min-width: 0; border-radius: 99px; background: var(--sg-up); transition: width .4s ease; }
.income-cap-foot { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 4px 12px; margin-top: 14px; color: var(--sg-muted); font-size: .85rem; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
</style>
