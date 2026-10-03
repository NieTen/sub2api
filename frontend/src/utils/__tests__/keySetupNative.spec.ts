import { describe, expect, it } from 'vitest'
import { generateKeySetupNative, type KeySetupNativeOptions } from '@/utils/keySetupNative'

const options: KeySetupNativeOptions = {
  baseUrl: 'https://api.example.com/sub2api/v1/',
  apiKey: 'sk-test-private',
  model: 'test-model',
  os: 'linux',
  platform: 'openai'
}

describe('原生客户端配置片段', () => {
  it('OpenClaw 使用兼容网关并保留子路径和精确模型标识', () => {
    const result = generateKeySetupNative('openclaw', options)!
    const file = result.files[0]!
    const config = JSON.parse(file.content)
    expect(file.path).toBe('~/.openclaw/openclaw.json')
    expect(file.merge).toBe(true)
    expect(config.models.mode).toBe('merge')
    expect(config.models.providers.sub2api).toEqual({
      baseUrl: 'https://api.example.com/sub2api/v1',
      apiKey: options.apiKey,
      api: 'openai-completions',
      models: [{ id: options.model, name: options.model }]
    })
    expect(config.agents.defaults.model.primary).toBe('sub2api/test-model')
    expect(result.instructions[0]?.zh).toContain('不要直接覆盖整个文件')
  })

  it('Hermes 原生 Windows 使用实际数据目录，WSL 使用 Linux 目录', () => {
    const windows = generateKeySetupNative('hermes', { ...options, os: 'windows' })!
    const linux = generateKeySetupNative('hermes', options)!
    expect(windows.files[0]?.path).toBe('%LOCALAPPDATA%\\hermes\\config.yaml')
    expect(linux.files[0]?.path).toBe('~/.hermes/config.yaml')
    expect(windows.files[0]?.content).toBe(linux.files[0]?.content)
    expect(windows.files[0]?.content).toContain('provider: "custom"')
    expect(windows.instructions.some(item => item.en.includes('HERMES_HOME'))).toBe(true)
  })

  it.each(['openclaw', 'hermes'])('%s 不向 URL、启动命令和说明复制密钥', client => {
    const result = generateKeySetupNative(client, options)!
    expect(result.files[0]?.content).toContain(options.apiKey)
    expect(JSON.stringify({ ...result, files: undefined })).not.toContain(options.apiKey)
    expect(result.files[0]?.content).not.toContain(`https://${options.apiKey}`)
    expect(result.files[0]?.content).not.toContain(`?api_key=${options.apiKey}`)
  })

  it('动态引号、换行和 shell 语法保留为 JSON 数据，不能注入配置字段', () => {
    const apiKey = 'sk-"\\\n$(Write-Output secret)`; & malicious'
    const model = 'model"\n}, "evil": true, "x": {'
    const result = generateKeySetupNative('openclaw', { ...options, apiKey, model })!
    const config = JSON.parse(result.files[0]!.content)
    expect(config.models.providers.sub2api.apiKey).toBe(apiKey)
    expect(config.models.providers.sub2api.models[0].id).toBe(model)
    expect(config.agents.defaults.model.primary).toBe(`sub2api/${model}`)
    expect(config.evil).toBeUndefined()
    expect(result.launchCommand).toBe('openclaw onboard')
  })

  it('Hermes 的 YAML 标量转义引号与所有换行，防止注入额外 YAML 字段', () => {
    const apiKey = 'sk-"\\\n# comment\u0085\u2028\u2029$(command)'
    const model = 'yes:\n  other: false'
    const result = generateKeySetupNative('hermes', { ...options, apiKey, model })!
    const lines = result.files[0]!.content.split('\n')
    expect(lines).toHaveLength(5)
    expect(JSON.parse(lines[1]!.slice('  default: '.length))).toBe(model)
    expect(JSON.parse(lines[4]!.slice('  api_key: '.length))).toBe(apiKey)
    expect(result.files[0]!.content).not.toMatch(/[\u0085\u2028\u2029]/)
    expect(result.launchCommand).toBe('hermes')
  })

  it.each(['openclaw', 'hermes'])('%s 对 Antigravity 使用已存在的通用聊天接口', client => {
    const result = generateKeySetupNative(client, {
      ...options,
      platform: 'antigravity',
      baseUrl: 'https://api.example.com/sub2api/antigravity/v1'
    })!
    expect(result.files[0]?.content).toContain('https://api.example.com/sub2api/v1')
    expect(result.files[0]?.content).not.toContain('/antigravity')
  })

  it.each([
    '', '/relative', 'javascript:alert(1)', 'https://user:password@example.com',
    'https://example.com?token=secret', 'https://example.com#fragment'
  ])('拒绝无效或可能携带凭据的地址：%s', baseUrl => {
    expect(generateKeySetupNative('openclaw', { ...options, baseUrl })).toBeNull()
    expect(generateKeySetupNative('hermes', { ...options, baseUrl })).toBeNull()
  })

  it('缺失必要数据或不支持的客户端不生成配置', () => {
    expect(generateKeySetupNative('claude-desktop', options)).toBeNull()
    expect(generateKeySetupNative('openclaw', { ...options, apiKey: ' ' })).toBeNull()
    expect(generateKeySetupNative('hermes', { ...options, model: '' })).toBeNull()
    expect(generateKeySetupNative('hermes', { ...options, os: 'unknown' as KeySetupNativeOptions['os'] })).toBeNull()
  })
})
