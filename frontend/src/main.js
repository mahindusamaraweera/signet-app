import { createApp } from 'vue'
import { createPinia } from 'pinia'
// Volt (Themesberg's Bootstrap 5 admin theme) is the actual design system
// the original Blade app uses — it's a full standalone build (reboot, grid,
// components) so it replaces stock bootstrap.min.css rather than layering
// on top of it. custome.css is the original's own project-specific overrides.
// signet-ui.css + signet-dashboard.css are the Signet design layer the
// original loads last in layouts/app.blade.php (dark palette, fonts, glass
// surfaces; dashboard HUD/motion scoped to body.sg-dashboard).
//
// Every page except the public welcome page is rendered through
// layouts/app.blade.php in the original, and the welcome page ships its own
// unrelated stylesheet (resources/site/css/style.css). The app skin is
// therefore injected as a single <style> element that is switched off while
// a route with `meta.skin === 'site'` is showing, so the two never mix.
import 'bootstrap/dist/js/bootstrap.bundle.min.js'
import sweetalertCss from 'sweetalert2/dist/sweetalert2.min.css?inline'
import voltCss from './assets/theme/volt.css?inline'
import customeCss from './assets/theme/custome.css?inline'
import appCss from './assets/app.css?inline'
import workspaceCss from './assets/workspace.css?inline'
// Select2's stylesheet (the original loads it on the pages that use Select2);
// signet-ui.css restyles it, so it goes before the Signet layer.
import select2Css from 'select2/dist/css/select2.min.css?inline'
import signetUiCss from './assets/theme/signet-ui.css?inline'
import signetDashboardCss from './assets/theme/signet-dashboard.css?inline'

import App from './App.vue'
import router from './router'
import { useAuthStore } from './store/auth'

const appSkin = document.createElement('style')
appSkin.id = 'signet-app-skin'
appSkin.textContent = [sweetalertCss, voltCss, customeCss, appCss, select2Css, signetUiCss, signetDashboardCss, workspaceCss].join('\n')
document.head.appendChild(appSkin)

router.afterEach((to) => {
  appSkin.media = to.meta.skin === 'site' ? 'not all' : 'all'
  // layouts/app.blade.php: <body class="@yield('body_class')"> — the four
  // dashboards set 'sg-dashboard'.
  document.body.className = to.meta.bodyClass || ''
})

const app = createApp(App)
app.use(createPinia())
app.use(router)

// Exposed so api/client.js's 401 interceptor can force-logout without a
// circular import between the store and the api client.
const authStore = useAuthStore()
window.__signetAuthStore = authStore

// Resolve the session once before mounting so the router's auth guard has a
// definitive answer on first navigation instead of racing the /me request.
authStore.bootstrap().finally(() => {
  app.mount('#app')
})
