import type { AdminDataAccount, AdminDataPayload, AdminDataProxy } from '@/types'

export interface ImportSourceEntry {
  name: string
  value: unknown
}

export interface ImportParseIssue {
  source: string
  reason: string
}

export interface ImportPreviewRow {
  name: string
  source: 'CPA' | 'sub2api'
  platform: string
  type: string
  status: string
}

export interface NormalizedImportResult {
  payload: AdminDataPayload
  outputText: string
  previewRows: ImportPreviewRow[]
  issues: ImportParseIssue[]
  inputCount: number
  accountCount: number
  proxyCount: number
  skippedCount: number
}

const SUPPORTED_DATA_TYPES = new Set(['sub2api-data', 'sub2api-bundle'])
const SUPPORTED_DATA_VERSION = 1

const PROVIDER_TO_PLATFORM: Record<string, string> = {
  codex: 'openai',
  openai: 'openai',
  claude: 'anthropic',
  anthropic: 'anthropic',
  gemini: 'gemini',
  'gemini-cli': 'gemini',
  antigravity: 'antigravity',
  xai: 'grok'
}

const XAI_DEFAULT_CLIENT_ID = 'b1a00492-073a-47ea-816f-4c329264a828'
const XAI_DEFAULT_SCOPE = 'openid profile email offline_access grok-cli:access api:access'
const XAI_DEFAULT_CLI_BASE_URL = 'https://cli-chat-proxy.grok.com/v1'

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function asString(value: unknown): string {
  if (typeof value === 'string') return value
  if (typeof value === 'number' && Number.isFinite(value)) {
    return Number.isInteger(value) ? String(Math.trunc(value)) : String(value)
  }
  return ''
}

function asNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value.trim())
    if (Number.isFinite(parsed)) {
      return parsed
    }
  }
  return null
}

function asInt(value: unknown, fallback: number | null): number | null {
  const parsed = asNumber(value)
  if (parsed === null) return fallback
  return Math.trunc(parsed)
}

function boolValue(value: unknown): boolean {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number' && Number.isFinite(value)) return value !== 0
  if (typeof value === 'string') return ['1', 'true', 'yes', 'y', 'on'].includes(value.trim().toLowerCase())
  return false
}

function firstString(obj: Record<string, unknown> | null | undefined, keys: string[]): string {
  if (!obj) return ''
  for (const key of keys) {
    const value = asString(obj[key]).trim()
    if (value) return value
  }
  return ''
}

function nestedToken(obj: Record<string, unknown> | null | undefined): Record<string, unknown> {
  if (!obj) return {}
  const token = obj.token
  return isPlainObject(token) ? token : {}
}

function stripBOM(text: string): string {
  return text.replace(/^\uFEFF/, '')
}

function buildProxyKey(protocol: string, host: string, port: number, username = '', password = ''): string {
  return `${protocol.trim()}|${host.trim()}|${port}|${username.trim()}|${password.trim()}`
}

function normalizeProxyStatus(status: unknown): 'active' | 'inactive' {
  const normalized = asString(status).trim().toLowerCase()
  if (normalized === 'inactive' || normalized === 'disabled' || normalized === 'expired') {
    return 'inactive'
  }
  return 'active'
}

function unwrapDataEnvelope(value: unknown): unknown {
  if (!isPlainObject(value)) return value
  const data = value.data
  if (!isPlainObject(data)) return value

  if (
    'accounts' in value ||
    'proxies' in value ||
    'platform' in value ||
    'credentials' in value ||
    'access_token' in value ||
    'refresh_token' in value ||
    'id_token' in value ||
    'provider' in value
  ) {
    return value
  }

  return data
}

function unwrapJSONValue(value: unknown, sourceName: string): ImportSourceEntry[] {
  if (Array.isArray(value)) {
    return value.flatMap((item, index) => unwrapJSONValue(item, `${sourceName}:${index + 1}`))
  }

  const unwrapped = unwrapDataEnvelope(value)
  if (unwrapped !== value) {
    return unwrapJSONValue(unwrapped, sourceName)
  }

  return [{ name: sourceName, value }]
}

