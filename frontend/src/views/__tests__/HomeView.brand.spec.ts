import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import HomeView from '../HomeView.vue'

const { appStore, authStore, changeLocale } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: {} as Record<string, unknown>, siteName: '众智AI', siteLogo: '', docUrl: '',
    publicSettingsLoaded: true, fetchPublicSettings: vi.fn(),
  },
  authStore: { isAuthenticated: false, isAdmin: false, user: null, checkAuth: vi.fn() },
  changeLocale: vi.fn(),
}))
vi.mock('@/stores', () => ({ useAppStore: () => appStore, useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/i18n', () => ({ setLocale: changeLocale }))

const defaultModels = [{
  name: 'gpt-test', vendor: 'openai', type: 'text',
  input: 1, output: 2, cachedInput: null, flexInput: null,
}]
const wrappers: VueWrapper[] = []
const fetchModels = vi.fn()

async function mountHome(path = '/home', payload: unknown = { code: 0, data: defaultModels }) {
  fetchModels.mockResolvedValueOnce({ ok: true, json: async () => payload })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: HomeView },
      { path: '/home', component: HomeView },
      ...['/login', '/register', '/dashboard', '/admin/dashboard'].map(route => ({ path: route, component: { template: '<div />' } })),
    ],
  })
  const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh: {}, en: {} } })
  changeLocale.mockImplementation(async (locale: 'zh' | 'en') => { i18n.global.locale.value = locale })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(HomeView, {
    global: { plugins: [router, i18n], stubs: { LocaleSwitcher: true, Icon: true } },
  })
  wrappers.push(wrapper)
  await flushPromises()
  return { wrapper, router }
}

