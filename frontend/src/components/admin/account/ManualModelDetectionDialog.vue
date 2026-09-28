<template>
  <BaseDialog :show="show" :title="t('modelDetection.manualTitle')" @close="close">
    <form class="space-y-4" @submit.prevent="submit">
      <p class="font-medium text-gray-900 dark:text-white">{{ account?.name }} <span class="font-mono text-xs text-gray-500">#{{ account?.id }}</span></p>
      <p class="text-sm text-gray-500">{{ t('modelDetection.manualHint') }}</p>
      <p class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">{{ t('modelDetection.costHint', { count: 4 }) }}</p>
      <label class="block"><span class="mb-1 block text-sm font-medium">{{ t('modelDetection.model') }}</span><input v-model="modelId" required maxlength="200" class="input" list="manual-detection-models" :disabled="saving" :placeholder="t('modelDetection.modelHint')" /><datalist id="manual-detection-models"><option v-for="model in models" :key="model.id" :value="model.id">{{ model.display_name }}</option></datalist></label>
      <p v-if="modelsFailed" class="text-xs text-gray-500">{{ t('modelDetection.modelsUnavailable') }}</p>
      <label class="block"><span class="mb-1 block text-sm font-medium">{{ t('modelDetection.reference') }}</span><select v-model="referenceModel" class="input" :disabled="saving"><option value="">{{ t('modelDetection.automaticReference') }}</option><option v-for="model in referenceModels" :key="model.id" :value="model.id">{{ model.display_name }}</option></select><span class="mt-1 block text-xs text-gray-500">{{ t('modelDetection.referenceHint') }}</span></label>
      <p v-if="catalogFailed" class="text-xs text-gray-500">{{ t('modelDetection.catalogUnavailable') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('modelDetection.cancel') }}</button><button type="submit" class="btn btn-primary" :disabled="saving || !modelId.trim()">{{ t(saving ? 'modelDetection.running' : 'modelDetection.run') }}</button></div>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { getAvailableModels } from '@/api/admin/accounts'
import { modelDetectionAPI, type ModelDetectionCatalog } from '@/api/admin/modelDetection'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { ClaudeModel } from '@/types'

const props = defineProps<{ show: boolean; account: { id: number; name: string } | null }>()
const emit = defineEmits<{ close: []; queued: [] }>()
const { t } = useI18n()
const modelId = ref('')
const models = ref<ClaudeModel[]>([])
const modelsFailed = ref(false)
const referenceModel = ref('')
const referenceModels = ref<ModelDetectionCatalog['reference_models']>([])
const catalogFailed = ref(false)
const saving = ref(false)
const error = ref('')
let requestId = 0

watch(() => [props.show, props.account?.id] as const, async ([show, accountId]) => {
  const current = ++requestId
  if (!show || !accountId) return
  modelId.value = ''; models.value = []; modelsFailed.value = false; error.value = ''
  referenceModel.value = ''; referenceModels.value = []; catalogFailed.value = false
  const [modelResult, catalogResult] = await Promise.allSettled([getAvailableModels(accountId), modelDetectionAPI.catalog()])
  if (current !== requestId) return
  if (modelResult.status === 'fulfilled') models.value = modelResult.value ?? []
  else modelsFailed.value = true
  if (catalogResult.status === 'fulfilled') referenceModels.value = catalogResult.value.reference_models ?? []
  else catalogFailed.value = true
}, { immediate: true })

function close() { if (!saving.value) emit('close') }
async function submit() {
  if (saving.value || !props.account || !modelId.value.trim()) return
  saving.value = true; error.value = ''
  try {
    await modelDetectionAPI.runAccount(props.account.id, modelId.value.trim(), referenceModel.value)
    emit('queued'); emit('close')
  } catch (cause) { error.value = extractApiErrorMessage(cause, t('modelDetection.actionFailed')) }
  finally { saving.value = false }
}
</script>
