<template>
  <BaseDialog :show="show" :title="t('keys.oneClickSetup.title')" width="extra-wide" panel-class="quick-setup-panel" body-class="!p-0 min-h-0 lg:!overflow-hidden" trap-focus @close="emit('close')">
    <div class="setup-shell">
      <section class="setup-keybar" data-testid="setup-connection">
        <span class="hidden rounded-xl bg-primary-100 p-2.5 text-primary-700 sm:block dark:bg-primary-900/30 dark:text-primary-300"><Icon name="key" size="md" /></span>
        <div class="min-w-0 flex-1">
          <label for="quick-setup-key" class="mb-1 block text-xs font-medium text-gray-500 dark:text-dark-300">{{ t('keys.quickSetup.currentKey') }}</label>
          <select id="quick-setup-key" :value="keyInfo?.id" class="w-full max-w-md cursor-pointer truncate rounded-md border-0 bg-transparent py-1 pl-0 pr-7 text-sm font-semibold text-gray-900 focus:ring-2 focus:ring-primary-500 dark:text-white" @change="selectKey">
            <option v-for="item in keyChoices" :key="item.id" :value="item.id">{{ item.name }} · {{ item.group?.name || t('keys.quickSetup.noGroup') }}</option>
          </select>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <code class="hidden text-xs text-gray-600 sm:block dark:text-dark-300" data-testid="setup-masked-key">{{ maskApiKey(keyInfo?.key || '') }}</code>
          <button type="button" class="setup-icon-button" :aria-label="t('keys.quickSetup.copyKey')" :title="t('keys.quickSetup.copyKey')" data-testid="copy-setup-key" :disabled="!keyInfo?.key" @click="copyValue(keyInfo?.key || '', 'key')"><Icon :name="copied === 'key' ? 'check' : 'copy'" size="md" /></button>
        </div>
      </section>

      <div class="setup-layout">
        <aside class="setup-sidebar">
          <div class="mb-3 flex items-center justify-between gap-2 px-1">
            <h4 class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-300">{{ t('keys.quickSetup.clients') }}</h4>
            <span class="text-xs tabular-nums text-gray-400 dark:text-dark-400">{{ availableClients.length }}</span>
          </div>
          <div class="grid grid-cols-2 gap-1.5 sm:grid-cols-3 lg:grid-cols-1" role="group" :aria-label="t('keys.quickSetup.clients')">
            <button v-for="item in clientCards" :key="item.id" type="button" :data-testid="`setup-client-${item.id}`" :aria-pressed="client === item.id" :disabled="!availableClients.includes(item.id)" :title="availableClients.includes(item.id) ? item.label : t('keys.quickSetup.clientUnavailable')" :class="['setup-client', { 'setup-client-active': client === item.id }]" @click="client = item.id">
              <span :class="['setup-client-icon', item.id.startsWith('claude') ? 'text-orange-700 dark:text-orange-300' : 'text-slate-700 dark:text-slate-200']"><Icon :name="item.icon" size="md" /></span>
              <span class="min-w-0 flex-1 text-left"><span class="block text-xs font-semibold sm:text-sm">{{ item.label }}</span><span class="mt-0.5 hidden text-[11px] font-normal text-gray-500 lg:block dark:text-dark-300">{{ t(`keys.quickSetup.kind.${item.kind}`) }}</span></span>
              <Icon v-if="client === item.id" name="check" size="sm" class="shrink-0 text-primary-600 dark:text-primary-300" />
            </button>
          </div>
          <p class="mt-4 hidden px-1 text-xs leading-5 text-gray-500 lg:block dark:text-dark-300">{{ t('keys.quickSetup.compatibilityHint') }}</p>
        </aside>

        <main class="setup-main">
          <div v-if="!keyInfo?.group" class="setup-notice" role="status">{{ t('keys.useKeyModal.noGroupDescription') }}</div>
          <div v-else-if="keyInfo.status !== 'active'" class="setup-notice" role="status">{{ t('keys.quickSetup.inactiveKey') }}</div>
          <section v-else-if="platform === 'typesafe'" class="space-y-4" data-testid="setup-systemone-panel">
            <h4 class="text-xl font-semibold text-gray-950 dark:text-white">System One · TypeSafe / Jev</h4>
            <p class="setup-notice" role="status">{{ t('keys.quickSetup.systemOneOnly') }}</p>
            <button type="button" class="btn btn-primary min-h-11" data-testid="setup-open-native" @click="emit('open-native')">{{ t('keys.quickSetup.openSystemOne') }}</button>
          </section>
          <template v-else>
            <header class="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div><p class="mb-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-primary-700 dark:text-primary-300">{{ t('keys.quickSetup.workspace') }}</p><h4 class="text-xl font-semibold tracking-tight text-gray-950 dark:text-white">{{ activeCard?.label }}</h4></div>
              <a v-if="guide?.links[0]" :href="guide.links[0].url" target="_blank" rel="noopener noreferrer" class="setup-link"><Icon name="book" size="sm" />{{ t('keys.quickSetup.officialDocs') }}<Icon name="externalLink" size="sm" /></a>
            </header>

            <div class="setup-modes" :style="{ gridTemplateColumns: `repeat(${modes.length}, minmax(0, 1fr))` }" role="group" :aria-label="t('keys.oneClickSetup.method')">
              <button v-for="item in modes" :key="item.id" type="button" :data-testid="`setup-mode-${item.id}`" :aria-pressed="mode === item.id" :class="['setup-mode', { 'setup-mode-active': mode === item.id }]" @click="mode = item.id"><Icon :name="item.icon" size="sm" /><span>{{ t(`keys.quickSetup.modes.${item.id}`) }}</span></button>
            </div>

            <div class="mt-4 grid gap-4 xl:grid-cols-[1fr_1.1fr]">
              <fieldset>
                <legend class="setup-label">{{ t('keys.quickSetup.system') }}</legend>
                <div class="flex flex-wrap gap-1.5">
                  <button v-for="item in systems" :key="item.id" type="button" :data-testid="`setup-os-${item.id}`" :aria-pressed="os === item.id" :class="['setup-os', { 'setup-os-active': os === item.id }]" @click="os = item.id">{{ item.label }}</button>
                </div>
              </fieldset>
              <div>
                <div class="mb-2 flex items-center justify-between"><label for="quick-setup-model" class="setup-label !mb-0">{{ t('keys.quickSetup.model') }}</label><button type="button" class="inline-flex cursor-pointer items-center gap-1 text-xs text-primary-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:opacity-50 dark:text-primary-300" :disabled="modelsLoading" @click="refreshModels"><Icon name="refresh" size="sm" :class="modelsLoading ? 'motion-safe:animate-spin' : ''" />{{ t(modelsLoading ? 'keys.quickSetup.loadingModels' : 'keys.quickSetup.refreshModels') }}</button></div>
                <input id="quick-setup-model" v-model="selectedModel" list="quick-setup-model-options" class="input h-10 font-mono text-sm" :placeholder="t('keys.quickSetup.modelPlaceholder')" :aria-invalid="!!selectedModel && !modelValid" autocomplete="off" spellcheck="false" />
                <datalist id="quick-setup-model-options"><option v-for="model in models" :key="model" :value="model" /></datalist>
              </div>
            </div>
            <p v-if="modelsError" class="mt-2 text-xs leading-5 text-amber-700 dark:text-amber-300" role="status">{{ t('keys.quickSetup.modelsUnavailable') }}</p>
            <p v-else class="mt-2 text-xs leading-5 text-gray-500 dark:text-dark-300">{{ t('keys.quickSetup.modelHint') }}</p>
            <p v-if="selectedModel && !modelValid" class="mt-2 text-xs text-red-600 dark:text-red-400" role="alert">{{ t('keys.quickSetup.invalidModel') }}</p>

            <div class="setup-endpoint">
              <Icon name="globe" size="sm" class="shrink-0 text-gray-500" /><code class="min-w-0 flex-1 break-all text-xs">{{ endpoint || t('keys.quickSetup.invalidEndpoint') }}</code>
              <button type="button" class="setup-icon-button !min-h-8 !min-w-8" :disabled="!endpoint" :aria-label="t('keys.quickSetup.copyEndpoint')" :title="t('keys.quickSetup.copyEndpoint')" data-testid="copy-setup-endpoint" @click="copyValue(endpoint, 'endpoint')"><Icon :name="copied === 'endpoint' ? 'check' : 'copy'" size="sm" /></button>
            </div>

            <section v-if="mode === 'install'" class="setup-step" data-testid="setup-install-panel">
              <div class="setup-step-heading"><span class="setup-step-number">01</span><h5>{{ t('keys.quickSetup.installClient', { client: activeCard?.label }) }}</h5><span class="ml-auto text-xs font-normal text-gray-500 dark:text-dark-300">{{ t('keys.quickSetup.skipInstalled') }}</span></div>
              <div class="space-y-3">
                <p v-for="(line, index) in guide?.prerequisites" :key="index" class="setup-description">{{ localize(line) }}</p>
                <p v-if="guide" class="setup-description">{{ localize(guide.install.description) }}</p>
                <SetupCodeBlock v-if="guide?.install.command" :label="localize(guide.install.shellLabel) || shellLabel" :content="guide.install.command" />
                <div class="flex flex-wrap gap-x-4 gap-y-2"><a v-for="link in guide?.links" :key="link.url" :href="link.url" target="_blank" rel="noopener noreferrer" class="setup-link">{{ localize(link.label) }}<Icon name="externalLink" size="sm" /></a></div>
              </div>
            </section>

            <section v-if="mode !== 'native'" class="setup-step" data-testid="setup-import-panel">
              <div class="setup-step-heading"><span v-if="mode === 'install'" class="setup-step-number">02</span><h5>{{ t('keys.quickSetup.connectClient') }}</h5></div>
              <template v-if="hideCcsImport || client === 'codex-ws'">
                <p class="setup-description">{{ t('keys.quickSetup.useNativeHint') }}</p><button type="button" class="btn btn-primary mt-3" @click="mode = 'native'">{{ t('keys.quickSetup.openNative') }}<Icon name="chevronRight" size="sm" class="ml-2" /></button>
              </template>
              <template v-else>
                <p class="setup-description">{{ t('keys.quickSetup.importDescription') }}</p>
                <div v-if="client === 'claude-desktop'" class="setup-notice mt-3" data-testid="ccs-desktop-guide">
                  <p class="font-medium">{{ t('keys.ccsClientSelect.desktopCompatibility') }}</p>
                  <ol class="mt-2 list-decimal space-y-1 pl-4"><li>{{ t('keys.ccsClientSelect.desktopStepImport') }}</li><li>{{ t('keys.ccsClientSelect.desktopStepMigrate') }}</li><li>{{ t('keys.ccsClientSelect.desktopStepRestart') }}</li></ol>
                  <p class="mt-2">{{ t('keys.ccsClientSelect.desktopExistingProviders') }}</p>
                  <a :href="CC_SWITCH_DESKTOP_GUIDE_URL" target="_blank" rel="noopener noreferrer" class="setup-link mt-2">{{ t('keys.ccsClientSelect.desktopGuide') }}<Icon name="externalLink" size="sm" /></a>
                </div>
                <p v-if="ccsTarget === 'codex'" class="mt-3 text-xs leading-5 text-amber-700 dark:text-amber-300">{{ t('keys.quickSetup.codexReasoningHint') }}</p>
                <div class="mt-3 flex flex-wrap items-center gap-3">
                  <button type="button" class="btn btn-primary inline-flex min-h-11 items-center gap-2" :disabled="!canConfigure" data-testid="setup-open-ccs" @click="launchImport"><Icon name="externalLink" size="md" />{{ t(client === 'claude-desktop' ? 'keys.quickSetup.desktopImport' : 'keys.quickSetup.importAction') }}</button>
                  <button type="button" class="setup-link min-h-11" @click="mode = 'native'">{{ t('keys.quickSetup.openNative') }}<Icon name="chevronRight" size="sm" /></button>
                </div>
                <p v-if="launchState === 'requested'" class="mt-3 text-sm text-primary-700 dark:text-primary-300" role="status">{{ t('keys.ccsClientSelect.importRequested') }}</p>
                <p v-if="launchState === 'failed'" class="mt-3 text-sm text-red-600 dark:text-red-400" role="alert">{{ t('keys.ccsClientSelect.launchFailed') }}</p>
                <p class="mt-3 text-xs text-gray-500 dark:text-dark-300">{{ t('keys.quickSetup.requiresCcs') }} <a href="https://github.com/farion1231/cc-switch/releases/tag/v3.20.4" target="_blank" rel="noopener noreferrer" class="underline underline-offset-4">{{ t('keys.oneClickSetup.downloadCcs') }}</a></p>
              </template>
            </section>

            <section v-if="mode === 'native'" class="setup-step" data-testid="setup-native-panel">
              <div class="setup-step-heading"><Icon name="document" size="md" class="text-primary-600" /><h5>{{ t('keys.quickSetup.nativeTitle') }}</h5><button type="button" class="setup-link ml-auto text-xs" :aria-pressed="!maskSecrets" @click="maskSecrets = !maskSecrets"><Icon :name="maskSecrets ? 'eye' : 'eyeOff'" size="sm" />{{ t(maskSecrets ? 'keys.quickSetup.showSecrets' : 'keys.quickSetup.hideSecrets') }}</button></div>
              <p class="setup-description mb-4">{{ t('keys.quickSetup.mergeHint') }}</p>
              <div v-if="!canConfigure" class="setup-notice">{{ t('keys.quickSetup.chooseModelFirst') }}</div>
              <template v-else-if="client === 'claude-desktop'">
                <p class="setup-description">{{ t('keys.quickSetup.desktopNativeHint') }}</p><button type="button" class="btn btn-primary mt-3" :disabled="hideCcsImport" @click="mode = 'import'">{{ t('keys.quickSetup.desktopImport') }}</button>
                <a :href="CC_SWITCH_DESKTOP_GUIDE_URL" target="_blank" rel="noopener noreferrer" class="setup-link ml-3">{{ t('keys.ccsClientSelect.desktopGuide') }}<Icon name="externalLink" size="sm" /></a>
              </template>
              <template v-else-if="customNative">
                <p v-for="(line, index) in customNative.instructions" :key="index" class="setup-description mb-2">{{ localize(line) }}</p>
                <div v-for="file in customNative.files" :key="file.path" class="mb-4 space-y-2"><SetupCodeBlock :label="file.path" :content="file.content" :secret="keyInfo?.key" :mask="maskSecrets" /><button type="button" class="setup-link text-xs" @click="downloadFile(file.path, file.content)"><Icon name="download" size="sm" />{{ t('keys.quickSetup.downloadFile') }}</button></div>
              </template>
              <UseKeyModal v-else :show="show && mode === 'native'" embedded :api-key="keyInfo?.key || ''" :base-url="baseUrl" :platform="platform" :claude-code-only="context.claudeCodeOnly" :allow-messages-dispatch="context.allowMessagesDispatch" :selected-client="nativeClient" :selected-shell="nativeShell" :selected-model="selectedModel.trim()" :mask-secrets="maskSecrets" />
              <p class="mt-4 text-xs leading-5 text-gray-500 dark:text-dark-300">{{ t('keys.quickSetup.secretHint') }}</p>
            </section>

            <section v-if="mode === 'install'" class="setup-step">
              <div class="setup-step-heading"><span class="setup-step-number">03</span><h5>{{ t('keys.quickSetup.launchVerify') }}</h5></div>
              <p v-for="(line, index) in guide?.restart" :key="index" class="setup-description mb-2">{{ localize(line) }}</p>
              <SetupCodeBlock v-if="verificationCommand" :label="shellLabel" :content="verificationCommand" />
              <p v-if="guide" class="setup-description mt-3">{{ localize(guide.verify.description) }} {{ localize(guide.launch.description) }}</p>
            </section>

            <details class="setup-details"><summary>{{ t('keys.quickSetup.advanced') }}</summary><div class="space-y-3 pb-4 pt-2">
              <label for="quick-setup-provider-name" class="setup-label">{{ t('keys.quickSetup.providerName') }}</label><input id="quick-setup-provider-name" v-model="providerName" class="input text-sm" maxlength="100" />
              <p class="setup-description">{{ t('keys.quickSetup.duplicateImportHint') }}</p>
              <div class="flex flex-wrap gap-3"><a href="https://github.com/farion1231/cc-switch/releases/tag/v3.20.4" target="_blank" rel="noopener noreferrer" class="setup-link">{{ t('keys.quickSetup.ccsVersion') }}<Icon name="externalLink" size="sm" /></a><span class="text-xs text-gray-500 dark:text-dark-300">{{ t('keys.quickSetup.architectureHint') }}</span></div>
              <p v-for="(line, index) in guide?.notices" :key="index" class="setup-description">{{ localize(line) }}</p>
            </div></details>
            <details class="setup-details"><summary>{{ t('keys.quickSetup.verifyConnection') }}</summary><div class="space-y-3 pb-4 pt-2"><p class="setup-description">{{ t('keys.quickSetup.verifyDescription') }}</p><SetupCodeBlock v-if="verificationCommand && mode !== 'install'" :label="shellLabel" :content="verificationCommand" /><a href="/usage" target="_blank" rel="noopener noreferrer" class="setup-link">{{ t('keys.quickSetup.viewUsage') }}<Icon name="externalLink" size="sm" /></a></div></details>
          </template>
        </main>
      </div>
    </div>
    <template #footer><span class="mr-auto hidden items-center gap-1.5 text-xs text-gray-500 sm:inline-flex dark:text-dark-300"><Icon name="lock" size="sm" />{{ t('keys.quickSetup.footerHint') }}</span><button type="button" class="btn btn-secondary min-h-10" @click="emit('close')">{{ t('common.close') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import type { ApiKey } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import UseKeyModal from './UseKeyModal.vue'
