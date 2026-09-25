import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatDateLocalInput } from '@/utils/format'
import { createSidebarPageOverlay } from '../sidebar-overlay.js'

const modelCatalog = [
  { name: 'gpt-5.5', vendor: 'OpenAI', type: 'text', input: 5, output: 30 },
  { name: 'gpt-5.4-mini', vendor: 'OpenAI', type: 'text', input: 0.75, output: 4.5 },
  { name: 'claude-sonnet-4-6', vendor: 'ANTHROPIC', type: 'text', input: 3, output: 15 },
]

function dateAt(offset: number) {
  const date = new Date()
  date.setDate(date.getDate() + offset)
  return formatDateLocalInput(date)
}

function statsPath(offset: number) {
  const date = dateAt(offset)
  return `/usage/stats?start_date=${date}&end_date=${date}`
}

function apiData(): Record<string, unknown> {
  return {
    '/auth/me': { username: '测试用户', balance: 128.5 },
    '/usage/dashboard/stats': {
      total_api_keys: 2, active_api_keys: 2, today_requests: 2481,
      today_input_tokens: 910000, today_output_tokens: 510000,
      today_tokens: 1420000, today_actual_cost: 4.16,
      average_duration_ms: 842, rpm: 18, tpm: 12600,
    },
    [statsPath(0)]: {
      total_requests: 210, total_tokens: 11900000,
      total_input_tokens: 9100000, total_output_tokens: 2800000, average_duration_ms: 17280,
    },
    [statsPath(-1)]: {
      total_requests: 200, total_tokens: 14000000, average_duration_ms: 21600,
    },
    '/usage/dashboard/trend': {
      trend: [{ date: dateAt(0), requests: 2481, total_tokens: 1420000 }],
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
let modelListFetch: ReturnType<typeof vi.fn>

function mountOverlay(overrides: Record<string, unknown> = {}) {
  const target = document.createElement('div')
  document.body.appendChild(target)
  const data = { ...apiData(), ...overrides }
  const request = vi.fn(async (path: string) => {
    const url = new URL(path, 'https://example.test')
    const endpoint = url.pathname === '/usage/stats'
      ? `${url.pathname}?start_date=${url.searchParams.get('start_date')}&end_date=${url.searchParams.get('end_date')}`
      : url.pathname
    if (!(endpoint in data)) throw new Error(`未模拟接口：${endpoint}`)
    if (data[endpoint] instanceof Error) throw data[endpoint]
    return data[endpoint]
  })
  const navigate = vi.fn()
  const overlay = createSidebarPageOverlay({ target, request, navigate })
  overlays.push(overlay)
  return { target, request, navigate, overlay, data }
}

function click(target: HTMLElement, selector: string) {
  const button = target.querySelector<HTMLElement>(selector)
  expect(button, `缺少操作入口：${selector}`).not.toBeNull()
  button!.click()
}

function mockLocationAssign() {
  const assign = vi.fn()
  const browserWindow = window
  vi.stubGlobal('window', new Proxy(browserWindow, {
    get(target, property) {
      if (property === 'location') return { origin: browserWindow.location.origin, assign }
      return Reflect.get(target, property)
    },
  }))
  return assign
}

beforeEach(() => {
  copy = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText: copy } })
  modelListFetch = vi.fn(async (_url: string, options: RequestInit) => {
    const authorization = new Headers(options.headers).get('Authorization')
    const names = authorization === 'Bearer sk-ant-test-654321'
      ? ['claude-sonnet-4-6']
      : ['gpt-5.5', 'gpt-5.4-mini']
    return { ok: true, json: async () => ({ data: names.map((id) => ({ id })) }) }
  })
  vi.stubGlobal('fetch', modelListFetch)
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
    expect(target.querySelector('[data-dashboard-requests]')?.textContent).toBe('210')
    expect(target.querySelector('[data-dashboard-tokens]')?.textContent).toBe('11.90M')
    expect(target.querySelector('[data-dashboard-duration]')?.textContent).toBe('17.28s')
    expect(target.querySelector('[data-dashboard-change="requests"]')?.textContent).toBe('↑ 5%')
    expect(target.querySelector('[data-dashboard-change="tokens"]')?.textContent).toBe('↓ 15%')
    expect(target.querySelector('[data-dashboard-change="duration"]')?.textContent).toBe('↓ 20%')
    expect(request.mock.calls.filter(([path]) => path.startsWith('/usage/stats?')).map(([path]) => path))
      .toEqual(expect.arrayContaining([statsPath(0), statsPath(-1)]))
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
    expect(query.get('end_date')).toBe(dateAt(0))
    expect(target.querySelector('[data-dashboard-range]')?.getAttribute('data-dashboard-range')).toBe('30')
    expect(target.querySelector('[data-dashboard-start-date]')?.getAttribute('data-dashboard-start-date')).toBe(query.get('start_date'))
    expect(target.querySelectorAll('[data-trend-point]')).toHaveLength(30)
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
      [statsPath(0)]: { total_requests: 0, total_tokens: 0, average_duration_ms: 0 },
      [statsPath(-1)]: { total_requests: 0, total_tokens: 0, average_duration_ms: 0 },
      '/usage/dashboard/trend': { trend: [] },
      '/usage/dashboard/models': { models: [] },
    })
    overlay.render('dashboard')
    await flushPromises()
    expect(target.querySelector('[data-dashboard-balance]')?.textContent).toBe('$128.50')
    expect(target.querySelector('.s2-dashboard-empty')?.textContent).toContain('第一次模型调用')
    expect(target.querySelector('.s2-donut-wrap.is-empty')).not.toBeNull()
    expect(target.querySelector('[data-dashboard-donut]')?.textContent).toContain('0%')
    expect(target.querySelector('[data-dashboard-donut]')?.textContent).not.toContain('100%')
    expect(target.querySelector('[data-dashboard-change="requests"]')?.textContent).toMatch(/持平|0%|昨日暂无数据/)
    expect(target.querySelector('[data-dashboard-duration]')?.textContent).toBe('—')
    expect(target.querySelector('[data-dashboard-change="duration"]')?.textContent).toBe('今日暂无数据')
    expect(target.textContent).not.toMatch(/NaN|Infinity/)
  })

  it('昨日无调用时说明缺少比较基数，单日统计不可用时回退原统计', async () => {
    const { target, overlay, data } = mountOverlay({
      [statsPath(-1)]: { total_requests: 0, total_tokens: 0, average_duration_ms: 0 },
    })
    overlay.render('dashboard')
    await flushPromises()
    expect(target.querySelector('[data-dashboard-change="requests"]')?.textContent).toContain('昨日暂无数据')
    expect(target.querySelector('[data-dashboard-change="tokens"]')?.textContent).toContain('昨日暂无数据')
    expect(target.textContent).not.toMatch(/NaN|Infinity/)

    data[statsPath(0)] = new Error('今日统计暂不可用')
    data[statsPath(-1)] = new Error('昨日统计暂不可用')
    click(target, '[data-dashboard-days="30"]')
    await flushPromises()
    expect(target.querySelector('[data-dashboard-requests]')?.textContent).toBe('2,481')
    expect(target.querySelector('[data-dashboard-tokens]')?.textContent).toBe('1.42M')
    expect(target.querySelector('[data-dashboard-change="requests"]')?.textContent).toMatch(/暂无|无法/)
    expect(target.querySelector('[data-dashboard-token-note]')?.textContent).toContain('输入 910K · 输出 510K')
  })

  it('趋势分别绘制请求与 Token，鼠标和键盘都能查看当天真实数据', async () => {
    const { target, overlay } = mountOverlay({
      '/usage/dashboard/trend': {
        trend: [
          { date: dateAt(-2), requests: 100, total_tokens: 400000 },
          { date: dateAt(-1), requests: 300, total_tokens: 200000 },
          { date: dateAt(0), requests: 50, total_tokens: 900000 },
        ],
      },
    })
    overlay.render('dashboard')
    await flushPromises()
    const requests = target.querySelector('[data-trend-series="requests"]')
    const tokens = target.querySelector('[data-trend-series="tokens"]')
    expect(requests?.getAttribute('d')).toBeTruthy()
    expect(tokens?.getAttribute('d')).toBeTruthy()
    expect(requests?.getAttribute('d')).not.toBe(tokens?.getAttribute('d'))
    expect(target.querySelectorAll('[data-trend-point]')).toHaveLength(7)

    const today = target.querySelector<HTMLElement>(`[data-trend-point="${dateAt(0)}"]`)!
    expect(today.getAttribute('tabindex')).toBe('0')
    today.dispatchEvent(new FocusEvent('focusin', { bubbles: true }))
    let tooltip = target.querySelector('[data-trend-tooltip]')?.textContent || ''
    expect(tooltip).toContain(dateAt(0))
    expect(tooltip).toContain('50')
    expect(tooltip).toMatch(/900,000|900K/)

    const yesterday = target.querySelector(`[data-trend-point="${dateAt(-1)}"]`)!
    yesterday.dispatchEvent(new Event('pointerover', { bubbles: true }))
    tooltip = target.querySelector('[data-trend-tooltip]')?.textContent || ''
    expect(tooltip).toContain(dateAt(-1))
    expect(tooltip).toContain('300')
    expect(tooltip).toMatch(/200,000|200K/)
  })

  it('模型占比使用全部有效请求，前三名以外合并为其他且每段有独立百分比', async () => {
    const { target, overlay } = mountOverlay({
      '/usage/dashboard/models': {
        models: [
          { model: 'alpha', requests: 40 }, { model: 'beta', requests: 30 },
          { model: 'gamma', requests: 20 }, { model: 'delta', requests: 5 },
          { model: 'epsilon', requests: 5 }, { model: '无效请求', requests: -50 },
          { model: '仅 Token', total_tokens: 999999 }, { model: '无穷值', requests: Infinity },
        ],
      },
    })
    overlay.render('dashboard')
    await flushPromises()
    const legends = Array.from(target.querySelectorAll('[data-dashboard-model-legend]'))
    expect(legends).toHaveLength(4)
    expect(legends.map((item) => item.querySelector('.s2-donut-label')?.textContent))
      .toEqual(['alpha', 'beta', 'gamma', '其他'])
    expect(legends.map((item) => item.querySelector('.s2-donut-percent')?.textContent))
      .toEqual(['40%', '30%', '20%', '10%'])
    expect(target.querySelectorAll('.s2-donut-segment')).toHaveLength(4)
    expect(target.querySelector('[data-dashboard-donut]')?.textContent).toContain('100%')
    expect(target.querySelector('.s2-donut-copy')?.textContent).not.toMatch(/仅 Token|无效请求|无穷值/)
  })

  it('统计失败显示重试入口，恢复后读取真实数据', async () => {
    const { target, overlay, data } = mountOverlay({
      '/usage/dashboard/stats': new Error('暂不可用'),
      '/usage/dashboard/trend': new Error('暂不可用'),
      '/usage/dashboard/models': new Error('暂不可用'),
      [statsPath(0)]: new Error('暂不可用'),
      [statsPath(-1)]: new Error('暂不可用'),
    })
    overlay.render('dashboard')
    await flushPromises()
    expect(target.querySelector('[data-dashboard-retry]')).not.toBeNull()
    Object.assign(data, apiData())
    click(target, '[data-dashboard-retry]')
    await flushPromises()
    expect(target.querySelector('[data-dashboard-requests]')?.textContent).toBe('210')
    expect(target.querySelector('[data-dashboard-retry]')).toBeNull()
  })

  it('完整走通凭证复制、应用配置、分组模型选择和完成步骤', async () => {
    const { target, navigate, request, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()
    const scrollIntoView = vi.fn()
    Object.defineProperty(target.firstElementChild, 'scrollIntoView', { value: scrollIntoView })

    expect(target.querySelectorAll('.s2-steps-list [data-guide-step]')).toHaveLength(4)
    expect(target.textContent).not.toContain('sk-test-abcdef123456')
    click(target, '[data-copy-secret]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith('sk-test-abcdef123456')
    click(target, '[data-copy-text="https://api.example.test"]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith('https://api.example.test')

    click(target, '[data-guide-step="2"]')
    expect(scrollIntoView).toHaveBeenLastCalledWith({ block: 'start', behavior: 'auto' })
    expect(target.querySelectorAll('[data-guide-app]').length).toBeGreaterThanOrEqual(6)
    expect(target.querySelector('.s2-app-detail')?.textContent).toContain('OpenAI 密钥 → Codex')
    click(target, '[data-guide-app="codex"]')
    click(target, '[data-guide-app-action="codex"]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith(expect.stringContaining('Base URL: https://api.example.test/v1'))
    expect(copy).toHaveBeenLastCalledWith(expect.stringContaining('API Key: sk-test-abcdef123456'))

    click(target, '[data-guide-step="3"]')
    expect(request.mock.calls.filter(([path]) => path === '/settings/home-models')).toHaveLength(1)
    expect(modelListFetch).toHaveBeenCalledWith('https://api.example.test/v1/models', expect.objectContaining({
      headers: { Authorization: 'Bearer sk-test-abcdef123456' },
      signal: expect.any(AbortSignal),
    }))
    expect(modelListFetch).not.toHaveBeenCalledWith('/model-data.json', expect.anything())
    expect(target.querySelector('[data-guide-model="gpt-5.5"] .s2-model-price')?.textContent).toContain('输入 $5')
    expect(Array.from(target.querySelectorAll<HTMLElement>('[data-guide-model]'), (item) => item.dataset.guideModel))
      .toEqual(['gpt-5.5', 'gpt-5.4-mini'])
    click(target, '[data-guide-model="gpt-5.4-mini"]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith('gpt-5.4-mini')
    click(target, '[data-guide-step="2"]')
    click(target, '[data-guide-app-action="codex"]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith(expect.stringContaining('Model: gpt-5.4-mini'))

    click(target, '[data-guide-step="1"]')
    click(target, '[data-guide-key-toggle]')
    expect(target.querySelector('[data-guide-key-toggle]')?.getAttribute('aria-expanded')).toBe('true')
    click(target, '[data-guide-key-option="2"]')
    await flushPromises()
    click(target, '[data-copy-secret]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith('sk-ant-test-654321')
    click(target, '[data-guide-step="3"]')
    expect(Array.from(target.querySelectorAll<HTMLElement>('[data-guide-model]'), (item) => item.dataset.guideModel))
      .toEqual(['claude-sonnet-4-6'])
    click(target, '[data-guide-step="4"]')
    expect(target.querySelector('.s2-guide-panel')?.textContent).toContain('接入准备完成')
    click(target, '[data-route="/usage"]')
    expect(navigate).toHaveBeenLastCalledWith('/usage')
  })

  it('可用模型查询失败时提供重试，不把首页价格目录当成权限列表', async () => {
    modelListFetch.mockRejectedValueOnce(new Error('网络中断'))
    const { target, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="3"]')
    expect(target.querySelector('[data-guide-models-retry]')).not.toBeNull()
    expect(target.querySelectorAll('[data-guide-model]')).toHaveLength(0)

    modelListFetch.mockResolvedValueOnce({
      ok: true, json: async () => ({ data: [{ id: 'gpt-5.5' }, { id: 'custom-private-model' }] }),
    })
    click(target, '[data-guide-models-retry]')
    await flushPromises()
    expect(target.querySelector('[data-guide-models-retry]')).toBeNull()
    expect(Array.from(target.querySelectorAll<HTMLElement>('[data-guide-model]'), (item) => item.dataset.guideModel))
      .toEqual(['gpt-5.5', 'custom-private-model'])
    expect(target.querySelector('[data-guide-model="gpt-5.5"] .s2-model-price')?.textContent).toContain('输入 $5')
    expect(target.querySelector('[data-guide-model="gpt-5.4-mini"]')).toBeNull()
    expect(target.querySelector('[data-guide-model="custom-private-model"] .s2-model-price')?.textContent).not.toContain('$0')
  })

  it('接口返回空模型列表时保留空态，不插入硬编码推荐模型', async () => {
    modelListFetch.mockResolvedValueOnce({ ok: true, json: async () => ({ data: [] }) })
    const { target, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="3"]')
    expect(target.querySelectorAll('[data-guide-model]')).toHaveLength(0)
    expect(target.querySelector('.s2-guide-panel')?.textContent).toMatch(/暂无|暂未|没有/)
    expect(target.querySelector('.s2-guide-panel')?.textContent).not.toContain('gpt-5.5')
  })

  it.each(['加载中', '失败', '空列表'])('模型列表%s时禁用所有客户端配置入口，事件也不能绕过校验', async (state) => {
    const assign = mockLocationAssign()
    if (state === '加载中') modelListFetch.mockReturnValueOnce(deferred<unknown>().promise)
    else if (state === '失败') modelListFetch.mockRejectedValueOnce(new Error('网络中断'))
    else modelListFetch.mockResolvedValueOnce({ ok: true, json: async () => ({ data: [] }) })
    const { target, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="2"]')
    const apps = Array.from(target.querySelectorAll<HTMLElement>('[data-guide-app]'), (item) => item.dataset.guideApp)
    expect(apps).toHaveLength(6)
    for (const app of apps) {
      click(target, `[data-guide-app="${app}"]`)
      const action = target.querySelector<HTMLButtonElement>(`[data-guide-app-action="${app}"]`)!
      expect(action.disabled).toBe(true)
      action.disabled = false
      action.click()
    }
    await new Promise((resolve) => setTimeout(resolve, 70))
    expect(copy).not.toHaveBeenCalled()
    expect(assign).not.toHaveBeenCalled()
  })

  it.each(['/keys', '/settings/public'])('初始化 %s 失败后重新获取会恢复凭证和真实模型', async (failedEndpoint) => {
    const { target, overlay, request, data } = mountOverlay({ [failedEndpoint]: new Error('初始化接口暂不可用') })
    overlay.render('guide')
    await flushPromises()
    expect(target.querySelector('[role="alert"]')?.textContent).toContain('加载失败')
    expect(modelListFetch).not.toHaveBeenCalled()
    if (failedEndpoint === '/keys') click(target, '[data-guide-step="3"]')
    data[failedEndpoint] = apiData()[failedEndpoint]
    click(target, '[data-guide-models-retry]')
    await flushPromises()
    for (const endpoint of ['/keys', '/settings/public', '/settings/home-models']) {
      expect(request.mock.calls.filter(([path]) => new URL(path, 'https://example.test').pathname === endpoint))
        .toHaveLength(2)
    }
    expect(target.querySelector('[role="alert"]')).toBeNull()
    expect(modelListFetch).toHaveBeenCalledWith('https://api.example.test/v1/models', expect.objectContaining({
      headers: { Authorization: 'Bearer sk-test-abcdef123456' },
    }))
    click(target, '[data-guide-step="3"]')
    expect(target.querySelector('[data-guide-model="gpt-5.5"]')).not.toBeNull()
    click(target, '[data-guide-step="2"]')
    expect(target.querySelector<HTMLButtonElement>('[data-guide-app-action="ccswitch"]')?.disabled).toBe(false)
  })

  it('切换密钥会取消旧模型查询，迟到列表不能覆盖新密钥的可用模型', async () => {
    const pendingModels = deferred<{ ok: boolean; json: () => Promise<unknown> }>()
    modelListFetch.mockReturnValueOnce(pendingModels.promise)
    const { target, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()
    const firstOptions = modelListFetch.mock.calls[0][1] as RequestInit
    click(target, '[data-guide-key-toggle]')
    click(target, '[data-guide-key-option="2"]')
    await flushPromises()
    expect(firstOptions.signal?.aborted).toBe(true)
    expect(modelListFetch).toHaveBeenLastCalledWith('https://api.example.test/v1/models', expect.objectContaining({
      headers: { Authorization: 'Bearer sk-ant-test-654321' },
    }))
    click(target, '[data-guide-step="3"]')
    pendingModels.resolve({ ok: true, json: async () => ({ data: [{ id: 'gpt-5.5' }] }) })
    await flushPromises()
    expect(Array.from(target.querySelectorAll<HTMLElement>('[data-guide-model]'), (item) => item.dataset.guideModel))
      .toEqual(['claude-sonnet-4-6'])
  })

  it('模型目录价格保留极小值并区分空价与零价', async () => {
    modelListFetch.mockResolvedValueOnce({
      ok: true, json: async () => ({ data: [{ id: 'gpt-tiny' }, { id: 'gpt-empty-input' }] }),
    })
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

  it.each([
    ['openai', '/v1/chat/completions', 'Authorization: Bearer YOUR_API_KEY'],
    ['responses', '/v1/responses', 'Authorization: Bearer YOUR_API_KEY'],
    ['anthropic', '/v1/messages', 'x-api-key: YOUR_API_KEY'],
    ['gemini', '/v1beta/models/gemini-2.5-pro:generateContent', 'x-goog-api-key: YOUR_API_KEY'],
  ])('API 模式按 %s 协议生成并复制 PowerShell 7 请求代码', async (protocol, path, authHeader) => {
    const overrides: Record<string, unknown> = {}
    if (protocol === 'gemini') {
      modelListFetch.mockResolvedValueOnce({ ok: true, json: async () => ({ data: [{ id: 'gemini-2.5-pro' }] }) })
      overrides['/keys'] = { items: [{
        id: 3, name: 'Gemini 密钥', key: 'sk-gemini-test', status: 'active', group: { platform: 'gemini' },
      }] }
    }
    const { target, overlay } = mountOverlay(overrides)
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="2"]')
    click(target, '[data-guide-mode="api"]')
    click(target, `[data-guide-protocol="${protocol}"]`)
    expect(target.querySelector('.s2-code-toolbar')?.textContent).toContain('PowerShell 7 · curl.exe')
    const code = target.querySelector('.s2-code')?.textContent || ''
    expect(code).toContain(`curl.exe 'https://api.example.test${path}'`)
    expect(code).toContain('`\n  --request POST')
    expect(code).toContain(`--header '${authHeader}'`)
    expect(code).toContain("--data-raw '{")
    expect(code).not.toContain('^\n')
    expect(code).not.toContain('sk-test-abcdef123456')
    click(target, '.s2-guide-panel [data-copy-text]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith(code)
  })

  it('CC Switch 导入携带当前平台、所选模型、站点地址和用量查询配置', async () => {
    const assign = mockLocationAssign()
    const { target, overlay } = mountOverlay()
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="3"]')
    click(target, '[data-guide-model="gpt-5.4-mini"]')
    await flushPromises()
    click(target, '[data-guide-step="2"]')
    click(target, '[data-guide-app-action="ccswitch"]')
    await vi.waitFor(() => expect(assign).toHaveBeenCalledOnce())

    const deepLink = new URL(assign.mock.calls[0][0])
    expect(deepLink.protocol).toBe('ccswitch:')
    expect(deepLink.searchParams.get('app')).toBe('codex')
    expect(deepLink.searchParams.get('model')).toBe('gpt-5.4-mini')
    expect(deepLink.searchParams.get('endpoint')).toBe('https://api.example.test/v1')
    expect(deepLink.searchParams.get('apiKey')).toBe('sk-test-abcdef123456')
    expect(deepLink.searchParams.get('usageEnabled')).toBe('true')
    expect(deepLink.searchParams.get('usageAutoInterval')).toBe('30')
    expect(atob(deepLink.searchParams.get('usageScript')!)).toContain('/v1/usage')
  })

  it('Antigravity 密钥选择 Gemini 模型后导入 Gemini CLI 并保留专用地址', async () => {
    const assign = mockLocationAssign()
    modelListFetch.mockResolvedValueOnce({
      ok: true, json: async () => ({ data: [{ id: 'claude-sonnet-4-6' }, { id: 'gemini-2.5-pro' }] }),
    })
    const { target, overlay } = mountOverlay({
      '/settings/public': { site_name: '测试中转站', api_base_url: 'https://api.example.test/v1/' },
      '/keys': { items: [{
        id: 3, name: 'Antigravity 密钥', key: 'sk-antigravity-test', status: 'active',
        group: { name: '综合线路', platform: 'antigravity' },
      }] },
    })
    overlay.render('guide')
    await flushPromises()
    expect(modelListFetch).toHaveBeenCalledWith('https://api.example.test/antigravity/v1/models', expect.objectContaining({
      headers: { Authorization: 'Bearer sk-antigravity-test' },
    }))
    click(target, '[data-guide-step="3"]')
    click(target, '[data-guide-model="gemini-2.5-pro"]')
    await flushPromises()
    click(target, '[data-guide-step="2"]')
    expect(target.querySelector('.s2-app-detail')?.textContent).toContain('Gemini 密钥 → Gemini CLI')
    click(target, '[data-guide-app-action="ccswitch"]')
    await vi.waitFor(() => expect(assign).toHaveBeenCalledOnce())
    const deepLink = new URL(assign.mock.calls[0][0])
    expect(deepLink.searchParams.get('app')).toBe('gemini')
    expect(deepLink.searchParams.get('endpoint')).toBe('https://api.example.test/antigravity')
    expect(deepLink.searchParams.get('apiKey')).toBe('sk-antigravity-test')

    click(target, '[data-guide-app="claude"]')
    click(target, '[data-guide-app-action="claude"]')
    await flushPromises()
    expect(copy).toHaveBeenLastCalledWith(expect.stringContaining('ANTHROPIC_BASE_URL=https://api.example.test/antigravity'))
    click(target, '[data-guide-mode="api"]')
    for (const [protocol, path] of [
      ['anthropic', '/antigravity/v1/messages'],
      ['gemini', '/antigravity/v1beta/models/gemini-2.5-pro:generateContent'],
      ['openai', '/v1/chat/completions'],
      ['responses', '/v1/responses'],
    ]) {
      click(target, `[data-guide-protocol="${protocol}"]`)
      expect(target.querySelector('.s2-code')?.textContent).toContain(`curl.exe 'https://api.example.test${path}'`)
    }
  })

  it('Claude Code 专用分组明确限制普通 API 和不支持的客户端', async () => {
    modelListFetch.mockResolvedValueOnce({ ok: true, json: async () => ({ data: [{ id: 'claude-sonnet-4-6' }] }) })
    const { target, overlay } = mountOverlay({
      '/keys': { items: [{
        id: 3, name: 'Claude Code 专用密钥', key: 'sk-claude-only-test', status: 'active',
        group: { platform: 'anthropic', claude_code_only: true },
      }] },
    })
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="2"]')
    click(target, '[data-guide-app="codex"]')
    expect(target.querySelector('[role="status"]')?.textContent).toContain('仅允许 Claude Code')
    expect(target.querySelector<HTMLButtonElement>('[data-guide-app-action="codex"]')?.disabled).toBe(true)
    click(target, '[data-guide-app="claude"]')
    expect(target.querySelector<HTMLButtonElement>('[data-guide-app-action="claude"]')?.disabled).toBe(false)
    click(target, '[data-guide-mode="api"]')
    expect(target.querySelector('[role="status"]')?.textContent).toContain('仅允许 Claude Code')
    expect(target.querySelector('.s2-code')).toBeNull()
    expect(target.querySelector<HTMLButtonElement>('[data-guide-api-copy]')?.disabled).toBe(true)
    expect(target.querySelector<HTMLButtonElement>('[data-test-connection]')?.disabled).toBe(true)
  })

  it('OpenAI 分组关闭 Messages 转发后禁用 Claude 配置并说明不支持的原生协议', async () => {
    const { target, overlay } = mountOverlay({
      '/keys': { items: [{
        id: 1, name: 'OpenAI 密钥', key: 'sk-test-abcdef123456', status: 'active',
        group: { platform: 'openai', allow_messages_dispatch: false },
      }] },
    })
    overlay.render('guide')
    await flushPromises()
    click(target, '[data-guide-step="2"]')
    click(target, '[data-guide-app="claude"]')
    expect(target.querySelector('[role="status"]')?.textContent).toContain('未开启 Claude Code 接入')
    expect(target.querySelector<HTMLButtonElement>('[data-guide-app-action="claude"]')?.disabled).toBe(true)
    click(target, '[data-guide-app="codex"]')
    expect(target.querySelector<HTMLButtonElement>('[data-guide-app-action="codex"]')?.disabled).toBe(false)
    click(target, '[data-guide-mode="api"]')
    click(target, '[data-guide-protocol="anthropic"]')
    expect(target.querySelector('[role="status"]')?.textContent).toContain('未开启 Anthropic Messages 接入')
    expect(target.querySelector<HTMLButtonElement>('[data-guide-api-copy]')?.disabled).toBe(true)
    click(target, '[data-guide-protocol="gemini"]')
    expect(target.querySelector('[role="status"]')?.textContent).toContain('不支持 Gemini 原生协议')
    expect(target.querySelector('.s2-code')).toBeNull()
    expect(target.querySelector<HTMLButtonElement>('[data-guide-api-copy]')?.disabled).toBe(true)
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
