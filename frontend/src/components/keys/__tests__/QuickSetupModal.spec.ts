import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref, type Ref } from 'vue'
import type { ApiKey, GroupPlatform } from '@/types'
import type { KeySetupModelsOptions } from '@/composables/useKeySetupModels'
import QuickSetupModal from '../QuickSetupModal.vue'
import { CC_SWITCH_USAGE_SCRIPT } from '@/utils/ccswitchImport'

const { copyMock, saveAsMock, modelHookMock } = vi.hoisted(() => ({
  copyMock: vi.fn(), saveAsMock: vi.fn(), modelHookMock: vi.fn()
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: copyMock }) }))
vi.mock('file-saver', () => ({ saveAs: saveAsMock }))
vi.mock('@/composables/useKeySetupModels', () => ({ useKeySetupModels: (options: KeySetupModelsOptions) => modelHookMock(options) }))

function makeKey(platform: GroupPlatform = 'anthropic', extra: Partial<ApiKey> = {}): ApiKey {
  return {
    id: 11, user_id: 1, name: '测试密钥', key: 'sk-local-quick-setup-secret-123456',
    status: 'active', group_id: 4,
    group: { id: 4, name: '测试分组', platform, claude_code_only: false, allow_messages_dispatch: false },
    ...extra
  } as ApiKey
}

type Props = InstanceType<typeof QuickSetupModal>['$props']
const wrappers: VueWrapper[] = []
let modelsState: {
  models: Ref<string[]>
  selectedModel: Ref<string>
  loading: Ref<boolean>
  error: Ref<string | null>
  refresh: ReturnType<typeof vi.fn>
}

function mountModal(extra: Partial<Props> = {}) {
  const key = extra.keyInfo ?? makeKey()
  const wrapper = mount(QuickSetupModal, {
    props: {
      show: true, keyInfo: key, keys: key ? [key] : [],
      baseUrl: 'https://api.example.test/proxy/v1/', siteName: '测试站点', ...extra
    },
    global: {
      stubs: {
        BaseDialog: { props: ['show'], emits: ['close'], template: '<section v-if="show"><slot /><slot name="footer" /></section>' },
        Icon: { props: ['name'], template: '<span :data-icon="name" />' }
      }
    }
  })
  wrappers.push(wrapper)
  return wrapper
}

function importedParams(call = 0) {
  return new URL(String(vi.mocked(window.open).mock.calls[call]?.[0])).searchParams
}

async function readBlob(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error)
    reader.readAsText(blob)
  })
}

