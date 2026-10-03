import { describe, expect, it } from 'vitest'
import {
  CC_SWITCH_USAGE_SCRIPT,
  GROK_CC_SWITCH_MODEL,
  OPENAI_CC_SWITCH_CODEX_MODEL,
  buildCcSwitchImportDeeplink
} from '@/utils/ccswitchImport'
import type { GroupPlatform } from '@/types'
import type { CcSwitchTargetApp, KeySetupClient } from '../keySetupClients'

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
}

describe('ccswitchImport utils', () => {
  it('defaults OpenAI CC Switch imports to the current Codex model', () => {
    expect(OPENAI_CC_SWITCH_CODEX_MODEL).toBe('gpt-5.5')
  })

  it('defaults Grok Build imports to the current Grok model', () => {
    expect(GROK_CC_SWITCH_MODEL).toBe('grok-4.5')
  })

  const baseInput = {
    baseUrl: 'https://api.example.com',
    providerName: 'Sub2API',
    apiKey: 'sk-test',
    usageScript: 'return true'
  }

  it.each([
    ['https://api.example.com', 'https://api.example.com'],
    ['https://api.example.com/', 'https://api.example.com'],
    ['https://api.example.com/v1', 'https://api.example.com/v1'],
    ['https://api.example.com/v1/', 'https://api.example.com/v1']
  ])('keeps Codex imports on the configured endpoint for base URL %s', (baseUrl, endpoint) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        platform: 'openai',
        clientType: 'claude'
      })
    )

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe(endpoint)
    expect(params.get('model')).toBe(OPENAI_CC_SWITCH_CODEX_MODEL)
    expect(atob(params.get('usageScript') || '')).toBe(baseInput.usageScript)
  })

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('imports Grok Build with one /v1 suffix for base URL %s', (baseUrl) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        platform: 'grok',
        clientType: 'claude'
      })
    )

    expect(params.get('app')).toBe('grokbuild')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('model')).toBe(GROK_CC_SWITCH_MODEL)
  })

  it.each([
    { platform: 'anthropic' as GroupPlatform, clientType: 'claude' as const, app: 'claude' },
    { platform: 'gemini' as GroupPlatform, clientType: 'gemini' as const, app: 'gemini' }
  ])('does not add a model parameter for $platform imports', ({ platform, clientType, app }) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform,
        clientType
      })
    )

    expect(params.get('app')).toBe(app)
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
  })

  it('keeps Antigravity imports on the selected client endpoint without a model parameter', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'antigravity',
        clientType: 'gemini'
      })
    )

    expect(params.get('app')).toBe('gemini')
    expect(params.get('endpoint')).toBe(`${baseInput.baseUrl}/antigravity`)
    expect(params.has('model')).toBe(false)
  })

  it.each([
    ['anthropic', 'https://api.example.com'],
    ['antigravity', 'https://api.example.com/antigravity']
  ] as const)('Desktop 为 %s 生成可用的 Claude 兼容导入，不生成不受支持的目标', (platform, endpoint) => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({
      ...baseInput,
      platform,
      clientType: 'claude-desktop'
    }))
    expect(params.get('app')).toBe('claude')
    expect(params.get('endpoint')).toBe(endpoint)
    expect(params.get('apiKey')).toBe(baseInput.apiKey)
    expect(params.has('model')).toBe(false)
  })
})

