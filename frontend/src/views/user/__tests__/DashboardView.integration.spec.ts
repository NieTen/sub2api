import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { RouterView, createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DashboardView from '../DashboardView.vue'

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  refreshUser: vi.fn(),
  getDashboardStats: vi.fn(),
  getDashboardTrend: vi.fn(),
  getDashboardModels: vi.fn(),
  getByDateRange: vi.fn(),
  getMyPlatformQuotas: vi.fn(),
  user: { username: '集成测试用户', balance: 256.75 },
}))

vi.mock('@/api/client', () => ({ default: { get: mocks.get } }))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: mocks.user, refreshUser: mocks.refreshUser, isSimpleMode: false }),
}))
vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardStats: mocks.getDashboardStats,
    getDashboardTrend: mocks.getDashboardTrend,
    getDashboardModels: mocks.getDashboardModels,
    getByDateRange: mocks.getByDateRange,
  },
}))
vi.mock('@/api/user', () => ({ getMyPlatformQuotas: mocks.getMyPlatformQuotas }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/common/LoadingSpinner.vue', () => ({ default: { template: '<span role="status">正在加载</span>' } }))
vi.mock('@/components/user/dashboard/UserDashboardStats.vue', () => ({
  default: { props: ['stats', 'balance', 'platformQuotas', 'isSimple'], template: '<section data-testid="classic-stats">{{ stats.today_requests }} 次请求，余额 {{ balance }}</section>' },
}))
vi.mock('@/components/user/dashboard/UserDashboardCharts.vue', () => ({
  default: { props: ['startDate', 'endDate', 'granularity', 'loading', 'trend', 'models'], emits: ['refresh'], template: '<button data-testid="classic-refresh" @click="$emit(\'refresh\')">刷新详细统计</button>' },
}))
vi.mock('@/components/user/dashboard/UserDashboardRecentUsage.vue', () => ({
  default: { props: ['data', 'loading'], template: '<section data-testid="classic-recent">{{ data.length }} 条使用记录</section>' },
}))
vi.mock('@/components/user/dashboard/UserDashboardQuickActions.vue', () => ({
  default: { template: '<section data-testid="classic-actions">快捷操作</section>' },
}))

const wrappers: VueWrapper[] = []
const classicRequests = () => [mocks.getDashboardStats, mocks.getDashboardTrend, mocks.getDashboardModels, mocks.getByDateRange, mocks.getMyPlatformQuotas]

async function openDashboard(path = '/dashboard') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', component: DashboardView },
      { path: '/keys', component: { template: '<main data-testid="keys-page">API 密钥管理</main>' } },
    ],
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(RouterView, { attachTo: document.body, global: { plugins: [router] } })
  wrappers.push(wrapper)
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.refreshUser.mockResolvedValue(undefined)
  mocks.getDashboardStats.mockResolvedValue({ today_requests: 37 })
  mocks.getDashboardTrend.mockResolvedValue({ trend: [] })
  mocks.getDashboardModels.mockResolvedValue({ models: [] })
  mocks.getByDateRange.mockResolvedValue({ items: [{ id: 1 }, { id: 2 }] })
  mocks.getMyPlatformQuotas.mockResolvedValue({ platform_quotas: [] })
  mocks.get.mockImplementation(async (path: string) => {
    const data: Record<string, unknown> = {
      '/usage/dashboard/stats': { today_requests: 37, today_tokens: 42000 },
      '/usage/dashboard/trend': { trend: [] },
      '/usage/dashboard/models': { models: [] },
      '/settings/public': { site_name: '测试站点', api_base_url: 'https://api.example.test' },
      '/settings/home-models': [
        { name: 'gpt-5.5', vendor: 'OpenAI', type: 'text', input: 5, output: 30 },
      ],
      '/keys': { items: [{ id: 1, name: '项目密钥', key: 'sk-integration-test', status: 'active', group: { platform: 'openai' } }] },
    }
    const pathname = new URL(path, 'https://example.test').pathname
    if (!(pathname in data)) throw new Error(`未模拟接口：${pathname}`)
    return { data: data[pathname] }
  })
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => [] }))
})

