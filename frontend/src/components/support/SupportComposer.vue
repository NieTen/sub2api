<template>
  <div class="space-y-3" @paste="pasteImages">
    <label class="block">
      <span class="mb-1 block text-sm font-medium">{{ label || t('support.content') }}</span>
      <textarea :value="modelValue" :disabled="disabled" :maxlength="20000" :placeholder="t('support.contentPlaceholder')" class="input min-h-32 resize-y" rows="5" @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"></textarea>
    </label>
    <div v-if="attachments.length" class="flex flex-wrap gap-3">
      <div v-for="attachment in attachments" :key="attachment.id" class="relative">
        <SupportImage :attachment="attachment" :admin="admin" />
        <button type="button" :disabled="disabled || busy" class="absolute -right-2 -top-2 flex h-6 w-6 items-center justify-center rounded-full border border-gray-200 bg-white text-gray-600 shadow-sm disabled:opacity-50 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200" :aria-label="t('support.removeImage') + ': ' + attachment.file_name" @click="removeImage(attachment.id)">×</button>
      </div>
    </div>
    <div class="flex flex-wrap items-center gap-3">
      <button type="button" class="btn btn-secondary btn-sm" :disabled="disabled || busy || attachments.length >= 4" @click="fileInput?.click()">{{ busy ? t('common.loading') : t('support.addImages') }}</button>
      <span class="text-xs text-gray-500">{{ modelValue.length }} / 20000</span>
      <input ref="fileInput" type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple hidden @change="filesChanged" />
    </div>
    <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('support.imageHint') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import SupportImage from './SupportImage.vue'
import { supportAPI, supportError, type SupportAttachment } from '@/api/support'

const props = withDefaults(defineProps<{ modelValue: string; attachments: SupportAttachment[]; admin?: boolean; disabled?: boolean; label?: string }>(), { admin: false, disabled: false })
const emit = defineEmits<{ 'update:modelValue': [value: string]; 'update:attachments': [value: SupportAttachment[]]; busy: [value: boolean] }>()
const { t } = useI18n()
const fileInput = ref<HTMLInputElement>()
const busy = ref(false)
const error = ref('')
let disposed = false

async function upload(files: File[]) {
  if (props.disabled || busy.value || files.length === 0) return
  error.value = ''
  if (files.length + props.attachments.length > 4) {
    error.value = t('support.tooManyImages')
    return
  }
  if (files.some(file => !['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(file.type) || file.size > 5 * 1024 * 1024 || file.size === 0)) {
    error.value = t('support.invalidImage')
    return
  }
  busy.value = true
  emit('busy', true)
  const attachments = [...props.attachments]
  try {
    for (const file of files) {
      const attachment = await supportAPI.upload(props.admin, file)
      if (disposed) return
      attachments.push(attachment)
      emit('update:attachments', [...attachments])
    }
  } catch (cause) {
    error.value = supportError(cause, t('support.uploadFailed'))
  } finally {
    busy.value = false
    emit('busy', false)
  }
}

function filesChanged(event: Event) {
  const input = event.target as HTMLInputElement
  void upload(Array.from(input.files || []))
  input.value = ''
}

function pasteImages(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.items || [])
    .filter(item => item.kind === 'file' && item.type.startsWith('image/'))
    .map(item => item.getAsFile()).filter((file): file is File => file !== null)
  if (files.length) {
    event.preventDefault()
    void upload(files)
  }
}

async function removeImage(id: number) {
  if (props.disabled || busy.value) return
  busy.value = true
  emit('busy', true)
  error.value = ''
  try {
    await supportAPI.deleteAttachment(props.admin, id)
    emit('update:attachments', props.attachments.filter(attachment => attachment.id !== id))
  } catch (cause) {
    error.value = supportError(cause, t('support.saveFailed'))
  } finally {
    busy.value = false
    emit('busy', false)
  }
}

onBeforeUnmount(() => { disposed = true })
</script>
