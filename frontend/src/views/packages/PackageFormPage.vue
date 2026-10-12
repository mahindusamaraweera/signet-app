<script setup>
// Ports packages/create.blade.php + packages/edit.blade.php, which share
// packages/form.blade.php — one component for both routes, `id` undefined
// on create. See package_handler.go's packageStoreHandler/
// packageEditHandler/packageUpdateHandler.
//
// As in the original, the Telegram Link input carries no `required`
// attribute, but the server requires it; a rejected submit is reported
// above the form. A successful save lands on the package list with the
// original's success flash.
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '@/api/client'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import { useFlashStore } from '@/store/flash'
import { nullStr } from '@/utils/format'

const props = defineProps({
  id: { type: [String, Number], default: undefined },
})

const router = useRouter()
const flashStore = useFlashStore()

const isEdit = computed(() => props.id !== undefined && props.id !== null && props.id !== '')

const formError = ref('')
const form = ref({ name: '', price: '', commission: '', rank: '', telegram_link: '', status: 'active' })

async function loadPackage() {
  try {
    const { data } = await api.get(`/packages/${props.id}/edit`)
    const pkg = data.package || {}
    form.value = {
      name: pkg.name || '',
      price: pkg.price ?? '',
      commission: pkg.commission ?? '',
      rank: nullStr(pkg.rank),
      telegram_link: nullStr(pkg.telegram_link),
      status: pkg.status || '',
    }
  } catch (err) {
    formError.value = err?.response?.data?.message || 'Could not load package.'
  }
}

async function submitForm() {
  formError.value = ''
  const payload = {
    name: form.value.name,
    price: form.value.price === '' ? null : Number(form.value.price),
    commission: form.value.commission === '' ? null : Number(form.value.commission),
    rank: form.value.rank,
    telegram_link: form.value.telegram_link,
    status: form.value.status,
  }
  try {
    const { data } = isEdit.value ? await api.put(`/packages/${props.id}`, payload) : await api.post('/packages', payload)
    flashStore.flash('success', data?.message || (isEdit.value ? 'Package updated successfully.' : 'Package created successfully.'))
    router.push({ name: 'packages.index' })
  } catch (err) {
    const errs = err?.response?.data?.errors
    formError.value = (errs && Object.values(errs)[0]?.[0]) || err?.response?.data?.message || 'Could not save package.'
  }
}

onMounted(() => {
  if (isEdit.value) loadPackage()
})
</script>

<template>
  <DashboardLayout>
    <div class="container">
      <h3>{{ isEdit ? 'Edit Package' : 'Create New Package' }}</h3>
      <div v-if="formError" class="alert alert-danger">{{ formError }}</div>
      <form method="POST" @submit.prevent="submitForm">
        <div class="mb-3">
          <label>Name</label>
          <input v-model="form.name" type="text" name="name" class="form-control" required />
        </div>

        <div class="mb-3">
          <label>Price</label>
          <input v-model="form.price" type="number" step="1" min="1" max="2147483647" name="price" class="form-control" required />
        </div>

        <div class="mb-3">
          <label>Commission</label>
          <input v-model="form.commission" type="number" step="1" min="0" max="100" name="commission" class="form-control" required />
        </div>

        <div class="mb-3">
          <label>Rank</label>
          <input v-model="form.rank" type="text" name="rank" class="form-control" />
        </div>

        <div class="mb-3">
          <label>Telegram Link</label>
          <input v-model="form.telegram_link" type="text" name="telegram_link" class="form-control" placeholder="https://t.me/yourchannel" />
        </div>

        <div class="mb-3">
          <label>Status</label>
          <select v-model="form.status" name="status" class="form-control" required>
            <option value="active">Active</option>
            <option value="deactive">Deactive</option>
          </select>
        </div>
        <button type="submit" class="btn btn-primary">{{ isEdit ? 'Update' : 'Save' }}</button>
      </form>
    </div>
  </DashboardLayout>
</template>