function parseJSONSequence(text: string, sourceName: string): { entries: ImportSourceEntry[]; errors: ImportParseIssue[] } {
  const entries: ImportSourceEntry[] = []
  const errors: ImportParseIssue[] = []
  let depth = 0
  let inString = false
  let escaped = false
  let start = -1

  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i]
    if (inString) {
      if (escaped) {
        escaped = false
      } else if (ch === '\\') {
        escaped = true
      } else if (ch === '"') {
        inString = false
      }
      continue
    }

    if (ch === '"') {
      inString = true
      continue
    }

    if (ch === '{' || ch === '[') {
      if (depth === 0) start = i
      depth += 1
      continue
    }

    if (ch === '}' || ch === ']') {
      depth -= 1
      if (depth < 0) {
        errors.push({ source: sourceName, reason: 'JSON 括号不匹配' })
        return { entries, errors }
      }
      if (depth === 0 && start >= 0) {
        const chunk = text.slice(start, i + 1)
        try {
          entries.push(...unwrapJSONValue(JSON.parse(chunk), `${sourceName}:${entries.length + 1}`))
        } catch (error: any) {
          errors.push({ source: sourceName, reason: error?.message || 'JSON 解析失败' })
        }
        start = -1
      }
    }
  }

  if (depth !== 0 || inString) {
    errors.push({ source: sourceName, reason: 'JSON 未闭合' })
  }

  return { entries, errors }
}

export function parseInputText(text: string, sourceName: string): { entries: ImportSourceEntry[]; errors: ImportParseIssue[] } {
  const clean = stripBOM(text || '').trim()
  if (!clean) return { entries: [], errors: [] }

  try {
    return { entries: unwrapJSONValue(JSON.parse(clean), sourceName), errors: [] }
  } catch {
    const lines = clean.split(/\r?\n/).map((line) => line.trim()).filter(Boolean)
    if (lines.length > 1) {
      const entries: ImportSourceEntry[] = []
      for (let i = 0; i < lines.length; i += 1) {
        try {
          entries.push(...unwrapJSONValue(JSON.parse(lines[i]), `${sourceName}:${i + 1}`))
        } catch (error: any) {
          return parseJSONSequence(clean, sourceName)
        }
      }
      if (entries.length > 0) {
        return { entries, errors: [] }
      }
    }
  }

  const sequence = parseJSONSequence(clean, sourceName)
  if (sequence.entries.length > 0) return sequence
  return {
    entries: [],
    errors: sequence.errors.length > 0 ? sequence.errors : [{ source: sourceName, reason: 'JSON 解析失败' }]
  }
}

function isSub2ApiPayload(value: unknown): value is Record<string, unknown> {
  if (!isPlainObject(value)) return false
  if (SUPPORTED_DATA_TYPES.has(asString(value.type))) return true
  if (Array.isArray(value.accounts) || Array.isArray(value.proxies)) return true
  return value.version !== undefined || value.exported_at !== undefined
}

function validateSub2ApiHeader(data: Record<string, unknown>): string {
  const hasCollections = data.accounts !== undefined || data.proxies !== undefined
  const hasKnownType = SUPPORTED_DATA_TYPES.has(asString(data.type))
  const requiresFullPayload =
    hasKnownType ||
    data.version !== undefined ||
    data.exported_at !== undefined ||
    (hasCollections && data.type !== undefined)

  if (!requiresFullPayload && !hasCollections) return ''

  if (data.type !== undefined && asString(data.type).trim() !== '' && !hasKnownType) {
    return `不支持的 sub2api data type: ${asString(data.type) || typeof data.type}`
  }
  if (data.version !== undefined && data.version !== 0 && data.version !== SUPPORTED_DATA_VERSION) {
    return `不支持的 sub2api data version: ${asString(data.version) || typeof data.version}`
  }
  if (data.accounts !== undefined && !Array.isArray(data.accounts)) return 'accounts 必须是数组'
  if (data.proxies !== undefined && !Array.isArray(data.proxies)) return 'proxies 必须是数组'
  if (requiresFullPayload && !Array.isArray(data.accounts)) return 'accounts is required'
  if (requiresFullPayload && !Array.isArray(data.proxies)) return 'proxies is required'
  return ''
}

