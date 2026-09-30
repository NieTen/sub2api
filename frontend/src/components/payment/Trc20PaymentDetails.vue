<template>
  <div class="w-full space-y-4 rounded-xl border border-teal-200 bg-teal-50/60 p-4 text-left dark:border-teal-900 dark:bg-teal-950/20" data-test="trc20-payment-details">
    <div class="flex items-center justify-between gap-3">
      <span class="text-sm text-gray-600 dark:text-gray-300">{{ t('payment.crypto.network') }}</span>
      <span class="rounded-md bg-teal-100 px-2 py-1 text-xs font-semibold text-teal-900 dark:bg-teal-900 dark:text-teal-100">TRON (TRC20)</span>
    </div>
    <div class="rounded-lg border border-teal-300 bg-white p-4 dark:border-teal-700 dark:bg-dark-800">
      <p class="mb-2 text-sm font-bold text-teal-900 dark:text-teal-100">{{ t('payment.crypto.exactAmount') }}</p>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <p class="break-all font-mono text-3xl font-bold tracking-tight text-teal-900 dark:text-teal-100"><span data-test="trc20-exact-amount">{{ amountExact }}</span> <span class="text-sm">USDT</span></p>
        <button type="button" class="btn btn-secondary btn-sm" data-test="copy-trc20-amount" @click="copyToClipboard(amountExact)">{{ t('payment.crypto.copyAmount') }}</button>
      </div>
    </div>
    <div class="rounded-lg border border-amber-300 bg-amber-50 p-3 text-amber-950 dark:border-amber-700 dark:bg-amber-950/40 dark:text-amber-100">
      <p class="font-bold">{{ t('payment.crypto.networkFeeTitle') }}</p>
      <p class="mt-1 text-sm leading-6">{{ t('payment.crypto.networkFeeHint') }}</p>
    </div>
    <div v-if="billAmount !== undefined" class="flex items-center justify-between gap-3 text-sm">
      <span class="text-gray-600 dark:text-gray-300">{{ t('payment.crypto.billAmount') }}</span>
      <span class="font-semibold text-gray-900 dark:text-gray-100" data-test="trc20-bill-amount">{{ formatPaymentAmount(billAmount, 'USDT') }}</span>
    </div>
    <div>
      <p class="mb-1 text-sm text-gray-600 dark:text-gray-300">{{ t('payment.crypto.address') }}</p>
      <p class="select-all break-all font-mono text-sm leading-6 text-gray-900 dark:text-gray-100" data-test="trc20-address">{{ address }}</p>
      <button type="button" class="btn btn-secondary btn-sm mt-2" data-test="copy-trc20-address" @click="copyToClipboard(address)">{{ t('payment.crypto.copyAddress') }}</button>
    </div>
    <p class="text-sm leading-6 text-amber-800 dark:text-amber-200">{{ t('payment.crypto.transferHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import { formatPaymentAmount } from '@/components/payment/currency'

defineProps<{ address: string; amountExact: string; billAmount?: number }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
</script>
