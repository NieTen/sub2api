<template>
  <section class="rounded-xl border border-teal-200 bg-teal-50/40 p-5 dark:border-teal-900 dark:bg-teal-950/10" data-test="usdt-rate-settings">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h4 class="font-semibold text-gray-900 dark:text-white">{{ t('payment.exchange.settingsTitle') }}</h4>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('payment.exchange.settingsHint') }}</p>
      </div>
      <span class="rounded-md bg-teal-100 px-2 py-1 text-xs font-medium text-teal-800 dark:bg-teal-900/50 dark:text-teal-200">{{ t('payment.exchange.everyThirtyMinutes') }}</span>
    </div>
    <p v-if="loadError" class="mt-4 text-sm text-red-600 dark:text-red-400" role="alert">{{ loadError }} <button type="button" class="underline" @click="load">{{ t('common.tryAgain') }}</button></p>
    <div class="mt-4 flex flex-wrap items-end gap-3">
      <div class="w-full sm:w-72">
        <label for="usdt-fallback-rate" class="input-label">{{ t('payment.exchange.fallbackSetting') }}</label>
        <div class="relative">
          <span class="pointer-events-none absolute inset-y-0 left-3 flex items-center text-sm text-gray-500">1 USDT =</span>
          <input id="usdt-fallback-rate" v-model="fallbackRate" type="number" min="1" max="100" step="0.000001" inputmode="decimal" class="input pl-24 pr-12" :placeholder="t('payment.exchange.notConfigured')" :disabled="loading || saving || !!loadError" @keydown.enter.prevent="save" />
          <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-gray-500">CNY</span>
        </div>
      </div>
      <button type="button" class="btn btn-primary" :disabled="loading || saving || !!loadError" @click="save">{{ saving ? t('common.saving') : t('payment.exchange.saveFallback') }}</button>
    </div>
    <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('payment.exchange.separateSaveHint') }}</p>
    <p v-if="validationError" class="mt-2 text-sm text-red-600 dark:text-red-400" role="alert">{{ validationError }}</p>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminPaymentAPI } from '@/api/admin/payment'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const fallbackRate = ref<number | string>('')
const loading = ref(true)
const saving = ref(false)
const validationError = ref('')
const loadError = ref('')

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const { data } = await adminPaymentAPI.getConfig()
    fallbackRate.value = data.usdt_cny_fallback_rate > 0 ? data.usdt_cny_fallback_rate : ''
  } catch (error) {
    loadError.value = extractI18nErrorMessage(error, t, 'payment.errors', t('payment.exchange.loadFailed'))
  } finally { loading.value = false }
}

async function save() {
  if (loading.value || saving.value || loadError.value) return
  const value = Number(fallbackRate.value)
  if (!Number.isFinite(value) || value < 1 || value > 100 || !/^\d+(?:\.\d{1,6})?$/.test(String(fallbackRate.value))) {
    validationError.value = t('payment.exchange.invalidFallback')
    return
  }
  validationError.value = ''
  saving.value = true
  try {
    await adminPaymentAPI.updateConfig({ usdt_cny_fallback_rate: value })
    appStore.showSuccess(t('common.saved'))
  } catch (error) {
    validationError.value = extractI18nErrorMessage(error, t, 'payment.errors', t('common.error'))
  } finally { saving.value = false }
}
onMounted(load)
</script>
