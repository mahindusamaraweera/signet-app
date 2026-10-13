<script setup>
// "4× Income Limit" tile on the Admin/Agent/User dashboards (replaces the
// old "My Wallet" tile). `cap` is the dashboard's total_value — sum of
// active package price × 4 (see dashMyWallet) — and `earned` is
// wallet_balance, so remaining is the same figure the old tile showed.
// Uses DashboardStatCard's markup so the signet-dashboard theme (icon ring,
// mono label/readout, card frame) applies; signetDashboard.js picks the
// "income" icon from the label.
import { computed } from 'vue'

const props = defineProps({
  colClass: { type: String, default: 'col-12 col-sm-12 col-xl-4 mb-4' },
  earned: { type: [String, Number], default: 0 },
  cap: { type: [String, Number], default: 0 },
})

const label = '4× Income Limit'
const earnedNum = computed(() => Number(props.earned) || 0)
const capNum = computed(() => Number(props.cap) || 0)
const remaining = computed(() => Math.max(capNum.value - earnedNum.value, 0))
const percent = computed(() => (capNum.value > 0 ? Math.min((earnedNum.value / capNum.value) * 100, 100) : 0))

const fmt = (n) => n.toLocaleString('en-US', { maximumFractionDigits: 8 })
</script>

<template>
  <div :class="colClass">
    <div class="card border-0 shadow income-cap-card">
      <div class="card-body">
        <div class="row d-block d-xl-flex align-items-center">
          <div class="col-12 col-xl-5 text-xl-center mb-3 mb-xl-0 d-flex align-items-center justify-content-xl-center">
            <div class="icon-shape icon-shape-secondary rounded me-4 me-sm-0">
              <svg class="icon" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path d="M2 11a1 1 0 011-1h2a1 1 0 011 1v5a1 1 0 01-1 1H3a1 1 0 01-1-1v-5zM8 7a1 1 0 011-1h2a1 1 0 011 1v9a1 1 0 01-1 1H9a1 1 0 01-1-1V7zM14 4a1 1 0 011-1h2a1 1 0 011 1v12a1 1 0 01-1 1h-2a1 1 0 01-1-1V4z"></path></svg>
            </div>
            <div class="d-sm-none">
              <h2 class="fw-extrabold h5">{{ label }}</h2>
              <h3 class="mb-1">{{ fmt(earnedNum) }}<span class="text-success fw-bold fs-5"> / {{ fmt(capNum) }} USDT</span></h3>
            </div>
          </div>
          <div class="col-12 col-xl-7 px-xl-0">
            <div class="d-none d-sm-block">
              <div class="income-cap-head">
                <h2 class="h6 text-gray-400 mb-0">{{ label }}</h2>
                <span class="income-cap-percent">{{ percent.toFixed(1) }}%</span>
              </div>
              <h3 class="fw-extrabold mb-2">{{ fmt(earnedNum) }}<span class="text-success fw-bold fs-5"> / {{ fmt(capNum) }} USDT</span></h3>
            </div>
            <div class="income-cap-track" role="progressbar" :aria-valuenow="percent.toFixed(1)" aria-valuemin="0" aria-valuemax="100" aria-label="Income limit used">
              <div class="income-cap-fill" :style="{ width: percent + '%' }"></div>
            </div>
            <small class="income-cap-foot">
              <span>{{ fmt(remaining) }} remaining</span>
              <span class="d-sm-none">{{ percent.toFixed(1) }}%</span>
            </small>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.income-cap-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
.income-cap-percent { font: 500 11px/1.4 var(--sg-font-mono); letter-spacing: .08em; color: var(--sg-up); }
.income-cap-track { height: 6px; border-radius: 99px; background: var(--sg-line-2); overflow: hidden; }
.income-cap-fill { height: 100%; border-radius: 99px; background: var(--sg-up); box-shadow: 0 0 10px rgba(61,230,168,.6); transition: width .4s ease; }
.income-cap-foot { display: flex; justify-content: space-between; gap: 12px; margin-top: 6px; color: var(--sg-muted); font: 500 11px/1.4 var(--sg-font-mono); letter-spacing: .06em; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
</style>
