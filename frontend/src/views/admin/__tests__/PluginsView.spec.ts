import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PluginsView from '../PluginsView.vue'

const {
  listPlugins,
  uploadPlugin,
  enablePlugin,
  savePluginConfig,
  getPluginConfig,
  createUISession,
  setHostAdaptation,
  getResources,
  runAction,
  showError,
  showSuccess,
  stepUpRun,
} = vi.hoisted(() => ({
  listPlugins: vi.fn(),
  uploadPlugin: vi.fn(),
  enablePlugin: vi.fn(),
  savePluginConfig: vi.fn(),
  getPluginConfig: vi.fn(),
  createUISession: vi.fn(),
  setHostAdaptation: vi.fn(),
  getResources: vi.fn(),
  runAction: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  stepUpRun: vi.fn((action: () => Promise<unknown>) => action()),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    plugins: {
      list: listPlugins,
      upload: uploadPlugin,
      enable: enablePlugin,
      disable: vi.fn(),
      remove: vi.fn(),
      getConfig: getPluginConfig,
      saveConfig: savePluginConfig,
      test: vi.fn().mockResolvedValue({ success: true, message: 'ok', latency_ms: 1 }),
      createUISession,
      setHostAdaptation,
      resources: getResources,
      action: runAction,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: stepUpRun }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

const plugin = {
  id: 7,
  plugin_key: 'local.test.transport',
  name: 'Test Transport',
  version: '1.0.0',
  description: '',
  author: 'test',
  manifest: {
    schema_version: 1,
    id: 'local.test.transport',
    name: 'Test Transport',
    version: '1.0.0',
    requires: {
      sub2api: '>=0.1.0',
      plugin_protocol: 1,
      transport_api: 1,
      ui_bridge: 1,
    },
    capabilities: [{ id: 'openai.oauth.outbound_transport.v1', platform: 'openai', account_type: 'oauth' }],
    ui: { entrypoint: 'ui/index.html' },
  },
  binary_sha256: 'a'.repeat(64),
  signature_status: 'trusted' as const,
  state: 'disabled' as const,
  last_error: '',
  installed_at: '2026-08-22T00:00:00Z',
  updated_at: '2026-08-22T00:00:00Z',
  bindings: [
    {
      id: 1,
      plugin_id: 7,
      capability: 'openai.oauth.outbound_transport.v1',
      platform: 'openai',
      account_type: 'oauth',
      enabled: false,
      rollout_percent: 100,
    },
  ],
  compatibility: {
    compatible: true,
    tested: true,
    status: 'compatible' as const,
    message: '',
    current_sub2api_version: '0.1.0',
    required_sub2api_version: '>=0.1.0',
    recommended_sub2api_version: '0.1.0',
    plugin_protocol: 1,
    transport_api: 1,
    ui_bridge: 1,
  },
  runtime_healthy: false,
  runtime_message: '',
  host_adaptation_enabled: false,
}

const resources = {
  accounts: [{ id: 15, name: 'OAuth 账号', group_ids: [3] }],
  groups: [{ id: 3, name: '测试分组' }],
  proxies: [{ id: 6, name: '前置代理', protocol: 'http', host: 'proxy.example', port: 8080 }],
}

const wrappers: VueWrapper[] = []

function mountView(realDialog = false) {
  const wrapper = mount(PluginsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: realDialog ? false : {
          props: ['show'],
          emits: ['close'],
          template: '<section v-if="show"><button data-test="close-configuration" @click="$emit(\'close\')">关闭</button><slot /></section>',
        },
        Icon: true,
        TotpStepUpDialog: true,
      },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

async function openConfiguration(wrapper: VueWrapper, index = 0) {
  const button = wrapper.findAll('article')[index]!.findAll('button').find((item) => item.text().includes('admin.plugins.configure'))
  await button!.trigger('click')
  await flushPromises()
  if (wrapper.find('iframe').exists()) await wrapper.get('iframe').trigger('load')
}

function dispatchBridge(wrapper: VueWrapper, message: Record<string, unknown>, overrides: MessageEventInit = {}) {
  const frame = wrapper.get('iframe').element as HTMLIFrameElement
  window.dispatchEvent(new MessageEvent('message', {
    source: frame.contentWindow,
    origin: 'null',
    data: { source: 'sub2api-plugin-ui', bridge_token: 'bridge', ...message },
    ...overrides,
  }))
}

describe('管理员插件页二次验证', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    stepUpRun.mockImplementation((action: () => Promise<unknown>) => action())
    listPlugins.mockResolvedValue([plugin])
    uploadPlugin.mockResolvedValue(plugin)
    enablePlugin.mockResolvedValue(plugin)
    savePluginConfig.mockResolvedValue({ enabled: true })
    getPluginConfig.mockResolvedValue({ enabled: true })
    setHostAdaptation.mockResolvedValue({ ...plugin, host_adaptation_enabled: true })
    getResources.mockResolvedValue(resources)
    runAction.mockResolvedValue({ accepted: true, message: '已接受' })
    createUISession.mockResolvedValue({
      url: '/api/v1/plugin-ui/token/index.html#bridge_token=bridge',
      bridge_token: 'bridge',
      ui_bridge_version: 1,
      expires_at: '2026-08-22T01:00:00Z',
    })
  })

  afterEach(() => {
    for (const wrapper of wrappers.splice(0)) wrapper.unmount()
    vi.useRealTimers()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('启用插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.plugins.enable'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(enablePlugin).toHaveBeenCalledWith(7, 100, false)
  })

  it('上传插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['plugin'], 'transport.s2plugin', { type: 'application/zip' })],
    })

    await input.trigger('change')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(uploadPlugin).toHaveBeenCalledTimes(1)
  })

  it('宿主适配默认关闭并单独经过二次验证，不更改启用和灰度', async () => {
    const wrapper = mountView()
    await flushPromises()
    const toggle = wrapper.get('[role="switch"]')
    expect(toggle.attributes('aria-checked')).toBe('false')
    await toggle.trigger('click')
    await flushPromises()
    expect(setHostAdaptation).toHaveBeenCalledWith(7, true)
    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(toggle.attributes('aria-checked')).toBe('true')
    expect(enablePlugin).not.toHaveBeenCalled()
    expect(wrapper.get('input[type="range"]').element).toHaveProperty('value', '100')
    expect(showSuccess).toHaveBeenCalledWith('admin.plugins.hostAdaptationEnabled')
  })

  it('不支持所需能力的插件不展示适配开关', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, manifest: { ...plugin.manifest, capabilities: [] } }])
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[role="switch"]').exists()).toBe(false)
  })

  it('适配设置失败保留原开关与配置窗口', async () => {
    setHostAdaptation.mockRejectedValue(new Error('重载失败'))
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    await wrapper.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.find('iframe').exists()).toBe(true)
    expect(showError).toHaveBeenCalledWith('重载失败')
  })

  it('关闭适配后关闭配置 iframe，插件保持启用状态', async () => {
    const enabled = { ...plugin, state: 'enabled', host_adaptation_enabled: true, bindings: [{ ...plugin.bindings[0], enabled: true }] }
    listPlugins.mockResolvedValue([enabled])
    setHostAdaptation.mockResolvedValue({ ...enabled, host_adaptation_enabled: false })
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    await wrapper.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(setHostAdaptation).toHaveBeenCalledWith(7, false)
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.plugins.disable')
    expect(showSuccess).toHaveBeenCalledWith('admin.plugins.hostAdaptationDisabled')
  })

  it('开关已保存但重连失败时显示真实开关、关闭旧窗口并提示重试状态', async () => {
    const enabled = { ...plugin, state: 'enabled', bindings: [{ ...plugin.bindings[0], enabled: true }] }
    listPlugins.mockResolvedValue([enabled])
    setHostAdaptation.mockResolvedValue({
      ...enabled,
      host_adaptation_enabled: true,
      runtime_healthy: false,
      runtime_message: '宿主适配设置已保存，插件重新连接失败，系统将自动重试',
    })
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    await wrapper.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.plugins.disable')
    expect(showError).toHaveBeenCalledWith('宿主适配设置已保存，插件重新连接失败，系统将自动重试')
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it.each(['plugin.resources', 'plugin.action'])('未开启适配拒绝 %s', async (type) => {
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    dispatchBridge(wrapper, { type, request_id: 'denied', action: { kind: 'model_test' } })
    await flushPromises()
    expect(getResources).not.toHaveBeenCalled()
    expect(runAction).not.toHaveBeenCalled()
    expect(stepUpRun).not.toHaveBeenCalled()
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ ok: false, error: 'admin.plugins.hostAdaptationRequired' }), '*')
  })

  it('目录请求只访问插件资源接口，并按 Bridge 契约返回完整元数据', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, host_adaptation_enabled: true }])
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    dispatchBridge(wrapper, { type: 'plugin.resources', request_id: 'catalog' })
    await flushPromises()
    expect(getResources).toHaveBeenCalledWith(7)
    expect(stepUpRun).not.toHaveBeenCalled()
    expect(post).toHaveBeenCalledWith({ source: 'sub2api-plugin-host', bridge_token: 'bridge', type: 'plugin.resources.result', request_id: 'catalog', ok: true, resources }, '*')
  })

  it('手动动作经过二次验证并强制采用 Bridge 请求 ID', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, host_adaptation_enabled: true }])
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    dispatchBridge(wrapper, { type: 'plugin.action', request_id: ' action-1 ', action: { kind: 'model_test', account_id: 15, request_id: '伪造' } })
    await flushPromises()
    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(runAction).toHaveBeenCalledWith(7, { kind: 'model_test', account_id: 15, request_id: 'action-1' })
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ ok: true, request_id: 'action-1', result: { accepted: true, message: '已接受' } }), '*')
  })

  it.each([null, [], 'model_test', 1])('拒绝非对象手动动作 %s', async (action) => {
    listPlugins.mockResolvedValue([{ ...plugin, host_adaptation_enabled: true }])
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    dispatchBridge(wrapper, { type: 'plugin.action', request_id: 'invalid', action })
    await flushPromises()
    expect(runAction).not.toHaveBeenCalled()
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ ok: false, error: 'admin.plugins.bridgeRejected' }), '*')
  })

  it.each([
    { origin: 'https://example.com' },
    { source: window },
    { data: { source: 'sub2api-plugin-ui', bridge_token: 'wrong', type: 'plugin.resources', request_id: 'bad' } },
    { data: { source: 'another-ui', bridge_token: 'bridge', type: 'plugin.resources', request_id: 'bad' } },
  ])('拒绝来源、窗口或令牌不匹配的 Bridge 消息 %#', async (overrides) => {
    listPlugins.mockResolvedValue([{ ...plugin, host_adaptation_enabled: true }])
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    dispatchBridge(wrapper, { type: 'plugin.resources', request_id: 'bad' }, overrides)
    await flushPromises()
    expect(getResources).not.toHaveBeenCalled()
  })

  it('重复动作在途去重，完成后重放结果，并禁止跨类型复用请求 ID', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, host_adaptation_enabled: true }])
    const pending = deferred<{ accepted: boolean; message: string }>()
    runAction.mockReturnValue(pending.promise)
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    const message = { type: 'plugin.action', request_id: 'unique', action: { kind: 'model_test' } }
    dispatchBridge(wrapper, message)
    dispatchBridge(wrapper, message)
    await flushPromises()
    expect(runAction).toHaveBeenCalledTimes(1)
    pending.resolve({ accepted: true, message: '已接受' })
    await flushPromises()
    dispatchBridge(wrapper, message)
    dispatchBridge(wrapper, { type: 'plugin.resources', request_id: 'unique' })
    await flushPromises()
    expect(runAction).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledTimes(2)
    expect(getResources).not.toHaveBeenCalled()
  })

  it('打开另一插件后丢弃迟到的配置会话', async () => {
    listPlugins.mockResolvedValue([plugin, { ...plugin, id: 8, name: 'Second plugin' }])
    const pending = deferred<{ url: string; bridge_token: string; ui_bridge_version: number; expires_at: string }>()
    createUISession.mockReturnValueOnce(pending.promise)
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    await openConfiguration(wrapper, 1)
    pending.resolve({ url: '/old-session', bridge_token: 'old', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    expect(wrapper.get('iframe').attributes('src')).toContain('/api/v1/plugin-ui/token/')
    expect(wrapper.get('iframe').attributes('title')).toBe('admin.plugins.configTitle')
  })

  it('旧会话响应不能占用新会话相同请求 ID 或泄露到新窗口', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, host_adaptation_enabled: true }, { ...plugin, id: 8, host_adaptation_enabled: true }])
    createUISession.mockResolvedValueOnce({ url: '/old', bridge_token: 'bridge', ui_bridge_version: 1, expires_at: '' })
      .mockResolvedValueOnce({ url: '/new', bridge_token: 'bridge', ui_bridge_version: 1, expires_at: '' })
    const oldRequest = deferred<typeof resources>()
    const newRequest = deferred<typeof resources>()
    getResources.mockReturnValueOnce(oldRequest.promise).mockReturnValueOnce(newRequest.promise)
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    dispatchBridge(wrapper, { type: 'plugin.resources', request_id: 'shared' })
    await openConfiguration(wrapper, 1)
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    dispatchBridge(wrapper, { type: 'plugin.resources', request_id: 'shared' })
    oldRequest.resolve(resources)
    await flushPromises()
    expect(post).not.toHaveBeenCalled()
    newRequest.resolve({ ...resources, groups: [{ id: 4, name: '新会话目录' }] })
    await flushPromises()
    expect(getResources.mock.calls).toEqual([[7], [8]])
    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ resources: expect.objectContaining({ groups: [{ id: 4, name: '新会话目录' }] }) }), '*')
  })

  it.each(['close', 'switch', 'navigate'])('二次验证等待期间 %s 配置后，不执行过期动作', async (change) => {
    listPlugins.mockResolvedValue([{ ...plugin, host_adaptation_enabled: true }, { ...plugin, id: 8, host_adaptation_enabled: true }])
    const approval = deferred<void>()
    stepUpRun.mockImplementation((action: () => Promise<unknown>) => approval.promise.then(action))
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    dispatchBridge(wrapper, { type: 'plugin.action', request_id: 'pending-approval', action: { kind: 'model_test' } })
    await flushPromises()
    if (change === 'close') await wrapper.get('[data-test="close-configuration"]').trigger('click')
    else if (change === 'switch') await openConfiguration(wrapper, 1)
    else await wrapper.get('iframe').trigger('load')
    approval.resolve()
    await flushPromises()
    expect(runAction).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
  })

  it('配置保存等待二次验证时切换插件，不将旧配置写入新插件', async () => {
    listPlugins.mockResolvedValue([plugin, { ...plugin, id: 8 }])
    const approval = deferred<void>()
    stepUpRun.mockImplementation((action: () => Promise<unknown>) => approval.promise.then(action))
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    dispatchBridge(wrapper, { type: 'config.save', request_id: 'save', config: { secret: 'first-plugin-config' } })
    await openConfiguration(wrapper, 1)
    approval.resolve()
    await flushPromises()
    expect(savePluginConfig).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('iframe 导航后的迟到响应不会重放到新文档', async () => {
    const pending = deferred<Record<string, unknown>>()
    getPluginConfig.mockReturnValue(pending.promise)
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    dispatchBridge(wrapper, { type: 'config.load', request_id: 'old-config' })
    await wrapper.get('iframe').trigger('load')
    pending.resolve({ secret: '旧配置' })
    await flushPromises()
    expect(post).not.toHaveBeenCalled()
  })

  it('手动动作被插件拒绝时保留原结果', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, host_adaptation_enabled: true }])
    runAction.mockResolvedValue({ accepted: false, message: '当前未运行' })
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    dispatchBridge(wrapper, { type: 'plugin.action', request_id: 'rejected', action: { kind: 'model_test' } })
    await flushPromises()
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ ok: false, result: { accepted: false, message: '当前未运行' } }), '*')
  })

  it('请求过期后完成二次验证不会再执行动作', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, host_adaptation_enabled: true }])
    const approval = deferred<void>()
    stepUpRun.mockImplementation((action: () => Promise<unknown>) => approval.promise.then(action))
    const wrapper = mountView()
    await flushPromises()
    await openConfiguration(wrapper)
    vi.useFakeTimers()
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    dispatchBridge(wrapper, { type: 'plugin.action', request_id: 'expired', action: { kind: 'model_test' } })
    await vi.advanceTimersByTimeAsync(30_001)
    approval.resolve()
    await flushPromises()
    expect(runAction).not.toHaveBeenCalled()
    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ ok: false, error: 'admin.plugins.bridgeExpired' }), '*')
  })

  it.each(['click', 'escape'])('真实配置弹窗通过 %s 关闭并释放滚动锁', async (method) => {
    const wrapper = mountView(true)
    await flushPromises()
    const configure = wrapper.findAll('article')[0]!.findAll('button').find((item) => item.text().includes('admin.plugins.configure'))!
    await configure.trigger('click')
    await flushPromises()
    expect(document.querySelector('[role="dialog"] iframe')).not.toBeNull()
    expect(document.body.classList.contains('modal-open')).toBe(true)
    if (method === 'click') {
      (document.querySelector('[aria-label="Close modal"]') as HTMLButtonElement).click()
    } else {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    }
    await flushPromises()
    await vi.waitFor(() => expect(document.querySelector('[role="dialog"]')).toBeNull())
    expect(document.querySelector('iframe')).toBeNull()
    expect(document.body.classList.contains('modal-open')).toBe(false)
  })
})
