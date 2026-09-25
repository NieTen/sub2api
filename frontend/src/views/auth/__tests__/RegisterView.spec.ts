import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import RegisterView from '@/views/auth/RegisterView.vue'

const {
  getPublicSettingsMock,
  sendVerifyCodeMock,
  validateInvitationCodeMock,
  validatePromoCodeMock,
  registerMock,
  showErrorMock,
  pushMock,
  verifyActionMock,
  resetCaptchaMock,
  appStoreMock
} = vi.hoisted(() => ({
  getPublicSettingsMock: vi.fn(),
  sendVerifyCodeMock: vi.fn(),
  validateInvitationCodeMock: vi.fn(),
  validatePromoCodeMock: vi.fn(),
  registerMock: vi.fn(),
  showErrorMock: vi.fn(),
  pushMock: vi.fn(),
  verifyActionMock: vi.fn(),
  resetCaptchaMock: vi.fn(),
  appStoreMock: {
    cachedPublicSettings: null as { promo_code_enabled?: boolean } | null,
    showError: (...args: unknown[]) => showErrorMock(...args),
    showSuccess: vi.fn(),
    showWarning: vi.fn()
  }
}))

const publicSettings = {
  registration_enabled: true,
  email_verify_enabled: false,
  promo_code_enabled: false,
  invitation_code_enabled: false,
  affiliate_enabled: true,
  turnstile_enabled: true,
  turnstile_site_key: 'site-key',
  site_name: 'Sub2API',
  registration_email_suffix_whitelist: [],
  linuxdo_oauth_enabled: false,
  wechat_oauth_enabled: false,
  oidc_oauth_enabled: false,
  github_oauth_enabled: false,
  google_oauth_enabled: false
}

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock }),
  useRoute: () => ({ query: {} })
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key
    }
  }),
  useI18n: () => ({
    t: (key: string) =>
      key === 'auth.emailDomainRegistrationLimit'
        ? '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
        : key,
    locale: { value: 'en' }
  })
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ register: (...args: unknown[]) => registerMock(...args) }),
  useAppStore: () => appStoreMock
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: unknown[]) => getPublicSettingsMock(...args),
    sendVerifyCode: (...args: unknown[]) => sendVerifyCodeMock(...args),
    validateInvitationCode: (...args: unknown[]) => validateInvitationCodeMock(...args),
    validatePromoCode: (...args: unknown[]) => validatePromoCodeMock(...args)
  }
})

function mountRegister() {
  return mount(RegisterView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true,
        TurnstileWidget: {
          template: '<div data-testid="turnstile-widget" />',
          methods: { verifyAction: verifyActionMock, reset: resetCaptchaMock }
        },
        LoginAgreementPrompt: {
          template: '<button data-testid="accept-registration-agreement" type="button" @click="$emit(\'accept\')">同意协议</button>'
        },
        EmailOAuthButtons: true,
        LinuxDoOAuthSection: true,
        WechatOAuthSection: true,
        OidcOAuthSection: true,
        RouterLink: true,
        transition: false
      }
    }
  })
}

enableAutoUnmount(afterEach)

