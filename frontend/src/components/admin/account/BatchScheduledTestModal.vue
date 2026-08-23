<template>
  <BaseDialog
    :show="show"
    :title="t('admin.scheduledTests.batchCreate.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
            {{ t('admin.scheduledTests.model') }}
          </label>
          <Select
            v-model="form.model_id"
            :options="modelOptions"
            :placeholder="t('admin.scheduledTests.model')"
            :searchable="modelOptions.length > 5"
            :creatable="true"
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
            {{ t('admin.scheduledTests.cronExpression') }}
          </label>
          <Input
            v-model="form.cron_expression"
            :placeholder="'*/30 * * * *'"
            :hint="t('admin.scheduledTests.cronHelp')"
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
            {{ t('admin.scheduledTests.maxResults') }}
          </label>
          <Input v-model="form.max_results" type="number" placeholder="100" />
        </div>
        <div class="flex items-end gap-5">
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <Toggle v-model="form.enabled" />
            {{ t('admin.scheduledTests.enabled') }}
          </label>
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <Toggle v-model="form.auto_recover" />
            {{ t('admin.scheduledTests.autoRecover') }}
          </label>
        </div>
      </div>

      <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
        <span class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.scheduledTests.batchCreate.selected', { count: accountIds.length }) }}
        </span>
        <div class="flex gap-2">
          <button class="btn btn-secondary" @click="emit('close')">
            {{ t('common.cancel') }}
          </button>
          <button class="btn btn-primary flex items-center gap-1.5" :disabled="submitting || !canSubmit" @click="submit">
            <Icon v-if="submitting" name="refresh" size="sm" class="animate-spin" />
            <Icon v-else name="calendar" size="sm" />
            {{ submitting ? t('common.saving') : t('admin.scheduledTests.batchCreate.submit') }}
          </button>
        </div>
      </div>

      <div v-if="result" class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
        <div class="flex flex-wrap gap-2 border-b border-gray-100 bg-gray-50 px-3 py-2 text-xs dark:border-dark-700 dark:bg-dark-800">
          <span class="font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.scheduledTests.batchCreate.summary', { success: result.success, failed: result.failed }) }}
          </span>
        </div>
        <div class="max-h-64 overflow-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-left text-xs font-medium uppercase text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2">{{ t('admin.scheduledTests.batchCreate.columns.account') }}</th>
                <th class="px-3 py-2">{{ t('admin.scheduledTests.batchCreate.columns.status') }}</th>
                <th class="px-3 py-2">{{ t('admin.scheduledTests.batchCreate.columns.message') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
              <tr v-for="item in result.results" :key="item.account_id">
                <td class="px-3 py-2 font-mono text-xs text-gray-500">#{{ item.account_id }}</td>
                <td class="px-3 py-2">
                  <span :class="statusClass(item.success)">
                    <Icon :name="item.success ? 'checkCircle' : 'xCircle'" size="xs" />
                    {{ item.success ? t('admin.scheduledTests.success') : t('admin.scheduledTests.failed') }}
                  </span>
                </td>
                <td class="px-3 py-2 text-xs text-gray-600 dark:text-gray-300">
                  {{ item.success ? t('admin.scheduledTests.batchCreate.created') : item.error_message }}
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
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Input from '@/components/common/Input.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { BatchCreateScheduledTestPlansResult } from '@/types'

const props = defineProps<{
  show: boolean
  accountIds: number[]
  modelOptions: SelectOption[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'created', result: BatchCreateScheduledTestPlansResult): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const submitting = ref(false)
const result = ref<BatchCreateScheduledTestPlansResult | null>(null)

const form = reactive({
  model_id: '',
  cron_expression: '*/30 * * * *',
  max_results: '100',
  enabled: true,
  auto_recover: false
})

const setDefaultModel = () => {
  const first = props.modelOptions.find(option => typeof option.value === 'string' && option.value)
  if (first && !form.model_id) {
    form.model_id = String(first.value)
  }
}

const reset = () => {
  form.model_id = ''
  form.cron_expression = '*/30 * * * *'
  form.max_results = '100'
  form.enabled = true
  form.auto_recover = false
  result.value = null
  setDefaultModel()
}

watch(
  () => props.show,
  (visible) => {
    if (visible) reset()
  }
)

watch(
  () => props.modelOptions,
  () => setDefaultModel()
)

const canSubmit = computed(() => props.accountIds.length > 0 && form.model_id.trim() !== '' && form.cron_expression.trim() !== '')

const submit = async () => {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  result.value = null
  try {
    const data = await adminAPI.scheduledTests.createBatch({
      account_ids: [...props.accountIds],
      model_id: form.model_id.trim(),
      cron_expression: form.cron_expression.trim(),
      max_results: Number(form.max_results) || 100,
      enabled: form.enabled,
      auto_recover: form.auto_recover
    })
    result.value = data
    emit('created', data)
    if (data.failed > 0) {
      appStore.showError(t('admin.scheduledTests.batchCreate.partial', { success: data.success, failed: data.failed }))
    } else {
      appStore.showSuccess(t('admin.scheduledTests.batchCreate.success', { count: data.success }))
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.scheduledTests.batchCreate.failed')))
  } finally {
    submitting.value = false
  }
}

const statusClass = (ok: boolean) => [
  'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium',
  ok
    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300'
    : 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300'
]
</script>
