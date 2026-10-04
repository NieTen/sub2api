import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CommunityMemberDialog from '../CommunityMemberDialog.vue'
import { communityAPI, type CommunityTelegramUser } from '@/api/community'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/api/client', () => ({ apiClient: {} }))
vi.mock('@/api/community', () => ({ communityAPI: { telegramUser: vi.fn(), unbind: vi.fn() } }))
vi.mock('@/components/common/BaseDialog.vue', () => ({ default: { props: ['show'], template: '<section v-if="show"><slot /><slot name="footer" /></section>' } }))
vi.mock('../CommunityAvatar.vue', () => ({ default: { emits: ['select'], template: '<button data-test="avatar" @click="$emit(\'select\')">头像</button>' } }))
const identity: CommunityTelegramUser = { telegram_user_id: 5939067819, telegram_name: '<b>测试用户</b>', telegram_username: 'tester', banned: true, member: { user_id: 7, email: 'member@example.com', username: '网站用户', user_status: 'active', telegram_user_id: 5939067819, telegram_name: '测试用户', telegram_username: 'tester', status: 'banned' } }

describe('社群用户详情与工单解绑', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(communityAPI.telegramUser).mockResolvedValue(identity)
    vi.mocked(communityAPI.unbind).mockResolvedValue({ user_id: 7, telegram_user_id: 5939067819, ticket_id: 15, unbound: true, banned: true })
  })

  it('展示 Telegram、网站账号与红色禁入状态，必须填写工单并复核后才能解绑', async () => {
    const wrapper = mount(CommunityMemberDialog, { props: { identity } })
    await flushPromises()
    expect(communityAPI.telegramUser).toHaveBeenCalledWith(5939067819)
    expect(wrapper.text()).toContain('member@example.com')
    expect(wrapper.text()).toContain('5939067819')
    expect(wrapper.find('.badge-danger').text()).toBe('community.members.banned')
    expect(wrapper.find('b').exists()).toBe(false)
    await wrapper.find('form').trigger('submit')
    expect(communityAPI.unbind).not.toHaveBeenCalled()
    await wrapper.find('input[name="unbind-ticket-id"]').setValue('15')
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(communityAPI.unbind).toHaveBeenCalledWith(7, 15)
    expect(wrapper.emitted('changed')).toHaveLength(1)
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.text()).toContain('community.members.unbound')
    expect(wrapper.find('.badge-danger').text()).toBe('community.members.banned')
    wrapper.unmount()
  })

  it('工单不匹配保留表单及后端错误，用户可修正工单后重试', async () => {
    vi.mocked(communityAPI.unbind).mockRejectedValueOnce({ message: '工单不属于该用户' })
    const wrapper = mount(CommunityMemberDialog, { props: { identity } })
    await flushPromises()
    await wrapper.find('input[name="unbind-ticket-id"]').setValue('99')
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toBe('工单不属于该用户')
    expect(wrapper.emitted('changed')).toBeUndefined()
    expect((wrapper.find('input[name="unbind-ticket-id"]').element as HTMLInputElement).value).toBe('99')
    wrapper.unmount()
  })

  it('匿名群身份只展示身份来源，不读取虚构个人详情或提供解绑', async () => {
    const wrapper = mount(CommunityMemberDialog, { props: { identity: { ...identity, telegram_user_id: 0, member: null, sender_kind: 'chat' } } })
    await flushPromises()
    expect(communityAPI.telegramUser).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('community.chat.anonymousHint')
    expect(wrapper.find('form').exists()).toBe(false)
    wrapper.unmount()
  })

  it('服务端已无绑定时不沿用成员表旧绑定信息', async () => {
    vi.mocked(communityAPI.telegramUser).mockResolvedValue({ ...identity, member: null })
    const wrapper = mount(CommunityMemberDialog, { props: { identity } })
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('member@example.com')
    expect(wrapper.find('.badge-danger').text()).toBe('community.members.banned')
    wrapper.unmount()
  })

  it('解绑处理中不重复提交，也不因点击头像重载而丢失完成结果', async () => {
    let finish: ((data: Awaited<ReturnType<typeof communityAPI.unbind>>) => void) | undefined
    vi.mocked(communityAPI.unbind).mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mount(CommunityMemberDialog, { props: { identity } })
    await flushPromises()
    await wrapper.find('input[name="unbind-ticket-id"]').setValue('15')
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.find('form').trigger('submit')
    await wrapper.find('form').trigger('submit')
    await wrapper.find('[data-test="avatar"]').trigger('click')
    expect(wrapper.get('[data-test="close-member-detail"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="refresh-member-detail"]').attributes('disabled')).toBeDefined()
    expect(communityAPI.unbind).toHaveBeenCalledTimes(1)
    expect(communityAPI.telegramUser).toHaveBeenCalledTimes(1)
    finish?.({ user_id: 7, telegram_user_id: 5939067819, ticket_id: 15, unbound: true, banned: true })
    await flushPromises()
    expect(wrapper.emitted('changed')).toHaveLength(1)
    expect(wrapper.text()).toContain('community.members.unbound')
    wrapper.unmount()
  })

  it('详情加载失败后可通过可见刷新按钮重试，并可从底部关闭', async () => {
    vi.mocked(communityAPI.telegramUser).mockRejectedValueOnce({ message: '暂时不可用' })
    const wrapper = mount(CommunityMemberDialog, { props: { identity } })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('暂时不可用')
    await wrapper.get('[data-test="refresh-member-detail"]').trigger('click')
    await flushPromises()
    expect(communityAPI.telegramUser).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('member@example.com')
    await wrapper.get('[data-test="close-member-detail"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})