describe('RegisterView', () => {
  beforeEach(() => {
    getPublicSettingsMock.mockReset()
    sendVerifyCodeMock.mockReset()
    validateInvitationCodeMock.mockReset()
    validatePromoCodeMock.mockReset()
    registerMock.mockReset()
    showErrorMock.mockReset()
    pushMock.mockReset()
    verifyActionMock.mockReset()
    resetCaptchaMock.mockReset()
    appStoreMock.showWarning.mockReset()
    appStoreMock.showSuccess.mockReset()
    appStoreMock.cachedPublicSettings = null
    sessionStorage.removeItem('register_data')
    localStorage.removeItem('sub2api_login_agreement_consent')
    verifyActionMock.mockResolvedValue({ token: 'ticket', randstr: 'randstr' })
    getPublicSettingsMock.mockResolvedValue(publicSettings)
    sendVerifyCodeMock.mockResolvedValue({ message: 'sent', countdown: 60 })
    validateInvitationCodeMock.mockResolvedValue({ valid: true })
    validatePromoCodeMock.mockResolvedValue({ valid: true, bonus_amount: 1 })
    registerMock.mockResolvedValue({})
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it.each([
    [true, 'auth.registerEmailVerificationDescription'],
    [false, 'auth.registerAccountDescription']
  ])('describes the configured email verification flow: %s', async (enabled, description) => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      site_name: '开发者服务',
      email_verify_enabled: enabled
    })

    const wrapper = mountRegister()
    await flushPromises()

    expect(wrapper.get('h2').text()).toBe('auth.signUp 开发者服务')
    expect(wrapper.text()).toContain(description)
    expect(wrapper.findAll('.auth-registration-benefits span')).toHaveLength(3)
    expect(wrapper.get('#confirmPassword').exists()).toBe(true)
  })

  it('hides the registration form and benefits when registration is disabled', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      registration_enabled: false
    })

    const wrapper = mountRegister()
    await flushPromises()

    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.find('.auth-registration-benefits').exists()).toBe(false)
    expect(wrapper.text()).toContain('auth.registrationDisabled')
  })

  it('does not flash the promo-code field before disabled settings finish loading', async () => {
    let resolveSettings!: (settings: typeof publicSettings) => void
    getPublicSettingsMock.mockReturnValueOnce(
      new Promise<typeof publicSettings>((resolve) => {
        resolveSettings = resolve
      })
    )

    const wrapper = mountRegister()

    expect(wrapper.find('#promo_code').exists()).toBe(false)

    resolveSettings(publicSettings)
    await flushPromises()

    expect(wrapper.find('#promo_code').exists()).toBe(false)
  })

  it('uses injected public settings to show an enabled promo-code field on first render', () => {
    appStoreMock.cachedPublicSettings = { promo_code_enabled: true }
    getPublicSettingsMock.mockReturnValueOnce(new Promise(() => {}))

    const wrapper = mountRegister()

    expect(wrapper.find('#promo_code').exists()).toBe(true)
  })

  it.each([
    ['', 'auth.confirmPasswordRequired'],
    ['different-password', 'auth.passwordsDoNotMatch']
  ])('blocks invalid confirmation %j before captcha and allows correction', async (confirmation, error) => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      turnstile_enabled: false,
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'app-id'
    })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue(confirmation)
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith(error)
    expect(wrapper.get('#confirmPassword').classes()).toContain('input-error')
    expect(registerMock).not.toHaveBeenCalled()
    expect(verifyActionMock).not.toHaveBeenCalled()
    expect(pushMock).not.toHaveBeenCalled()
    expect(sessionStorage.getItem('register_data')).toBeNull()

    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.get('#confirmPassword').classes()).not.toContain('input-error')
    expect(verifyActionMock).toHaveBeenCalledOnce()
    expect(registerMock).toHaveBeenCalledWith({
      email: 'user@example.com',
      password: 'secret-123',
      turnstile_token: undefined,
      tencent_captcha_ticket: 'ticket',
      tencent_captcha_randstr: 'randstr',
      promo_code: undefined,
      invitation_code: undefined
    })
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
  })

  it('邮箱验证在注册页提交验证码且不保存明文密码或跳转验证页', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      turnstile_enabled: false,
      email_verify_enabled: true
    })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#verify_code').setValue('123456')
    await wrapper.get('#confirmPassword').setValue('different-password')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(sessionStorage.getItem('register_data')).toBeNull()
    expect(pushMock).not.toHaveBeenCalled()

    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(expect.objectContaining({
      email: 'user@example.com',
      password: 'secret-123',
      verify_code: '123456'
    }))
    expect(sessionStorage.getItem('register_data')).toBeNull()
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
    expect(pushMock).not.toHaveBeenCalledWith('/email-verify')
  })

  it('只填写邮箱即可发送验证码，发送请求不包含密码或邀请码', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false,
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'app-id'
    })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('[data-testid="send-register-code"]').trigger('click')
    await flushPromises()

    expect(verifyActionMock).toHaveBeenCalledOnce()
    expect(sendVerifyCodeMock).toHaveBeenCalledWith({
      email: 'user@example.com',
      tencent_captcha_ticket: 'ticket',
      tencent_captcha_randstr: 'randstr'
    })
    expect(resetCaptchaMock).toHaveBeenCalledOnce()
    expect(registerMock).not.toHaveBeenCalled()
    expect(showErrorMock).not.toHaveBeenCalled()

    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('#verify_code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(verifyActionMock).toHaveBeenCalledOnce()
    expect(registerMock).toHaveBeenCalledWith(expect.objectContaining({ verify_code: '123456' }))
    expect(registerMock.mock.calls[0][0].tencent_captcha_ticket).toBeUndefined()
    expect(registerMock.mock.calls[0][0].tencent_captcha_randstr).toBeUndefined()
    expect(sessionStorage.getItem('register_data')).toBeNull()
  })

  it('发送验证码消耗 Turnstile 凭证后仍能直接注册', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({ ...publicSettings, email_verify_enabled: true })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    wrapper.findComponent({ ref: 'turnstileRef' }).vm.$emit('verify', 'turnstile-proof')
    await flushPromises()
    await wrapper.get('[data-testid="send-register-code"]').trigger('click')
    await flushPromises()

    expect(sendVerifyCodeMock).toHaveBeenCalledWith({
      email: 'user@example.com',
      turnstile_token: 'turnstile-proof'
    })
    expect(resetCaptchaMock).toHaveBeenCalledOnce()
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('#verify_code').setValue('123456')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledOnce()
    expect(registerMock.mock.calls[0][0].turnstile_token).toBeUndefined()
    expect(verifyActionMock).not.toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
  })

  it.each([
    ['', 'auth.emailRequired'],
    ['invalid-email', 'auth.invalidEmail'],
    ['user@blocked.com', 'auth.emailSuffixNotAllowedWithAllowed']
  ])('发送验证码前拦截不合规邮箱 %j', async (email, error) => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com']
    })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue(email)
    await wrapper.get('[data-testid="send-register-code"]').trigger('click')
    await flushPromises()

    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
    expect(verifyActionMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenCalledWith(error)
  })

  it('域名配额开启时允许发送验证码，由后端检查邮箱域名额度', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true
    })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@custom.example')
    await wrapper.get('[data-testid="send-register-code"]').trigger('click')
    await flushPromises()

    expect(sendVerifyCodeMock).toHaveBeenCalledWith({ email: 'user@custom.example' })
  })

  it('发送验证码须先同意注册协议', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false,
      login_agreement_enabled: true,
      login_agreement_mode: 'checkbox',
      login_agreement_revision: 'registration-test',
      login_agreement_documents: [{ id: 'terms', title: '服务条款', content: '条款正文' }]
    })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('[data-testid="send-register-code"]').trigger('click')
    await flushPromises()
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="accept-registration-agreement"]').trigger('click')
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('[data-testid="send-register-code"]').trigger('click')
    await flushPromises()
    expect(sendVerifyCodeMock).toHaveBeenCalledOnce()
  })

  it('请求进行中不可重复发送，冷却时长使用接口返回值', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date'] })
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false
    })
    let resolveSend!: (value: { countdown: number }) => void
    sendVerifyCodeMock.mockReturnValueOnce(new Promise((resolve) => { resolveSend = resolve }))
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    const sendButton = wrapper.get('[data-testid="send-register-code"]')
    await sendButton.trigger('click')
    await flushPromises()
    expect(sendButton.attributes('disabled')).toBeDefined()
    await sendButton.trigger('click')
    expect(sendVerifyCodeMock).toHaveBeenCalledOnce()

    resolveSend({ countdown: 3 })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2000)
    expect(sendButton.attributes('disabled')).toBeDefined()
    await vi.advanceTimersByTimeAsync(1000)
    expect(sendButton.attributes('disabled')).toBeUndefined()
    await sendButton.trigger('click')
    await flushPromises()
    expect(sendVerifyCodeMock).toHaveBeenCalledTimes(2)
  })

  it('验证码发送失败后重置人机验证并允许重试', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false,
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'app-id'
    })
    sendVerifyCodeMock.mockRejectedValueOnce({ message: '邮件发送失败' })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    const sendButton = wrapper.get('[data-testid="send-register-code"]')
    await sendButton.trigger('click')
    await flushPromises()

    expect(resetCaptchaMock).toHaveBeenCalledOnce()
    expect(showErrorMock).toHaveBeenCalled()
    expect(sendButton.attributes('disabled')).toBeUndefined()
    await sendButton.trigger('click')
    await flushPromises()
    expect(verifyActionMock).toHaveBeenCalledTimes(2)
    expect(sendVerifyCodeMock).toHaveBeenCalledTimes(2)
  })

  it.each(['', '12345', 'abcdef'])('缺少或无效的六位验证码 %j 不发起注册请求', async (code) => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false
    })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('#verify_code').setValue(code)
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).not.toHaveBeenCalled()
    expect(verifyActionMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenCalled()
  })

  it('修改邮箱时清空旧邮箱的验证码', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false
    })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('first@example.com')
    await wrapper.get('#verify_code').setValue('123456')
    await wrapper.get('#email').setValue('second@example.com')

    expect((wrapper.get('#verify_code').element as HTMLInputElement).value).toBe('')
  })

  it('验证码注册仍提交邀请码和优惠码', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false,
      invitation_code_enabled: true,
      promo_code_enabled: true
    })
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('#verify_code').setValue('123456')
    await wrapper.get('#invitation_code').setValue('INVITE-123')
    await wrapper.get('#promo_code').setValue('PROMO-123')
    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(expect.objectContaining({
      verify_code: '123456',
      invitation_code: 'INVITE-123',
      promo_code: 'PROMO-123'
    }))
  })

  it('等待邀请码校验时锁定邮箱并阻止发码与重复注册，校验成功后正常提交', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false,
      invitation_code_enabled: true
    })
    let resolveInvitation!: (value: { valid: boolean }) => void
    validateInvitationCodeMock.mockReturnValueOnce(new Promise((resolve) => {
      resolveInvitation = resolve
    }))
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('#verify_code').setValue('123456')
    await wrapper.get('#invitation_code').setValue('INVITE-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(validateInvitationCodeMock).toHaveBeenCalledOnce()
    expect(wrapper.get('#email').attributes('disabled')).toBeDefined()
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="send-register-code"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="send-register-code"]').trigger('click')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
    expect(registerMock).not.toHaveBeenCalled()
    expect(validateInvitationCodeMock).toHaveBeenCalledOnce()

    resolveInvitation({ valid: true })
    await flushPromises()
    expect(registerMock).toHaveBeenCalledOnce()
    expect(registerMock).toHaveBeenCalledWith(expect.objectContaining({
      email: 'user@example.com',
      verify_code: '123456',
      invitation_code: 'INVITE-123'
    }))
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
    expect(wrapper.get('#email').attributes('disabled')).toBeUndefined()
  })

  it('设置尚未加载时禁止提交注册', async () => {
    getPublicSettingsMock.mockReturnValueOnce(new Promise(() => {}))
    const wrapper = mountRegister()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).not.toHaveBeenCalled()
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
    expect(verifyActionMock).not.toHaveBeenCalled()
  })

  it('设置加载失败时禁止注册，重试成功后按邮箱验证设置显示表单', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    getPublicSettingsMock.mockRejectedValueOnce(new Error('设置加载失败'))
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(registerMock).not.toHaveBeenCalled()

    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      email_verify_enabled: true,
      turnstile_enabled: false
    })
    await wrapper.get('[data-testid="retry-registration-settings"]').trigger('click')
    await flushPromises()
    expect(getPublicSettingsMock).toHaveBeenCalledTimes(2)
    expect(wrapper.get('#verify_code').exists()).toBe(true)
    expect(wrapper.find('[data-testid="retry-registration-settings"]').exists()).toBe(false)
  })

  it('后端要求邮箱验证时自动显示验证码输入并给出中文提示键', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({ ...publicSettings, turnstile_enabled: false })
    registerMock.mockRejectedValueOnce({
      reason: 'EMAIL_VERIFY_REQUIRED',
      message: 'email verification is required'
    })
    const wrapper = mountRegister()
    await flushPromises()
    expect(wrapper.find('#verify_code').exists()).toBe(false)
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.get('#verify_code').exists()).toBe(true)
    expect(showErrorMock).toHaveBeenCalledWith('auth.emailVerificationRequired')
    expect(pushMock).not.toHaveBeenCalled()
    await wrapper.get('#verify_code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(registerMock).toHaveBeenLastCalledWith(expect.objectContaining({ verify_code: '123456' }))
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
  })

  it('keeps the optional affiliate invitation field before Turnstile', async () => {
    const wrapper = mountRegister()
    await flushPromises()

    const invitationField = wrapper.get('[data-testid="affiliate-invitation-field"]')
    const turnstile = wrapper.get('[data-testid="registration-turnstile"]')

    expect(invitationField.get('input').attributes('id')).toBe('affiliate_code')
    expect(invitationField.text()).toContain('common.optional')
    expect(
      invitationField.element.compareDocumentPosition(turnstile.element) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it('uses the mandatory invitation field without duplicating the affiliate field', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      invitation_code_enabled: true
    })

    const wrapper = mountRegister()
    await flushPromises()

    expect(wrapper.find('[data-testid="affiliate-invitation-field"]').exists()).toBe(false)
    expect(wrapper.get('#invitation_code').exists()).toBe(true)
  })

  it('submits a non-whitelist email domain so the backend can enforce its registration quota', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true
    })

    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('first@custom.example')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'first@custom.example' })
    )
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('shows the localized registration domain quota message returned by the backend', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true
    })
    registerMock.mockRejectedValueOnce({
      reason: 'EMAIL_DOMAIN_REGISTRATION_LIMIT',
      message: 'raw backend message'
    })

    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('second@custom.example')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith(
      '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
    )
  })

  // 域名限量注册开关默认关闭：恢复 PR5423 之前的客户端白名单预检，非白名单域名不发起注册请求。
  it('rejects a non-whitelist email domain locally when the domain quota switch is disabled', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com']
    })

    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('first@custom.example')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).not.toHaveBeenCalled()
    // 校验失败通过 validationToastMessage watcher 弹 toast
    expect(showErrorMock).toHaveBeenCalledWith('auth.emailSuffixNotAllowedWithAllowed')
    expect(wrapper.get('#email').classes()).toContain('input-error')
  })

  it('still submits whitelisted email domains when the domain quota switch is disabled', async () => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...publicSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com']
    })

    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@allowed.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'user@allowed.com' })
    )
    expect(showErrorMock).not.toHaveBeenCalled()
  })
})
