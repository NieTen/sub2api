<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.batchTest.title')"
    width="extra-wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex flex-wrap items-center gap-2 text-sm">
          <span class="font-medium text-gray-900 dark:text-gray-100">
            {{ t('admin.accounts.batchTest.selected', { count: accountIds.length }) }}
          </span>
          <span
            v-if="result"
            class="inline-flex items-center rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-900/25 dark:text-emerald-300"
          >
            {{ t('admin.accounts.batchTest.successCount', { count: result.success }) }}
          </span>
          <span
            v-if="result"
            class="inline-flex items-center rounded-md bg-red-50 px-2 py-1 text-xs font-medium text-red-700 dark:bg-red-900/25 dark:text-red-300"
          >
            {{ t('admin.accounts.batchTest.failedCount', { count: result.failed }) }}
          </span>
        </div>
        <button
          class="btn btn-primary flex items-center gap-1.5"
          :disabled="!canRun"
          @click="startBatchTest"
        >
          <Icon v-if="running" name="refresh" size="sm" class="animate-spin" />
          <Icon v-else name="beaker" size="sm" />
          {{ running ? t('admin.accounts.batchTest.running') : t('admin.accounts.batchTest.start') }}
        </button>
      </div>

      <div
        v-if="!result && !running"
        class="rounded-lg border border-dashed border-gray-300 py-10 text-center dark:border-dark-600"
      >
        <Icon name="beaker" size="lg" class="mx-auto mb-2 text-gray-400" />
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.batchTest.ready') }}
        </p>
      </div>

      <div v-else-if="running" class="flex items-center justify-center py-10 text-sm text-gray-500 dark:text-gray-400">
        <Icon name="refresh" size="md" class="mr-2 animate-spin" />
        {{ t('admin.accounts.batchTest.runningWithConcurrency') }}
      </div>

      <div v-else class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
        <div class="max-h-[440px] overflow-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="sticky top-0 bg-gray-50 text-left text-xs font-medium uppercase text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2">{{ t('admin.accounts.batchTest.columns.account') }}</th>
                <th class="px-3 py-2">{{ t('admin.accounts.batchTest.columns.latency') }}</th>
                <th class="px-3 py-2">{{ t('admin.accounts.batchTest.columns.responded') }}</th>
                <th class="px-3 py-2">{{ t('admin.accounts.batchTest.columns.success') }}</th>
                <th class="px-3 py-2">{{ t('admin.accounts.batchTest.columns.message') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
              <tr v-for="item in result?.results ?? []" :key="`${item.account_id}-${item.account_name}`">
                <td class="px-3 py-2">
                  <div class="font-medium text-gray-900 dark:text-gray-100">
                    {{ item.account_name || `#${item.account_id}` }}
                  </div>
                  <div class="font-mono text-xs text-gray-400">#{{ item.account_id }}</div>
                </td>
                <td class="whitespace-nowrap px-3 py-2 font-mono text-xs text-gray-600 dark:text-gray-300">
                  {{ formatLatency(item.latency_ms) }}
                </td>
                <td class="px-3 py-2">
                  <span :class="statusClass(item.responded)">
                    <Icon :name="item.responded ? 'checkCircle' : 'xCircle'" size="xs" />
                    {{ item.responded ? t('common.yes') : t('common.no') }}
                  </span>
                </td>
                <td class="px-3 py-2">
                  <span :class="statusClass(item.success)">
                    <Icon :name="item.success ? 'checkCircle' : 'xCircle'" size="xs" />
                    {{ item.success ? t('admin.scheduledTests.success') : t('admin.scheduledTests.failed') }}
                  </span>
                </td>
                <td class="max-w-[28rem] px-3 py-2">
                  <pre
                    class="max-h-24 overflow-auto whitespace-pre-wrap break-words rounded bg-gray-50 p-2 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300"
                  >{{ displayMessage(item) }}</pre>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { BatchAccountConnectionTestItem, BatchAccountConnectionTestResult } from '@/types'

const props = defineProps<{
  show: boolean
  accountIds: number[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const running = ref(false)
const result = ref<BatchAccountConnectionTestResult | null>(null)

const canRun = computed(() => props.accountIds.length > 0 && !running.value)

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      result.value = null
      running.value = false
    }
  }
)

const startBatchTest = async () => {
  if (!canRun.value) return
  running.value = true
  result.value = null
  try {
    const data = await adminAPI.accounts.batchTestConnections([...props.accountIds])
    result.value = data
    if (data.failed > 0) {
      appStore.showError(t('admin.accounts.batchTest.partial', { success: data.success, failed: data.failed }))
    } else {
      appStore.showSuccess(t('admin.accounts.batchTest.completed', { count: data.success }))
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.accounts.batchTest.failed')))
  } finally {
    running.value = false
  }
}

const formatLatency = (latencyMs: number) => {
  if (!latencyMs || latencyMs <= 0) return '-'
  return `${latencyMs}ms`
}

const statusClass = (ok: boolean) => [
  'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium',
  ok
    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300'
    : 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300'
]

const displayMessage = (item: BatchAccountConnectionTestItem) => {
  const value = item.success ? item.response_text : item.error_message || item.response_text
  return value?.trim() || '-'
}
</script>
