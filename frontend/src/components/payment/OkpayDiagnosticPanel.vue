<template>
  <section class="border-t border-gray-100 px-4 py-3 dark:border-dark-700" data-test="okpay-diagnostic-panel">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.settings.payment.okpayDiagnostic.title') }}</h3>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.okpayDiagnostic.scope') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="loading" :aria-busy="loading" data-test="diagnose-okpay" @click="diagnose">
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        {{ t(loading ? 'admin.settings.payment.okpayDiagnostic.loading' : 'admin.settings.payment.okpayDiagnostic.action') }}
      </button>
    </div>
    <p v-if="errorMessage" class="mt-3 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300" role="alert">{{ errorMessage }}</p>
    <div v-if="result" class="mt-3 space-y-3" aria-live="polite" data-test="okpay-diagnostic-result">
      <p class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ providerName }} · #{{ providerId }}</p>
      <div class="grid gap-3 sm:grid-cols-2">
        <div v-for="check in orderedChecks" :key="check.mode" class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800" :data-test="'okpay-check-' + check.mode">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h4 class="text-sm font-medium text-gray-900 dark:text-white">{{ t(modeLabels[check.mode]) }}</h4>
            <span class="rounded px-2 py-0.5 text-xs font-medium" :class="check.status === 'authenticated' ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300' : 'bg-amber-100 text-amber-900 dark:bg-amber-900/30 dark:text-amber-200'">{{ t(statusLabels[check.status]) }}</span>
          </div>
          <p class="mt-2 break-words text-sm leading-6 text-gray-600 dark:text-gray-300">{{ check.message }}</p>
        </div>
      </div>
      <p class="rounded-lg bg-blue-50 p-3 text-sm leading-6 text-blue-900 dark:bg-blue-950/30 dark:text-blue-200" data-test="okpay-diagnostic-conclusion">{{ t(conclusionLabels[result.conclusion]) }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminPaymentAPI } from '@/api/admin/payment'
import Icon from '@/components/icons/Icon.vue'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { OkpayDiagnosticConclusion, OkpayDiagnosticMode, OkpayDiagnosticResult, OkpayDiagnosticStatus } from '@/types/payment'

const props = defineProps<{ providerId: number; providerName: string }>()
const { t } = useI18n()
const loading = ref(false)
const errorMessage = ref('')
const result = ref<OkpayDiagnosticResult | null>(null)
let requestVersion = 0

const modes: OkpayDiagnosticMode[] = ['current', 'php_reference']
const modeLabels: Record<OkpayDiagnosticMode, string> = {
  current: 'admin.settings.payment.okpayDiagnostic.current',
  php_reference: 'admin.settings.payment.okpayDiagnostic.phpReference',
}
const statusLabels: Record<OkpayDiagnosticStatus, string> = {
  authenticated: 'admin.settings.payment.okpayDiagnostic.authenticated',
  rejected: 'admin.settings.payment.okpayDiagnostic.rejected',
  request_failed: 'admin.settings.payment.okpayDiagnostic.requestFailed',
  invalid_response: 'admin.settings.payment.okpayDiagnostic.invalidResponseStatus',
}
const conclusionLabels: Record<OkpayDiagnosticConclusion, string> = {
  both_authenticated: 'admin.settings.payment.okpayDiagnostic.bothAuthenticated',
  php_only_authenticated: 'admin.settings.payment.okpayDiagnostic.phpOnlyAuthenticated',
  current_only_authenticated: 'admin.settings.payment.okpayDiagnostic.currentOnlyAuthenticated',
  both_rejected: 'admin.settings.payment.okpayDiagnostic.bothRejected',
  inconclusive: 'admin.settings.payment.okpayDiagnostic.inconclusive',
}
const orderedChecks = computed(() => modes.flatMap(mode => result.value?.checks.filter(check => check.mode === mode) || []))

function reset() {
  requestVersion++
  loading.value = false
  errorMessage.value = ''
  result.value = null
}

watch(() => props.providerId, reset, { flush: 'sync' })
onUnmounted(reset)

async function diagnose() {
  if (loading.value || props.providerId <= 0) return
  const providerId = props.providerId
  const version = ++requestVersion
  loading.value = true
  errorMessage.value = ''
  result.value = null
  try {
    const { data } = await adminPaymentAPI.diagnoseOkpayProvider(providerId)
    // 切换实例、关闭组件或发起新请求后，旧请求不得污染当前结果与加载状态。
    if (version !== requestVersion || providerId !== props.providerId) return
    if (!data || data.provider_instance_id !== providerId || !Array.isArray(data.checks)
      || data.checks.length !== modes.length
      || !modes.every(mode => data.checks.filter(check => check?.mode === mode).length === 1)
      || !data.checks.every(check => Object.prototype.hasOwnProperty.call(statusLabels, check.status) && typeof check.message === 'string')
      || !Object.prototype.hasOwnProperty.call(conclusionLabels, data.conclusion)) {
      errorMessage.value = t('admin.settings.payment.okpayDiagnostic.invalidResponse')
      return
    }
    result.value = data
  } catch (error) {
    if (version !== requestVersion || providerId !== props.providerId) return
    errorMessage.value = extractI18nErrorMessage(error, t, 'payment.errors', t('admin.settings.payment.okpayDiagnostic.failed'))
  } finally {
    if (version === requestVersion) loading.value = false
  }
}
</script>
