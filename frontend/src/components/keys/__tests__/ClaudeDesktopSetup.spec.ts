import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import ClaudeDesktopSetup from '../ClaudeDesktopSetup.vue'

const { copyMock } = vi.hoisted(() => ({ copyMock: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: copyMock }) }))

type Props = InstanceType<typeof ClaudeDesktopSetup>['$props']
const wrappers: VueWrapper[] = []
const defaultProps = {
  apiKey: 'sk-desktop-test-secret-123456',
  baseUrl: 'https://api.example.test/proxy/v1/',
  providerName: '测试站点 · Desktop',
  platform: 'anthropic' as const
}

function mountSetup(props: Partial<Props> = {}) {
  const wrapper = mount(ClaudeDesktopSetup, {
    props: { ...defaultProps, ...props },
    global: { stubs: { Icon: { props: ['name'], template: '<span :data-icon="name" />' } } }
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('Claude Desktop 手动配置向导', () => {
  beforeEach(() => {
    copyMock.mockReset().mockResolvedValue(true)
    vi.spyOn(window, 'open').mockReturnValue(null)
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    vi.restoreAllMocks()
  })

  it('展示可添加的字段，默认隐藏密钥，复制取得原文而不打开外部应用', async () => {
    const wrapper = mountSetup({ model: 'claude-sonnet-4-6' })
    expect(wrapper.text()).toContain('keys.desktopSetup.addHint')
    expect(wrapper.html()).not.toContain(defaultProps.apiKey)
    for (const [field, value] of [
      ['name', defaultProps.providerName],
      ['endpoint', 'https://api.example.test/proxy'],
      ['key', defaultProps.apiKey],
      ['model', 'claude-sonnet-4-6']
    ]) {
      await wrapper.get(`[data-testid="desktop-copy-${field}"]`).trigger('click')
      expect(copyMock).toHaveBeenLastCalledWith(value, 'keys.copied')
      expect(wrapper.text()).toContain('keys.desktopSetup.fieldCopied')
      expect(wrapper.html()).not.toContain(defaultProps.apiKey)
    }
    expect(wrapper.find('[data-testid="desktop-copy-mode"]').exists()).toBe(false)
    expect(wrapper.get('a').attributes('href')).toContain('cc-switch/blob/v3.20.4/')
    expect(window.open).not.toHaveBeenCalled()
  })

  it.each([
    'https://api.example.test/proxy/',
    'https://api.example.test/proxy/antigravity/v1/'
  ])('Antigravity 地址 %s 只保留一份平台前缀和反代子路径', async baseUrl => {
    const wrapper = mountSetup({ platform: 'antigravity', baseUrl })
    expect(wrapper.get('[data-testid="desktop-field-endpoint"]').text()).toContain('https://api.example.test/proxy/antigravity')
    await wrapper.get('[data-testid="desktop-copy-endpoint"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith('https://api.example.test/proxy/antigravity', 'keys.copied')
  })

  it.each(['claude-sonnet-4-6', 'anthropic/claude-opus-4-6', 'CLAUDE-HAIKU-4-5', 'claude-fable-preview'])('角色模型 %s 使用直连配置', model => {
    const wrapper = mountSetup({ model })
    expect(wrapper.get('[data-testid="desktop-field-mode"]').text()).toContain('keys.desktopSetup.direct')
    expect(wrapper.get('[data-testid="desktop-model-guide"]').text()).toContain('keys.desktopSetup.directModelHint')
    expect(wrapper.get('[data-testid="desktop-field-model"]').text()).toContain(model)
    expect(wrapper.find('[data-testid="desktop-field-format"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('keys.desktopSetup.routingHint')
  })

  it.each(['gpt-6.1-sol', 'claude-3-7-sonnet-latest'])('模型 %s 提供映射与本地路由步骤', async model => {
    const wrapper = mountSetup({ model })
    expect(wrapper.get('[data-testid="desktop-field-mode"]').text()).toContain('keys.desktopSetup.mapped')
    expect(wrapper.get('[data-testid="desktop-field-format"]').text()).toContain('Anthropic Messages')
    expect(wrapper.get('[data-testid="desktop-model-guide"]').text()).toContain('keys.desktopSetup.mappingHint')
    expect(wrapper.get('[data-testid="desktop-model-guide"]').text()).toContain('keys.desktopSetup.routingHint')
    await wrapper.get('[data-testid="desktop-copy-model"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith(model, 'keys.copied')
  })

  it.each([
    ['claude-sonnet-4-6[1m]', 'claude-sonnet-4-6'],
    ['anthropic/claude-opus-4-6[1M]', 'anthropic/claude-opus-4-6']
  ])('角色模型的上下文标记 %s 转为独立提示，复制基础模型用于直连', async (model, baseModel) => {
    const wrapper = mountSetup({ model })
    expect(wrapper.get('[data-testid="desktop-field-mode"]').text()).toContain('keys.desktopSetup.direct')
    expect(wrapper.get('[data-testid="desktop-model-guide"]').text()).toContain('keys.desktopSetup.directModelHint')
    expect(wrapper.text()).toContain('keys.desktopSetup.legacyContextHint')
    expect(wrapper.find('[data-testid="desktop-field-format"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="desktop-field-model"]').text()).toContain(baseModel)
    expect(wrapper.get('[data-testid="desktop-field-model"]').text()).not.toContain(model)
    await wrapper.get('[data-testid="desktop-copy-model"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith(baseModel, 'keys.copied')
  })

  it('非角色模型去除末尾上下文标记后仍提供映射，复制值不携带标记', async () => {
    const wrapper = mountSetup({ model: 'gpt-6.1-sol[1M]' })
    expect(wrapper.get('[data-testid="desktop-field-mode"]').text()).toContain('keys.desktopSetup.mapped')
    expect(wrapper.get('[data-testid="desktop-model-guide"]').text()).toContain('keys.desktopSetup.mappingHint')
    expect(wrapper.text()).toContain('keys.desktopSetup.legacyContextHint')
    await wrapper.get('[data-testid="desktop-copy-model"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith('gpt-6.1-sol', 'keys.copied')
    expect(wrapper.get('[data-testid="desktop-field-model"]').text()).not.toContain('[1M]')
  })

  it('未选模型时保留默认角色模型，可选输入在填写后复制精确值', async () => {
    const wrapper = mountSetup()
    expect(wrapper.get('[data-testid="desktop-model-guide"]').text()).toContain('keys.desktopSetup.defaultModelHint')
    expect(wrapper.find('[data-testid="desktop-field-model"]').exists()).toBe(false)
    await wrapper.get('[data-testid="desktop-model-input"]').setValue('  vendor/model-selected  ')
    await wrapper.get('[data-testid="desktop-copy-model"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith('vendor/model-selected', 'keys.copied')
    expect(wrapper.get('[data-testid="desktop-model-guide"]').text()).toContain('keys.desktopSetup.mappingHint')
  })

  it.each([
    '/relative',
    'javascript:alert(1)',
    'https://hidden:password@example.test/proxy',
    'https://example.test/proxy?token=private',
    'https://example.test/proxy#private',
    'https://example.test/proxy\n'
  ])('非法地址 %s 不显示私密地址内容且禁止复制配置', async baseUrl => {
    const wrapper = mountSetup({ baseUrl, model: 'claude-sonnet-4-6' })
    expect(wrapper.text()).toContain('keys.quickSetup.invalidEndpoint')
    expect(wrapper.html()).not.toContain(baseUrl)
    for (const button of wrapper.findAll('[data-testid^="desktop-copy-"]')) {
      expect(button.attributes('disabled')).toBeDefined()
      await button.trigger('click')
    }
    expect(copyMock).not.toHaveBeenCalled()
    expect(wrapper.html()).not.toContain(defaultProps.apiKey)
  })

  it.each(['bad\nmodel', 'bad\u0085model', 'x'.repeat(257), 'claude-sonnet-[1m]-4-6'])('非法模型不生成可复制配置：%s', async model => {
    const wrapper = mountSetup({ model })
    expect(wrapper.text()).toContain('keys.quickSetup.invalidModel')
    expect(wrapper.find('[data-testid="desktop-copy-model"]').exists()).toBe(false)
    for (const button of wrapper.findAll('[data-testid^="desktop-copy-"]')) {
      expect(button.attributes('disabled')).toBeDefined()
      await button.trigger('click')
    }
    expect(copyMock).not.toHaveBeenCalled()
  })

  it('缺少密钥时不允许复制任何配置字段', async () => {
    const wrapper = mountSetup({ apiKey: '' })
    for (const button of wrapper.findAll('[data-testid^="desktop-copy-"]')) {
      expect(button.attributes('disabled')).toBeDefined()
      await button.trigger('click')
    }
    expect(copyMock).not.toHaveBeenCalled()
  })

  it('更换密钥后清空手填模型和复制提示，随后只复制新密钥', async () => {
    const wrapper = mountSetup()
    await wrapper.get('[data-testid="desktop-model-input"]').setValue('custom-model')
    await wrapper.get('[data-testid="desktop-copy-key"]').trigger('click')
    expect(wrapper.text()).toContain('keys.desktopSetup.fieldCopied')
    const replacement = 'sk-desktop-new-secret-987654'
    await wrapper.setProps({ apiKey: replacement })
    expect(wrapper.get('[data-testid="desktop-model-input"]').element).toHaveProperty('value', '')
    expect(wrapper.text()).not.toContain('keys.desktopSetup.fieldCopied')
    expect(wrapper.find('[data-testid="desktop-field-model"]').exists()).toBe(false)
    await wrapper.get('[data-testid="desktop-copy-key"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith(replacement, 'keys.copied')
    expect(wrapper.html()).not.toContain(defaultProps.apiKey)
    expect(wrapper.html()).not.toContain(replacement)
  })

  it('复制失败不会显示已复制，变更模型会清除旧反馈', async () => {
    const wrapper = mountSetup({ model: 'claude-sonnet-4-6' })
    await wrapper.get('[data-testid="desktop-copy-model"]').trigger('click')
    expect(wrapper.text()).toContain('keys.desktopSetup.fieldCopied')
    await wrapper.setProps({ model: 'claude-opus-4-6' })
    expect(wrapper.text()).not.toContain('keys.desktopSetup.fieldCopied')
    copyMock.mockResolvedValue(false)
    await wrapper.get('[data-testid="desktop-copy-key"]').trigger('click')
    expect(wrapper.text()).not.toContain('keys.desktopSetup.fieldCopied')
    expect(wrapper.html()).not.toContain(defaultProps.apiKey)
  })
})
