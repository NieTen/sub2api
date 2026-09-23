import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createSidebarPageOverlay } from '../sidebar-overlay.js'

const modelCatalog = [
  { name: 'gpt-5.5', vendor: 'OpenAI', type: 'text', input: 5, output: 30 },
  { name: 'gpt-5.4-mini', vendor: 'OpenAI', type: 'text', input: 0.75, output: 4.5 },
  { name: 'claude-sonnet-4-6', vendor: 'ANTHROPIC', type: 'text', input: 3, output: 15 },
]

function apiData(): Record<string, unknown> {
  return {
    '/auth/me': { username: '测试用户', balance: 128.5 },
    '/usage/dashboard/stats': {
      total_api_keys: 2, active_api_keys: 2, today_requests: 2481,
      today_input_tokens: 910000, today_output_tokens: 510000,
      today_tokens: 1420000, today_actual_cost: 4.16,
      average_duration_ms: 842, rpm: 18, tpm: 12600,
    },
    '/usage/dashboard/trend': {
      trend: [{ date: new Date().toISOString().slice(0, 10), requests: 2481, total_tokens: 1420000 }],
    },
    '/usage/dashboard/models': {
      models: [{ model: 'gpt-4.1', requests: 460 }, { model: 'claude-sonnet-4', requests: 280 }],
    },
    '/settings/public': { site_name: '测试中转站', api_base_url: 'https://api.example.test' },
    '/settings/home-models': modelCatalog,
    '/keys': {
      items: [
        { id: 1, name: '主密钥', key: 'sk-test-abcdef123456', status: 'active', group: { name: 'OpenAI 专线', platform: 'openai' } },
        { id: 2, name: 'Claude 密钥', key: 'sk-ant-test-654321', status: 'active', group: { name: 'Anthropic 专线', platform: 'anthropic' } },
      ],
    },
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((complete) => { resolve = complete })
  return { promise, resolve }
}

const overlays: ReturnType<typeof createSidebarPageOverlay>[] = []
let copy: ReturnType<typeof vi.fn>
let staticCatalogFetch: ReturnType<typeof vi.fn>

function mountOverlay(overrides: Record<string, unknown> = {}) {
  const target = document.createElement('div')
  document.body.appendChild(target)
  const data = { ...apiData(), ...overrides }
  const request = vi.fn(async (path: string) => {
    const pathname = new URL(path, 'https://example.test').pathname
    if (!(pathname in data)) throw new Error(`未模拟接口：${pathname}`)
    return data[pathname]
  })
  const navigate = vi.fn()
  const overlay = createSidebarPageOverlay({ target, request, navigate })
  overlays.push(overlay)
  return { target, request, navigate, overlay }
}

function click(target: HTMLElement, selector: string) {
  const button = target.querySelector<HTMLElement>(selector)
  expect(button, `缺少操作入口：${selector}`).not.toBeNull()
  button!.click()
}

beforeEach(() => {
  copy = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText: copy } })
  staticCatalogFetch = vi.fn().mockResolvedValue({ ok: true, json: async () => [] })
  vi.stubGlobal('fetch', staticCatalogFetch)
})

