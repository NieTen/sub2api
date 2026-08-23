import { describe, expect, it } from 'vitest'
import {
  CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY,
  CODEX_QUOTA_OVERDRAFT_ENABLED_EXTRA_KEY,
  readOpenAICodexQuotaOverdraftControls,
  writeOpenAICodexQuotaOverdraftControls,
} from '../openaiCodexOverdraftControls'

describe('openaiCodexOverdraftControls', () => {
  it('未配置账号类型时默认只允许 OAuth 参与透支', () => {
    expect(readOpenAICodexQuotaOverdraftControls({})).toEqual({
      enabled: true,
      allowOAuth: true,
      allowSetupToken: false,
    })
  })

  it('显式关闭账号透支时保留 false', () => {
    const extra = writeOpenAICodexQuotaOverdraftControls({}, {
      enabled: false,
      allowOAuth: true,
      allowSetupToken: false,
    })

    expect(extra[CODEX_QUOTA_OVERDRAFT_ENABLED_EXTRA_KEY]).toBe(false)
    expect(extra[CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY]).toBeUndefined()
  })

  it('允许 SetupToken 时写入账号类型白名单', () => {
    const extra = writeOpenAICodexQuotaOverdraftControls({}, {
      enabled: true,
      allowOAuth: true,
      allowSetupToken: true,
    })

    expect(extra[CODEX_QUOTA_OVERDRAFT_ENABLED_EXTRA_KEY]).toBeUndefined()
    expect(extra[CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY]).toEqual(['oauth', 'setup-token'])
  })

  it('没有选择任何账号类型时按关闭处理', () => {
    const extra = writeOpenAICodexQuotaOverdraftControls({}, {
      enabled: true,
      allowOAuth: false,
      allowSetupToken: false,
    })

    expect(extra[CODEX_QUOTA_OVERDRAFT_ENABLED_EXTRA_KEY]).toBe(false)
    expect(extra[CODEX_QUOTA_OVERDRAFT_ACCOUNT_TYPES_EXTRA_KEY]).toBeUndefined()
  })
})
