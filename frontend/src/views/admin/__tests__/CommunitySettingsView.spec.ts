import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CommunitySettingsView from '../CommunitySettingsView.vue'
import { communityAPI } from '@/api/community'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))
vi.mock('@/api/support', () => ({ supportError: (_error: unknown, fallback: string) => fallback }))
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

  it('启用时提交群组配置并规范化机器人用户名，不提交工单密钥', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('input[name="community-enabled"]').setValue(true)
    await wrapper.find('input[name="group-name"]').setValue('用户社群')
    await wrapper.find('input[name="group-chat-id"]').setValue('-1001234567890')
    await wrapper.find('input[name="bot-username"]').setValue('@test_bot')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(communityAPI.saveSettings).toHaveBeenCalledWith({ ...settings, enabled: true, contact_url: '', group_name: '用户社群', group_chat_id: '-1001234567890', bot_username: 'test_bot' })
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
