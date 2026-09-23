import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { runInNewContext } from 'node:vm'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const homeHtml = readFileSync(resolve(__dirname, '../../../public/i2.html'), 'utf8')
const homeScript = readFileSync(resolve(__dirname, '../../../public/i2.js'), 'utf8')

const defaultModels = [{
  name: 'gpt-test', vendor: 'openai', type: 'text',
  input: 1, output: 2, cachedInput: null, flexInput: null,
}]

function createBrandHome(embedded = false, responseBody: unknown = { code: 0, data: defaultModels }) {
  const homeDocument = document.implementation.createHTMLDocument()
  homeDocument.documentElement.innerHTML = homeHtml
  const parentDocument = document.implementation.createHTMLDocument()
  const handlers = new Map<string, () => void>()
  const pageLocation = { hash: '', reload: vi.fn() }
  const pageWindow: Record<string, unknown> = {
    document: homeDocument,
    addEventListener: vi.fn((name: string, handler: () => void) => handlers.set(name, handler)),
    scrollTo: vi.fn(),
    matchMedia: vi.fn(() => ({ matches: false })),
  }
  pageWindow.parent = embedded ? { document: parentDocument } : pageWindow
  const fetchModels = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => responseBody,
  })

  // 在独立文档运行静态脚本，检查直接访问和 iframe 两种真实入口。
  runInNewContext(homeScript, {
    document: homeDocument,
    window: pageWindow,
    location: pageLocation,
    localStorage,
    fetch: fetchModels,
    AbortSignal,
  })

  return { homeDocument, parentDocument, pageLocation, handlers, fetchModels }
}

