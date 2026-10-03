<template>
  <BaseDialog
    :show="show"
    :title="t('keys.ccsClientSelect.title')"
    width="normal"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <p class="text-sm leading-6 text-gray-600 dark:text-dark-300">
        {{ t(platform === 'typesafe' ? 'keys.quickSetup.systemOneOnly' : 'keys.ccsClientSelect.description') }}
      </p>

      <fieldset class="space-y-2">
        <legend class="sr-only">{{ t('keys.ccsClientSelect.title') }}</legend>
        <label
          v-for="option in clientOptions"
          :key="option.value"
          :data-testid="`ccs-target-${option.value}`"
          :class="[
            'flex cursor-pointer items-start gap-3 rounded-xl border p-4 transition-colors',
            selectedClient === option.value
              ? 'border-primary-500 bg-primary-50 dark:border-primary-400 dark:bg-primary-950/20'
              : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'
          ]"
        >
          <input
            v-model="selectedClient"
            type="radio"
            :name="radioGroupName"
            :value="option.value"
            class="mt-1 h-4 w-4 shrink-0 accent-primary-600 focus-visible:outline-primary-500"
          />
          <span class="min-w-0">
            <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ option.label }}</span>
            <span class="mt-1 block text-xs leading-5 text-gray-500 dark:text-dark-300">{{ option.description }}</span>
          </span>
        </label>
      </fieldset>

      <div
        v-if="isDesktop"
        data-testid="ccs-desktop-guide"
        class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm dark:border-amber-800 dark:bg-amber-950/20"
      >
        <div class="flex items-start gap-2 text-amber-900 dark:text-amber-100">
          <Icon name="exclamationCircle" size="md" class="mt-0.5 shrink-0" />
          <p class="leading-6">{{ t('keys.ccsClientSelect.desktopCompatibility') }}</p>
        </div>
        <ol class="mt-3 list-decimal space-y-2 pl-5 text-xs leading-5 text-amber-900 dark:text-amber-100">
          <li>{{ t('keys.ccsClientSelect.desktopStepImport') }}</li>
          <li>{{ t('keys.ccsClientSelect.desktopStepMigrate') }}</li>
          <li>{{ t('keys.ccsClientSelect.desktopStepRestart') }}</li>
        </ol>
        <p class="mt-3 text-xs leading-5 text-amber-800 dark:text-amber-200">
          {{ t('keys.ccsClientSelect.desktopExistingProviders') }}
        </p>
        <a
          :href="CC_SWITCH_DESKTOP_GUIDE_URL"
          target="_blank"
          rel="noopener noreferrer"
          class="mt-3 inline-flex items-center gap-1 text-xs font-medium text-primary-700 underline underline-offset-2 dark:text-primary-300"
        >
          {{ t('keys.ccsClientSelect.desktopGuide') }}
          <Icon name="externalLink" size="sm" />
        </a>
      </div>

      <div
        v-if="launchState === 'requested'"
        role="status"
        data-testid="ccs-import-requested"
        class="rounded-lg bg-primary-50 p-3 text-sm leading-6 text-primary-800 dark:bg-primary-950/20 dark:text-primary-200"
      >
        {{ t('keys.ccsClientSelect.importRequested') }}
      </div>
      <p
        v-if="launchState === 'failed'"
        role="alert"
        data-testid="ccs-import-failed"
        class="text-sm leading-6 text-red-600 dark:text-red-400"
      >
        {{ t('keys.ccsClientSelect.launchFailed') }}
      </p>
      <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">
        {{ t('keys.ccsClientSelect.openHint') }}
        <a
          href="https://github.com/farion1231/cc-switch/releases/latest"
          target="_blank"
          rel="noopener noreferrer"
          class="text-primary-600 underline underline-offset-2 dark:text-primary-400"
        >{{ t('keys.ccsClientSelect.download') }}</a>
      </p>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">
        {{ t('common.close') }}
      </button>
      <button
        type="button"
        class="btn btn-primary inline-flex items-center gap-2"
        data-testid="ccs-import-submit"
        :disabled="!canImport"
        @click="launchImport"
      >
        <Icon name="externalLink" size="sm" />
        {{ t('keys.ccsClientSelect.importAction') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script lang="ts">
let ccsImportModalCount = 0
</script>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { GroupPlatform } from '@/types'
import {
  CC_SWITCH_DESKTOP_GUIDE_URL,
  CC_SWITCH_USAGE_SCRIPT,
  buildCcSwitchImportDeeplink,
  type CcSwitchClientType
} from '@/utils/ccswitchImport'

interface Props {
  show: boolean
  apiKey: string
  baseUrl: string
  platform?: GroupPlatform | null
  providerName?: string
  claudeCodeOnly?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  platform: null,
  providerName: 'sub2api',
  claudeCodeOnly: false
})
const emit = defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()
const radioGroupName = `ccs-import-client-${++ccsImportModalCount}`
const selectedClient = ref<CcSwitchClientType>('claude')
const launchState = ref<'idle' | 'requested' | 'failed'>('idle')

const clientOptions = computed(() => {
  const option = (value: CcSwitchClientType, label: string, description: string) => ({
    value,
    label: t(`keys.ccsClientSelect.${label}`),
    description: t(`keys.ccsClientSelect.${description}`)
  })
  if (props.platform === 'typesafe') return []
  if (props.claudeCodeOnly) return [option('claude', 'claudeCode', 'claudeCodeDesc')]
  if (props.platform === 'openai') return [option('claude', 'codex', 'codexDesc')]
  if (props.platform === 'grok') return [option('claude', 'grokBuild', 'grokBuildDesc')]
  if (props.platform === 'gemini') return [option('gemini', 'geminiCli', 'geminiCliDesc')]

  const options = [
    option('claude', 'claudeCode', 'claudeCodeDesc'),
    option('claude-desktop', 'claudeDesktop', 'claudeDesktopDesc')
  ]
  if (props.platform === 'antigravity') options.push(option('gemini', 'geminiCli', 'geminiCliDesc'))
  return options
})
const isDesktop = computed(() => selectedClient.value === 'claude-desktop')
const canImport = computed(() => props.show && clientOptions.value.length > 0 && !!props.apiKey.trim() && !!props.baseUrl.trim())

// 换密钥、分组或重新打开时恢复默认，不能沿用另一个密钥的目标和发起提示。
watch(
  () => [props.show, props.apiKey, props.baseUrl, props.platform, props.claudeCodeOnly],
  () => {
    selectedClient.value = clientOptions.value[0]?.value ?? 'claude'
    launchState.value = 'idle'
  },
  { immediate: true }
)
watch(selectedClient, () => { launchState.value = 'idle' })

function launchImport() {
  if (!canImport.value) return
  try {
    // 受限分组始终走 Claude 协议，客户端自行追加 /v1/messages；保留 Antigravity 路由前缀。
    const baseUrl = props.claudeCodeOnly
      ? props.baseUrl.replace(/\/v1\/?$/, '').replace(/\/+$/, '')
      : props.baseUrl
    const platform = props.claudeCodeOnly && props.platform !== 'antigravity' ? 'anthropic' : props.platform
    const deeplink = buildCcSwitchImportDeeplink({
      baseUrl,
      platform,
      clientType: selectedClient.value,
      providerName: props.providerName.trim() || 'sub2api',
      apiKey: props.apiKey,
      usageScript: CC_SWITCH_USAGE_SCRIPT
    })
    window.open(deeplink, '_self')
    // 浏览器无法确认外部应用的处理结果，只提示已发起，不用焦点变化判断安装状态。
    launchState.value = 'requested'
  } catch {
    launchState.value = 'failed'
  }
}
</script>
