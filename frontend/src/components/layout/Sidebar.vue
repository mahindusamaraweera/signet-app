<script setup>
// Ports layouts/sidebar.blade.php 1:1 — same role-gated items, labels and
// inline SVG icons, the Signet logo, and the mobile close (×) button.
// Role gating reproduces the original's `@if(Auth::user()->role == ...)`
// blocks, including the dead branch (My Activations' inner admin check
// never fires because it's nested inside an outer role=='company' check —
// we just render the company link).
//
// Active item: custome.js marks the <li class="nav-item"> whose link has the
// longest path matching the current URL (the generic "/dashboard" link also
// matches every role dashboard). Reproduced here reactively off the route.
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import bootstrap from 'bootstrap/dist/js/bootstrap.bundle.min.js'
import { useAuthStore } from '@/store/auth'
import { useNewActivationsCount } from '@/composables/useNewActivationsCount'

const auth = useAuthStore()
const route = useRoute()
const role = computed(() => auth.role)
const { count: waitingCount } = useNewActivationsCount()

const PIE = '<svg class="icon icon-xs me-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path d="M2 10a8 8 0 018-8v8h8a8 8 0 11-16 0z"></path><path d="M12 2.252A8.014 8.014 0 0117.748 8H12V2.252z"></path></svg>'
const BOX = '<svg class="icon icon-xs me-2" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"> <path d="M21 16V8a2 2 0 00-1-1.73l-7-4a2 2 0 00-2 0l-7 4A2 2 0 003 8v8a2 2 0 001 1.73l7 4a2 2 0 002 0l7-4A2 2 0 0021 16z"/> <path d="M12 3v18"/> </svg>'
const GRID = '<svg class="icon icon-xs me-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path d="M5 3a2 2 0 00-2 2v2a2 2 0 002 2h2a2 2 0 002-2V5a2 2 0 00-2-2H5zM5 11a2 2 0 00-2 2v2a2 2 0 002 2h2a2 2 0 002-2v-2a2 2 0 00-2-2H5zM11 5a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V5zM11 13a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"></path></svg>'
// The original's "Waiting Activations" icon path is corrupted part-way
// through (full-width characters); browsers draw it up to the last valid
// command, i.e. two of the four squares. Reproduced as that rendered result.
const GRID_BROKEN = '<svg class="icon icon-xs me-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path d="M5 3a2 2 0 00-2 2v2a2 2 0 002 2h2a2 2 0 002-2V5a2 2 0 00-2-2H5zM5 11a2 2 0 00-2 2v2a2 2 0 002 2h2a2 2 0 002-2v-2a2 2 0 00-2-2H5zM11 5a2 2 0 012-2"></path></svg>'
const HELP = '<svg class="icon icon-xs me-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-8-3a1 1 0 00-.867.5 1 1 0 11-1.731-1A3 3 0 0113 8a3.001 3.001 0 01-2 2.83V11a1 1 0 11-2 0v-1a1 1 0 011-1 1 1 0 100-2zm0 8a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"></path></svg>'
const COG = '<svg class="icon icon-xs me-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" d="M11.49 3.17c-.38-1.56-2.6-1.56-2.98 0a1.532 1.532 0 01-2.286.948c-1.372-.836-2.942.734-2.106 2.106.54.886.061 2.042-.947 2.287-1.561.379-1.561 2.6 0 2.978a1.532 1.532 0 01.947 2.287c-.836 1.372.734 2.942 2.106 2.106a1.532 1.532 0 012.287.947c.379 1.561 2.6 1.561 2.978 0a1.533 1.533 0 012.287-.947c1.372.836 2.942-.734 2.106-2.106a1.533 1.533 0 01.947-2.287c1.561-.379 1.561-2.6 0-2.978a1.532 1.532 0 01-.947-2.287c.836-1.372-.734-2.942-2.106-2.106a1.532 1.532 0 01-2.287-.947zM10 13a3 3 0 100-6 3 3 0 000 6z" clip-rule="evenodd"></path></svg>'

// `wrap` = the original's `nav-link d-flex justify-content-between` variant,
// whose icon + text sit inside an extra <span>.
const items = computed(() => {
  const r = role.value
  const list = [{ to: '/dashboard', label: 'Dashboard', icon: PIE }]
  if (r === 'company') {
    list.push(
      { to: `/admin/${auth.user?.id}/setup-google-auth`, label: 'Google Auth Setup', icon: PIE },
      { to: '/countries', label: 'Countries', icon: PIE },
      { to: '/packages', label: 'Packages', icon: BOX },
      { to: '/mining/users', label: 'Mining Token', icon: BOX },
      { to: '/users', label: 'Users', icon: BOX },
      { to: '/roc', label: 'ROC Income', icon: BOX },
      { to: '/direct-share', label: 'Global Director Share', icon: BOX },
      { to: '/direct-share-log', label: 'GDS Log', icon: BOX },
      { to: '/salaries', label: 'Salaries', icon: BOX },
      { to: '/leaders/gain', label: 'Leaders Gain', icon: BOX },
      { to: '/executives/gain', label: 'Executives Gain', icon: BOX },
      { to: '/leader-code-logs', label: 'Leader Code Logs', icon: BOX },
      { to: '/leadership-bonus-log', label: 'LB Log', icon: BOX },
      { to: '/user-parent-logs', label: 'Fake Accounts', icon: BOX },
    )
  }
  if (r === 'admin') {
    list.push(
      { to: '/users', label: 'Users', icon: BOX },
      { to: '/leader-code-logs', label: 'Leader Code Logs', icon: BOX },
    )
  }
  if (r === 'admin' || r === 'user' || r === 'agent') {
    list.push(
      { to: '/token-shares', label: 'Token Share', icon: GRID, wrap: true },
      { to: '/token/share/logs', label: 'Token Share Log', icon: GRID, wrap: true },
      { to: '/buy-package-history', label: 'Top Up', icon: GRID, wrap: true },
      { to: '/earn/history', label: 'Earn Log', icon: GRID, wrap: true },
    )
  }
  if (r === 'company') {
    list.push(
      { to: '/company/pending-activation', label: 'My Activations', icon: GRID, wrap: true },
      { to: '/new-activations', label: 'Waiting Activations', icon: GRID_BROKEN, wrap: true, badge: true },
    )
  }
  if (r !== 'company') list.push({ to: '/my-geneology', label: 'Geneology', icon: GRID, wrap: true })
  list.push({ to: '/wallet/deposits', label: 'Deposit Wallet', icon: GRID, wrap: true })
  list.push({ to: '/wallet/security', label: 'Withdrawal Security', icon: GRID, wrap: true })
  list.push({ to: '/wallet/withdraw', label: 'Withdraw SIGNET', icon: GRID, wrap: true })
	list.push({ to: '/wallet/vesting', label: 'SIGNET Vesting', icon: GRID, wrap: true })
	if (r === 'company' || r === 'admin') list.push({ to: '/admin/vesting', label: 'Manage Vesting', icon: GRID, wrap: true })
  list.push({ to: r !== 'company' ? '/kyc/show' : '/kyc', label: 'KYC', icon: GRID, wrap: true })
  return list
})