function isSub2ApiAccount(value: unknown): value is Record<string, unknown> {
  if (!isPlainObject(value)) return false
  return 'platform' in value && 'credentials' in value
}

function hasCpaSignals(value: Record<string, unknown>): boolean {
  const provider = firstString(value, ['type', 'provider', 'Provider']).toLowerCase()
  if (provider && PROVIDER_TO_PLATFORM[provider]) return true
  if (firstString(value, ['access_token', 'accessToken'])) return true
  if (firstString(value, ['refresh_token', 'refreshToken'])) return true
  if (firstString(value, ['id_token', 'idToken'])) return true
  if (isPlainObject(value.token)) return true
  return false
}

function isCpaRecord(value: unknown): value is Record<string, unknown> {
  if (!isPlainObject(value)) return false
  if (isSub2ApiPayload(value) || isSub2ApiAccount(value)) return false
  return hasCpaSignals(value)
}

function addKnownCredential(target: Record<string, unknown>, source: Record<string, unknown>, key: string, outKey = key) {
  if (target[outKey] !== undefined) return
  const value = source[key]
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (trimmed) target[outKey] = trimmed
    return
  }
  if (typeof value === 'number' && Number.isFinite(value)) {
    target[outKey] = value
    return
  }
  if (typeof value === 'boolean') {
    target[outKey] = value
  }
}

function materializeCpaRecord(raw: Record<string, unknown>): Record<string, unknown> | null {
  const meta = isPlainObject(raw.metadata) ? { ...raw.metadata } : { ...raw }
  for (const key of ['id', 'label', 'status', 'disabled', 'priority']) {
    if (meta[key] === undefined && raw[key] !== undefined) meta[key] = raw[key]
  }
  if (!meta.type) {
    const provider = firstString(raw, ['provider', 'Provider'])
    if (provider) meta.type = provider
  }
  return meta
}

function extractAccessToken(meta: Record<string, unknown>): string {
  return (
    firstString(meta, ['access_token', 'accessToken']) ||
    firstString(nestedToken(meta), ['access_token', 'accessToken'])
  )
}

function extractRefreshToken(meta: Record<string, unknown>): string {
  return (
    firstString(meta, ['refresh_token', 'refreshToken']) ||
    firstString(nestedToken(meta), ['refresh_token', 'refreshToken'])
  )
}

function extractExpiresAt(meta: Record<string, unknown>): string {
  const direct = firstString(meta, ['expires_at', 'expired', 'expiry', 'expires', 'expire'])
  if (direct) return direct
  const tokenValue = firstString(nestedToken(meta), ['expires_at', 'expired', 'expiry', 'expires', 'expire'])
  if (tokenValue) return tokenValue

  const expiresIn = asInt(meta.expires_in, null)
  const timestamp = asInt(meta.timestamp, null)
  if (expiresIn !== null && timestamp !== null) {
    const seconds = timestamp > 1_000_000_000_000 ? Math.trunc(timestamp / 1000) : timestamp
    return new Date((seconds + expiresIn) * 1000).toISOString()
  }
  return '0'
}

