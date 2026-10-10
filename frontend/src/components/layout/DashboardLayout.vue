<script setup>
// Ports the layouts/app.blade.php body (`@yield('sidebar')` then
// `@yield('content')`) + sidebar.blade.php + topbar.blade.php shell used by
// every authenticated page. In the original, topbar.blade.php opens a single
// <main class="content"> that every view's own markup renders into
// (unclosed — no view defines @section('footer') to close it; harmless in
// a browser but not reproducible in a Vue template, so it's closed here
// around Topbar + the page slot instead, giving the same DOM Volt's CSS
// expects: one .content element offset from the fixed .sidebar).
import Sidebar from './Sidebar.vue'
import Topbar from './Topbar.vue'
import IncomeCapAlert from './IncomeCapAlert.vue'
</script>

<template>
  <Sidebar />
  <main class="content">
    <Topbar />
    <IncomeCapAlert />
    <slot />
  </main>
</template>
