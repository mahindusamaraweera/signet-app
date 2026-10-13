<script setup>
// Ports layouts/topbar.blade.php: slim top navbar with the user's name and
// a dropdown holding a single Logout item. The sidebar's own d-lg-none
// mobile bar (see Sidebar.vue) carries the only navbar-toggler in the
// original; this component intentionally has none. signet-ui.css styles the
// toggle as a glass pill with a status LED, and signet-dashboard (see
// useSignetDashboard.js) drops a live clock into the empty left-hand slot.
import { useAuthStore } from '@/store/auth'

const auth = useAuthStore()
</script>

<template>
  <nav class="navbar navbar-top navbar-expand navbar-dashboard navbar-dark ps-0 pe-2 pb-0">
    <div class="container-fluid px-0">
      <div id="navbarSupportedContent" class="d-flex justify-content-between w-100">
        <div class="d-flex align-items-center"></div>
        <!-- Navbar links -->
        <ul class="navbar-nav align-items-center">
          <li class="nav-item dropdown ms-lg-3">
            <a class="nav-link dropdown-toggle pt-1 px-0" href="/" role="button" data-bs-toggle="dropdown" aria-expanded="false" :aria-label="`Account menu for ${auth.user?.name || 'your account'}`" @click.prevent>
              <div class="media d-flex align-items-center">
                <div class="media-body ms-2 text-dark align-items-center d-none d-lg-block">
                  <span class="mb-0 font-small fw-bold text-gray-900">{{ auth.user?.name }}</span>
                </div>
              </div>
            </a>
            <div class="dropdown-menu dashboard-dropdown dropdown-menu-end mt-2 py-1">
              <div role="separator" class="dropdown-divider my-1"></div>
              <a class="dropdown-item d-flex align-items-center" href="/logout" @click.prevent="auth.logout()">
                <svg class="dropdown-icon text-danger me-2" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"></path></svg>
                Logout
              </a>
            </div>
          </li>
        </ul>
      </div>
    </div>
  </nav>
</template>
