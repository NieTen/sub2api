<template>
  <div v-if="!runs.length" class="px-4 py-9 text-center text-sm text-gray-500 dark:text-gray-400">{{ emptyText || t('modelDetection.noHistory') }}</div>
  <div v-else class="overflow-x-auto">
    <table class="w-full whitespace-nowrap text-left text-sm">
      <thead class="border-b border-gray-200 bg-gray-50 text-xs text-gray-500 dark:text-gray-400 dark:border-dark-700 dark:bg-dark-900"><tr><th class="px-4 py-3">{{ t('modelDetection.account') }} / {{ t('modelDetection.model') }}</th><th class="px-4 py-3">{{ t('modelDetection.result') }}</th><th class="px-4 py-3">{{ t('modelDetection.score') }}</th><th class="px-4 py-3">{{ t('modelDetection.time') }}</th><th class="px-4 py-3"><span class="sr-only">{{ t('modelDetection.detail') }}</span></th></tr></thead>
      <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
        <tr v-for="run in runs" :key="run.id" class="transition-colors hover:bg-gray-50/70 dark:hover:bg-dark-700/40">
          <td class="px-4 py-3"><router-link :to="{ path: '/admin/model-detection', query: { account_id: run.account_id } }" class="block max-w-64 truncate rounded font-medium text-primary-600 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:text-primary-400" :title="run.account_name">{{ run.account_name || `#${run.account_id}` }}</router-link><span class="mt-1 block max-w-64 truncate font-mono text-xs text-gray-500 dark:text-gray-400" :title="run.model_id">{{ run.model_id }}</span></td>
          <td class="px-4 py-3"><span :class="run.verdict === 'suspected_drop' || run.status === 'error' ? 'text-red-600 dark:text-red-400' : 'text-gray-700 dark:text-gray-200'">{{ t(detectionResultKey(run)) }}</span><span v-if="run.status === 'running'" class="mt-1 block font-mono text-xs text-gray-500 dark:text-gray-400">{{ run.progress }} / {{ run.requests_total }}</span></td>
          <td class="px-4 py-3"><span class="font-mono">{{ detectionScore(run.score) }}</span><span v-if="run.baseline_score != null" class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.baseline') }} {{ detectionScore(run.baseline_score) }}<span v-if="run.drop_points != null && run.drop_points > 0" class="ml-2 text-red-600 dark:text-red-400">↓ {{ detectionScore(run.drop_points) }}</span></span></td>
          <td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(run.finished_at || run.started_at || run.created_at) }}</td>
          <td class="px-4 py-3"><button type="button" class="inline-flex min-h-9 items-center gap-1.5 rounded-lg px-2 text-sm font-medium text-primary-600 transition-colors hover:bg-primary-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:text-primary-400 dark:hover:bg-primary-900/20" @click="$emit('detail', run.id)"><Icon name="eye" size="sm" aria-hidden="true" />{{ t('modelDetection.detail') }}</button></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { detectionResultKey, detectionScore, type ModelDetectionRun } from '@/api/admin/modelDetection'
import { formatDateTime } from '@/utils/format'
import Icon from '@/components/icons/Icon.vue'
defineProps<{ runs: ModelDetectionRun[]; emptyText?: string }>()
defineEmits<{ detail: [id: number] }>()
const { t } = useI18n()
</script>
