<template>
  <div v-if="isSplitMode" class="auth-page">
    <header class="auth-header">
      <router-link to="/home" class="auth-brand" :aria-label="siteName">
        <img :src="siteLogo || '/logo.svg'" :alt="siteName" class="auth-brand-logo" />
        <span class="auth-brand-name">{{ siteName }}</span>
      </router-link>

      <nav class="auth-navigation" :aria-label="t('auth.navigationLabel')">
        <router-link to="/home">{{ t('auth.homeLink') }}</router-link>
        <router-link to="/home#pricing">{{ t('auth.modelsLink') }}</router-link>
        <a v-if="documentationUrl" :href="documentationUrl">{{ t('auth.quickStartLink') }}</a>
        <router-link v-else to="/home#support">{{ t('auth.quickStartLink') }}</router-link>
      </nav>

      <div class="auth-header-actions">
        <button
          type="button"
          class="auth-language-button"
          :disabled="switchingLocale"
          :aria-label="t('auth.switchLanguage')"
          :title="t('auth.switchLanguage')"
          @click="toggleLocale"
        >
          <span aria-hidden="true">文<span class="auth-language-latin">A</span></span>
        </button>
        <nav class="auth-account-navigation" :aria-label="t('auth.accountNavigationLabel')">
          <router-link
            to="/login"
            :class="{ 'is-active': activePage === 'login' }"
            :aria-current="activePage === 'login' ? 'page' : undefined"
          >{{ t('auth.signIn') }}</router-link>
          <router-link
            v-if="registrationAvailable"
            to="/register"
            :class="{ 'is-active': activePage === 'register' }"
            :aria-current="activePage === 'register' ? 'page' : undefined"
          >{{ t('auth.signUp') }}</router-link>
        </nav>
      </div>
    </header>

    <main class="auth-shell">
      <div class="auth-grid">
        <section class="auth-introduction">
          <slot name="aside" />
        </section>
        <section class="auth-form-column">
          <div class="auth-card rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-8">
            <slot />
          </div>
          <div v-if="$slots.footer" class="mt-6 text-center text-sm">
            <slot name="footer" />
          </div>
          <footer class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
            &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
          </footer>
        </section>
      </div>
    </main>
  </div>

  <div v-else class="relative flex min-h-screen items-center justify-center overflow-hidden p-4">
    <!-- Background -->
    <div
      class="absolute inset-0 bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"
    ></div>

    <!-- Decorative Elements -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <!-- Gradient Orbs -->
      <div
        class="absolute -right-40 -top-40 h-80 w-80 rounded-full bg-primary-400/20 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-80 w-80 rounded-full bg-primary-500/15 blur-3xl"
      ></div>
      <div
        class="absolute left-1/2 top-1/2 h-96 w-96 -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary-300/10 blur-3xl"
      ></div>

      <!-- Grid Pattern -->
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"
      ></div>
    </div>

    <!-- Content Container -->
    <div class="relative z-10 w-full max-w-md">
      <!-- Logo/Brand -->
      <div class="mb-8 text-center">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div
            class="mb-4 inline-flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl shadow-lg shadow-primary-500/30"
          >
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="text-gradient mb-2 text-3xl font-bold">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div class="card-glass rounded-2xl p-8 shadow-glass">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { setLocale } from '@/i18n'
import { sanitizeUrl } from '@/utils/url'

const props = withDefaults(
  defineProps<{
    mode?: 'center' | 'split'
    activePage?: 'login' | 'register'
    showRegistration?: boolean
  }>(),
  {
    mode: 'center',
    activePage: undefined,
    showRegistration: undefined
  }
)

const appStore = useAppStore()
const { t, locale } = useI18n()
const switchingLocale = ref(false)

const isSplitMode = computed(() => props.mode === 'split')
const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true })
)
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)
const documentationUrl = computed(() =>
  sanitizeUrl(appStore.docUrl || '', { allowRelative: true })
)
const registrationAvailable = computed(() =>
  (props.showRegistration ?? appStore.cachedPublicSettings?.registration_enabled === true) &&
  appStore.cachedPublicSettings?.backend_mode_enabled !== true
)

const currentYear = computed(() => new Date().getFullYear())

async function toggleLocale() {
  if (switchingLocale.value) return
  switchingLocale.value = true
  try {
    await setLocale(locale.value === 'zh' ? 'en' : 'zh')
  } finally {
    switchingLocale.value = false
  }
}

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-page {
  min-height: 100vh;
  overflow-x: hidden;
  color: #111827;
  background: radial-gradient(circle at 50% 48%, rgba(139, 92, 246, 0.1), transparent 28rem),
    linear-gradient(#fff, #fbfcff);
  font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
}

.auth-header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: center;
  gap: 1rem;
  padding: 1.25rem clamp(1.25rem, 3vw, 3.5rem) 0;
}

.auth-brand {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  justify-self: start;
  gap: 0.75rem;
  color: inherit;
  text-decoration: none;
}

.auth-brand-logo {
  width: 36px;
  height: 36px;
  flex-shrink: 0;
  border-radius: 10px;
  object-fit: contain;
}