describe('显式客户端 CC Switch 导入', () => {
  const baseInput = {
    baseUrl: 'https://api.example.com/sub/v1/',
    platform: 'anthropic' as GroupPlatform,
    providerName: '中文站点',
    apiKey: 'sk-local-test',
    usageScript: CC_SWITCH_USAGE_SCRIPT
  }

  it.each([
    ['claude', 'claude', 'https://api.example.com/sub'],
    ['codex-app', 'codex', 'https://api.example.com/sub/v1'],
    ['codex', 'codex', 'https://api.example.com/sub/v1'],
    ['opencode', 'opencode', 'https://api.example.com/sub/v1'],
    ['openclaw', 'openclaw', 'https://api.example.com/sub/v1'],
    ['hermes', 'hermes', 'https://api.example.com/sub/v1']
  ] as const)('%s 使用明确目标 %s、端点及调用者模型', (client, app, endpoint) => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...baseInput, client, model: 'claude-sonnet-custom' }))
    expect(params.get('app')).toBe(app)
    expect(params.get('endpoint')).toBe(endpoint)
    expect(params.get('model')).toBe('claude-sonnet-custom')
    expect(params.get('name')).toBe(baseInput.providerName)
    expect(params.get('apiKey')).toBe(baseInput.apiKey)
    expect(atob(params.get('usageScript') || '')).toBe(CC_SWITCH_USAGE_SCRIPT)
    expect(params.get('usageAutoInterval')).toBe('30')
  })

  it('指定 targetApp 可以跨平台选择 Codex，不要求旧 clientType', () => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...baseInput, platform: 'gemini', targetApp: 'codex' }))
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe('https://api.example.com/sub/v1')
    expect(params.has('model')).toBe(false)
  })

  it.each(['opencode', 'openclaw', 'hermes', 'codex'] as const)('Antigravity 的 %s 导入使用通用兼容路由', targetApp => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({
      ...baseInput, platform: 'antigravity', targetApp,
      baseUrl: 'https://api.example.com/sub/antigravity/v1/'
    }))
    expect(params.get('app')).toBe(targetApp)
    expect(params.get('endpoint')).toBe('https://api.example.com/sub/v1')
    expect(params.has('model')).toBe(false)
  })

  it.each(['claude', 'gemini'] as const)('Antigravity 的 %s 原生协议导入只携带一次前缀', targetApp => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...baseInput, platform: 'antigravity', targetApp }))
    expect(params.get('endpoint')).toBe('https://api.example.com/sub/antigravity')
  })

  it('OpenCode Gemini 导入使用 CCS 固定的 OpenAI 兼容协议，不误用原生 v1beta', () => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...baseInput, platform: 'gemini', targetApp: 'opencode' }))
    expect(params.get('endpoint')).toBe('https://api.example.com/sub/v1')
  })

  it('Grok 显式模型优先，不被旧默认值覆盖', () => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...baseInput, platform: 'grok', client: 'grok', model: 'grok-custom' }))
    expect(params.get('app')).toBe('grokbuild')
    expect(params.get('model')).toBe('grok-custom')
  })

  it('旧调用支持覆盖或删除默认模型，但不传时仍保留默认', () => {
    const input = { ...baseInput, platform: 'openai' as const, clientType: 'claude' as const }
    expect(paramsFromDeeplink(buildCcSwitchImportDeeplink(input)).get('model')).toBe(OPENAI_CC_SWITCH_CODEX_MODEL)
    expect(paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...input, model: 'chosen-model' })).get('model')).toBe('chosen-model')
    expect(paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...input, model: '' })).has('model')).toBe(false)
  })

  it('OpenAI Messages 权限同时约束显式客户端和 targetApp', () => {
    for (const target of [{ client: 'claude' as const }, { targetApp: 'claude' as const }]) {
      expect(() => buildCcSwitchImportDeeplink({ ...baseInput, platform: 'openai', ...target })).toThrow('当前分组不支持')
      const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...baseInput, platform: 'openai', ...target, allowMessagesDispatch: true }))
      expect(params.get('app')).toBe('claude')
      expect(params.get('endpoint')).toBe('https://api.example.com/sub')
    }
  })

  it.each(['openai', 'grok', 'gemini', 'antigravity'] as const)('%s 仅 Claude Code 限制不能经导入绕过', platform => {
    expect(() => buildCcSwitchImportDeeplink({ ...baseInput, platform, client: 'codex', claudeCodeOnly: true })).toThrow('当前分组不支持')
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...baseInput, platform, claudeCodeOnly: true, clientType: 'gemini' }))
    expect(params.get('app')).toBe('claude')
    expect(params.get('endpoint')).toBe(platform === 'antigravity' ? 'https://api.example.com/sub/antigravity' : 'https://api.example.com/sub')
  })

  it.each(['claude-desktop', 'codex-ws'] as KeySetupClient[])('不为 %s 生成不能表达目标配置的链接', client => {
    expect(() => buildCcSwitchImportDeeplink({ ...baseInput, platform: 'openai', allowMessagesDispatch: true, client })).toThrow('CC Switch 不支持')
  })

  it('拒绝冲突或未知目标，不静默降级为另一个客户端', () => {
    expect(() => buildCcSwitchImportDeeplink({ ...baseInput, client: 'claude', targetApp: 'codex' })).toThrow('CC Switch 不支持')
    expect(() => buildCcSwitchImportDeeplink({ ...baseInput, targetApp: 'claude-desktop' as CcSwitchTargetApp })).toThrow('当前分组不支持')
    expect(() => buildCcSwitchImportDeeplink({ ...baseInput, platform: null, client: 'codex' })).toThrow('当前分组不支持')
  })
})

describe('CC Switch usage script', () => {
  // Mirrors CC Switch: substitute the template vars as text, evaluate, read request.url.
  function usageUrlFor(baseUrl: string): string {
    const script = CC_SWITCH_USAGE_SCRIPT.split('{{baseUrl}}').join(baseUrl).split('{{apiKey}}').join('sk-test')
    // eslint-disable-next-line no-new-func
    const config = new Function(`return ${script}`)() as { request: { url: string } }
    return config.request.url
  }

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('queries exactly one /v1/usage for base URL %s', (baseUrl) => {
    expect(usageUrlFor(baseUrl)).toBe('https://api.example.com/v1/usage')
  })

  it('works against the endpoint every platform import stores', () => {
    for (const platform of ['anthropic', 'openai', 'grok', 'gemini'] as GroupPlatform[]) {
      const endpoint = paramsFromDeeplink(
        buildCcSwitchImportDeeplink({
          baseUrl: 'https://api.example.com',
          platform,
          clientType: platform === 'gemini' ? 'gemini' : 'claude',
          providerName: 'Sub2API',
          apiKey: 'sk-test',
          usageScript: CC_SWITCH_USAGE_SCRIPT
        })
      ).get('endpoint') as string
      expect(usageUrlFor(endpoint)).toBe('https://api.example.com/v1/usage')
    }
  })
})
