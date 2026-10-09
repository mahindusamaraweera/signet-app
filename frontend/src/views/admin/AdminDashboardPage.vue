<script setup>
// Ports admin/dashboard.blade.php (route('admin.dashboard')). Differs from the
// agent/user views in layout: My Token and My Wallet are half-width on small
// screens, My Wallet appears twice (the second full-width copy without the
// USDT suffix — a copy-paste artifact in the original, kept), the mining
// card closes the first `.row`, the activations table sits in its own
// `.row.mt-4`, and an empty "Mining Dashboard" container trails the page.
// Its activation alerts use fixed text. See dashboard_handler.go's
// adminDashboardHandler.
import { computed } from 'vue'
import DashboardLayout from '@/components/layout/DashboardLayout.vue'
import DashboardStatCard from '@/components/widgets/DashboardStatCard.vue'
import RankWidget from '@/components/widgets/RankWidget.vue'
import RocWidget from '@/components/widgets/RocWidget.vue'
import MiningWidget from '@/components/widgets/MiningWidget.vue'
import Paginator from '@/components/shared/Paginator.vue'
import FlashAlert from '@/components/shared/FlashAlert.vue'
import { useRoleDashboard } from '@/composables/useRoleDashboard'
import { useSignetDashboard } from '@/composables/useSignetDashboard'

// Unlike the original admin view (fixed "Error updating package."), show the
// server's reason — "Not enough tokens.", "Please top up your wallet." —
// as the user and agent dashboards do.
const { authStore, resp, loadError, onVacation, urlDisplay, fetchDashboard, toggleVacation, copyLink, needTokens, activate } = useRoleDashboard('/admin/dashboard', {
  errorFromResponse: true,
})
useSignetDashboard(resp)

const user = computed(() => authStore.user)
const myWallet = computed(() => Number(resp.value?.total_value || 0) - Number(resp.value?.wallet_balance || 0))
</script>

<template>
  <DashboardLayout>
    <FlashAlert type="danger" :message="loadError" @close="loadError = ''" />
    <div v-if="resp" class="row mt-4">
      <div class="col-12 mb-4">
        <div class="card bg-yellow-100 border-0 shadow">
          <div class="card-header d-sm-flex flex-row align-items-center flex-0 justify-content-between">
            <div class="d-block mb-3 mb-sm-0">
              <div class="fs-5 fw-normal mb-2"></div>
              <h2 class="fs-3 fw-extrabold">Welcome, <span class="text-capitalize">{{ user?.role }}</span> Head</h2>
              <h3 class="fs-3 fw-extrabold">
                <span class="text-capitalize">{{ resp.my_package?.rank }} {{ 'SIG-00' + user?.id }}</span>
              </h3>
              <RankWidget :rank="resp.rank" />
              <RocWidget :roc="resp.roc" :roc-status="user?.roc_status" />
            </div>

            <div class="form-check form-switch">
              <input id="vacationSwitch" class="form-check-input" type="checkbox" :checked="user?.on_vacation" @change="toggleVacation" />
              <label class="form-check-label" for="vacationSwitch">
                Vacation {{ onVacation ? 'ON' : 'OFF' }}
              </label>
            </div>
          </div>
          <div class="card-body p-2">
            <div class="container ref">
              <label>Your Referral Link</label>
              <input id="urlInput" type="text" class="form-control" placeholder="Enter a URL here" :value="resp.ref_link" readonly />
              <button id="copyButton" type="button" class="btn btn-primary d-inline-flex align-items-center" @click="copyLink">
                Copy URL
                <svg class="icon icon-xxs ms-2" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M2 9.5A3.5 3.5 0 005.5 13H9v2.586l-1.293-1.293a1 1 0 00-1.414 1.414l3 3a1 1 0 001.414 0l3-3a1 1 0 00-1.414-1.414L11 15.586V13h2.5a4.5 4.5 0 10-.616-8.958 4.002 4.002 0 10-7.753 1.977A3.5 3.5 0 002 9.5zm9 3.5H9V8a1 1 0 012 0v5z" clip-rule="evenodd"></path></svg>
              </button>
              <div id="urlDisplay" class="url-display">{{ urlDisplay }}</div>
            </div>
          </div>
        </div>
      </div>

      <DashboardStatCard col-class="col-12 col-sm-6 col-xl-4 mb-4" primary label="My Token" :value="resp.my_tokens" usdt />
      <DashboardStatCard col-class="col-12 col-sm-6 col-xl-4 mb-4" label="My Wallet" :value="myWallet" usdt />
      <DashboardStatCard label="My Wallet" :value="myWallet" />
      <DashboardStatCard v-if="resp.my_globle_director_share" label="My Global Director Share Wallet" :value="resp.my_globle_director_share.balance ?? 0" />
      <DashboardStatCard label="My Earn" :value="resp.wallet_balance ?? 0" usdt />
      <DashboardStatCard label="This Time Direct Share Pool value" :value="resp.pool_amount ?? 0" usdt />
      <DashboardStatCard label="This Time All Share Value" :value="resp.total_poolshare_value ?? 0" />
      <DashboardStatCard v-if="user?.global_director_share_status" label="This Time My Share Portion" :value="Math.trunc(resp.my_share_value ?? 0)" usdt>
        <small class="text-muted">
          This Time My Share Value:
          <span class="fw-bold text-primary">
            {{ Math.trunc(user?.global_director_share ?? 0) }}
          </span>
        </small>
      </DashboardStatCard>

      <MiningWidget v-if="user" :user-id="user.id" :all-users-count="resp.all_users || 0" col-class="col-lg-12" />
    </div>

    <div v-if="resp" class="row mt-4">
        <div class="col-12 col-xl-12">
          <div class="row">
            <div class="col-12 mb-4">
              <div class="card border-0 shadow">
                <div class="card-header">
                  <div class="row align-items-center">
                    <div class="col">
                      <h2 class="fs-5 fw-bold mb-0">Activations</h2>
                    </div>
                    <div class="col text-end">
                      <a href="#" class="btn btn-sm btn-primary" @click.prevent>See all</a>
                    </div>
                  </div>
                </div>
                <div class="table-responsive">
                  <table class="table align-items-center table-flush">
                    <thead class="thead-light">
                      <tr>
                        <th class="border-bottom" scope="col">User name</th>
                        <th class="border-bottom" scope="col">Whats app</th>
                        <th class="border-bottom" scope="col">Need Tokens</th>
                        <th class="border-bottom" scope="col">Action</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="row in resp.activations?.data" :key="row.id">
                        <td>{{ row.user_name || 'Unknown User' }}</td>
                        <td>{{ row.user_whatsapp }}</td>
                        <td><span class="badge bg-success">{{ needTokens(row) }} USDT</span></td>
                        <td>
                          <span v-if="row.company_status == 0" class="badge bg-success p-2">Wait for company activation</span>
                          <button v-else class="btn btn-primary active-package" @click="activate(row.id)">Active</button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                  <div class="d-flex justify-content-center">
                    <Paginator :pagination="resp.activations" @change="(p) => fetchDashboard(p)" />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
    </div>

    <!-- Mining Dashboard -->
    <div class="container-fluid mt-4">
      <div class="row justify-content-center"></div>
    </div>
  </DashboardLayout>
</template>
