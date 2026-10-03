<template>
  <div class="overflow-hidden rounded-xl border border-slate-700 bg-slate-950">
    <div class="flex min-h-11 items-center justify-between gap-3 border-b border-slate-700 bg-slate-900 px-4 py-2">
      <span class="min-w-0 break-all font-mono text-xs text-slate-300">{{ label }}</span>
      <button type="button" class="inline-flex min-h-8 shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-2 text-xs text-slate-200 transition-colors hover:bg-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400" :aria-label="`${t('keys.useKeyModal.copy')} ${label}`" @click="copy">
        <Icon :name="copied ? 'check' : 'copy'" size="sm" />
        {{ t(copied ? 'keys.useKeyModal.copied' : 'keys.useKeyModal.copy') }}
      </button>
    </div>
    <pre class="overflow-x-auto p-4 font-mono text-[13px] leading-6 text-slate-100"><code>{{ preview }}</code></pre>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{ label: string; content: string; secret?: string; mask?: boolean }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const copied = ref(false)
const preview = computed(() => props.mask && props.secret ? props.content.split(props.secret).join('<API_KEY>') : props.content)
watch(() => props.content, () => { copied.value = false })
async function copy() {
  copied.value = await copyToClipboard(props.content, t('keys.copied'))
}
</script>