import SetupCodeBlock from './SetupCodeBlock.vue'
import { maskApiKey } from '@/utils/maskApiKey'
import { useClipboard } from '@/composables/useClipboard'
import { useKeySetupModels } from '@/composables/useKeySetupModels'
import { getKeySetupClientOptions, normalizeKeySetupEndpoint, normalizeKeySetupBaseUrl, resolveKeySetupCcSwitchTarget, type KeySetupClient } from '@/utils/keySetupClients'
import { getKeySetupGuide, type KeySetupOS, type KeySetupText } from '@/utils/keySetupGuides'
import { generateKeySetupNative } from '@/utils/keySetupNative'
import { buildCcSwitchImportDeeplink, CC_SWITCH_DESKTOP_GUIDE_URL, CC_SWITCH_USAGE_SCRIPT } from '@/utils/ccswitchImport'

const props = withDefaults(defineProps<{ show: boolean; keyInfo: ApiKey | null; keys: ApiKey[]; baseUrl: string; siteName?: string; hideCcsImport?: boolean }>(), { siteName: 'Sub2API', hideCcsImport: false })
const emit = defineEmits<{ (event: 'close'): void; (event: 'select-key', id: number): void; (event: 'open-native'): void }>()
const { t, locale } = useI18n()
const { copyToClipboard } = useClipboard()
type SetupMode = 'install' | 'import' | 'native'
const mode = ref<SetupMode>('install')
const client = ref<KeySetupClient>('claude')
const os = ref<KeySetupOS>(/Win/i.test(navigator.userAgent) ? 'windows' : /Mac/i.test(navigator.userAgent) ? 'macos' : 'linux')
const maskSecrets = ref(true)
const copied = ref('')
const providerName = ref('')
const launchState = ref<'idle' | 'requested' | 'failed'>('idle')
const platform = computed(() => props.keyInfo?.group?.platform || null)
const context = computed(() => ({ platform: platform.value, claudeCodeOnly: !!props.keyInfo?.group?.claude_code_only, allowMessagesDispatch: !!props.keyInfo?.group?.allow_messages_dispatch }))
const availableClients = computed(() => getKeySetupClientOptions(context.value))
const keyChoices = computed(() => props.keyInfo && !props.keys.some(key => key.id === props.keyInfo?.id) ? [props.keyInfo, ...props.keys] : props.keys)
const { models, loading: modelsLoading, error: modelsError, refresh: refreshModels, selectedModel } = useKeySetupModels({ keyId: () => props.keyInfo?.id, platform, enabled: () => props.show && props.keyInfo?.status === 'active' && platform.value !== 'typesafe' })
const clientCards = computed(() => [
  { id: 'codex-app', label: 'Codex App', icon: 'chatBubble', kind: 'desktop' },
  { id: 'claude', label: 'Claude Code', icon: 'terminal', kind: 'cli' },
  { id: 'claude-desktop', label: 'Claude Desktop', icon: 'chatBubble', kind: 'desktop' },
  { id: 'openclaw', label: 'OpenClaw', icon: 'bolt', kind: 'agent' },
  { id: 'hermes', label: 'Hermes', icon: 'cube', kind: 'agent' },
  { id: 'opencode', label: 'OpenCode', icon: 'terminal', kind: 'cli' },
  { id: 'codex', label: 'Codex CLI', icon: 'terminal', kind: 'cli' },
  ...(platform.value === 'openai' ? [{ id: 'codex-ws', label: 'Codex WebSocket', icon: 'bolt', kind: 'advanced' }] : []),
  { id: 'gemini', label: 'Gemini CLI', icon: 'sparkles', kind: 'cli' },
  { id: 'grok', label: 'Grok Build', icon: 'terminal', kind: 'cli' }
] as { id: KeySetupClient; label: string; icon: 'chatBubble' | 'terminal' | 'bolt' | 'cube' | 'sparkles'; kind: string }[])
const activeCard = computed(() => clientCards.value.find(item => item.id === client.value))
const systems: { id: KeySetupOS; label: string }[] = [{ id: 'macos', label: 'macOS' }, { id: 'windows', label: 'Windows' }, { id: 'linux', label: 'Linux / WSL' }]
const modes = computed(() => [
  { id: 'install' as const, icon: 'bolt' as const },
  ...(!props.hideCcsImport ? [{ id: 'import' as const, icon: 'externalLink' as const }] : []),
  { id: 'native' as const, icon: 'terminal' as const }
])
const guide = computed(() => getKeySetupGuide(client.value, os.value))
const shellLabel = computed(() => os.value === 'windows' ? 'PowerShell' : 'Terminal · Bash / Zsh')
const nativeClient = computed(() => client.value === 'codex-app' ? 'codex' : client.value)
const nativeShell = computed(() => os.value !== 'windows' ? 'unix' : ['codex', 'codex-app', 'codex-ws', 'grok'].includes(client.value) ? 'windows' : 'powershell')
const modelValid = computed(() => {
  const value = selectedModel.value.trim()
  const characters = Array.from(value)
  if (!value || characters.length > 256 || characters.some(character => character.charCodeAt(0) < 32 || (character.charCodeAt(0) >= 127 && character.charCodeAt(0) <= 159))) return false
  // 原生配置和导入链接各自转义内容；这里只排除无法编码的残缺字符。
  try { encodeURIComponent(value); return true } catch { return false }
})
const ccsTarget = computed(() => client.value === 'claude-desktop' ? 'claude' : resolveKeySetupCcSwitchTarget(client.value))
const endpoint = computed(() => {
  try {
    // CCS 的 OpenCode 导入器使用 OpenAI 兼容协议；原生配置保留各分组协议。
    if (mode.value !== 'native' && client.value === 'opencode') return `${normalizeKeySetupBaseUrl(props.baseUrl, platform.value)}/v1`
    return normalizeKeySetupEndpoint(props.baseUrl, platform.value, client.value)
  } catch { return '' }
})
const canConfigure = computed(() => !!props.keyInfo?.key && props.keyInfo.status === 'active' && !!platform.value && availableClients.value.includes(client.value) && !!endpoint.value && modelValid.value)
const customNative = computed(() => canConfigure.value ? generateKeySetupNative(client.value, { baseUrl: props.baseUrl, apiKey: props.keyInfo?.key || '', model: selectedModel.value.trim(), os: os.value, platform: platform.value }) : null)
const verificationCommand = computed(() => [guide.value?.verify.command, guide.value?.launch.command].filter(Boolean).join('\n'))

