<script setup>
// Ports packages/buy-package-done.blade.php (post-purchase "Upliner
// Activation" wait screen — the original renders this as the direct
// server response to the buy.packages POST, no separate GET route).
//
// There is no re-fetch endpoint for this screen's data (see
// package_handler.go's buyPackagesHandler doc comment) — BuyPackagePage.vue
// stashes the POST /buy-packages response's `parent_data` in
// sessionStorage before navigating here; this page just reads it back. A
// direct/refreshed visit with nothing stashed shows a fallback instead of
// fabricating data.
//
// parent_data is a raw models.User struct: binance_pay_id/whatsapp_number
// are sql.NullString (no custom MarshalJSON), so they may arrive as
// {String,Valid} objects — handled defensively, same as elsewhere.
import { ref, onMounted } from 'vue'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'
import { nullStr } from '@/utils/format'


const parentData = ref(null)

onMounted(() => {
  try {
    const raw = sessionStorage.getItem('signet:buyPackageDone')
    if (raw) parentData.value = JSON.parse(raw)
  } catch {
    parentData.value = null
  }
})
</script>

<template>
  <!-- buy-package-done.blade.php renders its card inside the app shell (sidebar + topbar). -->
  <DashboardLayout>
    <AuthCardLayout embedded :show-back-link="false">
      <div class="mt-4">
        <h3>Upliner Activation</h3>
        Binance ID: {{ nullStr(parentData?.binance_pay_id) }}
        <br />
        <i class="fab fa-whatsapp mt-2"></i> WhatsApp no: {{ nullStr(parentData?.whatsapp_number) }}
        <br />
        <!-- Call Now button with link -->
        <a :href="'tel:' + nullStr(parentData?.whatsapp_number)" class="btn btn-success mt-2">
          <i class="fas fa-phone-alt"></i> Call Now
        </a>
      </div>
      <RouterLink :to="{ name: 'buy.package.history' }" class="btn btn-primary mt-4">Next</RouterLink>
      <div class="form-group"></div>
    </AuthCardLayout>
  </DashboardLayout>
</template>
