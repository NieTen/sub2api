import type { GroupPlatform } from '@/types'

export type KeySetupClient =
  | 'codex-app'
  | 'claude'
  | 'claude-desktop'
  | 'openclaw'
  | 'hermes'
  | 'opencode'
  | 'codex'
  | 'codex-ws'
  | 'gemini'
  | 'grok'

export type CcSwitchTargetApp = 'claude' | 'codex' | 'gemini' | 'grokbuild' | 'opencode' | 'openclaw' | 'hermes'

export interface KeySetupContext {
  platform?: GroupPlatform | null
  claudeCodeOnly?: boolean
  allowMessagesDispatch?: boolean
}

const CLIENT_ORDER: KeySetupClient[] = [
  'codex-app', 'claude', 'claude-desktop', 'openclaw', 'hermes',
  'opencode', 'codex', 'codex-ws', 'gemini', 'grok'
]

export function getKeySetupClientOptions(context: KeySetupContext): KeySetupClient[] {
  const { platform, claudeCodeOnly, allowMessagesDispatch } = context
  // TypeSafe 只支持 System One，旧的 Claude 限制标记不能开放其他协议。
  if (!platform || platform === 'typesafe') return []
  if (claudeCodeOnly) return ['claude']

  // 保留已有客户端范围；跨平台 Codex、OpenClaw、Hermes 使用本站的兼容网关。
  const clients = new Set<KeySetupClient>(['codex-app', 'openclaw', 'hermes', 'opencode', 'codex'])
  if (platform !== 'gemini' && (platform !== 'openai' || allowMessagesDispatch)) {
    clients.add('claude')
    clients.add('claude-desktop')
  }
  if (platform === 'openai') clients.add('codex-ws')
  if (platform === 'gemini' || platform === 'antigravity') clients.add('gemini')
  if (platform === 'grok') clients.add('grok')
  return CLIENT_ORDER.filter(client => clients.has(client))
}

export function isKeySetupClientSupported(client: KeySetupClient, context: KeySetupContext): boolean {
  return getKeySetupClientOptions(context).includes(client)
}

export function resolveKeySetupCcSwitchTarget(client: KeySetupClient): CcSwitchTargetApp | null {
  switch (client) {
    case 'claude': return 'claude'
    case 'codex':
    case 'codex-app': return 'codex'
    case 'gemini': return 'gemini'
    case 'grok': return 'grokbuild'
    case 'opencode': return 'opencode'
    case 'openclaw': return 'openclaw'
    case 'hermes': return 'hermes'
    // 当前 CCS 协议没有 Desktop 入口，也不能表达 Codex WebSocket 配置。
    case 'claude-desktop':
    case 'codex-ws': return null
    default: return null
  }
}

export function normalizeKeySetupBaseUrl(baseUrl: string, platform?: GroupPlatform | null): string {
  const value = baseUrl.trim()
  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    throw new Error('接口地址必须是完整的 HTTP 或 HTTPS 地址')
  }
  if (!['http:', 'https:'].includes(parsed.protocol) || !parsed.hostname) {
    throw new Error('接口地址必须是完整的 HTTP 或 HTTPS 地址')
  }
  const hasControlChars = Array.from(baseUrl).some(char => {
    const code = char.charCodeAt(0)
    return code <= 31 || code === 127
  })
  if (parsed.username || parsed.password || parsed.search || parsed.hash || hasControlChars) {
    throw new Error('接口地址不能包含凭据、查询参数、片段或控制字符')
  }

  // 只处理末尾协议路径，站点反代子路径保持原样；兼容已带平台前缀的地址。
  let path = parsed.pathname.replace(/\/+$/, '').replace(/\/(?:v1|v1beta)$/, '')
  if (platform === 'antigravity') {
    path = path.replace(/\/antigravity$/, '').replace(/\/(?:v1|v1beta)$/, '')
  }
  return `${parsed.origin}${path.replace(/\/+$/, '')}`
}

export function normalizeKeySetupEndpoint(
  baseUrl: string,
  platform: GroupPlatform | null | undefined,
  client: KeySetupClient
): string {
  if (platform === 'typesafe') throw new Error('TypeSafe 仅支持 System One 原生配置')
  const root = normalizeKeySetupBaseUrl(baseUrl, platform)
  const nativeRoot = platform === 'antigravity' ? `${root}/antigravity` : root
  switch (client) {
    case 'claude':
    case 'claude-desktop':
    case 'gemini':
      // Claude 与 Gemini CLI 会自行追加各自的协议版本路径。
      return nativeRoot
    case 'opencode':
      if (platform === 'gemini') return `${root}/v1beta`
      if (platform === 'antigravity') return `${nativeRoot}/v1`
      return `${root}/v1`
    case 'codex':
    case 'codex-app':
    case 'codex-ws':
    case 'openclaw':
    case 'hermes':
    case 'grok':
      // Responses / Chat Completions 走通用网关，不能追加 Antigravity 专属前缀。
      return `${root}/v1`
    default:
      throw new Error('不支持此客户端的接口配置')
  }
}