describe('品牌首页静态页面集成', () => {
  beforeEach(() => localStorage.clear())

  it('通过同源外部脚本加载，兼容禁止内联脚本的 CSP', () => {
    const { homeDocument } = createBrandHome()
    const scripts = Array.from(homeDocument.querySelectorAll('script'))

    expect(scripts).toHaveLength(1)
    expect(scripts[0]?.getAttribute('src')).toBe('/i2.js')
    expect(scripts[0]?.hasAttribute('defer')).toBe(true)
    expect(scripts[0]?.textContent?.trim()).toBe('')
    expect(homeHtml).not.toMatch(/\son\w+\s*=/i)
  })

  it('直接访问仍能加载模型目录和切换模型广场', async () => {
    const { homeDocument, pageLocation, handlers, fetchModels } = createBrandHome()

    await vi.waitFor(() => expect(homeDocument.querySelector('.model-name')?.textContent).toBe('gpt-test'))
    expect(fetchModels).toHaveBeenCalledWith('/api/v1/settings/home-models', { cache: 'no-store', signal: expect.any(AbortSignal) })
    expect(homeDocument.querySelector('#modelCount')?.textContent).toBe('1')
    pageLocation.hash = '#pricing'
    handlers.get('hashchange')?.()

    expect(homeDocument.querySelector('#pricingView')?.hasAttribute('hidden')).toBe(false)
    expect(homeDocument.querySelector('#homeView')?.hasAttribute('hidden')).toBe(true)
  })

  it('应用导航使用顶层窗口，首页和模型广场继续在当前页面切换', () => {
    const { homeDocument } = createBrandHome(true)
    const appLinks = Array.from(homeDocument.querySelectorAll<HTMLAnchorElement>('a[href^="/"]'))
    const viewLinks = Array.from(homeDocument.querySelectorAll<HTMLAnchorElement>('a[href^="#"]'))

    expect(appLinks.map(link => link.getAttribute('href'))).toContain('/login')
    expect(appLinks.map(link => link.getAttribute('href'))).toContain('/register')
    expect(appLinks.every(link => link.target === '_top')).toBe(true)
    expect(viewLinks.length).toBeGreaterThan(0)
    expect(viewLinks.every(link => !link.target)).toBe(true)
  })

  it('展示后台文本和图片价格、零价、空价以及自定义厂商', async () => {
    const { homeDocument } = createBrandHome(false, { code: 0, data: [
      { ...defaultModels[0], input: 0, output: 0.000001, cachedInput: null, flexInput: 0 },
      { name: '自定义图片模型', vendor: '自定义厂商', type: 'image', resolutionPrices: { '1K': 0, '2K': 0.25, '4K': 1.125 } },
    ] })
    await vi.waitFor(() => expect(homeDocument.querySelectorAll('.model-card')).toHaveLength(2))
    const cards = homeDocument.querySelectorAll('.model-card')
    expect(cards[0]?.textContent).toContain('$0/M')
    expect(cards[0]?.textContent).toContain('$0.000001/M')
    expect(cards[0]?.textContent).toContain('缓存输入—/M')
    expect(cards[1]?.textContent).toContain('自定义厂商')
    expect(cards[1]?.textContent).toContain('$0.25')
    expect(cards[1]?.textContent).toContain('$1.125')
    expect(homeDocument.querySelector('#modelCount')?.textContent).toBe('2')
  })

  it('后台清空目录后展示空状态，不恢复静态模型', async () => {
    const { homeDocument, fetchModels } = createBrandHome(false, { code: 0, data: [] })
    await vi.waitFor(() => expect(homeDocument.querySelector('#modelGrid')?.textContent).toBe('暂无模型数据'))
    expect(homeDocument.querySelector('#modelCount')?.textContent).toBe('0')
    expect(fetchModels).toHaveBeenCalledOnce()
  })

  it('后台配置的模型名和厂商按文本展示', async () => {
    const modelName = '<img src=x onerror=alert(1)>'
    const vendorName = '<svg onload=alert(1)>'
    const { homeDocument } = createBrandHome(false, { code: 0, data: [
      { ...defaultModels[0], name: modelName, vendor: vendorName },
    ] })
    await vi.waitFor(() => expect(homeDocument.querySelector('.model-name')?.textContent).toBe(modelName))
    expect(homeDocument.querySelector('.vendor-label')?.textContent).toBe(vendorName)
    expect(homeDocument.querySelector('#modelGrid img, #modelGrid [onload], #modelGrid [onerror]')).toBeNull()
  })

  it.each([
    { code: 1, data: defaultModels },
    { code: 0, data: [{ ...defaultModels[0], input: -1 }] },
  ])('接口失败或价格非法时显示重试，恢复后展示最新报价', async (payload) => {
    const { homeDocument, fetchModels } = createBrandHome(false, payload)
    await vi.waitFor(() => expect(homeDocument.querySelector('[data-retry-models]')).not.toBeNull())
    expect(homeDocument.querySelectorAll('.model-card')).toHaveLength(0)
    expect(homeDocument.querySelector('#modelCount')?.textContent).toBe('—')
    fetchModels.mockResolvedValueOnce({ ok: true, json: async () => ({ code: 0, data: [{ ...defaultModels[0], input: 9.99 }] }) })
    homeDocument.querySelector<HTMLButtonElement>('[data-retry-models]')?.click()
    await vi.waitFor(() => expect(homeDocument.querySelector('.model-card')?.textContent).toContain('$9.99/M'))
    expect(fetchModels).toHaveBeenCalledTimes(2)
  })

  it('管理员控制台入口指向顶层管理仪表盘', () => {
    localStorage.setItem('auth_token', 'test-token')
    localStorage.setItem('auth_user', JSON.stringify({ role: 'admin' }))
    const { homeDocument } = createBrandHome(true)

    for (const link of homeDocument.querySelectorAll<HTMLAnchorElement>('.auth-only, #primaryCta')) {
      expect(link.getAttribute('href')).toBe('/admin/dashboard')
      expect(link.target).toBe('_top')
      expect(link.hidden).toBe(false)
    }
  })

  it('内嵌页面切换主题时同步父文档和持久化设置', () => {
    const { homeDocument, parentDocument } = createBrandHome(true)
    homeDocument.querySelector<HTMLButtonElement>('#themeButton')?.click()

    expect(homeDocument.documentElement.classList.contains('dark')).toBe(true)
    expect(parentDocument.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')

    homeDocument.querySelector<HTMLButtonElement>('#themeButton')?.click()
    expect(parentDocument.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem('theme')).toBe('light')
  })
})
