import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import VipCommunityPrompt from '../VipCommunityPrompt.vue'
import { communityAPI, type CommunityState } from '@/api/community'
import { vipCommunityPromptKey } from '@/utils/vipCommunityPrompt'

const mocks = vi.hoisted(() => ({ auth: {} as Record<string, unknown>, announcements: {} as Record<string, unknown>, compliance: {} as Record<string, unknown>, push: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, values?: { group: string }) => values?.group || key }) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: mocks.push }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/stores/announcements', () => ({ useAnnouncementStore: () => mocks.announcements }))
vi.mock('@/stores/adminCompliance', () => ({ useAdminComplianceStore: () => mocks.compliance }))
vi.mock('@/api/community', () => ({ communityAPI: { get: vi.fn(), invite: vi.fn() } }))

const state: CommunityState = { contact_url: '', enabled: true, require_paid_recharge: true, eligible: true, show_join_prompt: true, prompt_key: 'group-1', group_name: 'VIP 专享群', bot_username: 'test_bot', membership: null, invite: null }
const wrappers: VueWrapper[] = []
function render(blocked = false) {
  const wrapper = mount(VipCommunityPrompt, {
    props: { blocked },
    global: { stubs: { BaseDialog: { props: ['show', 'title'], emits: ['close'], template: '<section v-if="show" role="dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></section>' } } }
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('VIP 登录入群提醒', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
    mocks.auth = reactive({ isAuthenticated: true, user: { id: 1 }, isAdmin: false, token: 'token-1' })
    mocks.announcements = reactive({ popupBlocking: false })
    mocks.compliance = reactive({ loading: false, shouldShow: false })
    vi.mocked(communityAPI.get).mockResolvedValue({ ...state })
    mocks.push.mockResolvedValue(undefined)
  })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

  it('成功充值用户登录后收到提示，但不会预先生成邀请', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('VIP 专享群')
    expect(communityAPI.get).toHaveBeenCalledTimes(1)
    expect(communityAPI.invite).not.toHaveBeenCalled()
    expect(sessionStorage.length).toBe(0)
  })

  it.each([
    { enabled: false },
    { require_paid_recharge: false },
    { eligible: false },
    { show_join_prompt: false },
    { membership: { telegram_user_id: 123, telegram_username: 'alice', telegram_name: 'Alice', status: 'joined' as const } }
  ])('关闭功能、未充值、普通群或已加入时不弹窗：%j', async (override) => {
    vi.mocked(communityAPI.get).mockResolvedValue({ ...state, ...override })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(sessionStorage.length).toBe(0)
    expect(communityAPI.invite).not.toHaveBeenCalled()
  })

  it('关闭后本标签页刷新不再提示，群配置更新后可再次提示', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('[data-test="vip-later"]').trigger('click')
    expect(sessionStorage.getItem(vipCommunityPromptKey(1, 'group-1'))).toBe('1')
    wrapper.unmount()
    const refreshed = render()
    await flushPromises()
    expect(refreshed.find('[role="dialog"]').exists()).toBe(false)
    refreshed.unmount()
    vi.mocked(communityAPI.get).mockResolvedValue({ ...state, prompt_key: 'group-2' })
    const newGroup = render()
    await flushPromises()
    expect(newGroup.find('[role="dialog"]').exists()).toBe(true)
  })

  it('点击加入只前往领取页面并记住本次操作', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('[data-test="vip-join"]').trigger('click')
    expect(mocks.push).toHaveBeenCalledWith('/community')
    expect(communityAPI.invite).not.toHaveBeenCalled()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(sessionStorage.getItem(vipCommunityPromptKey(1, 'group-1'))).toBe('1')
  })

  it('公告加载与展示及管理员承诺窗口期间等待，不标记已提示', async () => {
    const wrapper = render(true)
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    mocks.announcements.popupBlocking = true
    await wrapper.setProps({ blocked: false })
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    mocks.auth.isAdmin = true
    mocks.compliance.loading = true
    mocks.announcements.popupBlocking = false
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    mocks.compliance.shouldShow = true
    mocks.compliance.loading = false
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(sessionStorage.length).toBe(0)
    mocks.compliance.shouldShow = false
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
  })

  it('切换账号后忽略旧账号迟到的响应，关闭记录按账号隔离', async () => {
    let resolveFirst: ((value: CommunityState) => void) | undefined
    vi.mocked(communityAPI.get).mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
    const wrapper = render()
    vi.mocked(communityAPI.get).mockResolvedValue({ ...state, eligible: false })
    mocks.auth.user = { id: 2 }
    await flushPromises()
    resolveFirst?.({ ...state })
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(sessionStorage.length).toBe(0)
    vi.mocked(communityAPI.get).mockResolvedValue({ ...state })
    mocks.auth.user = { id: 3 }
    await flushPromises()
    await wrapper.find('[data-test="vip-later"]').trigger('click')
    expect(sessionStorage.getItem(vipCommunityPromptKey(3, 'group-1'))).toBe('1')
    expect(sessionStorage.getItem(vipCommunityPromptKey(1, 'group-1'))).toBeNull()
  })

  it('刷新令牌或同一用户资料不会重发资格请求或再次弹窗', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('[data-test="vip-later"]').trigger('click')
    mocks.auth.token = 'refreshed-token'
    mocks.auth.user = { id: 1 }
    await flushPromises()
    expect(communityAPI.get).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('退出登录立即移除弹窗，迟到请求不会恢复旧资料', async () => {
    let finish: ((value: CommunityState) => void) | undefined
    vi.mocked(communityAPI.get).mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const wrapper = render()
    mocks.auth.isAuthenticated = false
    finish?.({ ...state })
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('加载失败静默跳过，不记已提示且不生成邀请', async () => {
    vi.mocked(communityAPI.get).mockRejectedValue(new Error('network'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(sessionStorage.length).toBe(0)
    expect(communityAPI.invite).not.toHaveBeenCalled()
  })
})