.auth-brand-name {
  overflow: hidden;
  font-size: 1.18rem;
  font-weight: 900;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.auth-navigation {
  display: flex;
  height: 48px;
  align-items: center;
  gap: 0.35rem;
  padding: 0 0.85rem;
  border: 1px solid rgba(226, 232, 240, 0.82);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 16px 40px #0f172a0f;
  backdrop-filter: blur(18px);
}

.auth-navigation a {
  padding: 0.52rem 0.7rem;
  border-radius: 999px;
  color: #6b7280;
  font-size: 0.92rem;
  font-weight: 800;
  white-space: nowrap;
}

.auth-navigation a:hover {
  color: #111827;
  background: #f4f4f5;
}

.auth-header-actions {
  display: flex;
  align-items: center;
  justify-self: end;
  gap: 0.75rem;
}

.auth-language-button {
  display: inline-flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: #f4f4f5;
  color: #3f3f46;
  font-size: 0.9rem;
  font-weight: 800;
}

.auth-language-button:disabled {
  cursor: wait;
  opacity: 0.6;
}

.auth-language-latin {
  margin-left: 1px;
  font-size: 0.7rem;
}

.auth-account-navigation {
  display: flex;
  height: 42px;
  align-items: center;
  padding: 0.18rem;
  border-radius: 999px;
  background: #f5f3ffeb;
}

.auth-account-navigation a {
  display: inline-flex;
  min-width: 64px;
  height: 36px;
  align-items: center;
  justify-content: center;
  padding: 0 1rem;
  border-radius: 999px;
  color: #6b7280;
  font-weight: 900;
  white-space: nowrap;
}

.auth-account-navigation a.is-active {
  background: linear-gradient(#111827, #25252b);
  box-shadow: 0 12px 24px #11182724;
  color: #fff;
}

.auth-header a:focus-visible,
.auth-header button:focus-visible {
  outline: 2px solid #8b5cf6;
  outline-offset: 4px;
}

.auth-shell {
  display: flex;
  min-height: calc(100vh - 5.25rem);
  align-items: center;
  padding: 2rem clamp(1rem, 3vw, 2rem) 2.5rem;
}

.auth-grid {
  display: grid;
  width: min(72rem, 100%);
  grid-template-columns: minmax(0, 1fr) minmax(360px, 440px);
  align-items: center;
  gap: 3.5rem;
  margin: auto;
}

.auth-introduction {
  min-width: 0;
  max-width: 36rem;
}

.auth-form-column,
.auth-card {
  min-width: 0;
}

.auth-card {
  overflow: hidden;
}

.auth-page :deep(.auth-kicker) {
  color: #4b5563;
  font-size: 0.88rem;
  font-weight: 900;
  letter-spacing: 0.02em;
}

.auth-page :deep(.input:not(.input-error):not(.border-red-500):not(.border-green-500)) {
  border-color: #e5e7eb;
  background: #fff;
  color: #111827;
}

.auth-page :deep(.input:not(.input-error):not(.border-red-500):not(.border-green-500):focus) {
  border-color: #8b5cf6;
  --tw-ring-color: rgba(139, 92, 246, 0.18);
}

.auth-page :deep(.auth-submit) {
  background: linear-gradient(#111827, #25252b);
  box-shadow: 0 14px 30px rgba(17, 24, 39, 0.16);
  color: #fff;
}

.auth-page :deep(.auth-submit:hover:not(:disabled)) {
  box-shadow: 0 18px 36px rgba(17, 24, 39, 0.2);
}

.auth-page :deep(.auth-link) {
  color: #111827;
}

.auth-page :deep(.auth-link:hover) {
  color: #374151;
}

.dark .auth-page {
  color: #f3f4f6;
  background: radial-gradient(circle at 50% 48%, rgba(139, 92, 246, 0.12), transparent 28rem),
    linear-gradient(#111827, #0f172a);
}

.dark .auth-navigation {
  border-color: #374151;
  background: rgba(17, 24, 39, 0.86);
}

.dark .auth-navigation a,
.dark .auth-account-navigation a,
.dark .auth-page :deep(.auth-link),
.dark .auth-page :deep(.auth-kicker) {
  color: #d1d5db;
}

.dark .auth-language-button,
.dark .auth-account-navigation,
.dark .auth-navigation a:hover {
  color: #f3f4f6;
  background: #1f2937;
}

.dark .auth-account-navigation a.is-active {
  background: #f3f4f6;
  color: #111827;
}

.dark .auth-page :deep(.input:not(.input-error):not(.border-red-500):not(.border-green-500)) {
  border-color: #374151;
  background: #111827;
  color: #f3f4f6;
}

.dark .auth-page :deep(.input:not(.input-error):not(.border-red-500):not(.border-green-500):focus) {
  border-color: #a78bfa;
}

@media (max-width: 960px) {
  .auth-header {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .auth-navigation,
  .auth-introduction {
    display: none;
  }

  .auth-grid {
    display: block;
    max-width: 440px;
  }
}

@media (max-width: 640px) {
  .auth-header {
    padding-right: 1rem;
    padding-left: 1rem;
  }

  .auth-brand-name {
    display: none;
  }

  .auth-header-actions {
    gap: 0.45rem;
  }

  .auth-account-navigation {
    height: 40px;
  }

  .auth-account-navigation a {
    min-width: 52px;
    height: 34px;
    padding: 0 0.75rem;
    font-size: 0.9rem;
  }

  .auth-shell {
    min-height: calc(100vh - 4.25rem);
    padding: 1.5rem 1rem 2rem;
  }
}

.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}
</style>