function normalizeSub2ApiProxy(raw: unknown, sourceName: string): { proxy?: AdminDataProxy; warning?: ImportParseIssue } {
  if (!isPlainObject(raw)) {
    return { warning: { source: sourceName, reason: 'proxy 记录不是 JSON 对象' } }
  }

  const protocol = asString(raw.protocol).trim()
  const host = asString(raw.host).trim()
  const port = asInt(raw.port, null)
  if (!protocol || !host || port === null || port <= 0 || port > 65535) {
    return {
      warning: { source: sourceName, reason: 'proxy 记录缺少 protocol/host/port 或格式无效' }
    }
  }

  const status = normalizeProxyStatus(raw.status)
  const proxyKey = asString(raw.proxy_key).trim() || buildProxyKey(protocol, host, port, asString(raw.username), asString(raw.password))

  return {
    proxy: {
      proxy_key: proxyKey,
      name: asString(raw.name).trim() || 'imported-proxy',
      protocol: protocol as AdminDataProxy['protocol'],
      host,
      port,
      username: asString(raw.username).trim() || undefined,
      password: asString(raw.password).trim() || undefined,
      status
    }
  }
}

function normalizeSub2ApiAccount(raw: unknown, sourceName: string): { account?: AdminDataAccount; warning?: ImportParseIssue } {
  if (!isPlainObject(raw)) {
    return { warning: { source: sourceName, reason: 'account 记录不是 JSON 对象' } }
  }

  const platform = asString(raw.platform).trim().toLowerCase()
  const type = asString(raw.type).trim().toLowerCase()
  const credentials = isPlainObject(raw.credentials) ? { ...raw.credentials } : {}
  if (!platform) {
    return { warning: { source: sourceName, reason: 'account 缺少 platform' } }
  }
  if (!type) {
    return { warning: { source: sourceName, reason: 'account 缺少 type' } }
  }
  if (Object.keys(credentials).length === 0) {
    return { warning: { source: sourceName, reason: 'account credentials is required' } }
  }

  const name =
    firstString(raw, ['name']) ||
    firstString(credentials, ['email', 'account_id', 'chatgpt_account_id', 'sub']) ||
    `${platform}:${sourceName.replace(/\.json$/i, '')}`

  const concurrencyDefault = platform === 'grok' ? 1 : 10
  const concurrency = Math.max(0, asInt(raw.concurrency, concurrencyDefault) ?? concurrencyDefault)
  const priority = Math.max(0, asInt(raw.priority, 1) ?? 1)
  const rateMultiplier = Math.max(0, asNumber(raw.rate_multiplier) ?? 1)
  const autoPauseOnExpired = raw.auto_pause_on_expired === undefined ? true : boolValue(raw.auto_pause_on_expired)
  const account: AdminDataAccount = {
    name,
    notes: asString(raw.notes).trim() || undefined,
    platform: platform as AdminDataAccount['platform'],
    type: type as AdminDataAccount['type'],
    credentials,
    extra: isPlainObject(raw.extra) ? { ...raw.extra } : undefined,
    proxy_key: asString(raw.proxy_key).trim() || undefined,
    concurrency,
    priority,
    rate_multiplier: rateMultiplier,
    expires_at: asInt(raw.expires_at, null),
    auto_pause_on_expired: autoPauseOnExpired
  }

  return { account }
}

