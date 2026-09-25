import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SupportSettingsView from '../SupportSettingsView.vue'
import { supportAPI, type SupportSettings } from '@/api/support'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))
vi.mock('@/api/client', () => ({ apiClient: {} }))
vi.mock('@/api/support', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/api/support')>(),
  supportAPI: { settings: vi.fn(), saveSettings: vi.fn() }
}))

const settings: SupportSettings = {
  enabled: false, admin_emails: ['support@example.com'], telegram_chat_id: '-1001234567890',
  telegram_allowed_user_ids: [123], telegram_bot_token_configured: true,
  telegram_webhook_secret_configured: true, telegram_webhook_path: '/api/v1/support/telegram/webhook'
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

  it('负数群组 ID 与超过 32 位的个人用户 ID 原样保存，并关联字段填写说明', async () => {
    const wrapper = render()
    await flushPromises()
    const chatId = wrapper.find('input[aria-describedby="support-chat-id-hint"]')
    const allowedUsers = wrapper.find('textarea[aria-describedby="support-allowed-users-hint"]')
    await chatId.setValue('-1001234567890')
    await allowedUsers.setValue('5939067819')
    expect(wrapper.find('#support-chat-id-hint').text()).toBe('support.chatIdHint')
    expect(wrapper.find('#support-webhook-secret-hint').text()).toBe('support.webhookSecretHint')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(supportAPI.saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      telegram_chat_id: '-1001234567890', telegram_allowed_user_ids: [5939067819]
    }))
    wrapper.unmount()
  })

  it.each([
    ['少于 16 个字符', 'short-secret'],
    ['超过 256 个字符', 'a'.repeat(257)],
    ['包含标点', 'invalid-secret!!'],
    ['包含中文', 'invalid-secret中文'],
    ['包含全角字母', 'invalid-secretＡＢ'],
    ['包含内部空格', 'invalid secret--']
  ])('新输入的 Webhook 密钥%s时阻止保存并显示准确提示', async (_name, secret) => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('input[aria-describedby="support-webhook-secret-hint"]').setValue(secret)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(supportAPI.saveSettings).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').text()).toBe('support.invalidWebhookSecret')
    wrapper.unmount()
  })

  it.each([16, 256])('接受 %i 个合法 ASCII 字符的 Webhook 密钥', async length => {
    const wrapper = render()
    await flushPromises()
    const secret = `Az09_-${'a'.repeat(length - 6)}`
    await wrapper.find('input[aria-describedby="support-webhook-secret-hint"]').setValue(secret)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(supportAPI.saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      telegram_webhook_secret: secret
    }))
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('已配置密钥时输入空白仍保留原值，不设置清除标记', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('input[aria-describedby="support-webhook-secret-hint"]').setValue('   ')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(supportAPI.saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      telegram_webhook_secret: '', clear_telegram_webhook_secret: false,
      telegram_bot_token: '', clear_telegram_bot_token: false
    }))
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('明确勾选清除密钥时不校验已禁用输入框中的旧值', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('input[aria-describedby="support-webhook-secret-hint"]').setValue('short')
    const clearSecret = wrapper.findAll('label').find(label => label.text() === 'support.clearSecret')!
    await clearSecret.find('input').setValue(true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(supportAPI.saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      telegram_webhook_secret: '', clear_telegram_webhook_secret: true
    }))
    wrapper.unmount()
  })

  it('保存失败持续展示后端具体错误，不替换成通用保存失败文案', async () => {
    const message = 'Webhook 验证密钥格式无效：长度必须为 16–256 个字符。'
    vi.mocked(supportAPI.saveSettings).mockRejectedValueOnce({
      message, reason: 'SUPPORT_DELIVERY_INVALID', metadata: { field: 'telegram_webhook_secret' }
    })
    const wrapper = render()
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toBe(message)
    await wrapper.find('input[aria-describedby="support-chat-id-hint"]').setValue('-1009876543210')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toBe(message)
    expect(wrapper.find('form').exists()).toBe(true)
    wrapper.unmount()
  })

  it('展示保存的 HTTPS 回调地址，配置加载失败时不得保存空值', async () => {
    const wrapper = render()
    await flushPromises()
    expect((wrapper.find('input[readonly]').element as HTMLInputElement).value).toBe(`${window.location.origin}${settings.telegram_webhook_path}`)
    expect(wrapper.text()).toContain('support.webhookHelp')
    wrapper.unmount()
    vi.mocked(supportAPI.settings).mockRejectedValue(new Error('无法读取设置'))
    const failed = render()
    await flushPromises()
    expect(failed.find('form').exists()).toBe(false)
    expect(failed.find('[role="alert"]').text()).toBe('无法读取设置')
    expect(supportAPI.saveSettings).not.toHaveBeenCalled()
    failed.unmount()
  })

  it('兼容旧版后端返回的完整回调地址', async () => {
    vi.mocked(supportAPI.settings).mockResolvedValue({
      ...settings,
      telegram_webhook_path: undefined,
      telegram_webhook_url: 'https://legacy.example.com/api/v1/support/telegram/webhook'
    })
    const wrapper = render()
    await flushPromises()
    expect((wrapper.find('input[readonly]').element as HTMLInputElement).value).toBe('https://legacy.example.com/api/v1/support/telegram/webhook')
    wrapper.unmount()
  })
})
