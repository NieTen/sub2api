<template>
  <div class="rounded-xl border p-4 text-sm" :class="quote?.source === 'fallback' ? 'border-orange-200 bg-orange-50/70 dark:border-orange-900 dark:bg-orange-950/20' : 'border-teal-200 bg-teal-50/60 dark:border-teal-900 dark:bg-teal-950/20'" data-test="usdt-exchange-preview">
    <template v-if="usableUsdtQuote(quote)">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-semibold" :class="quote.source === 'fallback' ? 'text-orange-800 dark:text-orange-200' : 'text-teal-800 dark:text-teal-200'">{{ t(quote.source === 'fallback' ? 'payment.exchange.fallbackPayment' : 'payment.exchange.livePayment') }}</span>
        <span class="font-mono font-medium text-gray-800 dark:text-gray-100">1 USDT = ¥{{ formatUsdtRate(quote.rate) }}</span>
      </div>
      <div v-if="cnyAmount > 0" class="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-gray-200/80 pt-3 dark:border-dark-600">
        <span class="text-gray-600 dark:text-gray-300">{{ t('payment.exchange.cnyBill') }} {{ formatPaymentAmount(cnyAmount, 'CNY') }}</span>
        <strong class="font-mono text-lg text-gray-900 dark:text-white" data-test="usdt-preview-pay-amount">{{ formatPaymentAmount(convertCnyToUsdt(cnyAmount, quote.rate), 'USDT') }}</strong>
      </div>
      <p class="mt-2 text-xs leading-5 text-gray-600 dark:text-gray-300">{{ t('payment.exchange.previewHint') }}</p>
      <p v-if="trc20" class="mt-1 text-xs leading-5 text-gray-600 dark:text-gray-300">{{ t('payment.exchange.trc20SuffixHint') }}</p>
      <p v-if="quote.source === 'fallback'" class="mt-2 text-xs leading-5 text-orange-800 dark:text-orange-200">{{ t('payment.exchange.fallbackExplanation') }}</p>
    </template>
    <p v-else role="alert" class="text-orange-800 dark:text-orange-200">{{ error || t('payment.exchange.unavailable') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UsdtCnyQuote } from '@/types/payment'
import { formatPaymentAmount } from './currency'
import { convertCnyToUsdt, formatUsdtRate, usableUsdtQuote } from './usdtExchange'

defineProps<{ quote?: UsdtCnyQuote | null; cnyAmount: number; trc20?: boolean; error?: string }>()
const { t } = useI18n()
</script>