afterEach(() => {
  for (const overlay of overlays) overlay.destroy()
  overlays.length = 0
  document.body.replaceChildren()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('仪表盘页面模块', () => {
  it('展示真实余额、用量和模型统计，同视图重复渲染不重复请求', async () => {
    const { target, request, overlay } = mountOverlay()
    overlay.render('dashboard')
    expect(target.querySelector('[data-dashboard-loading]')).not.toBeNull()
    await flushPromises()

    expect(target.querySelector('[data-dashboard-balance]')?.textContent).toBe('$128.50')
    expect(target.querySelector('[data-dashboard-requests]')?.textContent).toBe('2,481')
    expect(target.querySelector('[data-dashboard-token-note]')?.textContent).toContain('输入 910K · 输出 510K')
    expect(target.querySelector('.s2-donut-copy')?.textContent).toContain('gpt-4.1')
    expect(target.querySelectorAll('[data-dashboard-model-legend]')).toHaveLength(2)

    const requestCount = request.mock.calls.length
    overlay.render('dashboard')
    await flushPromises()
    expect(request).toHaveBeenCalledTimes(requestCount)
  })

  it('切换 30 天会查询完整日期范围，重复点击不重复加载', async () => {
    const { target, request, overlay } = mountOverlay()
    overlay.render('dashboard')
    await flushPromises()
    click(target, '[data-dashboard-days="30"]')
    await flushPromises()

    const trendRequests = request.mock.calls.filter(([path]) => path.startsWith('/usage/dashboard/trend'))
    expect(trendRequests).toHaveLength(2)
    const query = new URL(trendRequests[1][0], 'https://example.test').searchParams
    const start = Date.parse(`${query.get('start_date')}T00:00:00Z`)
    const end = Date.parse(`${query.get('end_date')}T00:00:00Z`)
    expect((end - start) / 86400000).toBe(29)
    expect(target.querySelector('[data-dashboard-days="30"]')?.getAttribute('aria-pressed')).toBe('true')
    expect(request.mock.calls.some(([path]) => path.startsWith('/keys'))).toBe(false)

    const requestCount = request.mock.calls.length
    click(target, '[data-dashboard-days="30"]')
    await flushPromises()
    expect(request).toHaveBeenCalledTimes(requestCount)
  })

  it('空用量显示开始调用入口，并保留真实账户余额', async () => {
    const { target, overlay } = mountOverlay({
      '/usage/dashboard/stats': { today_requests: 0, today_tokens: 0 },
      '/usage/dashboard/trend': { trend: [] },
      '/usage/dashboard/models': { models: [] },
    })
    overlay.render('dashboard')
    await flushPromises()
    expect(target.querySelector('[data-dashboard-balance]')?.textContent).toBe('$128.50')
    expect(target.querySelector('.s2-dashboard-empty')?.textContent).toContain('第一次模型调用')
    expect(target.querySelector('.s2-donut-wrap.is-empty')).not.toBeNull()
  })

  it('完整走通凭证复制、应用配置、分组模型选择和完成步骤', async () => {
    const { target, navigate, request, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()

    expect(target.querySelectorAll('.s2-steps-list [data-guide-step]')).toHaveLength(4)
    expect(target.textContent).not.toContain('sk-test-abcdef123456')
    click(target, '[data-copy-secret]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith('sk-test-abcdef123456')
    click(target, '[data-copy-text="https://api.example.test"]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith('https://api.example.test')

    click(target, '[data-guide-step="2"]')
    expect(target.querySelectorAll('[data-guide-app]').length).toBeGreaterThanOrEqual(6)
    expect(target.querySelector('.s2-app-detail')?.textContent).toContain('OpenAI 密钥 → Codex')
    click(target, '[data-guide-app="codex"]')
    click(target, '[data-guide-app-action="codex"]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith(expect.stringContaining('Base URL: https://api.example.test/v1'))
    expect(copy).toHaveBeenLastCalledWith(expect.stringContaining('API Key: sk-test-abcdef123456'))

    click(target, '[data-guide-step="3"]')
    expect(request.mock.calls.filter(([path]) => path === '/settings/home-models')).toHaveLength(1)
    expect(staticCatalogFetch).not.toHaveBeenCalledWith('/model-data.json', expect.anything())
    expect(target.querySelector('[data-guide-model="gpt-5.5"] .s2-model-price')?.textContent).toContain('输入 $5')
    expect(Array.from(target.querySelectorAll<HTMLElement>('[data-guide-model]'), (item) => item.dataset.guideModel))
      .toEqual(['gpt-5.5', 'gpt-5.4-mini'])
    click(target, '[data-guide-model="gpt-5.4-mini"]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith('gpt-5.4-mini')

    click(target, '[data-guide-step="1"]')
    click(target, '[data-guide-key-toggle]')
    expect(target.querySelector('[data-guide-key-toggle]')?.getAttribute('aria-expanded')).toBe('true')
    click(target, '[data-guide-key-option="2"]')
    click(target, '[data-copy-secret]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith('sk-ant-test-654321')
    click(target, '[data-guide-step="3"]')
    expect(Array.from(target.querySelectorAll<HTMLElement>('[data-guide-model]'), (item) => item.dataset.guideModel))
      .toEqual(['claude-sonnet-4-6'])
    click(target, '[data-guide-step="4"]')
    expect(target.querySelector('.s2-guide-panel')?.textContent).toContain('接入完成')
    click(target, '[data-route="/usage"]')
    expect(navigate).toHaveBeenLastCalledWith('/usage')
  })

  it('模型目录价格保留极小值并区分空价与零价', async () => {
    const { target, overlay } = mountOverlay({
      '/settings/home-models': [
        { name: 'gpt-tiny', vendor: 'OpenAI', type: 'text', input: 0.0001, output: 0 },
        { name: 'gpt-empty-input', vendor: 'OpenAI', type: 'text', input: null, output: 0 },
      ],
    })
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="3"]')

    expect(target.querySelector('[data-guide-model="gpt-tiny"] .s2-model-price')?.textContent)
      .toContain('输入 $0.0001 · 输出 $0')
    expect(target.querySelector('[data-guide-model="gpt-empty-input"] .s2-model-price')?.textContent)
      .toContain('输出 $0')
    expect(target.querySelector('[data-guide-model="gpt-empty-input"] .s2-model-price')?.textContent)
      .not.toContain('输入 $0')
  })

  it('API 模式按协议生成并复制本站请求代码', async () => {
    const { target, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="2"]')
    click(target, '[data-guide-mode="api"]')
    click(target, '[data-guide-protocol="anthropic"]')
    expect(target.querySelector('.s2-code')?.textContent).toContain('https://api.example.test/v1/messages')
    click(target, '.s2-guide-panel [data-copy-text]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith(expect.stringContaining('x-api-key: YOUR_API_KEY'))
  })

  it('CC Switch 导入携带当前平台、站点地址和用量查询配置', async () => {
    const assign = vi.fn()
    const browserWindow = window
    vi.stubGlobal('window', new Proxy(browserWindow, {
      get(target, property) {
        if (property === 'location') return { origin: browserWindow.location.origin, assign }
        return Reflect.get(target, property)
      },
    }))
    const { target, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="2"]')
    click(target, '[data-guide-app-action="ccswitch"]')
    await vi.waitFor(() => expect(assign).toHaveBeenCalledOnce())

    const deepLink = new URL(assign.mock.calls[0][0])
    expect(deepLink.protocol).toBe('ccswitch:')
    expect(deepLink.searchParams.get('app')).toBe('codex')
    expect(deepLink.searchParams.get('endpoint')).toBe('https://api.example.test/v1')
    expect(deepLink.searchParams.get('apiKey')).toBe('sk-test-abcdef123456')
    expect(deepLink.searchParams.get('usageEnabled')).toBe('true')
    expect(deepLink.searchParams.get('usageAutoInterval')).toBe('30')
    expect(atob(deepLink.searchParams.get('usageScript')!)).toContain('/v1/usage')
  })

  it('页面跳转交给主站路由，不改动其他 main 和侧栏节点', async () => {
    const outside = document.createElement('main')
    outside.innerHTML = '<a class="sidebar-link" href="/keys">原有密钥菜单</a><section>主站内容</section>'
    document.body.appendChild(outside)
    const oldMenu = outside.querySelector('a')
    const previousHtml = outside.outerHTML
    const { target, navigate, overlay } = mountOverlay()
    overlay.render('dashboard')
    await flushPromises()
    click(target, '[data-route="/keys"]')
    expect(navigate).toHaveBeenLastCalledWith('/keys')
    click(target, '[data-overlay-view="guide"]')
    expect(navigate).toHaveBeenLastCalledWith('/dashboard?view=guide')
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-overlay-view="dashboard"]')
    expect(navigate).toHaveBeenLastCalledWith('/dashboard')
    overlay.destroy()
    expect(outside.outerHTML).toBe(previousHtml)
    expect(outside.querySelector('a')).toBe(oldMenu)
    expect(target.childElementCount).toBe(0)
  })

  it('快速切换视图后忽略上一视图迟到的数据', async () => {
    const lateStats = deferred<unknown>()
    const { target, overlay } = mountOverlay({ '/usage/dashboard/stats': lateStats.promise })
    overlay.render('dashboard')
    overlay.render('guide')
    await flushPromises()
    expect(target.querySelector('[data-sub2api-page="guide"]')).not.toBeNull()
    lateStats.resolve({ today_requests: 999999 })
    await flushPromises()
    expect(target.querySelector('[data-sub2api-page="guide"]')).not.toBeNull()
    expect(target.querySelector('[data-sub2api-page="dashboard"]')).toBeNull()
    expect(target.querySelector('[data-guide-key-toggle]')?.textContent).toContain('主密钥')
  })

  it('卸载后迟到的接口和复制结果不能重新插入页面或提示', async () => {
    const pendingCopy = deferred<void>()
    copy.mockReturnValueOnce(pendingCopy.promise)
    const { target, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-copy-secret]')
    overlay.destroy()
    pendingCopy.resolve(undefined)
    await flushPromises()
    expect(target.childElementCount).toBe(0)
    expect(document.querySelector('.s2-toast')).toBeNull()

    const lateStats = deferred<unknown>()
    const pending = mountOverlay({ '/usage/dashboard/stats': lateStats.promise })
    pending.overlay.render('dashboard')
    pending.overlay.destroy()
    lateStats.resolve({ today_requests: 456 })
    await flushPromises()
    expect(pending.target.childElementCount).toBe(0)
  })
})