function normalizeCpaRecord(raw: Record<string, unknown>, sourceName: string): {
  account?: AdminDataAccount
  warning?: ImportParseIssue
} {
  const meta = materializeCpaRecord(raw)
  if (!meta) {
    return { warning: { source: sourceName, reason: '记录不是 JSON 对象' } }
  }

  const provider = firstString(meta, ['type', 'provider', 'Provider']).toLowerCase()
  if (!provider) {
    return { warning: { source: sourceName, reason: '缺少 type/provider' } }
  }

  const platform = PROVIDER_TO_PLATFORM[provider]
  if (!platform) {
    return { warning: { source: sourceName, reason: `暂不支持 provider: ${provider}` } }
  }

  if (boolValue(meta.disabled)) {
    return { warning: { source: sourceName, reason: 'disabled=true 已跳过' } }
  }

  const accessToken = extractAccessToken(meta)
  if (!accessToken) {
    return { warning: { source: sourceName, reason: '缺少 access_token' } }
  }

  const refreshToken = extractRefreshToken(meta)
  const expiresAt = extractExpiresAt(meta)
  const email = firstString(meta, ['email', 'Email'])
  const idToken = firstString(meta, ['id_token', 'idToken'])
  const projectId = firstString(meta, ['project_id', 'projectId'])
  const subject = firstString(meta, ['sub', 'subject'])
  const name = email || subject || firstString(meta, ['label', 'name']) || `${provider}:${sourceName.replace(/\.json$/i, '')}`
  const token = nestedToken(meta)

  const credentials: Record<string, unknown> = {
    access_token: accessToken,
    expires_at: expiresAt
  }

  if (refreshToken) credentials.refresh_token = refreshToken
  if (idToken) credentials.id_token = idToken
  if (email) credentials.email = email
  if ((platform === 'gemini' || platform === 'antigravity') && projectId) credentials.project_id = projectId

  for (const key of [
    'oauth_type',
    'tier_id',
    'token_type',
    'scope',
    'client_id',
    'client_secret',
    'token_uri',
    'plan_type',
    'subscription_expires_at',
    'chatgpt_account_id',
    'chatgpt_user_id',
    'organization_id',
    'chatgpt_account_is_fedramp',
    'openai_auth_mode',
    'auth_mode',
    'user_agent',
    'base_url',
    'sub',
    'team_id',
    'subscription_tier',
    'entitlement_status',
    'using_api'
  ]) {
    addKnownCredential(credentials, meta, key)
  }
  for (const key of ['client_id', 'client_secret', 'token_uri', 'scope', 'token_type']) {
    addKnownCredential(credentials, token, key)
  }

  const accountId = firstString(meta, ['account_id', 'accountId'])
  if (accountId) {
    credentials.account_id = accountId
    if (platform === 'openai' && !credentials.chatgpt_account_id) {
      credentials.chatgpt_account_id = accountId
    }
  }
  if (platform === 'gemini' && projectId && !credentials.oauth_type) {
    credentials.oauth_type = 'code_assist'
  }
  if (platform === 'antigravity' && !projectId) {
    credentials.project_id = undefined
  }
  if (platform === 'grok') {
    if (!credentials.client_id) credentials.client_id = XAI_DEFAULT_CLIENT_ID
    if (!credentials.scope) credentials.scope = XAI_DEFAULT_SCOPE
    if (!credentials.token_type) credentials.token_type = 'Bearer'
    credentials.base_url = XAI_DEFAULT_CLI_BASE_URL
    if (!refreshToken) {
      credentials.refresh_token = undefined
    }
  }

  const account: AdminDataAccount = {
    name,
    platform: platform as AdminDataAccount['platform'],
    type: 'oauth',
    credentials,
    concurrency: platform === 'grok' ? 1 : 10,
    priority: 1,
    rate_multiplier: 1,
    auto_pause_on_expired: true
  }

  if (platform === 'grok') {
    const extra: Record<string, unknown> = {}
    for (const key of ['email', 'subscription_tier', 'entitlement_status']) {
      if (credentials[key] !== undefined) {
        extra[key] = credentials[key]
      }
    }
    if (Object.keys(extra).length > 0) {
      account.extra = extra
    }
  }

  const warnings: ImportParseIssue[] = []
  if (platform === 'antigravity' && !projectId) {
    warnings.push({
      source: sourceName,
      reason: 'antigravity 缺少 project_id，导入后可能无法刷新'
    })
  }
  if (platform === 'grok' && !refreshToken) {
    warnings.push({
      source: sourceName,
      reason: 'xai 缺少 refresh_token，访问令牌过期后可能无法刷新'
    })
  }

  return {
    account,
    warning: warnings[0]
  }
}

