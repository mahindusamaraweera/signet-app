import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/store/auth'

// Route map ports ui_spec.md's "Roles & route map" table. `meta.roles` is
// the role allowlist (`role:` middleware equivalent); omitted/empty means
// "any authenticated user"; `meta.public: true` means no auth required at
// all (login, register, password reset, the marketing homepage).
const routes = [
  { path: '/wallet/tier-link', name: 'wallet.tier-link', component: () => import('@/views/vesting/TierAccountLinkPage.vue') },
  { path: '/wallet/tier-link/callback', name: 'wallet.tier-link.callback', component: () => import('@/views/vesting/TierAccountLinkPage.vue') },
  { path: '/wallet/deposits', name: 'wallet.deposits', component: () => import('@/views/vesting/BlockchainWalletPage.vue') },
  { path: '/wallet/security', name: 'wallet.security', component: () => import('@/views/vesting/WithdrawalSecurityPage.vue') },
  { path: '/wallet/withdraw', name: 'wallet.withdraw', component: () => import('@/views/vesting/WithdrawalPage.vue') },
	{ path: '/wallet/vesting', name: 'wallet.vesting', component: () => import('@/views/vesting/BlockchainVestingPage.vue') },
	{ path: '/admin/vesting', name: 'admin.vesting', component: () => import('@/views/vesting/BlockchainVestingPage.vue'), props: { admin: true }, meta: { roles: ['company', 'admin'] } },
	{ path: '/register/funding/:id', name: 'register.funding', component: () => import('@/views/auth/RegisterFundingPage.vue'), meta: { public: true } },
  { path: '/', name: 'home', component: () => import('@/views/PublicHomePage.vue'), meta: { public: true, skin: 'site' } },
  { path: '/login', name: 'login', component: () => import('@/views/auth/LoginPage.vue'), meta: { public: true } },
  { path: '/password/reset', name: 'password.request', component: () => import('@/views/auth/ForgotPasswordPage.vue'), meta: { public: true } },
  { path: '/password/reset/verify', name: 'password.reset.verify', component: () => import('@/views/auth/ResetPasswordPage.vue'), meta: { public: true } },
  { path: '/password/reset/:token', name: 'password.reset', component: () => import('@/views/auth/ResetPasswordPage.vue'), meta: { public: true }, props: true },
  { path: '/register', name: 'register.step1', component: () => import('@/views/auth/RegisterStep1Page.vue'), meta: { public: true } },
  { path: '/register/verify', name: 'register.verify', component: () => import('@/views/auth/RegisterVerifyEmailPage.vue'), meta: { public: true } },
  { path: '/register/step2/:id', name: 'register.step2', component: () => import('@/views/auth/RegisterStep2Page.vue'), meta: { public: true }, props: true },
  { path: '/register/step3/:id', name: 'register.step3.wait', component: () => import('@/views/auth/RegisterStep3WaitPage.vue'), meta: { public: true }, props: true },
  { path: '/register/status/:id', name: 'register.step3.status', component: () => import('@/views/auth/RegisterStatusPage.vue'), meta: { public: true }, props: true },

  { path: '/dashboard', redirect: () => {
      const auth = useAuthStore()
      return auth.dashboardRoute
    } },
  { path: '/admin/dashboard', name: 'admin.dashboard', component: () => import('@/views/admin/AdminDashboardPage.vue'), meta: { roles: ['admin'], bodyClass: 'sg-dashboard' } },
  { path: '/admin/:userId/setup-google-auth', name: 'setup.google.auth', component: () => import('@/views/admin/SetupGoogleAuthPage.vue'), meta: { roles: ['company', 'admin'] }, props: true },
  { path: '/agent/dashboard', name: 'agent.dashboard', component: () => import('@/views/agent/AgentDashboardPage.vue'), meta: { roles: ['agent'], bodyClass: 'sg-dashboard' } },
  { path: '/user/dashboard', name: 'user.dashboard', component: () => import('@/views/user/UserDashboardPage.vue'), meta: { roles: ['user'], bodyClass: 'sg-dashboard' } },

  { path: '/company/dashboard', name: 'company.dashboard', component: () => import('@/views/company/CompanyDashboardPage.vue'), meta: { roles: ['company'], bodyClass: 'sg-dashboard' } },
  { path: '/company/pending-activation', name: 'company.pending.activation', component: () => import('@/views/company/PendingActivationPage.vue'), meta: { roles: ['company'] } },
  { path: '/new-activations', name: 'new.activations', component: () => import('@/views/company/NewActivationsPage.vue'), meta: { roles: ['company'] } },
  { path: '/users', name: 'company.users', component: () => import('@/views/company/UsersPage.vue'), meta: { roles: ['company', 'admin'] } },
  { path: '/leader-code-logs', name: 'leader.code.logs', component: () => import('@/views/company/LeaderCodeLogsPage.vue'), meta: { roles: ['company', 'admin'] } },
  { path: '/executive-code-logs', name: 'executive.code.logs', component: () => import('@/views/company/ExecutiveCodeLogsPage.vue'), meta: { roles: ['company', 'admin'] } },
  { path: '/mining/users', name: 'mining.users', component: () => import('@/views/company/MiningUsersPage.vue'), meta: { roles: ['company'] } },
  { path: '/roc', name: 'company.roc', component: () => import('@/views/company/RocIncomePage.vue'), meta: { roles: ['company'] } },
  { path: '/direct-share', name: 'company.direct_share', component: () => import('@/views/company/DirectSharePage.vue'), meta: { roles: ['company'] } },
  { path: '/direct-share-log', name: 'company.direct_share_log', component: () => import('@/views/company/DirectShareLogPage.vue'), meta: { roles: ['company'] } },
  { path: '/user-parent-logs', name: 'userparentlogs.index', component: () => import('@/views/company/UserParentLogsPage.vue'), meta: { roles: ['company'] } },
  { path: '/salaries', name: 'salaries.index', component: () => import('@/views/company/SalariesPage.vue'), meta: { roles: ['company'] } },
  { path: '/leaders/gain', name: 'leaders.gain', component: () => import('@/views/company/LeadersGainPage.vue'), meta: { roles: ['company'] } },
  { path: '/executives/gain', name: 'executives.gain', component: () => import('@/views/company/ExecutivesGainPage.vue'), meta: { roles: ['company'] } },
  { path: '/leadership-bonus-log', name: 'company.leadership_bonus_log', component: () => import('@/views/company/LeadershipBonusLogPage.vue'), meta: { roles: ['company'] } },

  { path: '/countries', name: 'countries.index', component: () => import('@/views/countries/CountriesIndexPage.vue'), meta: { roles: ['company'] } },

  { path: '/packages', name: 'packages.index', component: () => import('@/views/packages/PackagesIndexPage.vue'), meta: { roles: ['company'] } },
  { path: '/packages/create', name: 'packages.create', component: () => import('@/views/packages/PackageFormPage.vue'), meta: { roles: ['company'] } },
  { path: '/packages/:id/edit', name: 'packages.edit', component: () => import('@/views/packages/PackageFormPage.vue'), meta: { roles: ['company'] }, props: true },
  { path: '/buy-package', name: 'buy.package', component: () => import('@/views/packages/BuyPackagePage.vue') },
  { path: '/buy-package-done', name: 'buy.package.done', component: () => import('@/views/packages/BuyPackageDonePage.vue') },
  { path: '/buy-package-history', name: 'buy.package.history', component: () => import('@/views/packages/BuyPackageHistoryPage.vue') },

  { path: '/earn/history', name: 'earn.history', component: () => import('@/views/earn/EarnHistoryPage.vue') },

  { path: '/token-shares', name: 'token.share', component: () => import('@/views/tokens/ShareTokenPage.vue'), meta: { roles: ['admin', 'user', 'agent'] } },
  { path: '/token/share/logs', name: 'token.share.log', component: () => import('@/views/tokens/ShareTokenLogPage.vue'), meta: { roles: ['admin', 'user', 'agent'] } },
  { path: '/view-tokens/:userId', name: 'view.tokens', component: () => import('@/views/tokens/ViewTokensPage.vue'), meta: { roles: ['company'] }, props: true },

  { path: '/my-geneology', name: 'my.geneology', component: () => import('@/views/geneology/GeneologyIndexPage.vue') },
  { path: '/geneology/:userId', name: 'geneology.show', component: () => import('@/views/geneology/GeneologyShowPage.vue'), props: true },

  { path: '/kyc', name: 'kyc.index', component: () => import('@/views/kyc/KycIndexPage.vue') },
  { path: '/kyc/show', name: 'kyc.show', component: () => import('@/views/kyc/KycShowPage.vue') },
  { path: '/kyc/create', name: 'kyc.create', component: () => import('@/views/kyc/KycCreatePage.vue') },
  { path: '/kyc/:id/edit', name: 'kyc.edit', component: () => import('@/views/kyc/KycEditPage.vue'), props: true },
  { path: '/kyc/verified', name: 'kyc.verified', component: () => import('@/views/kyc/KycVerifiedPage.vue'), meta: { roles: ['company'] } },

  { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('@/views/NotFoundPage.vue'), meta: { public: true } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to, from) {
    // In-page anchors (the welcome page's #about, #faq, ...) are plain
    // <a href="#..."> links: leave the browser's own jump alone.
    if (to.hash && to.path === from.path) return false
    if (to.hash) return { el: to.hash }
    return { top: 0 }
  },
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.bootstrapped) {
    await auth.bootstrap()
  }

  if (to.meta.public) {
    // Logged-in users hitting /login or / should land on their dashboard,
    // matching the original's `guest` middleware redirect behavior.
    if ((to.name === 'login' || to.name === 'home') && auth.authenticated) {
      return auth.dashboardRoute
    }
    return true
  }

  if (!auth.authenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }

  const allowedRoles = to.meta.roles
  if (allowedRoles && allowedRoles.length && !allowedRoles.includes(auth.role)) {
    return auth.dashboardRoute
  }

  return true
})

export default router
