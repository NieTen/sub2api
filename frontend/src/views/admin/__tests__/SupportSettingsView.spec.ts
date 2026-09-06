import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SupportSettingsView from '../SupportSettingsView.vue'
import { supportAPI, type SupportSettings } from '@/api/support'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))
vi.mock('@/api/support', () => ({
  supportAPI: { settings: vi.fn(), saveSettings: vi.fn() },
  supportError: (_cause: unknown, fallback: string) => fallback
}))

const settings: SupportSettings = {
  enabled: false, admin_emails: ['support@example.com'], telegram_chat_id: '-1001234567890',
  telegram_allowed_user_ids: [123], telegram_bot_token_configured: true,
  telegram_webhook_secret_configured: true, telegram_webhook_url: 'https://site.example.com/api/v1/support/telegram/webhook'
}
const render = () => mount(SupportSettingsView, { global: { stubs: { RouterLink: RouterLinkStub } } })

describe('机器人与群发管理页面', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(supportAPI.settings).mockResolvedValue(settings)
    vi.mocked(supportAPI.saveSettings).mockResolvedValue(settings)
  })

  it('入口直接加载可操作配置，并提供群发、VIP、成员和工单导航', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('h1').text()).toBe('communications.title')
    expect(wrapper.find('form').exists()).toBe(true)
    const links = wrapper.findAllComponents(RouterLinkStub).map(link => link.props('to'))
    expect(links).toEqual(expect.arrayContaining(['/admin/communications', '/admin/bulk-emails', '/admin/community/settings', '/admin/community/members', '/admin/tickets']))
    expect(wrapper.find('nav [aria-current="page"]').text()).toContain('communications.bot')
    expect(supportAPI.saveSettings).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('关闭工单通知仍能配置社群共用机器人，保存成功后清空新密钥', async () => {
    const wrapper = render()
    await flushPromises()
    const passwords = wrapper.findAll('input[type="password"]')
    expect(passwords[0].attributes('disabled')).toBeUndefined()
    expect((passwords[0].element as HTMLInputElement).value).toBe('')
    await passwords[0].setValue('123:new-token')
    await passwords[1].setValue('new-webhook-secret')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(supportAPI.saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      enabled: false, telegram_bot_token: '123:new-token', telegram_webhook_secret: 'new-webhook-secret',
      admin_emails: ['support@example.com'], telegram_allowed_user_ids: [123]
    }))
    expect((wrapper.find('input[type="password"]').element as HTMLInputElement).value).toBe('')
    wrapper.unmount()
  })

  it('展示保存的 HTTPS 回调地址，配置加载失败时不得保存空值', async () => {
    const wrapper = render()
    await flushPromises()
    expect((wrapper.find('input[readonly]').element as HTMLInputElement).value).toBe(settings.telegram_webhook_url)
    expect(wrapper.text()).toContain('support.webhookHelp')
    wrapper.unmount()
    vi.mocked(supportAPI.settings).mockRejectedValue(new Error('无法读取设置'))
    const failed = render()
    await flushPromises()
    expect(failed.find('form').exists()).toBe(false)
    expect(failed.find('[role="alert"]').text()).toBe('support.loadFailed')
    expect(supportAPI.saveSettings).not.toHaveBeenCalled()
    failed.unmount()
  })
})
