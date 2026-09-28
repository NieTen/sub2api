import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import CommunityMessagesPanel from '../CommunityMessagesPanel.vue'
import { communityAPI, type CommunityChatMessage } from '@/api/community'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/api/client', () => ({ apiClient: {} }))
vi.mock('@/api/community', () => ({ communityAPI: { messages: vi.fn(), sendMessage: vi.fn() } }))
vi.mock('../CommunityAvatar.vue', () => ({ default: { props: ['name', 'telegramId'], emits: ['select'], template: '<button data-test="avatar" @click="$emit(\'select\')">{{ name }}</button>' } }))
vi.mock('../CommunityMessageMedia.vue', () => ({ default: { props: ['message', 'label'], template: '<div data-test="media">{{ label }} {{ message.file_name }}</div>' } }))

const message = (id: number, patch: Partial<CommunityChatMessage> = {}): CommunityChatMessage => ({ id, telegram_message_id: id, telegram_user_id: 100, telegram_name: '测试用户', telegram_username: 'tester', sender_kind: 'user', text: `消息 ${id}`, message_type: 'text', created_at: '2026-09-29T12:00:00Z', media_available: false, outgoing: false, ...patch })

describe('管理员群消息', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.mocked(communityAPI.messages).mockResolvedValue({ items: [message(10)], has_more: true, latest_id: 10 })
    vi.mocked(communityAPI.sendMessage).mockResolvedValue(message(20, { outgoing: true }))
  })
  afterEach(() => { vi.useRealTimers() })

  it('先加载当前页，早期分页与增量刷新合并去重，不遗漏积压页', async () => {
    const wrapper = mount(CommunityMessagesPanel)
    await flushPromises()
    vi.mocked(communityAPI.messages).mockResolvedValueOnce({ items: [message(8), message(9)], has_more: false, latest_id: 10 })
    await wrapper.find('[data-test="older-messages"]').trigger('click')
    await flushPromises()
    expect(communityAPI.messages).toHaveBeenLastCalledWith({ before_id: 10 })
    vi.mocked(communityAPI.messages)
      .mockResolvedValueOnce({ items: [message(11)], has_more: true, latest_id: 11 })
      .mockResolvedValueOnce({ items: [message(11), message(12)], has_more: false, latest_id: 12 })
    await wrapper.find('[data-test="refresh-messages"]').trigger('click')
    await flushPromises()
    expect(communityAPI.messages).toHaveBeenLastCalledWith({ after_id: 11 })
    expect(wrapper.findAll('article').map(node => node.attributes('data-message-id'))).toEqual(['8', '9', '10', '11', '12'])
    expect(wrapper.find('[data-test="older-messages"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('姓名及消息按纯文本显示，匿名群身份头像不冒用 Telegram 用户', async () => {
    vi.mocked(communityAPI.messages).mockResolvedValue({ items: [message(10, { telegram_name: '<img src=x>', text: '<script>alert(1)</script>', sender_kind: 'chat', telegram_user_id: 999 })], has_more: false, latest_id: 10 })
    const wrapper = mount(CommunityMessagesPanel)
    await flushPromises()
    expect(wrapper.text()).toContain('<script>alert(1)</script>')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.find('img').exists()).toBe(false)
    await wrapper.find('[data-test="avatar"]').trigger('click')
    expect(wrapper.emitted('select')?.[0][0]).toMatchObject({ telegram_user_id: 0, sender_kind: 'chat', member: null })
    wrapper.unmount()
  })

  it('图片、语音、文档与未知消息都保留类型和说明', async () => {
    vi.mocked(communityAPI.messages).mockResolvedValue({ items: [message(1, { message_type: 'photo' }), message(2, { message_type: 'voice' }), message(3, { message_type: 'document', file_name: '使用说明.pdf' }), message(4, { message_type: 'new_type', text: '{"说明":"其他消息"}' })], has_more: false, latest_id: 4 })
    const wrapper = mount(CommunityMessagesPanel)
    await flushPromises()
    expect(wrapper.findAll('[data-test="media"]')).toHaveLength(3)
    expect(wrapper.text()).toContain('使用说明.pdf')
    expect(wrapper.text()).toContain('community.chat.types.unsupported')
    expect(wrapper.text()).toContain('{"说明":"其他消息"}')
    wrapper.unmount()
  })

  it('发送结果不确定时保留输入及请求编号，修改内容后才使用新编号', async () => {
    vi.mocked(communityAPI.sendMessage).mockRejectedValue({ reason: 'COMMUNITY_CHAT_SEND_UNCERTAIN', message: '发送结果待确认' })
    const wrapper = mount(CommunityMessagesPanel)
    await flushPromises()
    await wrapper.find('textarea').setValue('第一条消息')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toBe('发送结果待确认')
    expect((wrapper.find('textarea').element as HTMLTextAreaElement).value).toBe('第一条消息')
    const id = vi.mocked(communityAPI.sendMessage).mock.calls[0][1]
    expect(id).toMatch(/^[0-9a-f-]{36}$/)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(communityAPI.sendMessage).toHaveBeenLastCalledWith('第一条消息', id)
    await wrapper.find('textarea').setValue('第二条消息')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(vi.mocked(communityAPI.sendMessage).mock.calls[2][1]).not.toBe(id)
    wrapper.unmount()
  })

  it('Telegram 明确拒绝后，保留输入并允许新请求重试', async () => {
    vi.mocked(communityAPI.sendMessage).mockRejectedValueOnce({ reason: 'COMMUNITY_CHAT_SEND_FAILED', message: '机器人无发言权限' })
    const wrapper = mount(CommunityMessagesPanel)
    await flushPromises()
    await wrapper.find('textarea').setValue('管理员通知')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    const id = vi.mocked(communityAPI.sendMessage).mock.calls[0][1]
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(vi.mocked(communityAPI.sendMessage).mock.calls[1][1]).not.toBe(id)
    expect((wrapper.find('textarea').element as HTMLTextAreaElement).value).toBe('')
    wrapper.unmount()
  })

  it('请求未结束时阻止重复发送，发送成功不推进读取游标', async () => {
    let finish: ((data: CommunityChatMessage) => void) | undefined
    vi.mocked(communityAPI.sendMessage).mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mount(CommunityMessagesPanel)
    await flushPromises()
    await wrapper.find('textarea').setValue('通知')
    await wrapper.find('form').trigger('submit')
    await wrapper.find('form').trigger('submit')
    expect(communityAPI.sendMessage).toHaveBeenCalledTimes(1)
    finish?.(message(20, { outgoing: true }))
    await flushPromises()
    vi.mocked(communityAPI.messages).mockResolvedValue({ items: [message(11), message(20, { outgoing: true })], has_more: false, latest_id: 20 })
    await wrapper.find('[data-test="refresh-messages"]').trigger('click')
    await flushPromises()
    expect(communityAPI.messages).toHaveBeenLastCalledWith({ after_id: 10 })
    expect(wrapper.findAll('article').map(node => node.attributes('data-message-id'))).toEqual(['10', '11', '20'])
    wrapper.unmount()
  })

  it('非活动标签停止自动请求，卸载清理轮询', async () => {
    const wrapper = mount(CommunityMessagesPanel, { props: { active: false } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10000)
    expect(communityAPI.messages).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ active: true })
    await flushPromises()
    expect(communityAPI.messages).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(10000)
    expect(communityAPI.messages).toHaveBeenCalledTimes(2)
  })
})
