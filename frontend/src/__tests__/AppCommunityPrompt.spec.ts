import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import App from '../App.vue'

const mocks = vi.hoisted(() => ({
  auth: {} as Record<string, unknown>,
  announcements: { fetchAnnouncements: vi.fn(), reset: vi.fn() },
  subscriptions: { fetchActiveSubscriptions: vi.fn(), startPolling: vi.fn(), clear: vi.fn() },
  compliance: { fetchStatus: vi.fn(), reset: vi.fn(), requireAcknowledgement: vi.fn() }
}))
vi.mock('@/stores', () => ({
  useAuthStore: () => mocks.auth,
  useAnnouncementStore: () => mocks.announcements,
  useSubscriptionStore: () => mocks.subscriptions,
  useAdminComplianceStore: () => mocks.compliance,
  useAppStore: () => ({ fetchPublicSettings: vi.fn().mockResolvedValue(undefined), siteName: 'Test', siteLogo: '', cachedPublicSettings: null }),
  useAdminSettingsStore: () => ({ customMenuItems: [] })
}))
vi.mock('vue-router', () => ({ RouterView: { template: '<main />' }, useRouter: () => ({ afterEach: vi.fn(), replace: vi.fn() }), useRoute: () => ({ path: '/', fullPath: '/', meta: {} }) }))
vi.mock('@/api/setup', () => ({ getSetupStatus: vi.fn().mockResolvedValue({ needs_setup: false }) }))
vi.mock('@/router/title', () => ({ resolveRouteDocumentTitle: () => 'Test' }))
vi.mock('@/utils/branding', () => ({ updateFavicon: vi.fn() }))
vi.mock('@/utils/featureFlags', async importOriginal => ({
  ...(await importOriginal<typeof import('@/utils/featureFlags')>()),
  isFeatureFlagEnabled: () => true
}))
vi.mock('@/components/common/Toast.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/NavigationProgress.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/AnnouncementPopup.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/admin/AdminComplianceDialog.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/VipCommunityPrompt.vue', () => ({ default: { props: ['blocked'], template: '<div data-test="vip" :data-blocked="blocked" />' } }))

let wrapper: VueWrapper | undefined
describe('全局 VIP 提醒与登录公告调度', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    mocks.auth = reactive({ isAuthenticated: false, user: null, isAdmin: false })
    mocks.announcements.fetchAnnouncements.mockResolvedValue(undefined)
    mocks.subscriptions.fetchActiveSubscriptions.mockResolvedValue(undefined)
    mocks.compliance.fetchStatus.mockResolvedValue(undefined)
  })
  afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() })

  it('新登录的三秒延迟及公告请求期间阻塞 VIP 提醒', async () => {
    let finish: (() => void) | undefined
    mocks.announcements.fetchAnnouncements.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    wrapper = mount(App)
    mocks.auth.user = { id: 1 }
    mocks.auth.isAuthenticated = true
    await flushPromises()
    expect(wrapper.find('[data-test="vip"]').attributes('data-blocked')).toBe('true')
    await vi.advanceTimersByTimeAsync(2999)
    expect(mocks.announcements.fetchAnnouncements).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.announcements.fetchAnnouncements).toHaveBeenCalledWith(true)
    expect(wrapper.find('[data-test="vip"]').attributes('data-blocked')).toBe('true')
    finish?.()
    await flushPromises()
    expect(wrapper.find('[data-test="vip"]').attributes('data-blocked')).toBe('false')
  })

  it('页面恢复立即请求公告，退出时取消尚未执行的登录公告', async () => {
    mocks.auth.user = { id: 1 }
    mocks.auth.isAuthenticated = true
    wrapper = mount(App)
    await flushPromises()
    expect(mocks.announcements.fetchAnnouncements).toHaveBeenCalledWith(false)
    mocks.auth.isAuthenticated = false
    await flushPromises()
    mocks.auth.isAuthenticated = true
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    mocks.auth.isAuthenticated = false
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    expect(mocks.announcements.fetchAnnouncements).toHaveBeenCalledTimes(1)
  })
})
