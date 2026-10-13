<script setup>
// Ports the centered white-card-over-bg-soft chrome the auth-style Blade
// views share (login, password reset, register steps 2-4, buy package).
// The views differ in the details, exposed as props:
//  - backPlacement: login puts the "Back to Homepage" link above the
//    `.row.form-bg-image`; the others put their "Back to log in" link inside
//    it ('row'); buy-package has none (showBackLink false).
//  - cardClass: the white card's classes (login adds border-light, the
//    password-reset request card adds signin-inner my-3 my-lg-0).
//  - signinBackground: login's row carries volt.js's data-background-lg
//    illustration, shown only from the lg breakpoint up.
// Three register views link "Back to log in" to a broken relative
// "./sign-in.html"; that link goes to /login here instead.
defineProps({
  embedded: { type: Boolean, default: false },
  showBackLink: { type: Boolean, default: true },
  backText: { type: String, default: 'Back to Homepage' },
  backTo: { type: String, default: '/' },
  backPlacement: { type: String, default: 'container' }, // 'container' | 'row'
  cardClass: { type: String, default: 'bg-white shadow border-0 rounded p-4 p-lg-5 w-100 fmxw-500' },
  signinBackground: { type: Boolean, default: false },
})
</script>

<template>
  <component :is="embedded ? 'div' : 'main'" :class="{ 'embedded-auth-card': embedded }">
    <section class="auth-layout-shell bg-soft d-flex align-items-center">
      <div class="container">
        <p v-if="showBackLink && backPlacement === 'container'" class="text-center">
          <RouterLink :to="backTo" class="d-flex align-items-center justify-content-center">
            <svg class="icon icon-xs me-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" d="M7.707 14.707a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 1.414L5.414 9H17a1 1 0 110 2H5.414l2.293 2.293a1 1 0 010 1.414z" clip-rule="evenodd"></path></svg>
            {{ backText }}
          </RouterLink>
        </p>
        <div class="row justify-content-center form-bg-image" :class="{ 'auth-bg-image': signinBackground }">
          <p v-if="showBackLink && backPlacement === 'row'" class="text-center">
            <RouterLink :to="backTo" class="d-flex align-items-center justify-content-center">
              <svg class="icon icon-xs me-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" d="M7.707 14.707a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 1.414L5.414 9H17a1 1 0 110 2H5.414l2.293 2.293a1 1 0 010 1.414z" clip-rule="evenodd"></path></svg>
              {{ backText }}
            </RouterLink>
          </p>
          <div class="col-12 d-flex align-items-center justify-content-center">
            <div :class="cardClass">
              <slot />
            </div>
          </div>
        </div>
      </div>
    </section>
  </component>
</template>

<style scoped>
.auth-layout-shell { min-height:100vh;min-height:100svh;height:auto;padding:32px 0; }
/* volt.js's data-background-lg: the illustration is only applied when the
   viewport is wider than the lg breakpoint. */
@media (min-width: 992px) {
  .auth-bg-image {
    background-image: url('/resources/assets/img/illustrations/signin.svg');
  }
}
</style>
