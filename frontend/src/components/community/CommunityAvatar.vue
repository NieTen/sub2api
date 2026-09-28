<template>
  <button type="button" class="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-primary-100 text-sm font-semibold text-primary-700 ring-offset-2 transition hover:ring-2 hover:ring-primary-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:bg-primary-900/30 dark:text-primary-200" :aria-label="t('community.members.openDetail', { name: name || t('community.chat.unknownSender') })" @click="$emit('select')">
    <img v-if="url" :src="url" alt="" class="h-full w-full object-cover" @error="clearURL" />
    <span v-else aria-hidden="true">{{ name?.trim().slice(0, 1).toUpperCase() || '?' }}</span>
  </button>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { communityAPI } from '@/api/community'

const props = defineProps<{ telegramId?: number; name?: string }>()
defineEmits<{ select: [] }>()
const { t } = useI18n()
const url = ref('')
let generation = 0
function clearURL() {
  if (url.value) URL.revokeObjectURL(url.value)
  url.value = ''
}
watch(() => props.telegramId, async id => {
  const current = ++generation
  clearURL()
  if (!id || id <= 0) return
  try {
    const blob = await communityAPI.avatar(id)
    if (current === generation && /^image\/(jpeg|png|webp|gif)$/.test(blob.type)) url.value = URL.createObjectURL(blob)
  } catch { /* 无公开头像时保留文字头像，仍可打开详情。 */ }
}, { immediate: true })
onBeforeUnmount(() => { generation++; clearURL() })
</script>