describe('一键接入面板', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'open').mockReturnValue(null)
    copyMock.mockResolvedValue(true)
    modelsState = { models: ref(['model-default', 'vendor/model-selected']), selectedModel: ref('model-default'), loading: ref(false), error: ref(null), refresh: vi.fn() }
    modelHookMock.mockImplementation(() => modelsState)
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    vi.restoreAllMocks()
  })

  it.each([
    ['anthropic', 'claude', 'claude', 'https://api.example.test/proxy'],
    ['anthropic', 'codex-app', 'codex', 'https://api.example.test/proxy/v1'],
    ['anthropic', 'codex', 'codex', 'https://api.example.test/proxy/v1'],
    ['gemini', 'gemini', 'gemini', 'https://api.example.test/proxy'],
    ['gemini', 'opencode', 'opencode', 'https://api.example.test/proxy/v1'],
    ['antigravity', 'openclaw', 'openclaw', 'https://api.example.test/proxy/v1'],
    ['antigravity', 'hermes', 'hermes', 'https://api.example.test/proxy/v1'],
    ['grok', 'grok', 'grokbuild', 'https://api.example.test/proxy/v1']
  ] as const)('%s / %s 导入目标、模型、供应商名称与端点一致', async (platform, client, app, endpoint) => {
    const key = makeKey(platform)
    const wrapper = mountModal({ keyInfo: key })
    expect(window.open).not.toHaveBeenCalled()
    expect(wrapper.html()).not.toContain(key.key)
    await wrapper.get(`[data-testid="setup-client-${client}"]`).trigger('click')
    await wrapper.get('[data-testid="setup-mode-import"]').trigger('click')
    await wrapper.get('#quick-setup-model').setValue('vendor/model-selected')
    await wrapper.get('#quick-setup-provider-name').setValue('自定义供应商 & 测试')
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')

    const params = importedParams()
    expect(params.get('app')).toBe(app)
    expect(params.get('model')).toBe('vendor/model-selected')
    expect(params.get('name')).toBe('自定义供应商 & 测试')
    expect(params.get('endpoint')).toBe(endpoint)
    expect(params.get('apiKey')).toBe(key.key)
    expect(atob(params.get('usageScript') || '')).toBe(CC_SWITCH_USAGE_SCRIPT)
    expect(params.get('usageAutoInterval')).toBe('30')
    expect(vi.mocked(window.open).mock.calls[0]?.[1]).toBe('_self')
    expect(wrapper.text()).toContain('keys.ccsClientSelect.importRequested')
    expect(wrapper.text()).not.toContain('keys.ccsClientSelect.launchFailed')
    expect(wrapper.html()).not.toContain(key.key)
  })

  it('Desktop 安装与配置两页均可复制字段，不显示导入页或发起 CLI 深链', async () => {
    const key = makeKey()
    const wrapper = mountModal({ keyInfo: key })
    await wrapper.get('[data-testid="setup-client-claude-desktop"]').trigger('click')
    expect(wrapper.find('[data-testid="setup-mode-import"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="setup-open-ccs"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="desktop-model-input"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="ccs-desktop-guide"]').text()).toContain('keys.desktopSetup.addHint')
    expect(wrapper.get('[data-testid="desktop-field-model"]').text()).toContain('model-default')
    for (const mode of ['install', 'native']) {
      await wrapper.get(`[data-testid="setup-mode-${mode}"]`).trigger('click')
      expect(wrapper.find('[data-testid="ccs-desktop-guide"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="setup-open-ccs"]').exists()).toBe(false)
      await wrapper.get('[data-testid="desktop-copy-key"]').trigger('click')
      expect(copyMock).toHaveBeenLastCalledWith(key.key, 'keys.copied')
      expect(wrapper.html()).not.toContain(key.key)
    }
    expect(window.open).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('从 CLI 导入页切到 Desktop 自动进入配置页，切回后 CLI 导入保持正常', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="setup-mode-import"]').trigger('click')
    await wrapper.get('[data-testid="setup-client-claude-desktop"]').trigger('click')
    expect(wrapper.get('[data-testid="setup-mode-native"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="setup-mode-import"]').exists()).toBe(false)
    await wrapper.get('[data-testid="setup-client-claude"]').trigger('click')
    await wrapper.get('[data-testid="setup-mode-import"]').trigger('click')
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')
    expect(importedParams().get('app')).toBe('claude')
  })

  it('管理员隐藏 CCS 导入后 Desktop 仍可手动复制 Antigravity 配置', async () => {
    const wrapper = mountModal({ keyInfo: makeKey('antigravity'), hideCcsImport: true })
    await wrapper.get('[data-testid="setup-client-claude-desktop"]').trigger('click')
    for (const mode of ['install', 'native']) {
      await wrapper.get(`[data-testid="setup-mode-${mode}"]`).trigger('click')
      await wrapper.get('[data-testid="desktop-copy-endpoint"]').trigger('click')
      expect(copyMock).toHaveBeenLastCalledWith('https://api.example.test/proxy/antigravity', 'keys.copied')
    }
    expect(wrapper.find('[data-testid="setup-mode-import"]').exists()).toBe(false)
    expect(window.open).not.toHaveBeenCalled()
  })

  it('Desktop 更换密钥和关闭重开后不沿用复制反馈或显示密钥', async () => {
    const first = makeKey()
    const second = makeKey('anthropic', { id: 22, key: 'sk-new-desktop-secret-987654' })
    const wrapper = mountModal({ keyInfo: first, keys: [first, second] })
    await wrapper.get('[data-testid="setup-client-claude-desktop"]').trigger('click')
    await wrapper.get('[data-testid="desktop-copy-key"]').trigger('click')
    expect(wrapper.text()).toContain('keys.desktopSetup.fieldCopied')
    await wrapper.setProps({ keyInfo: second })
    await wrapper.get('[data-testid="setup-client-claude-desktop"]').trigger('click')
    expect(wrapper.text()).not.toContain('keys.desktopSetup.fieldCopied')
    await wrapper.get('[data-testid="desktop-copy-key"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith(second.key, 'keys.copied')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await wrapper.get('[data-testid="setup-client-claude-desktop"]').trigger('click')
    expect(wrapper.text()).not.toContain('keys.desktopSetup.fieldCopied')
    expect(wrapper.html()).not.toContain(first.key)
    expect(wrapper.html()).not.toContain(second.key)
    expect(window.open).not.toHaveBeenCalled()
  })

  it('切换 TypeSafe 密钥后提供原生出口，停止模型目录并清除原导入流程', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')
    const key = makeKey('typesafe', { id: 12 })
    key.group!.claude_code_only = true
    await wrapper.setProps({ keyInfo: key })
    expect(wrapper.get('[data-testid="setup-systemone-panel"]').text()).toContain('keys.quickSetup.systemOneOnly')
    expect(wrapper.find('[data-testid="setup-open-ccs"]').exists()).toBe(false)
    expect(wrapper.find('#quick-setup-model').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('keys.ccsClientSelect.importRequested')
    expect(wrapper.html()).not.toContain(key.key)
    for (const button of wrapper.findAll('[data-testid^="setup-client-"]')) expect(button.attributes('disabled')).toBeDefined()
    const hookOptions = modelHookMock.mock.calls[0]![0] as KeySetupModelsOptions
    expect(typeof hookOptions.enabled === 'function' && hookOptions.enabled()).toBe(false)
    await wrapper.get('[data-testid="setup-open-native"]').trigger('click')
    expect(wrapper.emitted('open-native')).toHaveLength(1)
    expect(window.open).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ keyInfo: makeKey('openai') })
    expect(wrapper.find('[data-testid="setup-systemone-panel"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="setup-client-codex-app"]').attributes('aria-pressed')).toBe('true')
  })

  it('Codex WebSocket 保留原生配置入口，不伪装成普通 CCS 导入', async () => {
    const wrapper = mountModal({ keyInfo: makeKey('openai') })
    await wrapper.get('[data-testid="setup-client-codex-ws"]').trigger('click')
    expect(wrapper.find('[data-testid="setup-open-ccs"]').exists()).toBe(false)
    await wrapper.get('[data-testid="setup-mode-native"]').trigger('click')
    expect(wrapper.findAll('pre').map(block => block.text()).join('\n')).toContain('supports_websockets = true')
    expect(window.open).not.toHaveBeenCalled()
  })

  it('OpenAI Messages 权限控制 Claude，并在收紧限制后重置客户端', async () => {
    const key = makeKey('openai')
    const wrapper = mountModal({ keyInfo: key })
    expect(wrapper.get('[data-testid="setup-client-claude"]').attributes('disabled')).toBeDefined()
    const allowed = { ...key, group: { ...key.group!, allow_messages_dispatch: true } }
    await wrapper.setProps({ keyInfo: allowed })
    await wrapper.get('[data-testid="setup-client-claude"]').trigger('click')
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')
    expect(importedParams().get('app')).toBe('claude')
    await wrapper.setProps({ keyInfo: key })
    expect(wrapper.get('[data-testid="setup-client-codex-app"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.text()).not.toContain('keys.ccsClientSelect.importRequested')
  })

  it.each(['openai', 'gemini', 'grok', 'antigravity'] as const)('%s 受限分组只允许 Claude 并生成正确协议', async platform => {
    const key = makeKey(platform)
    key.group!.claude_code_only = true
    const wrapper = mountModal({ keyInfo: key })
    for (const button of wrapper.findAll('[data-testid^="setup-client-"]')) {
      const isClaude = button.attributes('data-testid') === 'setup-client-claude'
      expect(button.attributes('disabled') !== undefined).toBe(!isClaude)
    }
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')
    expect(importedParams().get('app')).toBe('claude')
    expect(importedParams().get('endpoint')).toBe(`https://api.example.test/proxy${platform === 'antigravity' ? '/antigravity' : ''}`)
  })

  it('管理员隐藏 CCS 时不显示导入操作，动态隐藏也退出导入页', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="setup-mode-import"]').trigger('click')
    await wrapper.setProps({ hideCcsImport: true })
    expect(wrapper.find('[data-testid="setup-mode-import"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="setup-open-ccs"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="setup-native-panel"]').exists()).toBe(true)
    await wrapper.get('[data-testid="setup-mode-install"]').trigger('click')
    expect(wrapper.find('[data-testid="setup-open-ccs"]').exists()).toBe(false)
    expect(window.open).not.toHaveBeenCalled()
  })

  it.each(['inactive', 'quota_exhausted', 'expired'] as const)('%s 密钥不能导入或生成配置', status => {
    const wrapper = mountModal({ keyInfo: makeKey('anthropic', { status }) })
    expect(wrapper.text()).toContain('keys.quickSetup.inactiveKey')
    expect(wrapper.find('[data-testid="setup-open-ccs"]').exists()).toBe(false)
    expect(wrapper.find('#quick-setup-model').exists()).toBe(false)
    const options = modelHookMock.mock.calls[0]?.[0] as KeySetupModelsOptions
    expect((options.enabled as () => boolean)()).toBe(false)
    expect(window.open).not.toHaveBeenCalled()
  })

  it('未分组密钥显示分组指引，所有客户端不可选', () => {
    const wrapper = mountModal({ keyInfo: makeKey('anthropic', { group_id: null, group: undefined }) })
    expect(wrapper.text()).toContain('keys.useKeyModal.noGroupDescription')
    expect(wrapper.find('[data-testid="setup-open-ccs"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid^="setup-client-"]').every(button => button.attributes('disabled') !== undefined)).toBe(true)
  })

  it('密钥选择事件只传 ID，更换密钥清除导入状态并恢复默认遮罩', async () => {
    const first = makeKey()
    const second = makeKey('anthropic', { id: 22, name: '第二条密钥', key: 'sk-local-second-secret-987654' })
    const wrapper = mountModal({ keyInfo: first, keys: [first, second] })
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')
    await wrapper.get('[data-testid="setup-mode-native"]').trigger('click')
    await wrapper.get('[data-testid="setup-native-panel"] button[aria-pressed]').trigger('click')
    expect(wrapper.text()).toContain(first.key)
    await wrapper.get('#quick-setup-key').setValue('22')
    expect(wrapper.emitted('select-key')).toEqual([[22]])
    await wrapper.setProps({ keyInfo: second })
    expect(wrapper.find('[data-testid="setup-install-panel"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('keys.ccsClientSelect.importRequested')
    expect(wrapper.html()).not.toContain(first.key)
    expect(wrapper.html()).not.toContain(second.key)
    expect(wrapper.get('#quick-setup-provider-name').element).toHaveProperty('value', '测试站点 · 测试分组 · 第二条密钥')
    const options = modelHookMock.mock.calls[0]?.[0] as KeySetupModelsOptions
    expect((options.keyId as () => number)()).toBe(22)
    modelsState.selectedModel.value = 'second-key-model'
    await wrapper.get('[data-testid="setup-mode-native"]').trigger('click')
    expect(wrapper.text()).not.toContain(second.key)
    expect(wrapper.text()).toContain('<API_KEY>')
    await wrapper.get('[data-testid="setup-mode-import"]').trigger('click')
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')
    expect(importedParams(1).get('apiKey')).toBe(second.key)
    expect(importedParams(1).get('model')).toBe('second-key-model')
  })

  it.each(['openclaw', 'hermes'] as const)('%s 原生配置默认遮罩，复制和下载交付完整内容', async client => {
    const key = makeKey()
    const wrapper = mountModal({ keyInfo: key })
    await wrapper.get(`[data-testid="setup-client-${client}"]`).trigger('click')
    await wrapper.get('[data-testid="setup-mode-native"]').trigger('click')
    const panel = wrapper.get('[data-testid="setup-native-panel"]')
    expect(panel.text()).toContain('<API_KEY>')
    expect(wrapper.html()).not.toContain(key.key)
    await panel.get('button[aria-label^="keys.useKeyModal.copy"]').trigger('click')
    expect(copyMock.mock.calls.at(-1)?.[0]).toContain(key.key)
    expect(copyMock.mock.calls.at(-1)?.[0]).toContain('https://api.example.test/proxy/v1')
    const download = panel.findAll('button').find(button => button.text().includes('keys.quickSetup.downloadFile'))!
    await download.trigger('click')
    const [blob, name] = saveAsMock.mock.calls[0] as [Blob, string]
    expect(name).toBe(client === 'openclaw' ? 'openclaw.json' : 'config.yaml')
    expect(await readBlob(blob)).toContain(key.key)
    expect(wrapper.html()).not.toContain(key.key)
    await panel.get('button[aria-pressed]').trigger('click')
    expect(panel.text()).toContain(key.key)
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await wrapper.get('[data-testid="setup-mode-native"]').trigger('click')
    expect(wrapper.html()).not.toContain(key.key)
  })

  it('复制接口地址匹配当前导入协议，密钥复制不改变默认遮罩', async () => {
    const key = makeKey('antigravity')
    const wrapper = mountModal({ keyInfo: key })
    await wrapper.get('[data-testid="setup-client-opencode"]').trigger('click')
    await wrapper.get('[data-testid="copy-setup-endpoint"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith('https://api.example.test/proxy/v1', 'keys.copied')
    await wrapper.get('[data-testid="setup-mode-native"]').trigger('click')
    await wrapper.get('[data-testid="copy-setup-endpoint"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith('https://api.example.test/proxy/antigravity/v1', 'keys.copied')
    await wrapper.get('[data-testid="copy-setup-key"]').trigger('click')
    expect(copyMock).toHaveBeenLastCalledWith(key.key, 'keys.copied')
    expect(wrapper.html()).not.toContain(key.key)
  })

  it('空模型、非法模型和无效地址禁止发送导入请求', async () => {
    const wrapper = mountModal()
    for (const model of ['', 'bad\nmodel']) {
      modelsState.selectedModel.value = model
      await flushPromises()
      expect(wrapper.get('[data-testid="setup-open-ccs"]').attributes('disabled')).toBeDefined()
    }
    modelsState.selectedModel.value = 'valid-model'
    await wrapper.setProps({ baseUrl: 'https://user:private@example.test?token=private' })
    expect(wrapper.get('[data-testid="setup-open-ccs"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('keys.quickSetup.invalidEndpoint')
    expect(wrapper.text()).not.toContain('private')
    expect(window.open).not.toHaveBeenCalled()
  })

  it.each(['模型/版本 1 "精确"', 'vendor/model with space', 'x'.repeat(256)])('目录中的合法模型名可导入并用于原生配置：%s', async model => {
    const wrapper = mountModal()
    await wrapper.get('#quick-setup-model').setValue(model)
    expect(wrapper.get('[data-testid="setup-open-ccs"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')
    expect(importedParams().get('model')).toBe(model)
    await wrapper.get('[data-testid="setup-mode-native"]').trigger('click')
    const settings = wrapper.findAll('pre code').find(block => block.text().trim().startsWith('{'))!
    expect(JSON.parse(settings.text()).env.ANTHROPIC_MODEL).toBe(model)
    expect(wrapper.find('script').exists()).toBe(false)
  })

  it('首尾空格在导入和原生配置中得到一致处理', async () => {
    const wrapper = mountModal()
    await wrapper.get('#quick-setup-model').setValue('  vendor/model-name  ')
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')
    expect(importedParams().get('model')).toBe('vendor/model-name')
    await wrapper.get('[data-testid="setup-mode-native"]').trigger('click')
    const settings = wrapper.findAll('pre code').find(block => block.text().trim().startsWith('{'))!
    expect(JSON.parse(settings.text()).env.ANTHROPIC_MODEL).toBe('vendor/model-name')
  })

  it.each(['bad\u0085model', 'bad\ud800model', 'x'.repeat(257)])('不生成包含控制字符、残缺代理字符或超长模型的配置', async model => {
    const wrapper = mountModal()
    modelsState.selectedModel.value = model
    await flushPromises()
    expect(wrapper.get('[data-testid="setup-open-ccs"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('keys.quickSetup.invalidModel')
    expect(window.open).not.toHaveBeenCalled()
  })

  it('真实弹窗中的 Tab 和 Shift+Tab 保持焦点闭环，关闭后恢复原焦点', async () => {
    // jsdom 不进行布局，提供可见矩形来验证真实 BaseDialog 的键盘逻辑。
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([{}] as unknown as DOMRectList)
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    outside.focus()
    const key = makeKey()
    const wrapper = mount(QuickSetupModal, {
      props: { show: true, keyInfo: key, keys: [key], baseUrl: 'https://api.example.test' },
      attachTo: document.body,
      global: { stubs: { Icon: true } }
    })
    wrappers.push(wrapper)
    await flushPromises()
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    const first = dialog.querySelector<HTMLButtonElement>('button')!
    const buttons = dialog.querySelectorAll<HTMLButtonElement>('button')
    const last = buttons[buttons.length - 1]!
    last.focus()
    const forward = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    last.dispatchEvent(forward)
    expect(forward.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(first)
    const backward = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true })
    first.dispatchEvent(backward)
    expect(backward.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(last)
    await wrapper.setProps({ show: false })
    expect(document.activeElement).toBe(outside)
    outside.remove()
  })

  it('仅本地打开失败时显示固定错误，不展示异常密钥', async () => {
    const key = makeKey()
    vi.mocked(window.open).mockImplementation(() => { throw new Error(key.key) })
    const wrapper = mountModal({ keyInfo: key })
    await wrapper.get('[data-testid="setup-open-ccs"]').trigger('click')
    expect(wrapper.text()).toContain('keys.ccsClientSelect.launchFailed')
    expect(wrapper.html()).not.toContain(key.key)
  })
})
