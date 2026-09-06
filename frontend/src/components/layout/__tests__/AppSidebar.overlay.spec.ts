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

describe('AppSidebar regular user overlay', () => {
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
    appStore.cachedPublicSettings = {
      ...appStore.cachedPublicSettings,
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

  it('renders overlay sections directly from Vue code', () => {
    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
          VersionBadge: { template: '<span />' },
        },
      },
    })

    const sectionTitles = wrapper
      .findAll('.sidebar-section-title-text')
      .map((node) => node.text())

    expect(sectionTitles).toContain('nav.console')
    expect(sectionTitles).toContain('nav.account')
    expect(sectionTitles).toContain('nav.extension')

    const linkTargets = wrapper
      .findAllComponents(RouterLinkStub)
      .map((node) => node.props('to'))

    expect(linkTargets).toContain('/purchase')
    expect(linkTargets).toContain('/custom/toolbox')
    expect(linkTargets).not.toContain('/subscriptions')
    expect(linkTargets).not.toContain('/redeem')
    expect(linkTargets).toContain('/tickets')
    expect(linkTargets).toContain('/community')
    expect(linkTargets).not.toContain('/admin/communications')

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
})
