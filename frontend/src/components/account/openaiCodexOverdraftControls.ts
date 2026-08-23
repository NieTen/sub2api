export const CODEX_QUOTA_OVERDRAFT_ENABLED_EXTRA_KEY = 'codex_quota_overdraft_enabled'
export const CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY = 'codex_quota_overdraft_account_types'

export type CodexQuotaOverdraftAccountType = 'oauth' | 'setup-token'

export interface OpenAICodexQuotaOverdraftControls {
  enabled: boolean
  allowOAuth: boolean
  allowSetupToken: boolean
}

const supportedAccountTypes: CodexQuotaOverdraftAccountType[] = ['oauth', 'setup-token']

const normalizeAccountType = (value: unknown): CodexQuotaOverdraftAccountType | null => {
  if (typeof value !== 'string') return null
  const normalized = value.trim().toLowerCase()
  return supportedAccountTypes.includes(normalized as CodexQuotaOverdraftAccountType)
    ? normalized as CodexQuotaOverdraftAccountType
    : null
}

const readAccountTypes = (raw: unknown): Set<CodexQuotaOverdraftAccountType> => {
  const values: unknown[] = []
  if (Array.isArray(raw)) {
    values.push(...raw)
  } else if (typeof raw === 'string') {
    values.push(...raw.split(/[,\s;|]+/))
  }
  const out = new Set<CodexQuotaOverdraftAccountType>()
  for (const value of values) {
    const normalized = normalizeAccountType(value)
    if (normalized) out.add(normalized)
  }
  return out
}

export const readOpenAICodexQuotaOverdraftControls = (
  extra?: Record<string, unknown> | null,
): OpenAICodexQuotaOverdraftControls => {
  const enabled = extra?.[CODEX_QUOTA_OVERDRAFT_ENABLED_EXTRA_KEY] !== false
  const accountTypes = readAccountTypes(extra?.[CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY])
  if (accountTypes.size === 0) {
    return { enabled, allowOAuth: true, allowSetupToken: false }
  }
  return {
    enabled,
    allowOAuth: accountTypes.has('oauth'),
    allowSetupToken: accountTypes.has('setup-token'),
  }
}

export const writeOpenAICodexQuotaOverdraftControls = (
  extra: Record<string, unknown>,
  controls: OpenAICodexQuotaOverdraftControls,
): Record<string, unknown> => {
  if (!controls.enabled) {
    extra[CODEX_QUOTA_OVERDRAFT_ENABLED_EXTRA_KEY] = false
    delete extra[CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY]
    return extra
  }
  delete extra[CODEX_QUOTA_OVERDRAFT_ENABLED_EXTRA_KEY]

  const accountTypes: CodexQuotaOverdraftAccountType[] = []
  if (controls.allowOAuth) accountTypes.push('oauth')
  if (controls.allowSetupToken) accountTypes.push('setup-token')

  // 两个都未勾选时等价于关闭账号透支，避免后端把空数组解释成默认 OAuth。
  if (controls.enabled && accountTypes.length === 0) {
    extra[CODEX_QUOTA_OVERDRAFT_ENABLED_EXTRA_KEY] = false
    delete extra[CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY]
    return extra
  }
  if (accountTypes.length === 1 && accountTypes[0] === 'oauth') {
    delete extra[CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY]
  } else if (accountTypes.length > 0) {
    extra[CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY] = accountTypes
  } else {
    delete extra[CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY]
  }
  return extra
}
