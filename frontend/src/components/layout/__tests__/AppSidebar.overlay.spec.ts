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

  it.each([
    ['', '/dashboard'],
    ['guide', '/dashboard?view=guide'],
    ['classic', '/dashboard?view=classic'],
  ])('工作台在 %s 视图只选中对应导航', (view, activeTarget) => {
    route.query = view ? { view } : {}
    const wrapper = mount(AppSidebar, {
      props: { workspace: true },
      global: { stubs: { RouterLink: RouterLinkStub, VersionBadge: true } },
    })
    const links = wrapper.get('.workspace-navigation').findAllComponents(RouterLinkStub)
    expect(links.map(link => link.props('to'))).toEqual([
      '/dashboard', '/dashboard?view=guide', '/model-plaza?embedded=1', '/dashboard?view=classic', '/keys', '/profile',
    ])
    expect(links.filter(link => link.classes().includes('sidebar-link-active')).map(link => link.props('to'))).toEqual([activeTarget])
    expect(links.find(link => link.props('to') === activeTarget)?.attributes('aria-current')).toBe('page')
    wrapper.unmount()
  })

  it('工作台展开分组后仍能访问原有账户和扩展功能', async () => {
    const wrapper = mount(AppSidebar, {
      props: { workspace: true },
      global: { stubs: { RouterLink: RouterLinkStub, VersionBadge: true } },
    })
    const buttons = wrapper.findAll('.workspace-extra-section > button')
    expect(buttons).toHaveLength(2)
    expect(buttons.every(button => button.attributes('aria-expanded') === 'false')).toBe(true)
    for (const button of buttons) await button.trigger('click')
    const targets = wrapper.get('.workspace-navigation').findAllComponents(RouterLinkStub).map(link => link.props('to'))
    for (const path of ['/subscriptions', '/purchase', '/orders', '/redeem', '/affiliate', '/usage', '/batch-image', '/available-channels', '/monitor', '/tickets', '/community', '/custom/toolbox']) {
      expect(targets).toContain(path)
    }
    await buttons[0].trigger('click')
    expect(wrapper.find('#workspace-section-account').exists()).toBe(false)
    wrapper.unmount()
  })

  it('工作台仍遵循模型广场、订阅和支付开关', async () => {
    appStore.cachedPublicSettings.model_plaza_enabled = false
    appStore.cachedPublicSettings.subscription_enabled = false
    appStore.cachedPublicSettings.payment_enabled = false
    const wrapper = mount(AppSidebar, {
      props: { workspace: true },
      global: { stubs: { RouterLink: RouterLinkStub, VersionBadge: true } },
    })
    for (const button of wrapper.findAll('.workspace-extra-section > button')) await button.trigger('click')
    const targets = wrapper.get('.workspace-navigation').findAllComponents(RouterLinkStub).map(link => link.props('to'))
    for (const path of ['/model-plaza?embedded=1', '/subscriptions', '/purchase', '/orders']) expect(targets).not.toContain(path)
    expect(targets).toContain('/dashboard?view=guide')
    wrapper.unmount()
  })

})