afterEach(() => {
  for (const wrapper of wrappers) wrapper.unmount()
  wrappers.length = 0
  document.body.replaceChildren()
  vi.unstubAllGlobals()
})

describe('DashboardView 页面集成', () => {
  it('默认概览使用登录存储和共享客户端，切换接入指南不发起旧版统计请求', async () => {
    const { wrapper, router } = await openDashboard()
    expect(wrapper.get('[data-dashboard-balance]').text()).toBe('$256.75')
    expect(wrapper.get('[data-dashboard-requests]').text()).toBe('37')
    expect(mocks.refreshUser).toHaveBeenCalledOnce()
    expect(mocks.get.mock.calls.some(([path]) => path === '/auth/me')).toBe(false)
    for (const request of classicRequests()) expect(request).not.toHaveBeenCalled()

    expect(wrapper.find('nav[aria-label="控制台视图"]').exists()).toBe(false)
    await wrapper.get('[data-overlay-view="guide"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.view).toBe('guide')
    expect(wrapper.find('[data-sub2api-page="guide"]').exists()).toBe(true)
    expect(wrapper.get('[data-guide-key-toggle]').text()).toContain('项目密钥')
    expect(mocks.get.mock.calls.some(([path]) => path === '/settings/home-models')).toBe(true)
    expect(mocks.get.mock.calls.some(([path]) => path === '/model-data.json')).toBe(false)
    await wrapper.get('[data-guide-step="3"]').trigger('click')
    expect(wrapper.get('[data-guide-model="gpt-5.5"] .s2-model-price').text()).toContain('输入 $5')
    for (const request of classicRequests()) expect(request).not.toHaveBeenCalled()

    await wrapper.get('[data-overlay-view="dashboard"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/dashboard')
    expect(wrapper.find('[data-sub2api-page="dashboard"]').exists()).toBe(true)
  })

  it('详细统计保留用量、图表、记录及配额，并且只在进入或刷新时请求', async () => {
    const { wrapper, router } = await openDashboard('/dashboard?view=guide')
    for (const request of classicRequests()) expect(request).not.toHaveBeenCalled()

    await router.push('/dashboard?view=classic')
    await flushPromises()
    expect(wrapper.find('[data-testid="sidebar-dashboard"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="classic-stats"]').text()).toContain('37 次请求，余额 256.75')
    expect(wrapper.get('[data-testid="classic-recent"]').text()).toBe('2 条使用记录')
    expect(wrapper.find('[data-testid="classic-actions"]').exists()).toBe(true)
    for (const request of classicRequests()) expect(request).toHaveBeenCalledOnce()

    await router.push('/dashboard?view=classic&source=bookmark')
    await flushPromises()
    for (const request of classicRequests()) expect(request).toHaveBeenCalledOnce()
    await wrapper.get('[data-testid="classic-refresh"]').trigger('click')
    await flushPromises()
    for (const request of classicRequests()) expect(request).toHaveBeenCalledTimes(2)

    await router.push('/dashboard')
    await flushPromises()
    expect(wrapper.find('[data-sub2api-page="dashboard"]').exists()).toBe(true)
    for (const request of classicRequests()) expect(request).toHaveBeenCalledTimes(2)
  })

  it('直接打开详细统计兼容入口，不加载新增概览请求', async () => {
    const { wrapper } = await openDashboard('/dashboard?view=classic')
    expect(wrapper.find('[data-testid="dashboard-details"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="classic-stats"]').text()).toContain('37 次请求')
    for (const request of classicRequests()) expect(request).toHaveBeenCalledOnce()
    expect(mocks.get).not.toHaveBeenCalled()
  })

  it('概览中的路由按钮进入主站页面并清理挂载内容及样式', async () => {
    const { wrapper, router } = await openDashboard()
    await wrapper.get('[data-route="/keys"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/keys')
    expect(wrapper.get('[data-testid="keys-page"]').text()).toBe('API 密钥管理')
    expect(document.querySelector('[data-sub2api-page]')).toBeNull()
    expect(document.querySelector('style[id^="sub2api-page-overlay-root-"]')).toBeNull()
  })
})
