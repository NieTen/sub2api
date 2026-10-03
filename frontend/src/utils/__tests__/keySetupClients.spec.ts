import { describe, expect, it } from 'vitest'
import {
  getKeySetupClientOptions,
  isKeySetupClientSupported,
  normalizeKeySetupBaseUrl,
  normalizeKeySetupEndpoint,
  resolveKeySetupCcSwitchTarget,
  type KeySetupClient
} from '../keySetupClients'
import type { GroupPlatform } from '@/types'

const platforms: GroupPlatform[] = [
  'anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi',
  'zhipu', 'deepseek', 'minimax', 'opencode_go', 'composite'
]

describe('接入客户端限制', () => {
  it('未分组时不展示可用客户端', () => {
    expect(getKeySetupClientOptions({})).toEqual([])
    expect(getKeySetupClientOptions({ platform: null, claudeCodeOnly: true })).toEqual([])
  })

  it.each(platforms)('%s 的仅 Claude Code 限制优先于其他权限', platform => {
    expect(getKeySetupClientOptions({ platform, claudeCodeOnly: true, allowMessagesDispatch: true }))
      .toEqual(['claude'])
    expect(isKeySetupClientSupported('claude-desktop', { platform, claudeCodeOnly: true })).toBe(false)
  })

  it('OpenAI 仅在允许 Messages 分发时开放 Claude 及 Desktop', () => {
    const denied = getKeySetupClientOptions({ platform: 'openai' })
    const allowed = getKeySetupClientOptions({ platform: 'openai', allowMessagesDispatch: true })
    expect(denied).not.toContain('claude')
    expect(denied).not.toContain('claude-desktop')
    expect(allowed).toContain('claude')
    expect(allowed).toContain('claude-desktop')
    expect(denied).toEqual(expect.arrayContaining(['codex-app', 'codex', 'codex-ws', 'opencode']))
  })

  it.each(platforms)('%s 可通过兼容网关接入 OpenClaw 和 Hermes', platform => {
    expect(getKeySetupClientOptions({ platform })).toEqual(expect.arrayContaining(['openclaw', 'hermes']))
  })

  it('Gemini 原生入口与 Grok Build 只在对应分组开放', () => {
    expect(getKeySetupClientOptions({ platform: 'gemini' })).toContain('gemini')
    expect(getKeySetupClientOptions({ platform: 'antigravity' })).toContain('gemini')
    expect(getKeySetupClientOptions({ platform: 'grok' })).toContain('grok')
    expect(getKeySetupClientOptions({ platform: 'anthropic' })).not.toContain('gemini')
    expect(getKeySetupClientOptions({ platform: 'openai' })).not.toContain('grok')
  })

  it('返回独立数组，不让调用者修改全局客户端列表', () => {
    const options = getKeySetupClientOptions({ platform: 'anthropic' })
    options.length = 0
    expect(getKeySetupClientOptions({ platform: 'anthropic' })).toContain('claude')
  })
})

describe('接入端点规范化', () => {
  it.each([
    'https://api.example.com/sub',
    'https://api.example.com/sub/',
    'https://api.example.com/sub/v1///',
    'https://api.example.com/sub/v1beta/',
    'https://api.example.com/sub/antigravity',
    'https://api.example.com/sub/antigravity/v1/',
    'https://api.example.com/sub/antigravity/v1beta/',
    'https://api.example.com/sub/v1/antigravity/'
  ])('Antigravity 地址 %s 保留子路径且没有重复前缀', baseUrl => {
    expect(normalizeKeySetupBaseUrl(baseUrl, 'antigravity')).toBe('https://api.example.com/sub')
    for (const client of ['claude', 'claude-desktop', 'gemini'] as const) {
      expect(normalizeKeySetupEndpoint(baseUrl, 'antigravity', client)).toBe('https://api.example.com/sub/antigravity')
    }
    for (const client of ['codex', 'codex-app', 'openclaw', 'hermes'] as const) {
      expect(normalizeKeySetupEndpoint(baseUrl, 'antigravity', client)).toBe('https://api.example.com/sub/v1')
    }
    expect(normalizeKeySetupEndpoint(baseUrl, 'antigravity', 'opencode')).toBe('https://api.example.com/sub/antigravity/v1')
  })

  it('为各原生协议生成正确端点，不把版本路径复制两次', () => {
    const baseUrl = 'https://api.example.com/proxy/v1/'
    expect(normalizeKeySetupEndpoint(baseUrl, 'anthropic', 'claude')).toBe('https://api.example.com/proxy')
    expect(normalizeKeySetupEndpoint(baseUrl, 'gemini', 'gemini')).toBe('https://api.example.com/proxy')
    expect(normalizeKeySetupEndpoint(baseUrl, 'gemini', 'opencode')).toBe('https://api.example.com/proxy/v1beta')
    expect(normalizeKeySetupEndpoint(baseUrl, 'grok', 'grok')).toBe('https://api.example.com/proxy/v1')
    expect(normalizeKeySetupEndpoint(baseUrl, 'openai', 'codex-ws')).toBe('https://api.example.com/proxy/v1')
  })

  it('保留本地 HTTP、端口和子路径，仅 Antigravity 分组移除专属前缀', () => {
    expect(normalizeKeySetupEndpoint('http://localhost:8080/sub/v1/', 'openai', 'codex')).toBe('http://localhost:8080/sub/v1')
    expect(normalizeKeySetupBaseUrl('https://api.example.com/sub/antigravity', 'openai')).toBe('https://api.example.com/sub/antigravity')
  })

  it.each([
    '', '/relative', 'javascript:alert(1)',
    'https://private-token@example.com', 'https://example.com?key=private-token',
    'https://example.com#private-token', 'https://example.com/\nprivate-token'
  ])('拒绝无法安全拼接的地址，不在异常中回显输入', baseUrl => {
    expect(() => normalizeKeySetupEndpoint(baseUrl, 'openai', 'codex')).toThrow()
    try {
      normalizeKeySetupEndpoint(baseUrl, 'openai', 'codex')
    } catch (error) {
      expect(String(error)).not.toContain('private-token')
    }
  })
})

describe('CC Switch 协议目标', () => {
  it.each([
    ['claude', 'claude'], ['codex', 'codex'], ['codex-app', 'codex'],
    ['gemini', 'gemini'], ['grok', 'grokbuild'], ['opencode', 'opencode'],
    ['openclaw', 'openclaw'], ['hermes', 'hermes']
  ] as const)('%s 映射到已核实的官方目标 %s', (client, target) => {
    expect(resolveKeySetupCcSwitchTarget(client)).toBe(target)
  })

  it.each(['claude-desktop', 'codex-ws'] as KeySetupClient[])('%s 不伪装成直接支持的配置', client => {
    expect(resolveKeySetupCcSwitchTarget(client)).toBeNull()
  })
})
