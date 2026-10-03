import type { GroupPlatform } from '@/types'
import {
  isKeySetupClientSupported,
  normalizeKeySetupEndpoint,
  resolveKeySetupCcSwitchTarget,
  type CcSwitchTargetApp,
  type KeySetupClient
} from './keySetupClients'

export type { CcSwitchTargetApp } from './keySetupClients'

export const OPENAI_CC_SWITCH_CODEX_MODEL = 'gpt-5.5'
export const GROK_CC_SWITCH_MODEL = 'grok-4.5'

export type CcSwitchClientType = 'claude' | 'claude-desktop' | 'gemini'

export const CC_SWITCH_DESKTOP_GUIDE_URL = 'https://github.com/farion1231/cc-switch/blob/v3.20.4/docs/user-manual/zh/2-providers/2.6-claude-desktop.md'

export interface CcSwitchImportConfig {
  app: string
  endpoint: string
  model?: string
}

export interface CcSwitchImportDeeplinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  clientType?: CcSwitchClientType
  client?: KeySetupClient
  targetApp?: CcSwitchTargetApp
  model?: string
  claudeCodeOnly?: boolean
  allowMessagesDispatch?: boolean
  providerName: string
  apiKey: string
  usageScript: string
}

/**
 * Balance query CC Switch runs against the imported provider. CC Switch fills
 * `{{baseUrl}}` with the provider's base URL as stored — Codex and Grok imports
 * carry a trailing `/v1` (see `withV1Endpoint`), Claude ones do not, and users
 * may edit it either way afterwards — then evaluates the script, so the URL
 * strips an existing `/v1` instead of blindly appending one (`/v1/v1/usage`
 * is a 404 and CC Switch shows "query failed").
 */
export const CC_SWITCH_USAGE_SCRIPT = `({
    request: {
      url: "{{baseUrl}}".replace(/\\/+$/, "").replace(/\\/v1$/, "") + "/v1/usage",
      method: "GET",
      headers: { "Authorization": "Bearer {{apiKey}}" }
    },
    extractor: function(response) {
      const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
      const unit = response?.unit ?? response?.quota?.unit ?? "USD";
      return {
        isValid: response?.is_active ?? response?.isValid ?? true,
        remaining,
        unit
      };
    }
  })`

function withV1Endpoint(baseUrl: string): string {
  const normalizedBaseUrl = baseUrl.replace(/\/+$/, '')
  return normalizedBaseUrl.endsWith('/v1') ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`
}

function withoutTrailingSlashes(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, '')
}

export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string
): CcSwitchImportConfig {
  // CC Switch v3.20.4 尚未开放 Desktop 深链，先导入 Claude，再由应用内迁移到 Desktop。
  // 不能直接发送 app=claude-desktop，否则会在协议入口被拒绝。
  switch (platform || 'anthropic') {
    case 'antigravity':
      return {
        app: clientType === 'gemini' ? 'gemini' : 'claude',
        endpoint: normalizeKeySetupEndpoint(baseUrl, platform, clientType)
      }
    case 'openai':
      return {
        app: 'codex',
        // 保留旧入口传入的路径；新显式客户端入口统一使用 /v1。
        endpoint: withoutTrailingSlashes(baseUrl),
        model: OPENAI_CC_SWITCH_CODEX_MODEL
      }
    case 'gemini':
      return {
        app: 'gemini',
        endpoint: baseUrl
      }
    case 'grok':
      return {
        app: 'grokbuild',
        endpoint: withV1Endpoint(baseUrl),
        model: GROK_CC_SWITCH_MODEL
      }
    default:
      return {
        app: 'claude',
        endpoint: baseUrl
      }
  }
}

const CC_SWITCH_TARGET_CLIENTS: Record<CcSwitchTargetApp, KeySetupClient> = {
  claude: 'claude',
  codex: 'codex',
  gemini: 'gemini',
  grokbuild: 'grok',
  opencode: 'opencode',
  openclaw: 'openclaw',
  hermes: 'hermes'
}

function resolveExplicitCcSwitchConfig(input: CcSwitchImportDeeplinkInput): CcSwitchImportConfig {
  const client = input.client || (input.targetApp && CC_SWITCH_TARGET_CLIENTS[input.targetApp])
  if (!client || !isKeySetupClientSupported(client, input)) {
    throw new Error('当前分组不支持此客户端')
  }
  const target = resolveKeySetupCcSwitchTarget(client)
  if (!target || (input.targetApp && input.targetApp !== target)) {
    throw new Error('CC Switch 不支持此客户端的直接导入，请使用原生配置')
  }
  // 官方 CCS v3.20.4 的 OpenCode 导入固定使用 openai-compatible SDK，
  // 必须使用本站 Chat Completions 网关，而非原生 Gemini / Anthropic 路径。
  const endpointClient = target === 'opencode' ? 'hermes' : client
  return {
    app: target,
    endpoint: normalizeKeySetupEndpoint(input.baseUrl, input.platform, endpointClient),
    ...(input.model?.trim() ? { model: input.model.trim() } : {})
  }
}

export function buildCcSwitchImportDeeplink(input: CcSwitchImportDeeplinkInput): string {
  const explicitTarget = input.client !== undefined || input.targetApp !== undefined || input.claudeCodeOnly === true
  const config = explicitTarget
    ? resolveExplicitCcSwitchConfig(input.claudeCodeOnly && !input.client && !input.targetApp
      ? { ...input, client: 'claude' }
      : input)
    : resolveCcSwitchImportConfig(input.platform, input.clientType || 'claude', input.baseUrl)
  // 显式传入模型优先；空字符串表示不携带模型，旧调用不传时保留既有默认值。
  if (input.model !== undefined) config.model = input.model.trim() || undefined
  const entries: [string, string][] = [
    ['resource', 'provider'],
    ['app', config.app],
    ['name', input.providerName],
    ['homepage', input.baseUrl],
    ['endpoint', config.endpoint],
    ['apiKey', input.apiKey],
    ['configFormat', 'json'],
    ['usageEnabled', 'true'],
    ['usageScript', btoa(input.usageScript)],
    ['usageAutoInterval', '30']
  ]

  if (config.model) {
    entries.splice(2, 0, ['model', config.model])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}
