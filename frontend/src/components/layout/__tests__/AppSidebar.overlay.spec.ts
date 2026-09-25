import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

import AppSidebar from '../AppSidebar.vue'

const { appStore, authStore, adminSettingsStore, onboardingStore, batchImageAccess, route, routerPush } = vi.hoisted(
  () => {
    const routerPush = vi.fn()
    return {
      appStore: {
        backendModeEnabled: false,
        cachedPublicSettings: {
          available_channels_enabled: true,
          affiliate_enabled: true,
          model_plaza_enabled: true,
          subscription_enabled: true,
          payment_enabled: true,
          custom_menu_items: [
            {
              id: 'toolbox',
              label: '自定义入口',
              icon_svg: '<svg />',
              url: 'https://example.com/toolbox',
              visibility: 'user',
              sort_order: 1,
            },
          ],
        },
        publicSettingsLoaded: true,
        sidebarCollapsed: false,
        sidebarScrollTop: 0,
        siteLogo: '',
        siteName: 'Sub2API',
        siteVersion: '1.0.0',
        mobileOpen: false,
        fetchPublicSettings: vi.fn(),
        toggleSidebar: vi.fn(),
        setMobileOpen: vi.fn(),
      },
      authStore: {
        isAdmin: false,
        isSimpleMode: false,
        user: { role: 'user' },
      },
      adminSettingsStore: {
        customMenuItems: [],
        opsMonitoringEnabled: true,
        paymentEnabled: true,
        fetch: vi.fn(),
      },
      onboardingStore: {
        isCurrentStep: vi.fn(() => false),
        nextStep: vi.fn(),
        setReplayCallback: vi.fn(),
      },
      batchImageAccess: {
        canUseBatchImage: {
          value: false,
        },
        refreshBatchImageAccess: vi.fn(),
      },
      route: {
        path: '/dashboard',
        query: {} as Record<string, string>,
      },
      routerPush,
    }
  },
)

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ push: routerPush }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAdminSettingsStore: () => adminSettingsStore,
  useAuthStore: () => authStore,
  useOnboardingStore: () => onboardingStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => batchImageAccess,
}))

