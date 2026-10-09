<script setup>
// Ports auth/register_step3.blade.php — on-screen labelled "Step 3: Get Your
// Upliner Details" (route('register.step3')), the intermediate "wait for
// upliner" screen shown right after package selection (processStep2).
//
// Per the ui_spec.md file/label mismatch note, the original's "Next" button
// links back to this same view; the rebuild instead sends the user on to the
// dedicated Step 4 "Account Status" screen at /register/status/:id.
import { ref } from 'vue'
import AuthCardLayout from '@/components/layout/AuthCardLayout.vue'

const props = defineProps({
  id: { type: [String, Number], required: true },
})

const parent = ref(null)

try {
  const raw = sessionStorage.getItem(`signet_register_parent_${props.id}`)
  if (raw) parent.value = JSON.parse(raw)
} catch {
  parent.value = null
}
</script>

<template>
  <AuthCardLayout back-text="Back to log in" back-to="/login" back-placement="row">
    <h1 class="h3 mb-4">Step 3: Get Your Upliner Details</h1>
    <div class="mt-4">
      <h3>Upliner Activation</h3>
      Binance ID: {{ parent?.binance_pay_id }}
      <br />
      <i class="fab fa-whatsapp"></i> WhatsApp no: {{ parent?.whatsapp_number }}
      <br />
      <span v-if="parent?.on_vacation == 1" class="badge bg-warning">Your Upliner is On Vacation..Please Contact the Company</span>
      <!-- Call Now button with link -->
      <a :href="'tel:' + (parent?.whatsapp_number ?? '')" class="btn btn-success mt-2">
        <i class="fas fa-phone-alt"></i> Call Now
      </a>
    </div>
    <RouterLink :to="{ name: 'register.step3.status', params: { id } }" class="btn btn-primary mt-4">Next</RouterLink>
    <div class="form-group"></div>
  </AuthCardLayout>
</template>
