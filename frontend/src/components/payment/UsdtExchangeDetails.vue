<template>
  <div v-if="exchange || currency === 'USDT'" :class="compact ? 'mt-1 space-y-1 text-xs' : 'space-y-3 rounded-xl border border-gray-200 bg-gray-50/70 p-4 text-sm dark:border-dark-600 dark:bg-dark-800'" :data-test="compact ? 'usdt-rate-summary' : 'usdt-exchange-details'" :data-source="exchange?.source || 'legacy'">
    <template v-if="exchange">
      <div class="flex flex-wrap items-center gap-2">
        <span class="inline-flex rounded-md px-2 py-1 font-semibold" :class="exchange.source === 'fallback' ? 'bg-orange-100 text-orange-800 dark:bg-orange-950/60 dark:text-orange-200' : 'bg-teal-50 text-teal-800 dark:bg-teal-950/60 dark:text-teal-200'" data-test="usdt-rate-source">{{ t(exchange.source === 'fallback' ? 'payment.exchange.fallbackPayment' : 'payment.exchange.livePayment') }}</span>
        <span class="font-mono tabular-nums text-gray-700 dark:text-gray-200">1 USDT = ¥{{ formatUsdtRate(exchange.rate) }}</span>
      </div>
      <template v-if="!compact">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.exchange.lockedHint') }}</p>
        <div class="flex flex-wrap justify-between gap-2 text-gray-600 dark:text-gray-300"><span>{{ t('payment.exchange.cnyBase') }}</span><span class="font-mono">{{ formatPaymentAmount(exchange.cny_base_amount, 'CNY') }}</span></div>
        <div v-if="exchange.cny_pay_amount > exchange.cny_base_amount" class="flex flex-wrap justify-between gap-2 text-gray-600 dark:text-gray-300"><span>{{ t('payment.orders.fee') }}</span><span class="font-mono">{{ formatPaymentAmount(exchange.cny_pay_amount - exchange.cny_base_amount, 'CNY') }}</span></div>
        <div class="flex flex-wrap justify-between gap-2 text-gray-600 dark:text-gray-300">
          <span>{{ t('payment.exchange.cnyBill') }}</span>
          <span class="font-mono font-medium">{{ formatPaymentAmount(exchange.cny_pay_amount, 'CNY') }}</span>
        </div>
        <p v-if="exchange.source === 'fallback'" class="break-words text-sm leading-6 text-orange-800 dark:text-orange-200">{{ t('payment.exchange.fallbackExplanation') }} <span v-if="exchange.fallback_reason">{{ exchange.fallback_reason }}</span></p>
        <p v-if="exchange.fetched_at || exchange.observed_at" class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.exchange.quoteTime') }}：{{ formatTime(exchange.source === 'fallback' ? exchange.observed_at : exchange.fetched_at || exchange.observed_at) }}</p>
      </template>
    </template>
    <span v-else class="text-gray-500 dark:text-gray-400" data-test="usdt-rate-legacy">{{ t('payment.exchange.legacyPricing') }}</span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UsdtExchangeSnapshot } from '@/types/payment'
import { formatPaymentAmount } from './currency'
import { formatUsdtRate } from './usdtExchange'

defineProps<{ exchange?: UsdtExchangeSnapshot; currency?: string; compact?: boolean }>()
const { t } = useI18n()
function formatTime(value: string): string {
  const time = new Date(value)
  return Number.isFinite(time.getTime()) ? time.toLocaleString() : '—'
}
</script>
