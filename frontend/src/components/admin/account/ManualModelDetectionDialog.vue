<template>
  <BaseDialog :show="show" :title="t('modelDetection.manualTitle')" @close="close">
    <form class="space-y-4" @submit.prevent="submit">
      <p class="font-medium text-gray-900 dark:text-white">{{ account?.name }} <span class="font-mono text-xs text-gray-500">#{{ account?.id }}</span></p>
      <p class="text-sm text-gray-500">{{ t('modelDetection.manualHint') }}</p>
      <p class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">{{ t('modelDetection.costHint', { count: modelIds.length * requestsPerRun }) }}</p>
      <ModelDetectionModelPicker :key="account?.id" v-model="modelIds" :models="models" :disabled="saving" />
      <p v-if="modelsFailed" class="text-xs text-gray-500">{{ t('modelDetection.modelsUnavailable') }}</p>
      <p class="text-xs leading-5 text-gray-500">{{ t('modelDetection.automaticComparison') }}</p>
      <p class="text-xs leading-5 text-gray-500">{{ t('modelDetection.longRunningHint', { seconds: requestTimeout }) }}</p>
      <div v-if="failures.length" class="rounded-lg bg-red-50 p-3 text-sm dark:bg-red-900/20" role="alert"><p class="font-medium text-red-700 dark:text-red-300">{{ t('modelDetection.batchPartial', { success: successfulCount, failed: failures.length }) }}</p><ul class="mt-2 space-y-1 text-red-600 dark:text-red-400"><li v-for="failure in failures" :key="failure.model_id" class="break-words"><span class="font-mono">{{ failure.model_id }}</span>: {{ failure.error || failure.reason || t('modelDetection.actionFailed') }}</li></ul><p class="mt-2 text-xs text-gray-500">{{ t('modelDetection.retryFailedOnly') }}</p></div>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('modelDetection.cancel') }}</button><button type="submit" class="btn btn-primary" :disabled="saving || !modelIds.length">{{ t(saving ? 'modelDetection.running' : 'modelDetection.run') }}</button></div>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ModelDetectionModelPicker from './ModelDetectionModelPicker.vue'
import { getAvailableModels } from '@/api/admin/accounts'
import { modelDetectionAPI, type ModelDetectionBatchItem } from '@/api/admin/modelDetection'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { ClaudeModel } from '@/types'

const props = defineProps<{ show: boolean; account: { id: number; name: string } | null }>()
const emit = defineEmits<{ close: []; queued: [] }>()
const { t } = useI18n()
const modelIds = ref<string[]>([])
const models = ref<ClaudeModel[]>([])
const modelsFailed = ref(false)
const failures = ref<ModelDetectionBatchItem[]>([])
const successfulCount = ref(0)
const requestsPerRun = ref(4)
const requestTimeout = ref(600)
const saving = ref(false)
const error = ref('')
let requestId = 0

watch(() => [props.show, props.account?.id] as const, async ([show, accountId]) => {
  const current = ++requestId
  if (!show || !accountId) return
  modelIds.value = []; models.value = []; modelsFailed.value = false; error.value = ''; failures.value = []; successfulCount.value = 0
  requestsPerRun.value = 4; requestTimeout.value = 600
  const [modelResult, catalogResult] = await Promise.allSettled([getAvailableModels(accountId), modelDetectionAPI.catalog()])
  if (current !== requestId) return
  if (modelResult.status === 'fulfilled') models.value = modelResult.value ?? []
  else modelsFailed.value = true
  if (catalogResult.status === 'fulfilled') { requestsPerRun.value = catalogResult.value.requests_per_run || 4; requestTimeout.value = catalogResult.value.request_timeout_seconds || 600 }
}, { immediate: true })

function close() { if (!saving.value) emit('close') }
async function submit() {
  if (saving.value || !props.account || !modelIds.value.length) return
  saving.value = true; error.value = ''
  try {
    const result = await modelDetectionAPI.runAccountModels(props.account.id, modelIds.value)
    failures.value = result.results.filter(item => !item.run)
    successfulCount.value = result.success
    modelIds.value = failures.value.map(item => item.model_id)
    if (result.success > 0) emit('queued')
    if (!failures.value.length) emit('close')
  } catch (cause) { error.value = extractApiErrorMessage(cause, t('modelDetection.actionFailed')) }
  finally { saving.value = false }
}
</script>
