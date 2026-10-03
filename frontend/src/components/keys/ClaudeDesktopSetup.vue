<template>
  <section class="space-y-4" data-testid="ccs-desktop-guide">
    <div class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm leading-6 text-amber-900 dark:border-amber-800 dark:bg-amber-950/20 dark:text-amber-100">
      <p class="font-medium">{{ t('keys.ccsClientSelect.desktopCompatibility') }}</p>
      <p class="mt-2 text-xs leading-5">{{ t('keys.ccsClientSelect.desktopExistingProviders') }}</p>
    </div>

    <div>
      <h5 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keys.desktopSetup.addTitle') }}</h5>
      <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ t('keys.desktopSetup.addHint') }}</p>
    </div>
    <label v-if="model === undefined" class="block text-sm text-gray-700 dark:text-dark-200">
      <span class="mb-2 block font-medium">{{ t('keys.desktopSetup.optionalModel') }}</span>
      <input v-model="manualModel" class="input font-mono text-sm" data-testid="desktop-model-input" maxlength="256" :placeholder="t('keys.quickSetup.modelPlaceholder')" autocomplete="off" spellcheck="false" />
    </label>
    <p v-if="!endpoint" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ t('keys.quickSetup.invalidEndpoint') }}</p>
    <p v-if="!validModel" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ t('keys.quickSetup.invalidModel') }}</p>

    <dl class="divide-y divide-gray-200 overflow-hidden rounded-xl border border-gray-200 dark:divide-dark-600 dark:border-dark-600">
      <div v-for="field in fields" :key="field.id" class="flex items-center gap-3 px-4 py-3" :data-testid="`desktop-field-${field.id}`">
        <div class="min-w-0 flex-1">
          <dt class="text-xs font-medium text-gray-500 dark:text-dark-300">{{ field.label }}</dt>
          <dd class="mt-1 break-all font-mono text-sm text-gray-900 dark:text-white">{{ field.id === 'key' ? maskApiKey(field.value) : field.value }}</dd>
        </div>
        <button v-if="field.copyable" type="button" class="inline-flex min-h-11 min-w-11 shrink-0 cursor-pointer items-center justify-center rounded-lg text-primary-700 transition-colors hover:bg-primary-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40 dark:text-primary-300 dark:hover:bg-primary-950/30" :data-testid="`desktop-copy-${field.id}`" :aria-label="`${t('keys.useKeyModal.copy')} ${field.label}`" :disabled="!field.value || !endpoint || !apiKey.trim() || !validModel" @click="copyField(field.id, field.value)">
          <Icon :name="copied === field.id ? 'check' : 'copy'" size="md" />
        </button>
      </div>
    </dl>
    <p class="text-xs leading-5 text-gray-500 dark:text-dark-300">{{ t('keys.desktopSetup.keyHint') }}</p>
    <p v-if="copied" role="status" class="text-xs text-primary-700 dark:text-primary-300">{{ t('keys.desktopSetup.fieldCopied') }}</p>

    <div class="rounded-xl bg-gray-50 p-4 text-sm leading-6 text-gray-700 dark:bg-dark-900/50 dark:text-dark-200" data-testid="desktop-model-guide">
      <p>{{ t(needsMapping ? 'keys.desktopSetup.mappingHint' : effectiveModel ? 'keys.desktopSetup.directModelHint' : 'keys.desktopSetup.defaultModelHint') }}</p>
      <p v-if="hasLegacyContext" class="mt-2" data-testid="desktop-context-hint">{{ t('keys.desktopSetup.legacyContextHint') }}</p>
      <p v-if="needsMapping" class="mt-2">{{ t('keys.desktopSetup.routingHint') }}</p>
    </div>
    <div>
      <h5 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keys.desktopSetup.enableTitle') }}</h5>
      <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ t('keys.ccsClientSelect.desktopStepRestart') }}</p>
    </div>
    <a :href="CC_SWITCH_DESKTOP_GUIDE_URL" target="_blank" rel="noopener noreferrer" class="inline-flex min-h-11 items-center gap-2 rounded text-sm font-medium text-primary-700 underline underline-offset-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-primary-300">
      {{ t('keys.ccsClientSelect.desktopGuide') }}<Icon name="externalLink" size="sm" />
    </a>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GroupPlatform } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { maskApiKey } from '@/utils/maskApiKey'
import { normalizeKeySetupEndpoint } from '@/utils/keySetupClients'
import { CC_SWITCH_DESKTOP_GUIDE_URL } from '@/utils/ccswitchImport'

const props = defineProps<{ apiKey: string; baseUrl: string; providerName: string; platform?: GroupPlatform | null; model?: string }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const copied = ref('')
const manualModel = ref('')
const rawModel = computed(() => (props.model ?? manualModel.value).trim())
const hasLegacyContext = computed(() => /\[1m\]$/i.test(rawModel.value))
// 旧版 Claude Code 用后缀表达上下文能力；Desktop 的模型 ID 与 1M 选项分开填写。
const effectiveModel = computed(() => rawModel.value.replace(/\[1m\]$/i, '').trim())
const validModel = computed(() => {
  const value = effectiveModel.value
  try { encodeURIComponent(value) } catch { return false }
  return (!rawModel.value || !!value) && !/\[1m\]/i.test(value) && Array.from(value).length <= 256 && !Array.from(value).some(character => character.charCodeAt(0) < 32 || (character.charCodeAt(0) >= 127 && character.charCodeAt(0) <= 159))
})
const endpoint = computed(() => {
  try { return normalizeKeySetupEndpoint(props.baseUrl, props.platform, 'claude-desktop') } catch { return '' }
})
// 对齐 CC Switch v3.20.4 的角色模型校验；旧式 Claude ID 和其他模型需要映射。
const needsMapping = computed(() => !!effectiveModel.value && !/^(?:anthropic\/)?claude-(?:sonnet|opus|haiku|fable)-.+$/i.test(effectiveModel.value))
const fields = computed(() => [
  { id: 'name', label: t('keys.desktopSetup.name'), value: props.providerName.trim() || 'sub2api', copyable: true },
  { id: 'endpoint', label: t('keys.desktopSetup.endpoint'), value: endpoint.value, copyable: true },
  { id: 'key', label: 'API Key', value: props.apiKey, copyable: true },
  { id: 'mode', label: t('keys.desktopSetup.mode'), value: t(needsMapping.value ? 'keys.desktopSetup.mapped' : 'keys.desktopSetup.direct'), copyable: false },
  ...(needsMapping.value ? [{ id: 'format', label: t('keys.desktopSetup.format'), value: 'Anthropic Messages (原生 / Native)', copyable: false }] : []),
  ...(effectiveModel.value && validModel.value ? [{ id: 'model', label: t(needsMapping.value ? 'keys.desktopSetup.mappedModel' : 'keys.desktopSetup.directModel'), value: effectiveModel.value, copyable: true }] : [])
])
watch(() => [props.apiKey, props.baseUrl, props.platform], () => { manualModel.value = ''; copied.value = '' })
watch(() => [props.providerName, effectiveModel.value], () => { copied.value = '' })
async function copyField(id: string, value: string) {
  copied.value = ''
  if (await copyToClipboard(value, t('keys.copied'))) copied.value = id
}
</script>