describe('品牌首页原生 Home 集成', () => {
  beforeEach(() => {
    fetchModels.mockReset()
    changeLocale.mockReset()
    appStore.cachedPublicSettings = {}
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    localStorage.clear()
    document.documentElement.classList.remove('dark')
    vi.stubGlobal('fetch', fetchModels)
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList)
  })
  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it.each(['/', '/home'])('%s 直接渲染首页内容，不创建 iframe 或请求静态首页', async path => {
    const { wrapper } = await mountHome(path)
    expect(wrapper.get('[data-testid="brand-home"]').element.tagName).toBe('DIV')
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.get('h1').text()).toBe('众智AI')
    expect(wrapper.findAll('.feature-card')).toHaveLength(4)
    expect(fetchModels).toHaveBeenCalledOnce()
    expect(fetchModels).toHaveBeenCalledWith('/api/v1/settings/home-models', { cache: 'no-store', signal: expect.any(AbortSignal) })
    expect(wrapper.get('#modelCount').text()).toBe('1')
  })

  it.each([' /i2.html ', '/i2.html?version=old', '/i2.html#pricing'])('兼容旧首页配置 %s，直接渲染 Home', async address => {
    appStore.cachedPublicSettings = { home_content: address }
    const { wrapper } = await mountHome()
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('[data-testid="brand-home"]').exists()).toBe(true)
    expect(fetchModels).toHaveBeenCalledOnce()
  })

  it('模型广场和浏览器历史都由应用路由切换', async () => {
    const { wrapper, router } = await mountHome()
    await wrapper.get('.desktop-nav a[href="/home#pricing"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.hash).toBe('#pricing')
    expect(wrapper.get('#pricingView').attributes('hidden')).toBeUndefined()
    expect(wrapper.get('#homeView').attributes('hidden')).toBeDefined()
    router.back()
    await flushPromises()
    expect(wrapper.get('#homeView').attributes('hidden')).toBeUndefined()
    expect(wrapper.get('#pricingView').attributes('hidden')).toBeDefined()
    router.forward()
    await flushPromises()
    expect(wrapper.get('#pricingView').attributes('hidden')).toBeUndefined()
    expect(fetchModels).toHaveBeenCalledOnce()
  })

  it('旧品牌配置保留自定义首页优先级及初始模型广场入口', async () => {
    appStore.cachedPublicSettings = { home_content: '/i2.html#pricing', compact_home_enabled: true }
    const { wrapper, router } = await mountHome('/home?view=classic')
    expect(wrapper.find('[data-testid="brand-home"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
    expect(wrapper.find('.terminal-container').exists()).toBe(false)
    expect(wrapper.get('#pricingView').attributes('hidden')).toBeUndefined()
    await router.push('/home?view=classic#home')
    expect(wrapper.get('#homeView').attributes('hidden')).toBeUndefined()
  })

  it('直接进入模型广场可展示后台文本、图片、零价、空价和微小价格并筛选', async () => {
    const { wrapper } = await mountHome('/home#pricing', { code: 0, data: [
      { ...defaultModels[0], input: 0, output: 0.000001, cachedInput: null, flexInput: 0 },
      { name: '自定义图片模型', vendor: '自定义厂商', type: 'image', resolutionPrices: { '1K': 0, '2K': 0.25, '4K': 1.125 } },
    ] })
    const cards = wrapper.findAll('.model-card')
    expect(cards).toHaveLength(2)
    expect(cards[0]?.text()).toContain('$0/M')
    expect(cards[0]?.text()).toContain('$0.000001/M')
    expect(cards[0]?.text()).toContain('缓存输入—/M')
    expect(cards[1]?.text()).toContain('自定义厂商')
    expect(cards[1]?.text()).toContain('$0.25')
    expect(cards[1]?.text()).toContain('$1.125')
    expect(wrapper.get('#modelCount').text()).toBe('2')
    await wrapper.get('[data-filter-type="image"]').trigger('click')
    expect(wrapper.findAll('.model-card')).toHaveLength(1)
    expect(wrapper.get('.model-name').text()).toBe('自定义图片模型')
    expect(wrapper.get('[data-filter-type="image"]').attributes('aria-pressed')).toBe('true')
  })

  it('后台清空目录后展示空状态，不恢复静态模型', async () => {
    const { wrapper } = await mountHome('/home#pricing', { code: 0, data: [] })
    expect(wrapper.get('#modelGrid').text()).toBe('暂无模型数据')
    expect(wrapper.get('#modelCount').text()).toBe('0')
    expect(wrapper.findAll('.model-card')).toHaveLength(0)
  })

  it('后台模型名和厂商只按文本展示', async () => {
    const modelName = '<img src=x onerror=alert(1)>'
    const vendorName = '<svg onload=alert(1)>'
    const { wrapper } = await mountHome('/home#pricing', { code: 0, data: [{ ...defaultModels[0], name: modelName, vendor: vendorName }] })
    expect(wrapper.get('.model-name').text()).toBe(modelName)
    expect(wrapper.get('.vendor-label').text()).toBe(vendorName)
    expect(wrapper.find('#modelGrid img, #modelGrid [onload], #modelGrid [onerror]').exists()).toBe(false)
  })

  it.each([
    { code: 1, data: defaultModels },
    { code: 0, data: [{ ...defaultModels[0], input: -1 }] },
  ])('接口失败或价格非法时可重试并恢复最新报价', async payload => {
    const { wrapper } = await mountHome('/home#pricing', payload)
    expect(wrapper.findAll('.model-card')).toHaveLength(0)
    expect(wrapper.get('#modelCount').text()).toBe('—')
    fetchModels.mockResolvedValueOnce({ ok: true, json: async () => ({ code: 0, data: [{ ...defaultModels[0], input: 9.99 }] }) })
    await wrapper.get('[data-retry-models]').trigger('click')
    await flushPromises()
    expect(wrapper.get('.model-card').text()).toContain('$9.99/M')
    expect(fetchModels).toHaveBeenCalledTimes(2)
  })

  it.each([false, true])('登录用户的控制台路径匹配管理员状态 %s', async isAdmin => {
    authStore.isAuthenticated = true
    authStore.isAdmin = isAdmin
    const { wrapper } = await mountHome()
    for (const link of wrapper.findAll('.auth-only, #primaryCta')) {
      expect(link.attributes('href')).toBe(isAdmin ? '/admin/dashboard' : '/dashboard')
    }
    expect(wrapper.find('.guest-only').exists()).toBe(false)
  })

  it('游客登录注册导航与移动菜单保留，点击后菜单关闭', async () => {
    const { wrapper } = await mountHome()
    expect(wrapper.get('.login-button').attributes('href')).toBe('/login')
    expect(wrapper.get('.register-button').attributes('href')).toBe('/register')
    expect(wrapper.get('#primaryCta').attributes('href')).toBe('/register')
    await wrapper.get('#menuButton').trigger('click')
    expect(wrapper.get('#menuButton').attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('#mobileNav').attributes('hidden')).toBeUndefined()
    await wrapper.get('#mobileNav a[href="/home#pricing"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('#mobileNav').attributes('hidden')).toBeDefined()
  })

  it('主题更新当前应用文档，语言通过应用国际化同步', async () => {
    const { wrapper } = await mountHome()
    await wrapper.get('#themeButton').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(wrapper.get('.brand-home').classes()).toContain('is-dark')
    expect(localStorage.getItem('theme')).toBe('dark')
    await wrapper.get('#languageButton').trigger('click')
    await flushPromises()
    expect(changeLocale).toHaveBeenCalledWith('en')
    expect(wrapper.get('.login-button').text()).toBe('Log in')
    await wrapper.get('#themeButton').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('离开首页终止尚未完成的模型请求', async () => {
    fetchModels.mockImplementationOnce(() => new Promise(() => {}))
    const { wrapper } = await mountHome()
    const signal = fetchModels.mock.calls[0]?.[1].signal as AbortSignal
    expect(signal.aborted).toBe(false)
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
  })
})