const DASHBOARDS = ['/dashboard', '/admin/dashboard', '/company/dashboard', '/agent/dashboard', '/user/dashboard']
const clean = (p) => p.replace(/\/+$/, '') || '/'

const activeTo = computed(() => {
  const current = clean(route.path)
  let best = null
  let bestLength = -1
  for (const item of items.value) {
    const path = clean(item.to)
    let match = current === path || (path !== '/' && current.indexOf(path + '/') === 0)
    if (path === '/dashboard' && DASHBOARDS.includes(current)) match = true
    if (match && path.length > bestLength) {
      best = item.to
      bestLength = path.length
    }
  }
  return best
})

function closeSidebar() {
  const el = document.getElementById('sidebarMenu')
  if (el) bootstrap.Collapse.getOrCreateInstance(el, { toggle: false }).hide()
}

// A full page load closed the mobile menu in the original; do the same on
// in-app navigation.
watch(() => route.fullPath, closeSidebar)
</script>

<template>
  <nav class="navbar navbar-dark navbar-theme-primary px-4 col-12 d-lg-none">
    <RouterLink class="navbar-brand me-lg-5" to="/dashboard">
      <img class="navbar-brand-dark" src="/resources/site/assets/logo/signet-light.png" alt="Signet" />
      <img class="navbar-brand-light" src="/resources/site/assets/logo/signet-light.png" alt="Signet" />
    </RouterLink>
    <div class="d-flex align-items-center">
      <button
        class="navbar-toggler d-lg-none collapsed"
        type="button"
        data-bs-toggle="collapse"
        data-bs-target="#sidebarMenu"
        aria-controls="sidebarMenu"
        aria-expanded="false"
        aria-label="Toggle navigation"
      >
        <span class="navbar-toggler-icon"></span>
      </button>
    </div>
  </nav>

  <nav id="sidebarMenu" class="sidebar d-lg-block bg-gray-800 text-white collapse">
    <div class="sidebar-inner px-4 pt-3">
      <button id="sidebarMobileClose" type="button" class="sidebar-mobile-close d-lg-none" @click="closeSidebar">
        &times;
      </button>
      <ul class="nav flex-column pt-3 pt-md-0">
        <li class="nav-item">
          <RouterLink class="navbar-brand me-lg-5" to="/dashboard">
            <img class="navbar-brand-dark mb-3" src="/resources/site/assets/logo/signet-light.png" alt="Signet" style="height: 65px !important" />
          </RouterLink>
        </li>

        <li v-for="item in items" :key="item.label + item.to" class="nav-item" :class="{ active: activeTo === item.to }">
          <RouterLink
            :to="item.to"
            class="nav-link"
            :class="item.badge ? 'd-flex justify-content-between align-items-center' : item.wrap ? 'd-flex justify-content-between' : ''"
            :aria-current="activeTo === item.to ? 'page' : null"
            active-class=""
            exact-active-class=""
          >
            <span v-if="item.wrap">
              <span class="sidebar-icon" v-html="item.icon"></span>
              <span class="sidebar-text">{{ item.label }}</span>
            </span>
            <template v-else>
              <span class="sidebar-icon" v-html="item.icon"></span>
              <span class="sidebar-text">{{ item.label }}</span>
            </template>
            <span v-if="item.badge" class="badge bg-danger rounded-pill">
              {{ waitingCount || 0 }}
            </span>
          </RouterLink>
        </li>

        <li role="separator" class="dropdown-divider mt-4 mb-3 border-gray-700"></li>
        <li class="nav-item">
          <a href="javascript:void(0);" target="_blank" class="nav-link d-flex align-items-center">
            <span class="sidebar-icon" v-html="HELP"></span>
            <span class="sidebar-text">Support <span class="badge badge-sm bg-secondary ms-1 text-gray-800"></span></span>
          </a>
        </li>
        <li class="nav-item">
          <a href="/logout" class="nav-link d-flex align-items-center" @click.prevent="auth.logout()">
            <span class="sidebar-icon" v-html="COG"></span>
            <span class="sidebar-text">Logout <span class="badge badge-sm bg-secondary ms-1 text-gray-800"></span></span>
          </a>
        </li>
      </ul>
    </div>
  </nav>
</template>
