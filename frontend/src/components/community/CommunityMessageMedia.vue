<template>
  <div class="mt-3 space-y-2">
    <p class="break-all text-xs text-gray-500 dark:text-dark-400">{{ message.file_name || label }}</p>
    <template v-if="url">
      <img v-if="isImage" :src="url" :alt="message.text || label" loading="lazy" class="max-h-80 max-w-full rounded-lg object-contain" />
      <video v-else-if="isVideo" :src="url" controls preload="metadata" class="max-h-80 max-w-full rounded-lg" />
      <audio v-else-if="isAudio" :src="url" controls preload="metadata" class="max-w-full" />
      <a :href="url" :download="message.file_name || ('telegram-' + message.id)" class="inline-flex text-sm font-medium text-primary-600 underline dark:text-primary-400">{{ t('community.chat.download') }}</a>
    </template>
    <button v-else-if="message.media_available" type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">{{ t(loading ? 'common.loading' : 'community.chat.loadMedia') }}</button>
    <p v-else class="text-xs text-gray-500 dark:text-dark-400">{{ t('community.chat.mediaUnavailable') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { communityAPI, type CommunityChatMessage } from '@/api/community'
import { supportError } from '@/api/support'
const props = defineProps<{ message: CommunityChatMessage; label: string }>()
const { t } = useI18n()
const url = ref('')
const mime = ref('')
const loading = ref(false)
const error = ref('')
let disposed = false
const isImage = computed(() => /^image\/(jpeg|png|webp|gif)$/.test(mime.value))
const isVideo = computed(() => /^video\/(mp4|webm|ogg)$/.test(mime.value))
const isAudio = computed(() => /^audio\//.test(mime.value) || (mime.value === 'application/ogg' && ['voice', 'audio'].includes(props.message.message_type)))
async function load() {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    const blob = await communityAPI.media(props.message.id)
    if (!disposed) { mime.value = blob.type; url.value = URL.createObjectURL(blob) }
  } catch (cause) { if (!disposed) error.value = supportError(cause, t('community.chat.mediaFailed')) }
  finally { if (!disposed) loading.value = false }
}
onBeforeUnmount(() => { disposed = true; if (url.value) URL.revokeObjectURL(url.value) })
</script>
