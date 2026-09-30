<template>
  <section class="card overflow-hidden" data-test="usdt-rate-chart">
    <div class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-100 p-5 dark:border-dark-700">
      <div>
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('payment.exchange.historyTitle') }}</h3>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('payment.exchange.historyHint') }}</p>
      </div>
      <div class="flex items-center gap-3">
        <RouterLink to="/admin/settings?tab=payment" class="text-sm font-medium text-primary-600 hover:underline dark:text-primary-400">{{ t('payment.exchange.configure') }}</RouterLink>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" :aria-label="t('common.refresh')" @click="load"><Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" /></button>
      </div>
    </div>
    <div class="p-5">
      <div v-if="error" class="mb-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300" role="alert">{{ error }}</div>
      <div class="mb-4 flex flex-wrap items-center justify-between gap-4">
        <div class="space-y-1">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.exchange.currentRate') }}</p>
          <p class="font-mono text-2xl font-semibold tabular-nums text-gray-900 dark:text-white" data-test="current-usdt-rate">{{ currentQuote ? '1 USDT = ¥' + formatUsdtRate(currentQuote.rate) : '—' }}</p>
          <span v-if="data?.current" class="inline-flex rounded-md px-2 py-1 text-xs font-semibold" :class="currentQuote?.source === 'okx' ? 'bg-teal-50 text-teal-800 dark:bg-teal-950/50 dark:text-teal-200' : 'bg-orange-50 text-orange-800 dark:bg-orange-950/50 dark:text-orange-200'">{{ t(currentQuote?.source === 'okx' ? 'payment.exchange.livePayment' : currentQuote?.source === 'fallback' ? 'payment.exchange.fallbackPayment' : 'payment.exchange.quoteUnavailable') }}</span>
        </div>
        <dl class="flex flex-wrap gap-x-6 gap-y-2 text-sm">
          <div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.exchange.successCount') }}</dt><dd class="mt-1 font-mono font-semibold text-teal-700 dark:text-teal-300">{{ successCount }}</dd></div>
          <div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.exchange.fallbackCount') }}</dt><dd class="mt-1 font-mono font-semibold text-orange-700 dark:text-orange-300">{{ fallbackCount }}</dd></div>
          <div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.exchange.failedCount') }}</dt><dd class="mt-1 font-mono font-semibold text-gray-700 dark:text-gray-300">{{ unavailableCount }}</dd></div>
        </dl>
      </div>
      <p v-if="data?.error || data?.current?.fallback_reason" class="mb-3 rounded-lg bg-orange-50 p-3 text-sm leading-6 text-orange-800 dark:bg-orange-950/30 dark:text-orange-200" data-test="usdt-current-error">{{ data.error || data.current?.fallback_reason }}</p>
      <div class="h-64 sm:h-72">
        <div v-if="loading && !data" class="flex h-full items-center justify-center"><LoadingSpinner size="md" /></div>
        <Line v-else-if="hasPlottableHistory" :data="chartData" :options="chartOptions" role="img" :aria-label="t('payment.exchange.chartAccessible')" />
        <div v-else class="flex h-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-gray-200 px-4 text-center dark:border-dark-600" data-test="usdt-history-empty"><p class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('payment.exchange.noHistory') }}</p><p class="max-w-lg text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('payment.exchange.noHistoryHint') }}</p></div>
      </div>
      <div class="mt-3 flex flex-wrap justify-between gap-2 text-xs text-gray-500 dark:text-gray-400">
        <p v-if="fallbackCount || unavailableCount">{{ t('payment.exchange.historyFailure') }}</p>
        <p v-if="data?.current?.fetched_at">{{ t('payment.exchange.fetchedAt') }}：{{ formatTime(data.current.fetched_at) }}</p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { Chart as ChartJS, LinearScale, PointElement, LineElement, Tooltip, Legend, type ChartData, type ChartOptions } from 'chart.js'