describe('AppSidebar 普通用户参考布局', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(window, 'matchMedia').mockReturnValue({
      matches: false,
    } as MediaQueryList)
    localStorage.clear()
    routerPush.mockClear()
    adminSettingsStore.fetch.mockClear()
    onboardingStore.isCurrentStep.mockClear()
    onboardingStore.nextStep.mockClear()
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    route.path = '/dashboard'
    route.query = {}
    appStore.sidebarCollapsed = false
    batchImageAccess.canUseBatchImage.value = true
    appStore.cachedPublicSettings = {
      ...appStore.cachedPublicSettings,
      available_channels_enabled: true,
      affiliate_enabled: true,
      model_plaza_enabled: true,
      subscription_enabled: true,
      payment_enabled: true,
      custom_menu_items: [
        {
          id: 'toolbox',
          label: '自定义入口',
          icon_svg: '<svg />',
          url: 'https://example.com/toolbox',
          visibility: 'user',
          sort_order: 1,
        },
      ],
    }
  })

  it('按参考文件的顺序分组，隐藏指定入口并保留其他已有功能', () => {
    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
          VersionBadge: { template: '<span />' },
        },
      },
    })

    const sectionTitles = wrapper
      .findAll('.sidebar-user-group-title')
      .map((node) => node.text())

    expect(sectionTitles).toEqual(['nav.console', 'nav.account', 'nav.extension'])
    expect(wrapper.findAll('.sidebar-user-section')).toHaveLength(1)

    const linkTargets = wrapper
      .get('.sidebar-nav')
      .findAllComponents(RouterLinkStub)
      .map((node) => node.props('to'))

    expect(linkTargets).not.toContain('/subscriptions')
    expect(linkTargets).not.toContain('/redeem')
    expect(linkTargets).not.toContain('/admin/communications')
    expect(linkTargets).toEqual([
      '/dashboard',
      '/keys',
      '/usage',
      '/purchase',
      '/orders',
      '/profile',
      '/batch-image',
      '/available-channels',
      '/monitor',
      '/affiliate',
      '/tickets',
      '/community',
      '/custom/toolbox',
    ])

    const purchaseLink = wrapper
      .findAllComponents(RouterLinkStub)
      .find((node) => node.props('to') === '/purchase')

    expect(purchaseLink?.text()).toContain('nav.balanceRecharge')
  })

  it.each(['/admin/communications', '/admin/support/settings', '/admin/bulk-emails/7', '/admin/community/members', '/admin/tickets/3'])('管理员在 %s 可直接找到机器人与群发入口且菜单保持选中', (path) => {
    authStore.isAdmin = true
    authStore.isSimpleMode = true
    route.path = path
    const wrapper = mount(AppSidebar, {
      global: { stubs: { RouterLink: RouterLinkStub, VersionBadge: { template: '<span />' } } }
    })
    const link = wrapper.findAllComponents(RouterLinkStub).find(node => node.props('to') === '/admin/communications')
    expect(link?.text()).toContain('communications.title')
    expect(link?.classes()).toContain('sidebar-link-active')
    wrapper.unmount()
  })

  it('折叠时不保留分组标题或分隔线占位，并保留各组入口', () => {
    appStore.sidebarCollapsed = true
    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
          VersionBadge: { template: '<span />' },
        },
      },
    })

    expect(wrapper.find('.sidebar-user-group-title').exists()).toBe(false)
    expect(wrapper.find('.sidebar-section-title').exists()).toBe(false)
    expect(wrapper.findAll('.sidebar-user-section > .sidebar-link')).toHaveLength(13)
    const groupLinks = wrapper.findAll('.sidebar-user-section > [data-sidebar-group]')
    expect(groupLinks.map((link) => link.attributes('data-sidebar-group'))).toEqual([
      'console',
      'account',
      'extension',
    ])
    expect(groupLinks.every((link) => link.classes().includes('sidebar-link-collapsed'))).toBe(true)
  })

  it('管理员仍使用原有菜单及个人账户入口', () => {
    authStore.isAdmin = true
    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
          VersionBadge: { template: '<span />' },
        },
      },
    })

    expect(wrapper.find('.sidebar-user-section').exists()).toBe(false)
    expect(wrapper.get('.sidebar-section-title-text').text()).toBe('nav.myAccount')
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links.some((link) => link.props('to') === '/subscriptions')).toBe(true)
    expect(links.some((link) => link.props('to') === '/redeem')).toBe(true)
    expect(links.find((link) => link.props('to') === '/purchase')?.text()).toBe('nav.buySubscription')
  })

  it.each(['', 'guide', 'classic'])('仪表盘 %s 内容视图保留原菜单、名称和高亮', (view) => {
    route.query = view ? { view } : {}
    const wrapper = mount(AppSidebar, {
      global: { stubs: { RouterLink: RouterLinkStub, VersionBadge: true } },
    })
    const links = wrapper.get('.sidebar-user-section').findAllComponents(RouterLinkStub)
    expect(links.map(link => link.props('to'))).toEqual([
      '/dashboard', '/keys', '/usage', '/purchase', '/orders', '/profile',
      '/batch-image', '/available-channels', '/monitor', '/affiliate', '/tickets', '/community', '/custom/toolbox',
    ])
    expect(links.filter(link => link.classes().includes('sidebar-link-active')).map(link => link.props('to'))).toEqual(['/dashboard'])
    expect(links.find(link => link.props('to') === '/dashboard')?.text()).toContain('nav.dashboard')
    expect(links.find(link => link.props('to') === '/usage')?.text()).toContain('nav.usage')
    expect(links.find(link => link.props('to') === '/profile')?.text()).toContain('nav.profile')
    expect(wrapper.get('.sidebar-nav').findAll('button')).toHaveLength(0)
    expect(wrapper.findComponent({ name: 'VersionBadge' }).exists()).toBe(true)
    wrapper.unmount()
  })

  it('原菜单继续遵循可用渠道、返利、支付和批量生图权限', () => {
    appStore.cachedPublicSettings.available_channels_enabled = false
    appStore.cachedPublicSettings.affiliate_enabled = false
    appStore.cachedPublicSettings.payment_enabled = false
    batchImageAccess.canUseBatchImage.value = false
    const wrapper = mount(AppSidebar, {
      global: { stubs: { RouterLink: RouterLinkStub, VersionBadge: true } },
    })
    const targets = wrapper.get('.sidebar-nav').findAllComponents(RouterLinkStub).map(link => link.props('to'))
    for (const path of ['/available-channels', '/affiliate', '/purchase', '/orders', '/batch-image']) expect(targets).not.toContain(path)
    expect(targets).toContain('/keys')
    expect(targets).toContain('/usage')
    expect(targets).toContain('/custom/toolbox')
    wrapper.unmount()
  })

  it('简易模式仍保留原密钥、资料及自定义入口', () => {
    authStore.isSimpleMode = true
    const wrapper = mount(AppSidebar, {
      global: { stubs: { RouterLink: RouterLinkStub, VersionBadge: true } },
    })
    const targets = wrapper.get('.sidebar-nav').findAllComponents(RouterLinkStub).map(link => link.props('to'))
    expect(targets).toEqual(['/dashboard', '/keys', '/profile', '/monitor', '/tickets', '/community', '/custom/toolbox'])
    wrapper.unmount()
  })

  it.each([
    { view: '', simple: false, collapsed: false },
    { view: 'guide', simple: true, collapsed: false },
    { view: 'classic', simple: true, collapsed: true },
  ])('历史扩展在 $view 视图（简易 $simple、折叠 $collapsed）保留原配置并按顺序展示', ({ view, simple, collapsed }) => {
    route.query = view ? { view } : {}
    authStore.isSimpleMode = simple
    appStore.sidebarCollapsed = collapsed

    // 模拟公开接口返回空字符串、HTML 注入省略字段两种历史载荷，不能先补全可见范围。
    const configuredMenus: typeof appStore.cachedPublicSettings.custom_menu_items = JSON.parse(JSON.stringify([
      { id: 'current-tool', label: '现有工具', icon_svg: '<svg />', url: 'https://example.test/current', visibility: 'user', sort_order: 30, hide_open_button: true },
      { id: 'admin-only', label: '管理员工具', icon_svg: '<svg />', url: 'https://example.test/admin', visibility: 'admin', sort_order: 0 },
      { id: 'legacy-injected', label: '历史扩展 B', icon_svg: '<svg />', url: 'https://example.test/injected', sort_order: 20, page_slug: 'legacy-guide' },
      { id: 'legacy-public', label: '历史扩展 A', icon_svg: '<svg />', url: 'https://example.test/public', visibility: '', sort_order: 10 },
    ]))
    const originalConfig = JSON.parse(JSON.stringify(configuredMenus))
    configuredMenus.forEach(menu => Object.freeze(menu))
    Object.freeze(configuredMenus)
    appStore.cachedPublicSettings.custom_menu_items = configuredMenus

    const wrapper = mount(AppSidebar, {
      global: { stubs: { RouterLink: RouterLinkStub, VersionBadge: true } },
    })
    const links = wrapper.get('.sidebar-user-section').findAllComponents(RouterLinkStub)
    const customLinks = links.filter(link => String(link.props('to')).startsWith('/custom/'))
    expect(customLinks.map(link => ({ to: link.props('to'), label: link.get('.sidebar-label').text() }))).toEqual([
      { to: '/custom/legacy-public', label: '历史扩展 A' },
      { to: '/custom/legacy-injected', label: '历史扩展 B' },
      { to: '/custom/current-tool', label: '现有工具' },
    ])
    expect(customLinks[0].attributes('data-sidebar-group')).toBe('extension')
    expect(customLinks.slice(1).every(link => link.attributes('data-sidebar-group') === undefined)).toBe(true)
    if (collapsed) {
      expect(customLinks.every(link => link.classes().includes('sidebar-link-collapsed'))).toBe(true)
      expect(customLinks.map(link => link.attributes('title'))).toEqual(['历史扩展 A', '历史扩展 B', '现有工具'])
    } else {
      expect(customLinks[0].get('.sidebar-user-group-title').text()).toBe('nav.extension')
    }
    expect(links.some(link => link.props('to') === '/custom/admin-only')).toBe(false)
    expect(appStore.cachedPublicSettings.custom_menu_items).toBe(configuredMenus)
    expect(configuredMenus).toEqual(originalConfig)
    expect(Object.prototype.hasOwnProperty.call(configuredMenus[2], 'visibility')).toBe(false)
    wrapper.unmount()
  })

})