watch(() => [props.show, props.keyInfo?.id, platform.value, context.value.claudeCodeOnly, context.value.allowMessagesDispatch], () => {
  const preferred: KeySetupClient = context.value.claudeCodeOnly ? 'claude' : platform.value === 'openai' ? 'codex-app' : platform.value === 'gemini' ? 'gemini' : platform.value === 'grok' ? 'grok' : 'claude'
  client.value = availableClients.value.includes(preferred) ? preferred : availableClients.value[0] || 'claude'
  mode.value = 'install'
  maskSecrets.value = true
  copied.value = ''
  launchState.value = 'idle'
  providerName.value = [props.siteName, props.keyInfo?.group?.name, props.keyInfo?.name].filter(Boolean).join(' · ').slice(0, 100)
}, { immediate: true })
watch(() => [client.value, selectedModel.value, props.baseUrl, providerName.value], () => { launchState.value = 'idle'; copied.value = '' })
watch(() => props.hideCcsImport, hidden => { if (hidden && mode.value === 'import') mode.value = 'native' })

function localize(value?: KeySetupText) { return value ? (locale.value.startsWith('zh') ? value.zh : value.en) : '' }
function selectKey(event: Event) { emit('select-key', Number((event.target as HTMLSelectElement).value)) }
async function copyValue(value: string, field: string) { if (value && await copyToClipboard(value, t('keys.copied'))) copied.value = field }
function downloadFile(path: string, content: string) { saveAs(new Blob([content], { type: 'text/plain;charset=utf-8' }), path.split(/[/\\]/).pop() || 'config.txt') }
function launchImport() {
  if (!canConfigure.value || props.hideCcsImport || !ccsTarget.value) return
  try {
    const link = buildCcSwitchImportDeeplink({ baseUrl: props.baseUrl, platform: platform.value, ...(client.value === 'claude-desktop' ? { targetApp: 'claude' as const } : { client: client.value }), claudeCodeOnly: context.value.claudeCodeOnly, allowMessagesDispatch: context.value.allowMessagesDispatch, providerName: providerName.value.trim() || props.siteName, apiKey: props.keyInfo?.key || '', model: selectedModel.value.trim(), usageScript: CC_SWITCH_USAGE_SCRIPT })
    window.open(link, '_self')
    launchState.value = 'requested'
  } catch { launchState.value = 'failed' }
}
</script>

