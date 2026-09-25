import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CommunitySettingsView from '../CommunitySettingsView.vue'
import { communityAPI } from '@/api/community'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))
vi.mock('@/api/client', () => ({ apiClient: {} }))
vi.mock('@/api/community', () => ({ communityAPI: { settings: vi.fn(), saveSettings: vi.fn() } }))

const settings = { enabled: false, require_paid_recharge: false, login_prompt_enabled: false, contact_url: '', group_chat_id: '', group_name: '', bot_username: '' }
const render = () => mount(CommunitySettingsView, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })

describe('社群管理设置', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(communityAPI.settings).mockImplementation(async () => ({ ...settings }))
    vi.mocked(communityAPI.saveSettings).mockResolvedValue(settings)
  })

  it('拒绝有用户名密码的客服地址', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('input[name="contact-url"]').setValue('https://admin:secret@example.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(communityAPI.saveSettings).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').text()).toBe('community.invalidContactURL')
    wrapper.unmount()
  })

  it('客服网页可以在社群关闭时独立保存', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('input[name="contact-url"]').setValue('https://help.example.com/chat')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(communityAPI.saveSettings).toHaveBeenCalledWith({ ...settings, contact_url: 'https://help.example.com/chat' })
    wrapper.unmount()
  })

  it.each(['-5391524769', '-1001234567890'])('启用时原样提交群组 ID %s 并规范化机器人用户名，不提交工单密钥', async groupChatID => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('input[name="community-enabled"]').setValue(true)
    await wrapper.find('input[name="group-name"]').setValue('用户社群')
    await wrapper.find('input[name="group-chat-id"]').setValue(groupChatID)
    await wrapper.find('input[name="bot-username"]').setValue('@zzzaiprobot')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(communityAPI.saveSettings).toHaveBeenCalledWith({ ...settings, enabled: true, contact_url: '', group_name: '用户社群', group_chat_id: groupChatID, bot_username: 'zzzaiprobot' })
    wrapper.unmount()
  })

  it.each([
    '请先在“机器人设置”中配置并保存 Webhook 验证密钥',
    'Telegram 机器人 @zzzaiprobot 缺少“邀请用户”权限，请在群管理员设置中开启后重新保存'
  ])('保存失败时保留后端具体提示和全部输入，允许外部配置修正后重试：%s', async message => {
    const configured = {
      ...settings, enabled: true, require_paid_recharge: true, login_prompt_enabled: true,
      contact_url: 'https://help.example.com/chat', group_name: '用户社群',
      group_chat_id: '-5391524769', bot_username: 'zzzaiprobot'
    }
    vi.mocked(communityAPI.settings).mockResolvedValue(configured)
    vi.mocked(communityAPI.saveSettings).mockRejectedValueOnce({
      status: 400, reason: 'COMMUNITY_INVALID', message
    })
    const wrapper = render()
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').text()).toBe(message)
    expect(communityAPI.saveSettings).toHaveBeenCalledWith(configured)
    for (const [name, value] of [
      ['contact-url', configured.contact_url], ['group-name', configured.group_name],
      ['group-chat-id', configured.group_chat_id], ['bot-username', configured.bot_username]
    ]) {
      expect((wrapper.find(`input[name="${name}"]`).element as HTMLInputElement).value).toBe(value)
    }
    for (const name of ['community-enabled', 'require-paid-recharge', 'login-prompt-enabled']) {
      expect((wrapper.find(`input[name="${name}"]`).element as HTMLInputElement).checked).toBe(true)
    }
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()

    vi.mocked(communityAPI.saveSettings).mockResolvedValueOnce(configured)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(communityAPI.saveSettings).toHaveBeenLastCalledWith(configured)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('保存 VIP 充值限制与登录提醒开关', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('input[name="login-prompt-enabled"]').attributes('disabled')).toBeDefined()
    await wrapper.find('input[name="require-paid-recharge"]').setValue(true)
    await wrapper.find('input[name="login-prompt-enabled"]').setValue(true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(communityAPI.saveSettings).toHaveBeenCalledWith({ ...settings, require_paid_recharge: true, login_prompt_enabled: true })
    wrapper.unmount()
  })
})
