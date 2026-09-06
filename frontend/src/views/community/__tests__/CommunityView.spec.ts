import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import CommunityView from '../CommunityView.vue'
import { communityAPI, type CommunityState } from '@/api/community'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/api/support', () => ({ supportError: (_error: unknown, fallback: string) => fallback }))
vi.mock('@/api/community', () => ({ communityAPI: { get: vi.fn(), invite: vi.fn() } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: true, user: { id: 1 } }) }))

const expires = '2026-09-05T13:10:00Z'
const base: CommunityState = { contact_url: 'https://help.example.com/chat', enabled: true, require_paid_recharge: false, eligible: true, show_join_prompt: false, group_name: '用户群', bot_username: 'test_bot', membership: null, invite: null }
const invited: CommunityState = { ...base, invite: { url: 'https://t.me/+personal', expires_at: expires } }
const joined: CommunityState = { ...base, membership: { telegram_user_id: 123456789, telegram_username: 'my_account', telegram_name: '<img src=x onerror=alert(1)>', status: 'joined', joined_at: '2026-09-05T13:01:00Z' } }
const render = () => mount(CommunityView, { global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } } })

describe('个人邀请直接入群流程', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-05T13:00:00Z'))
    vi.mocked(communityAPI.get).mockResolvedValue(base)
    vi.mocked(communityAPI.invite).mockResolvedValue(invited)
  })
  afterEach(() => { vi.useRealTimers() })

  it('初始加载不生成邀请，用户点击后无需 Telegram 身份即可领取', async () => {
    const wrapper = render()
    await flushPromises()
    expect(communityAPI.invite).not.toHaveBeenCalled()
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
    await wrapper.find('[data-test="get-invite"]').trigger('click')
    await flushPromises()
    expect(communityAPI.invite).toHaveBeenCalledTimes(1)
    expect(communityAPI.invite).toHaveBeenCalledWith()
    const link = wrapper.find('[data-test="invite-link"]')
    expect(link.attributes('href')).toBe('https://t.me/+personal')
    expect(link.attributes('rel')).toBe('noopener noreferrer')
    expect(wrapper.text()).toContain('community.pending')
    expect(wrapper.find('[data-test="telegram-id"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('没有绑定身份也能展示已有个人邀请，不再领取', async () => {
    vi.mocked(communityAPI.get).mockResolvedValue(invited)
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="invite-link"]').exists()).toBe(true)
    expect(communityAPI.invite).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('领取请求尚未完成时阻止重复点击', async () => {
    let finish: ((data: CommunityState) => void) | undefined
    vi.mocked(communityAPI.invite).mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const wrapper = render()
    await flushPromises()
    const button = wrapper.find('[data-test="get-invite"]')
    await button.trigger('click')
    await button.trigger('click')
    expect(communityAPI.invite).toHaveBeenCalledTimes(1)
    expect(button.attributes('disabled')).toBeDefined()
    finish?.(invited)
    await flushPromises()
    wrapper.unmount()
  })

  it('入群后轮询显示实际 Telegram 绑定身份，页面卸载停止请求', async () => {
    vi.mocked(communityAPI.get).mockResolvedValue(invited)
    const wrapper = render()
    await flushPromises()
    vi.mocked(communityAPI.get).mockResolvedValue(joined)
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    expect(wrapper.find('[data-test="telegram-id"]').text()).toBe('123456789')
    expect(wrapper.find('[data-test="telegram-name"]').text()).toContain('<img')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('community.boundIdentity')
    expect(wrapper.text()).toContain('community.joined')
    expect(wrapper.find('[data-test="reissue"]').exists()).toBe(false)
    wrapper.unmount()
    const count = vi.mocked(communityAPI.get).mock.calls.length
    await vi.advanceTimersByTimeAsync(15000)
    expect(communityAPI.get).toHaveBeenCalledTimes(count)
  })

  it('首次申请尚未入群时不称为已绑定', async () => {
    vi.mocked(communityAPI.get).mockResolvedValue({ ...invited, membership: { ...joined.membership!, status: 'pending', joined_at: undefined } })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('community.requestIdentity')
    expect(wrapper.text()).not.toContain('community.boundIdentity')
    wrapper.unmount()
  })

  it('过期邀请不再显示可点击链接，可以重新领取', async () => {
    vi.mocked(communityAPI.get).mockResolvedValue({ ...invited, invite: { url: 'https://t.me/+old', expires_at: '2026-09-05T12:59:00Z' } })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="invite-link"]').exists()).toBe(false)
    await wrapper.find('[data-test="reissue"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="invite-link"]').attributes('href')).toBe('https://t.me/+personal')
    wrapper.unmount()
  })

  it('离群用户可以重新领取邀请，保留已绑定身份展示', async () => {
    vi.mocked(communityAPI.get).mockResolvedValue({ ...joined, membership: { ...joined.membership!, status: 'left' } })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('community.boundIdentity')
    await wrapper.find('[data-test="reissue"]').trigger('click')
    await flushPromises()
    expect(communityAPI.invite).toHaveBeenCalledWith()
    expect(wrapper.find('[data-test="invite-link"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('拒绝危险客服和邀请地址，并保留工单入口', async () => {
    vi.mocked(communityAPI.get).mockResolvedValue({ ...invited, contact_url: 'javascript:alert(1)', invite: { url: 'https://t.me.evil.example/+abc', expires_at: expires } })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="contact-link"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="invite-link"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/tickets"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('社群关闭时仍可联系管理员配置的客服网页', async () => {
    vi.mocked(communityAPI.get).mockResolvedValue({ ...base, enabled: false })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="contact-link"]').attributes('href')).toBe(base.contact_url)
    expect(wrapper.find('[data-test="get-invite"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('未充值用户看不到整个 VIP 群卡，仍可联系客服和提交工单', async () => {
    vi.mocked(communityAPI.get).mockResolvedValue({ ...base, enabled: false, require_paid_recharge: true, eligible: false })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="community-group"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('用户群')
    expect(wrapper.find('[data-test="contact-link"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/tickets"]').exists()).toBe(true)
    expect(communityAPI.invite).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('成功充值用户可以看到 VIP 群并主动领取邀请', async () => {
    vi.mocked(communityAPI.get).mockResolvedValue({ ...base, require_paid_recharge: true, eligible: true })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="community-group"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('community.vipEligibility')
    await wrapper.find('[data-test="get-invite"]').trigger('click')
    expect(communityAPI.invite).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})
