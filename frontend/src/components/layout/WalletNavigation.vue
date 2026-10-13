<script setup>
import { useRoute } from 'vue-router'
const route = useRoute()
const links = [
  { to: '/wallet/deposits', title: 'Wallet', detail: 'Deposit & activity' },
  { to: '/wallet/vesting', title: 'Vesting', detail: 'Allocation & releases' },
  { to: '/wallet/withdraw', title: 'Withdraw', detail: 'SIGNET payouts' },
  { to: '/wallet/security', title: 'Security', detail: 'Protect withdrawals' },
  { to: '/wallet/tier-link', title: 'Tier account', detail: 'Connect ownership' },
]
</script>

<template>
  <nav class="wallet-navigation" aria-label="Asset workspace">
    <RouterLink v-for="link in links" :key="link.to" :to="link.to"
      :class="{ selected: route.path === link.to || route.path.startsWith(link.to + '/') }"
      :aria-current="route.path === link.to || route.path.startsWith(link.to + '/') ? 'page' : null">
      <span>{{ link.title }}</span><small>{{ link.detail }}</small>
    </RouterLink>
  </nav>
</template>