import { Line } from 'vue-chartjs'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { UsdtRateHistoryResponse } from '@/types/payment'
import { formatUsdtRate, freshUsdtQuote, usableUsdtQuote } from '@/components/payment/usdtExchange'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'

ChartJS.register(LinearScale, PointElement, LineElement, Tooltip, Legend)
const { t } = useI18n()
const data = ref<UsdtRateHistoryResponse | null>(null)
const loading = ref(false)
const error = ref('')
const windowEnd = ref(Date.now())
const currentQuote = computed(() => freshUsdtQuote(data.value?.current, windowEnd.value) ? data.value?.current : null)
let refreshTimer: ReturnType<typeof setInterval> | undefined
let disposed = false
const history = computed(() => (data.value?.history || []).filter(point => {
  const time = Date.parse(point.fetched_at || point.observed_at)
  return Number.isFinite(time) && time >= windowEnd.value - 72 * 60 * 60 * 1000 && time <= windowEnd.value
}).sort((a, b) => Date.parse(a.fetched_at || a.observed_at) - Date.parse(b.fetched_at || b.observed_at)))
const successCount = computed(() => history.value.filter(point => point.source === 'okx').length)
const fallbackCount = computed(() => history.value.filter(point => point.source === 'fallback').length)
const unavailableCount = computed(() => history.value.filter(point => point.source === 'unavailable').length)
const hasPlottableHistory = computed(() => history.value.some(usableUsdtQuote))
const chartData = computed<ChartData<'line', { x: number; y: number | null }[]>>(() => ({
  datasets: [
    { source: 'okx', label: t('payment.exchange.successSeries'), color: '#0d9488' },
    { source: 'fallback', label: t('payment.exchange.fallbackSeries'), color: '#d97706' },
  ].map(series => ({
    label: series.label,
    data: history.value.map(point => ({ x: Date.parse(point.fetched_at || point.observed_at), y: point.source === series.source && usableUsdtQuote(point) ? point.rate : null })),
    borderColor: series.color,
    backgroundColor: series.color,
    pointRadius: 3,
    pointHoverRadius: 5,
    borderWidth: 2,
    borderDash: series.source === 'fallback' ? [5, 4] : undefined,
    tension: 0,
    // 缺采样或失败区间不连续绘线，不把连接线伪装成实际报价。
    spanGaps: 31 * 60 * 1000,
  })),
}))
const chartOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: false,
  parsing: false,
  interaction: { mode: 'nearest', intersect: false },
  scales: {
    x: { type: 'linear', min: windowEnd.value - 72 * 60 * 60 * 1000, max: windowEnd.value, grid: { display: false }, ticks: { maxTicksLimit: 7, callback: value => new Date(Number(value)).toLocaleString(undefined, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) } },
    y: { title: { display: true, text: 'CNY / USDT' }, ticks: { callback: value => '¥' + Number(value).toFixed(2) } },
  },
  plugins: { legend: { position: 'bottom', labels: { usePointStyle: true, padding: 16 } }, tooltip: { callbacks: { title: items => items.length && items[0].parsed.x !== null ? formatTime(new Date(items[0].parsed.x).toISOString()) : '', label: item => item.dataset.label + ': ¥' + formatUsdtRate(item.parsed.y ?? 0) } } },
}))
function formatTime(value: string): string { return new Date(value).toLocaleString() }
async function load() {
  if (loading.value) return
  loading.value = true
  windowEnd.value = Date.now()
  error.value = ''
  try {
    const response = await adminPaymentAPI.getUsdtRates()
    if (disposed) return
    data.value = response.data
    windowEnd.value = Date.now()
  } catch {
    if (!disposed) error.value = t('payment.exchange.loadFailed')
  } finally { loading.value = false }
}
onMounted(() => { void load(); refreshTimer = setInterval(load, 30 * 60 * 1000) })
onUnmounted(() => { disposed = true; if (refreshTimer) clearInterval(refreshTimer) })
</script>
