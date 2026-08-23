<template>
  <div
    v-if="isSplitMode"
    class="relative min-h-screen overflow-hidden bg-gradient-to-br from-white via-slate-50 to-primary-50/40 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"
  >
    <div
      class="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(15,23,42,0.035)_1px,transparent_1px),linear-gradient(90deg,rgba(15,23,42,0.035)_1px,transparent_1px)] bg-[size:72px_72px] opacity-70 dark:bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)]"
    ></div>

    <div class="relative z-10 flex min-h-screen flex-col px-4 py-4 sm:px-6 lg:px-8 lg:py-6">
      <header class="mx-auto flex w-full max-w-7xl items-center gap-3">
        <div
          class="flex h-12 w-12 items-center justify-center overflow-hidden rounded-2xl border border-white/70 bg-white/85 shadow-sm shadow-primary-500/10 dark:border-dark-700/70 dark:bg-dark-900/80"
        >
          <img :src="siteLogo || '/logo.svg'" :alt="`${siteName} logo`" class="h-full w-full object-contain" />
        </div>
        <div class="min-w-0">
          <div class="truncate text-lg font-semibold text-slate-900 dark:text-white">
            {{ siteName }}
          </div>
          <div class="truncate text-xs text-slate-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </div>
        </div>
      </header>

      <main class="mx-auto grid w-full max-w-7xl flex-1 gap-8 py-6 lg:grid-cols-2 lg:gap-12">
        <section class="flex items-center">
          <slot name="aside" />
        </section>

        <section class="flex items-center justify-center lg:justify-end">
          <div class="w-full max-w-[560px]">
            <div
              class="card-glass rounded-2xl border-white/70 bg-white/90 p-6 shadow-[0_24px_70px_rgba(15,23,42,0.12)] backdrop-blur-xl dark:border-dark-700/60 dark:bg-dark-900/80 sm:p-8"
            >
              <slot />
            </div>

            <div class="mt-6 text-center text-sm">
              <slot name="footer" />
            </div>
          </div>
        </section>
      </main>

      <footer class="mx-auto w-full max-w-7xl pb-2 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </footer>
    </div>
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
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const props = withDefaults(
  defineProps<{
    mode?: 'center' | 'split'
  }>(),
  {
    mode: 'center'
  }
)

const appStore = useAppStore()

const isSplitMode = computed(() => props.mode === 'split')
const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true })
)
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}
</style>
