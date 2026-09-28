<template>
  <fieldset :disabled="disabled" class="space-y-3">
    <legend class="mb-2 text-sm font-medium">{{ t('modelDetection.models') }} <span class="ml-2 font-normal text-gray-500">{{ modelValue.length }} / {{ limit }}</span></legend>
    <div v-if="modelValue.length" class="flex flex-wrap gap-2" data-testid="selected-models">
      <button v-for="model in modelValue" :key="model" type="button" class="inline-flex max-w-full items-center gap-2 rounded-lg bg-primary-50 px-2 py-1 text-xs text-primary-700 focus-visible:outline focus-visible:outline-2 disabled:opacity-50 dark:bg-primary-900/20 dark:text-primary-300" :aria-label="t('modelDetection.removeModel', { model })" :title="model" @click="toggle(model)"><span class="truncate font-mono">{{ model }}</span><span aria-hidden="true">×</span></button>
    </div>
    <label class="block"><span class="sr-only">{{ t('modelDetection.searchModels') }}</span><input v-model="search" type="search" class="input" :placeholder="t('modelDetection.searchModels')" data-testid="model-search" /></label>
    <div class="max-h-44 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-600">
      <label v-for="model in filtered" :key="model.id" class="flex cursor-pointer items-center gap-3 border-b border-gray-100 px-3 py-2 text-sm last:border-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700"><input type="checkbox" class="h-4 w-4 rounded" :value="model.id" :checked="modelValue.includes(model.id)" :disabled="disabled || (!modelValue.includes(model.id) && modelValue.length >= limit)" @change="toggle(model.id)" /><span class="min-w-0"><span class="block truncate font-mono" :title="model.id">{{ model.id }}</span><span v-if="model.display_name && model.display_name !== model.id" class="block truncate text-xs text-gray-500">{{ model.display_name }}</span></span></label>
      <p v-if="!filtered.length" class="px-3 py-4 text-center text-xs text-gray-500">{{ t('modelDetection.noModelMatches') }}</p>
    </div>
    <label class="block"><span class="mb-1 block text-xs text-gray-500">{{ t('modelDetection.customModel') }}</span><span class="flex gap-2"><input v-model="customModel" class="input min-w-0 flex-1" maxlength="200" data-testid="custom-model" :placeholder="t('modelDetection.customModelPlaceholder')" @keydown.enter.prevent="addCustom" @compositionstart="composing = true" @compositionend="composing = false" /><button type="button" class="btn btn-secondary shrink-0" :disabled="disabled || !customModel.trim() || modelValue.length >= limit" @click="addCustom">{{ t('modelDetection.addModel') }}</button></span></label>
    <p v-if="error" role="alert" class="text-xs text-red-600 dark:text-red-400">{{ error }}</p>
    <p class="text-xs text-gray-500">{{ t('modelDetection.modelSelectionHint', { count: limit }) }}</p>
  </fieldset>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(defineProps<{ modelValue: string[]; models: { id: string; display_name?: string }[]; disabled?: boolean; limit?: number }>(), { disabled: false, limit: 20 })
const emit = defineEmits<{ 'update:modelValue': [models: string[]] }>()
const { t } = useI18n()
const search = ref('')
const customModel = ref('')
const error = ref('')
const composing = ref(false)
const filtered = computed(() => {
  const all = new Map(props.models.map(model => [model.id, model]))
  for (const id of props.modelValue) if (!all.has(id)) all.set(id, { id, display_name: id })
  const term = search.value.trim().toLowerCase()
  return [...all.values()].filter(model => model.id.toLowerCase().includes(term) || model.display_name?.toLowerCase().includes(term))
})
function toggle(model: string) {
  if (props.disabled) return
  error.value = ''
  if (props.modelValue.includes(model)) emit('update:modelValue', props.modelValue.filter(id => id !== model))
  else if (props.modelValue.length < props.limit) emit('update:modelValue', [...props.modelValue, model])
}
function addCustom() {
  if (props.disabled || composing.value) return
  const model = customModel.value.trim()
  if (!model) return
  if (model.length > 200 || /[\r\n]/.test(model) || model.includes('\u0000')) { error.value = t('modelDetection.invalidModel'); return }
  if (!props.modelValue.includes(model)) {
    if (props.modelValue.length >= props.limit) { error.value = t('modelDetection.modelSelectionHint', { count: props.limit }); return }
    emit('update:modelValue', [...props.modelValue, model])
  }
  customModel.value = ''; error.value = ''
}
</script>
