import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CcSwitchImportModal from '../CcSwitchImportModal.vue'
import { CC_SWITCH_USAGE_SCRIPT } from '@/utils/ccswitchImport'
import type { GroupPlatform } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const defaultProps = {
  show: true,
  apiKey: 'sk-private-test-key',
  baseUrl: 'https://api.example.com',
  platform: 'anthropic' as GroupPlatform,
  providerName: '示例站点'
}

function mountModal(props: Partial<typeof defaultProps> & { claudeCodeOnly?: boolean } = {}) {
  return mount(CcSwitchImportModal, {
    props: { ...defaultProps, ...props },
    global: {
      stubs: {
        BaseDialog: {
          name: 'BaseDialog',
          props: ['show'],
          emits: ['close'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>'
        },
        Icon: { template: '<span />' }
      }
    }
  })
}

describe('CC Switch 目标选择', () => {
  beforeEach(() => {
    vi.spyOn(window, 'open').mockReturnValue(null)
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('Anthropic 可选择 CLI 或 Desktop，未确认时不打开应用且不展示密钥', () => {
    const wrapper = mountModal()
    expect(wrapper.findAll('input[type="radio"]')).toHaveLength(2)
    expect(wrapper.find('[data-testid="ccs-target-claude"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="ccs-target-claude-desktop"]').exists()).toBe(true)
    expect(wrapper.html()).not.toContain(defaultProps.apiKey)
    expect(window.open).not.toHaveBeenCalled()
  })

  it('Desktop 发起兼容的 Claude 导入并保留迁移说明，不误报已完成或安装失败', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="ccs-target-claude-desktop"] input').setValue(true)
    expect(wrapper.get('[data-testid="ccs-desktop-guide"]').text()).toContain('keys.ccsClientSelect.desktopStepMigrate')
    await wrapper.get('[data-testid="ccs-import-submit"]').trigger('click')

    const args = vi.mocked(window.open).mock.calls[0]
    const url = new URL(String(args?.[0]))
    expect(args?.[1]).toBe('_self')
    expect(url.searchParams.get('app')).toBe('claude')
    expect(url.searchParams.get('endpoint')).toBe(defaultProps.baseUrl)
    expect(url.searchParams.get('apiKey')).toBe(defaultProps.apiKey)
    expect(url.searchParams.get('name')).toBe(defaultProps.providerName)
    expect(atob(url.searchParams.get('usageScript') || '')).toBe(CC_SWITCH_USAGE_SCRIPT)
    expect(url.searchParams.get('usageEnabled')).toBe('true')
    expect(url.searchParams.get('usageAutoInterval')).toBe('30')
    expect(wrapper.get('[data-testid="ccs-import-requested"]').text()).toBe('keys.ccsClientSelect.importRequested')
    expect(wrapper.find('[data-testid="ccs-import-failed"]').exists()).toBe(false)
    expect(document.hasFocus).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.get('[data-testid="ccs-desktop-guide"] a').attributes('href')).toContain('farion1231/cc-switch/blob/v3.20.4/')
  })

  it('Antigravity 三个目标均使用原平台路径', async () => {
    const wrapper = mountModal({ platform: 'antigravity', baseUrl: 'https://api.example.com/sub/' })
    expect(wrapper.findAll('input[type="radio"]')).toHaveLength(3)
    for (const client of ['claude', 'claude-desktop', 'gemini']) {
      await wrapper.get(`[data-testid="ccs-target-${client}"] input`).setValue(true)
      await wrapper.get('[data-testid="ccs-import-submit"]').trigger('click')
      const calls = vi.mocked(window.open).mock.calls
      const url = new URL(String(calls[calls.length - 1]?.[0]))
      expect(url.searchParams.get('endpoint')).toBe('https://api.example.com/sub/antigravity')
      expect(url.searchParams.get('app')).toBe(client === 'gemini' ? 'gemini' : 'claude')
    }
  })

  it.each([
    ['anthropic', 'https://api.example.com/sub'],
    ['openai', 'https://api.example.com/sub'],
    ['grok', 'https://api.example.com/sub'],
    ['gemini', 'https://api.example.com/sub'],
    ['antigravity', 'https://api.example.com/sub/antigravity']
  ] as const)('仅允许 Claude Code 的 %s 密钥使用 Claude 导入及正确端点', async (platform, endpoint) => {
    const wrapper = mountModal({ platform, claudeCodeOnly: true, baseUrl: 'https://api.example.com/sub/v1/' })
    expect(wrapper.findAll('input[type="radio"]')).toHaveLength(1)
    expect(wrapper.get('input[type="radio"]').attributes('value')).toBe('claude')
    expect(wrapper.get('[data-testid="ccs-target-claude"]').text()).toContain('keys.ccsClientSelect.claudeCode')
    expect(wrapper.find('[data-testid="ccs-desktop-guide"]').exists()).toBe(false)
    await wrapper.get('[data-testid="ccs-import-submit"]').trigger('click')
    const url = new URL(String(vi.mocked(window.open).mock.calls[0]?.[0]))
    expect(url.searchParams.get('app')).toBe('claude')
    expect(url.searchParams.get('endpoint')).toBe(endpoint)
    expect(url.searchParams.has('model')).toBe(false)
    expect(atob(url.searchParams.get('usageScript') || '')).toBe(CC_SWITCH_USAGE_SCRIPT)
  })

  it.each([
    ['openai', 'codex', 'https://api.example.com', 'gpt-5.5'],
    ['grok', 'grokbuild', 'https://api.example.com/v1', 'grok-4.5'],
    ['gemini', 'gemini', 'https://api.example.com', null]
  ] as const)('%s 保留原导入目标、端点和模型', async (platform, app, endpoint, model) => {
    const wrapper = mountModal({ platform })
    expect(wrapper.findAll('input[type="radio"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="ccs-target-claude-desktop"]').exists()).toBe(false)
    await wrapper.get('[data-testid="ccs-import-submit"]').trigger('click')
    const url = new URL(String(vi.mocked(window.open).mock.calls[0]?.[0]))
    expect(url.searchParams.get('app')).toBe(app)
    expect(url.searchParams.get('endpoint')).toBe(endpoint)
    expect(url.searchParams.get('model')).toBe(model)
  })

  it('更换密钥或限制后重置目标和请求提示，避免沿用之前的导入状态', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="ccs-target-claude-desktop"] input').setValue(true)
    await wrapper.get('[data-testid="ccs-import-submit"]').trigger('click')
    await wrapper.setProps({ apiKey: 'sk-replacement', claudeCodeOnly: true })
    expect(wrapper.find('[data-testid="ccs-import-requested"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ccs-desktop-guide"]').exists()).toBe(false)
    await wrapper.get('[data-testid="ccs-import-submit"]').trigger('click')
    const url = new URL(String(vi.mocked(window.open).mock.calls[1]?.[0]))
    expect(url.searchParams.get('apiKey')).toBe('sk-replacement')
  })

  it('只有浏览器抛出异常时显示打开失败，不输出错误内容或密钥', async () => {
    vi.mocked(window.open).mockImplementation(() => { throw new Error(defaultProps.apiKey) })
    const wrapper = mountModal()
    await wrapper.get('[data-testid="ccs-import-submit"]').trigger('click')
    expect(wrapper.get('[data-testid="ccs-import-failed"]').text()).toBe('keys.ccsClientSelect.launchFailed')
    expect(wrapper.find('[data-testid="ccs-import-requested"]').exists()).toBe(false)
    expect(wrapper.html()).not.toContain(defaultProps.apiKey)
  })

  it.each([
    '/relative/antigravity',
    'https://private-credential@api.example.com',
    'https://api.example.com?token=private-query'
  ])('非法 Antigravity 地址 %s 显示固定失败提示，不抛出未处理异常', async baseUrl => {
    const wrapper = mountModal({ platform: 'antigravity', baseUrl })
    await wrapper.get('[data-testid="ccs-import-submit"]').trigger('click')
    expect(wrapper.get('[data-testid="ccs-import-failed"]').text()).toBe('keys.ccsClientSelect.launchFailed')
    expect(wrapper.find('[data-testid="ccs-import-requested"]').exists()).toBe(false)
    expect(window.open).not.toHaveBeenCalled()
    expect(wrapper.html()).not.toContain(baseUrl)
    expect(wrapper.html()).not.toContain(defaultProps.apiKey)
  })

  it('缺少密钥时禁止导入，关闭事件交给父页面处理', async () => {
    const wrapper = mountModal({ apiKey: '' })
    expect(wrapper.get('[data-testid="ccs-import-submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="ccs-import-submit"]').trigger('click')
    expect(window.open).not.toHaveBeenCalled()
    wrapper.findComponent({ name: 'BaseDialog' }).vm.$emit('close')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
