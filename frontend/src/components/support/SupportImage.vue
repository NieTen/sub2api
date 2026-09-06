<template>
  <div class="w-36">
    <button type="button" class="flex h-28 w-full items-center justify-center overflow-hidden rounded-lg border border-gray-200 bg-gray-50 text-xs text-gray-500 focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-dark-600 dark:bg-dark-900" :aria-label="t('support.preview') + ': ' + attachment.file_name" @click="url ? preview = true : load()">
      <img v-if="url" :src="url" :alt="attachment.file_name" class="h-full w-full object-cover" />
      <span v-else class="p-2">{{ t(failed ? 'support.imageFailed' : 'support.loadingImage') }}</span>
    </button>
    <p class="mt-1 truncate text-xs text-gray-500" :title="attachment.file_name">{{ attachment.file_name }}</p>
    <BaseDialog :show="preview" :title="attachment.file_name" width="wide" @close="preview = false">
      <img v-if="url" :src="url" :alt="attachment.file_name" class="mx-auto max-h-[65vh] max-w-full object-contain" />
      <template #footer>
        <a v-if="url" :href="url" :download="attachment.file_name" class="btn btn-primary">{{ t('support.download') }}</a>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { bulkEmailAPI, supportAPI, type SupportAttachment } from '@/api/support'

const props = defineProps<{ attachment: SupportAttachment; admin: boolean; bulkId?: number }>()
const { t } = useI18n()
const url = ref('')
const failed = ref(false)
const preview = ref(false)
let generation = 0
let loading = false

async function load() {
  if (loading) return
  loading = true
  failed.value = false
  const current = generation
  try {
    const blob = props.bulkId
      ? await bulkEmailAPI.image(props.bulkId, props.attachment.id)
      : await supportAPI.attachment(props.admin, props.attachment.id)
    if (current !== generation) return
    // 仅将服务端已验证的位图类型交给浏览器预览，拒绝 HTML/SVG。
    if (!['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(blob.type)) throw new Error('Unsupported image')
    url.value = URL.createObjectURL(blob)
  } catch {
    if (current === generation) failed.value = true
  } finally {
    loading = false
  }
}

watch(() => [props.attachment.id, props.bulkId], () => {
  generation++
  if (url.value) URL.revokeObjectURL(url.value)
  url.value = ''
  loading = false
  void load()
}, { immediate: true })

onBeforeUnmount(() => {
  generation++
  if (url.value) URL.revokeObjectURL(url.value)
})
</script>