<style scoped>
.setup-shell { @apply flex min-h-0 flex-col; }
.setup-keybar { @apply flex shrink-0 items-center gap-3 border-b border-gray-200 bg-gray-50/80 px-5 py-2 dark:border-dark-700 dark:bg-dark-900/40; }
.setup-layout { @apply min-h-0 lg:grid lg:grid-cols-[256px_minmax(0,1fr)]; }
.setup-sidebar { @apply border-b border-gray-200 bg-gray-50/70 p-4 lg:overflow-y-auto lg:border-b-0 lg:border-r dark:border-dark-700 dark:bg-dark-900/30; }
.setup-client { @apply flex min-h-12 cursor-pointer items-center gap-2 rounded-lg border border-transparent px-2.5 py-2 text-gray-600 transition-colors hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent dark:text-dark-200 dark:hover:bg-dark-700; }
.setup-client-active { @apply border-primary-200 bg-white text-primary-800 shadow-sm hover:bg-white dark:border-primary-700 dark:bg-primary-950/30 dark:text-primary-200; }
.setup-client-icon { @apply hidden h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 sm:flex dark:bg-dark-700; }
.setup-main { @apply min-w-0 p-5 lg:overflow-y-auto; }
.setup-modes { @apply grid grid-cols-3 gap-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-900; }
.setup-mode { @apply flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-lg px-2 text-xs font-medium text-gray-600 transition-colors hover:text-gray-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 sm:text-sm dark:text-dark-300 dark:hover:text-white; }
.setup-mode-active { @apply bg-white text-primary-800 shadow-sm dark:bg-dark-700 dark:text-primary-200; }
.setup-label { @apply mb-2 block text-xs font-semibold text-gray-700 dark:text-dark-200; }
.setup-os { @apply min-h-10 cursor-pointer rounded-lg border border-gray-200 px-3 text-xs font-medium text-gray-600 transition-colors hover:border-primary-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-dark-600 dark:text-dark-200; }
.setup-os-active { @apply border-primary-500 bg-primary-50 text-primary-800 dark:border-primary-500 dark:bg-primary-950/30 dark:text-primary-200; }
.setup-endpoint { @apply mt-3 flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-1.5 text-gray-600 dark:bg-dark-900/50 dark:text-dark-300; }
.setup-step { @apply border-b border-gray-200 py-4 dark:border-dark-700; }
.setup-step-heading { @apply mb-3 flex flex-wrap items-center gap-2.5 text-sm font-semibold text-gray-900 dark:text-white; }
.setup-step-number { @apply font-mono text-xs font-medium text-primary-700 dark:text-primary-300; }
.setup-description { @apply text-sm leading-6 text-gray-600 dark:text-dark-300; }
.setup-link { @apply inline-flex cursor-pointer items-center gap-1.5 rounded text-sm font-medium text-primary-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-primary-300; }
.setup-icon-button { @apply inline-flex min-h-10 min-w-10 shrink-0 cursor-pointer items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-gray-200 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40 dark:text-dark-300 dark:hover:bg-dark-600 dark:hover:text-white; }
.setup-notice { @apply rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs leading-6 text-amber-900 dark:border-amber-800 dark:bg-amber-950/20 dark:text-amber-200; }
.setup-details { @apply border-b border-gray-200 text-sm dark:border-dark-700; }
.setup-details summary { @apply cursor-pointer py-4 font-medium text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-200; }
@media (min-width: 1024px) { .setup-shell { height: min(720px, calc(90dvh - 133px)); } .setup-layout { flex: 1; overflow: hidden; } }
@media (prefers-reduced-motion: reduce) { *, *::before, *::after { transition: none !important; } }
</style>