function normalizeSub2ApiPayload(raw: Record<string, unknown>, sourceName: string): {
  accounts: AdminDataAccount[]
  proxies: AdminDataProxy[]
  previewRows: ImportPreviewRow[]
  warnings: ImportParseIssue[]
} {
  const headerError = validateSub2ApiHeader(raw)
  if (headerError) {
    return {
      accounts: [],
      proxies: [],
      previewRows: [],
      warnings: [{ source: sourceName, reason: headerError }]
    }
  }

  const accounts: AdminDataAccount[] = []
  const proxies: AdminDataProxy[] = []
  const previewRows: ImportPreviewRow[] = []
  const warnings: ImportParseIssue[] = []

  const proxyItems = Array.isArray(raw.proxies) ? raw.proxies : []
  for (let i = 0; i < proxyItems.length; i += 1) {
    const normalized = normalizeSub2ApiProxy(proxyItems[i], `${sourceName}:proxies[${i}]`)
    if (normalized.warning) {
      warnings.push(normalized.warning)
      continue
    }
    if (normalized.proxy) {
      proxies.push(normalized.proxy)
    }
  }

  const accountItems = Array.isArray(raw.accounts) ? raw.accounts : []
  for (let i = 0; i < accountItems.length; i += 1) {
    const normalized = normalizeSub2ApiAccount(accountItems[i], `${sourceName}:accounts[${i}]`)
    if (normalized.warning) {
      warnings.push(normalized.warning)
      continue
    }
    if (normalized.account) {
      accounts.push(normalized.account)
      previewRows.push({
        name: normalized.account.name,
        source: 'sub2api',
        platform: normalized.account.platform,
        type: normalized.account.type,
        status: '可导入'
      })
    }
  }

  return { accounts, proxies, previewRows, warnings }
}

export function buildImportPayload(entries: ImportSourceEntry[]): NormalizedImportResult {
  const accounts: AdminDataAccount[] = []
  const proxies: AdminDataProxy[] = []
  const previewRows: ImportPreviewRow[] = []
  const issues: ImportParseIssue[] = []
  let skippedCount = 0

  for (const entry of entries) {
    const value = unwrapDataEnvelope(entry.value)

    if (isPlainObject(value) && isSub2ApiPayload(value)) {
      const normalized = normalizeSub2ApiPayload(value, entry.name)
      accounts.push(...normalized.accounts)
      proxies.push(...normalized.proxies)
      previewRows.push(...normalized.previewRows)
      issues.push(...normalized.warnings)
      skippedCount += normalized.warnings.length
      continue
    }

    if (isSub2ApiAccount(value)) {
      const normalized = normalizeSub2ApiAccount(value, entry.name)
      if (normalized.warning) {
        issues.push(normalized.warning)
        skippedCount += 1
        continue
      }
      if (normalized.account) {
        accounts.push(normalized.account)
        previewRows.push({
          name: normalized.account.name,
          source: 'sub2api',
          platform: normalized.account.platform,
          type: normalized.account.type,
          status: '可导入'
        })
      }
      continue
    }

    if (isCpaRecord(value)) {
      const normalized = normalizeCpaRecord(value, entry.name)
      if (normalized.warning) {
        issues.push(normalized.warning)
        skippedCount += 1
        continue
      }
      if (normalized.account) {
        accounts.push(normalized.account)
        previewRows.push({
          name: normalized.account.name,
          source: 'CPA',
          platform: normalized.account.platform,
          type: normalized.account.type,
          status: '可导入'
        })
      }
      continue
    }

    issues.push({ source: entry.name, reason: '不支持的 JSON 结构' })
    skippedCount += 1
  }

  const payload: AdminDataPayload = {
    type: 'sub2api-data',
    version: 1,
    exported_at: new Date().toISOString(),
    proxies,
    accounts
  }

  return {
    payload,
    outputText: JSON.stringify(payload, null, 2),
    previewRows,
    issues,
    inputCount: entries.length,
    accountCount: accounts.length,
    proxyCount: proxies.length,
    skippedCount
  }
}

export function isValidImportInput(entries: ImportSourceEntry[]): boolean {
  return entries.length > 0
}
