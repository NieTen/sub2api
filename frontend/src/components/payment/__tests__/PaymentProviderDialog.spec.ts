import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import PaymentProviderDialog from '@/components/payment/PaymentProviderDialog.vue'
import { STRIPE_SDK_API_VERSION } from '@/components/payment/providerConfig'
import type { ProviderInstance } from '@/types/payment'

const mockShowError = vi.fn()

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: mockShowError }),
}))

const messages: Record<string, string> = {
  'admin.settings.payment.providerConfig': 'Credentials',
  'admin.settings.payment.easypayCustomMethods': 'Custom EasyPay methods',
  'admin.settings.payment.easypayCustomMethodsHint': 'Add provider-specific EasyPay type values.',
  'admin.settings.payment.addCustomMethod': 'Add method',
  'admin.settings.payment.customMethodType': 'Payment type',
  'admin.settings.payment.customMethodUpstreamType': 'Upstream type',
  'admin.settings.payment.customMethodDisplayName': 'Display name',
  'admin.settings.payment.customMethodDisplayNamePlaceholder': '信用卡',
  'admin.settings.payment.paymentGuideTrigger': 'View payment guide',
  'admin.settings.payment.alipayGuideSummary': 'Desktop prefers QR precreate and falls back to cashier; mobile prefers WAP checkout.',
  'admin.settings.payment.wxpayGuideSummary': 'Desktop prefers Native QR; mobile routes to JSAPI or H5 based on browser context.',
  'admin.settings.payment.airwallexGuideSummary': 'Use Payment Acceptance read/write only.',
  'admin.settings.payment.stripeWebhookHint': 'Configure Stripe webhook.',
  'admin.settings.payment.stripeWebhookApiVersionHint': 'Use Stripe API version {version}.',
  'admin.settings.payment.airwallexWebhookHint': 'Select payment_intent.succeeded and use the latest stable API version.',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      const message = messages[key] ?? key
      if (!params) return message
      return Object.entries(params).reduce(
        (value, [name, replacement]) => value.replaceAll(`{${name}}`, replacement),
        message,
      )
    },
  }),
}))

function providerFactory(overrides: Partial<ProviderInstance> = {}): ProviderInstance {
  return {
    id: 1,
    provider_key: 'airwallex',
    name: 'Airwallex',
    supported_types: ['airwallex'],
    enabled: true,
    payment_mode: '',
    refund_enabled: false,
    allow_user_refund: false,
    limits: '',
    sort_order: 0,
    ...overrides,
    config: overrides.provider_key === 'okpay'
      ? {
          notifyUrl: 'https://site.example.com/api/v1/payment/webhook/okpay',
          returnUrl: 'https://site.example.com/payment/result',
          ...overrides.config,
        }
      : overrides.config ?? {},
  }
}

function mountDialog(options: { editing?: ProviderInstance | null } = {}) {
  return mount(PaymentProviderDialog, {
    props: {
      show: true,
      saving: false,
      editing: options.editing ?? null,
      allKeyOptions: [
        { value: 'easypay', label: 'EasyPay' },
        { value: 'alipay', label: 'Alipay' },
        { value: 'wxpay', label: 'WeChat Pay' },
        { value: 'stripe', label: 'Stripe' },
        { value: 'airwallex', label: 'Airwallex' },
      ],
      enabledKeyOptions: [
        { value: 'easypay', label: 'EasyPay' },
        { value: 'alipay', label: 'Alipay' },
        { value: 'wxpay', label: 'WeChat Pay' },
        { value: 'airwallex', label: 'Airwallex' },
      ],
      allPaymentTypes: [
        { value: 'alipay', label: 'Alipay' },
        { value: 'wxpay', label: 'WeChat Pay' },
      ],
      redirectLabel: 'Redirect',
    },
    global: {
      stubs: {
        BaseDialog: {
          template: '<div><slot /><slot name="footer" /></div>',
        },
        Select: {
          props: ['modelValue', 'options', 'disabled'],
          template: '<div />',
        },
        ToggleSwitch: {
          template: '<div />',
        },
      },
    },
  })
}

describe('PaymentProviderDialog callback URLs', () => {
  it.each([
    ['https://notify.example.com/', 'https://return.example.com///', 'https://notify.example.com', 'https://return.example.com'],
    [' https://notify.example.com/sub/ ', ' https://return.example.com/site/ ', 'https://notify.example.com/sub', 'https://return.example.com/site'],
    ['https://notify.example.com', 'https://return.example.com', 'https://notify.example.com', 'https://return.example.com'],
    ['', '', window.location.origin, window.location.origin],
  ])('joins callback paths to %s and %s', async (notify, returnUrl, expectedNotify, expectedReturn) => {
    const provider = providerFactory({
      provider_key: 'easypay', name: 'EasyPay',
      config: { pid: 'pid-1', apiBase: 'https://pay.example.com' },
      supported_types: ['alipay'], payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })
    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    const bases = wrapper.findAll('input').filter(input => input.classes().includes('!rounded-r-none'))
    await bases[0].setValue(notify)
    await bases[1].setValue(returnUrl)
    await wrapper.find('form').trigger('submit')
    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> }
    expect(payload.config.notifyUrl).toBe(expectedNotify + '/api/v1/payment/webhook/easypay')
    expect(payload.config.returnUrl).toBe(expectedReturn + '/payment/result')
    wrapper.unmount()
  })
})

describe('PaymentProviderDialog payment guide', () => {
  it('创建 OKPay 使用正式 id/token 配置、固定跳转与禁用退款', async () => {
    const wrapper = mountDialog()
    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset('okpay')
    await nextTick()
    await wrapper.find('input[type="text"]').setValue('原生 OKPay')
    await wrapper.get('input[name="provider-id"]').setValue('shop-1')
    await wrapper.get('input[name="provider-token"]').setValue('token-1')
    await wrapper.get('input[name="provider-notify-base"]').setValue('https://site.example.com')
    await wrapper.get('input[name="provider-return-base"]').setValue('https://site.example.com')
    expect(wrapper.text()).not.toContain('admin.settings.payment.refundEnabled')
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ provider_key: 'okpay', payment_mode: 'redirect', supported_types: ['okpay'], refund_enabled: false, allow_user_refund: false, config: { id: 'shop-1', token: 'token-1', apiBase: 'https://api.okaypay.me/shop' } })
    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> }
    expect(payload.config.notifyUrl).toMatch(/\/api\/v1\/payment\/webhook\/okpay$/)
    expect(payload.config.signatureAlgorithm).toBe('hmac_sha256')
    expect(payload.config.debugLogging).toBe('false')
  })

  it('编辑 OKPay 空 token 保留服务端凭据且纠正旧退款开关', async () => {
    const provider = providerFactory({ provider_key: 'okpay', name: 'OKPay', refund_enabled: true, allow_user_refund: true, config: { id: 'shop-1', apiBase: 'https://api.okaypay.me/shop' } })
    const wrapper = mountDialog({ editing: provider })
    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    await wrapper.get('form').trigger('submit.prevent')
    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string>; refund_enabled: boolean; allow_user_refund: boolean }
    expect(payload.config).not.toHaveProperty('token')
    expect(payload.config.signatureAlgorithm).toBe('hmac_sha256')
    expect(payload.config.debugLogging).toBe('false')
    expect(payload.refund_enabled).toBe(false)
    expect(payload.allow_user_refund).toBe(false)
  })

  it('OKPay 缺省使用 HMAC，管理员可明确切换旧 MD5 并保留已保存选择', async () => {
    const provider = providerFactory({ provider_key: 'okpay', name: 'OKPay', config: { id: 'shop-1', apiBase: 'https://api.okaypay.me/shop' } })
    const wrapper = mountDialog({ editing: provider })
    const dialog = wrapper.vm as unknown as { loadProvider: (value: ProviderInstance) => void }
    dialog.loadProvider(provider)
    await nextTick()
    const selector = wrapper.getComponent('[name="provider-signatureAlgorithm"]')
    expect(selector.props('modelValue')).toBe('hmac_sha256')
    expect(selector.props('options')).toEqual([
      expect.objectContaining({ value: 'hmac_sha256', label: 'admin.settings.payment.okpaySignatureHmac' }),
      expect.objectContaining({ value: 'legacy_md5', label: 'admin.settings.payment.okpaySignatureLegacy' }),
    ])
    selector.vm.$emit('update:modelValue', 'legacy_md5')
    await nextTick()
    await wrapper.get('form').trigger('submit.prevent')
    const payload = wrapper.emitted('save')?.at(-1)?.[0] as { config: Record<string, string> }
    expect(payload.config.signatureAlgorithm).toBe('legacy_md5')
    expect(payload.config).not.toHaveProperty('token')

    dialog.loadProvider({ ...provider, config: { ...provider.config, signatureAlgorithm: 'legacy_md5' } })
    await nextTick()
    expect(selector.props('modelValue')).toBe('legacy_md5')
    dialog.loadProvider(provider)
    await nextTick()
    expect(selector.props('modelValue')).toBe('hmac_sha256')
    wrapper.unmount()
  })

  it('OKPay 调试日志默认关闭，支持保存开关并保留服务端 Token', async () => {
    const provider = providerFactory({ provider_key: 'okpay', name: 'OKPay', config: { id: 'shop-1', apiBase: 'https://api.okaypay.me/shop' } })
    const wrapper = mountDialog({ editing: provider })
    const dialog = wrapper.vm as unknown as { loadProvider: (value: ProviderInstance) => void }
    dialog.loadProvider(provider)
    await nextTick()
    const selector = wrapper.getComponent('[name="provider-debugLogging"]')
    expect(selector.props('modelValue')).toBe('false')
    expect(selector.props('options')).toEqual([
      expect.objectContaining({ value: 'false', label: 'admin.settings.payment.okpayDebugLoggingDisabled' }),
      expect.objectContaining({ value: 'true', label: 'admin.settings.payment.okpayDebugLoggingEnabled' }),
    ])
    expect(selector.attributes('aria-label')).toBe('admin.settings.payment.field_debugLogging')
    expect(wrapper.text()).toContain('admin.settings.payment.field_okpayDebugLoggingHint')

    for (const enabled of ['true', 'false']) {
      selector.vm.$emit('update:modelValue', enabled)
      await nextTick()
      await wrapper.get('form').trigger('submit.prevent')
      const payload = wrapper.emitted('save')?.at(-1)?.[0] as { config: Record<string, string> }
      expect(payload.config.debugLogging).toBe(enabled)
      expect(payload.config).not.toHaveProperty('token')
      dialog.loadProvider({ ...provider, config: payload.config })
      await nextTick()
      expect(selector.props('modelValue')).toBe(enabled)
    }

    dialog.loadProvider({ ...provider, config: { ...provider.config, debugLogging: 'true' } })
    await nextTick()
    expect(selector.props('modelValue')).toBe('true')
    dialog.loadProvider(provider)
    await nextTick()
    expect(selector.props('modelValue')).toBe('false')
    wrapper.unmount()
  })

  it('切换 OKPay 调试日志不会改写新输入 Token 的空白和特殊字符', async () => {
    const provider = providerFactory({ provider_key: 'okpay', name: 'OKPay', config: { id: 'shop-1', apiBase: 'https://api.okaypay.me/shop' } })
    const wrapper = mountDialog({ editing: provider })
    ;(wrapper.vm as unknown as { loadProvider: (value: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    const token = '  token+%20&=中文  '
    await wrapper.get('input[name="provider-token"]').setValue(token)
    const selector = wrapper.getComponent('[name="provider-debugLogging"]')

    for (const enabled of ['true', 'false']) {
      selector.vm.$emit('update:modelValue', enabled)
      await nextTick()
      await wrapper.get('form').trigger('submit.prevent')
      const payload = wrapper.emitted('save')?.at(-1)?.[0] as { config: Record<string, string> }
      expect(payload.config.debugLogging).toBe(enabled)
      expect(payload.config.token).toBe(token)
    }
    wrapper.unmount()
  })

  it.each(['http://api.example.com/shop', 'https://user:password@api.example.com/shop', 'https://api.example.com/shop?token=test'])('拒绝不安全 OKPay API 地址 %s', async apiBase => {
    const provider = providerFactory({ provider_key: 'okpay', config: { id: 'shop-1', apiBase } })
    const wrapper = mountDialog({ editing: provider })
    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it.each([
    ['notify', 'admin.settings.payment.validationOkpayNotifyBase'],
    ['return', 'admin.settings.payment.validationOkpayReturnBase'],
  ])('OKPay 保存时区分 %s 地址错误，不自动修改协议，允许 HTTPS 反代子路径', async (field, errorKey) => {
    const provider = providerFactory({ provider_key: 'okpay', name: 'OKPay', config: { id: 'shop-1', apiBase: 'https://api.okaypay.me/shop' } })
    const wrapper = mountDialog({ editing: provider })
    ;(wrapper.vm as unknown as { loadProvider: (value: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    const input = wrapper.get(`input[name="provider-${field}-base"]`)
    for (const invalid of [
      'http://site.example.com/sub', '/relative', '//site.example.com', 'https:///missing-host',
      'https://user:password@site.example.com', 'https://@site.example.com',
      'https://site.example.com/sub?token=private', 'https://site.example.com/sub?',
      'https://site.example.com/sub#section', 'https://site.example.com/sub#',
      'https://site.example.com\\sub', 'https://site.example.com/%ZZ', 'https://site.example.com/%2', 'https://site.example.com/%',
    ]) {
      mockShowError.mockClear()
      await input.setValue(invalid)
      await wrapper.get('form').trigger('submit.prevent')
      await flushPromises()
      expect(wrapper.emitted('save')).toBeUndefined()
      expect(mockShowError).toHaveBeenLastCalledWith(errorKey)
      expect((input.element as HTMLInputElement).value).toBe(invalid)
    }
    const configKey = field === 'notify' ? 'notifyUrl' : 'returnUrl'
    const suffix = field === 'notify' ? '/api/v1/payment/webhook/okpay' : '/payment/result'
    ;(wrapper.vm as unknown as { loadProvider: (value: ProviderInstance) => void }).loadProvider({
      ...provider, config: { ...provider.config, [configKey]: 'https://si\tte.example.com' + suffix },
    })
    await nextTick()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(wrapper.emitted('save')).toBeUndefined()
    expect(mockShowError).toHaveBeenLastCalledWith(errorKey)
    await wrapper.get('input[name="provider-notify-base"]').setValue(' https://notify.example.com/proxy/sub/ ')
    await wrapper.get('input[name="provider-return-base"]').setValue(' https://return.example.com/site/ ')
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({
      config: {
        notifyUrl: 'https://notify.example.com/proxy/sub/api/v1/payment/webhook/okpay',
        returnUrl: 'https://return.example.com/site/payment/result',
      },
    })
    wrapper.unmount()
  })

  it('OKPay 基础地址保留合法百分号转义和中文子路径', async () => {
    const provider = providerFactory({ provider_key: 'okpay', name: 'OKPay', config: { id: 'shop-1', apiBase: 'https://api.okaypay.me/shop' } })
    const wrapper = mountDialog({ editing: provider })
    ;(wrapper.vm as unknown as { loadProvider: (value: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    await wrapper.get('input[name="provider-notify-base"]').setValue('https://site.example.com/proxy%20path/中文/')
    await wrapper.get('input[name="provider-return-base"]').setValue('https://site.example.com/返回/%20/')
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({
      config: {
        notifyUrl: 'https://site.example.com/proxy%20path/中文/api/v1/payment/webhook/okpay',
        returnUrl: 'https://site.example.com/返回/%20/payment/result',
      },
    })
    wrapper.unmount()
  })

  it('已有 HTTP 回调配置开启调试时仍需先修正地址，空值不能绕过 HTTP 当前站点校验', async () => {
    const provider = providerFactory({ provider_key: 'okpay', name: 'OKPay', config: {
      id: 'shop-1', apiBase: 'https://api.okaypay.me/shop', notifyUrl: 'http://site.example.com/api/v1/payment/webhook/okpay',
    } })
    const wrapper = mountDialog({ editing: provider })
    ;(wrapper.vm as unknown as { loadProvider: (value: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    wrapper.getComponent('[name="provider-debugLogging"]').vm.$emit('update:modelValue', 'true')
    await nextTick()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(wrapper.emitted('save')).toBeUndefined()
    expect(mockShowError).toHaveBeenLastCalledWith('admin.settings.payment.validationOkpayNotifyBase')
    expect(window.location.protocol).toBe('http:')
    await wrapper.get('input[name="provider-notify-base"]').setValue('')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(wrapper.emitted('save')).toBeUndefined()
    await wrapper.get('input[name="provider-notify-base"]').setValue('https://site.example.com')
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ config: { debugLogging: 'true', notifyUrl: 'https://site.example.com/api/v1/payment/webhook/okpay' } })
    wrapper.unmount()
  })

  it.each([undefined, 'test-key'])('TRC20 仅必填有效收款地址，支持可选密钥 %s 并隐藏官方节点', async (apiKey) => {
    const wrapper = mountDialog()
    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset('usdt_trc20')
    await nextTick()
    await wrapper.find('input[type="text"]').setValue('TRC20 直收')
    await wrapper.get('input[name="provider-walletAddress"]').setValue('无效地址')
    if (apiKey) await wrapper.get('[name="provider-apiKey"]').setValue(apiKey)
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('save')).toBeUndefined()
    await wrapper.get('input[name="provider-walletAddress"]').setValue('TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE')
    expect(wrapper.find('[name="provider-apiBase"]').exists()).toBe(false)
    expect(wrapper.find('[name="clear-trc20-api-key"]').exists()).toBe(false)
    await wrapper.get('form').trigger('submit.prevent')
    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> }
    expect(payload).toMatchObject({ provider_key: 'usdt_trc20', payment_mode: 'qrcode', supported_types: ['usdt_trc20'], refund_enabled: false, config: { apiBase: 'https://api.trongrid.io', walletAddress: 'TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE' } })
    if (apiKey) expect(payload.config.apiKey).toBe(apiKey)
    else expect(payload.config).not.toHaveProperty('apiKey')
    wrapper.unmount()
  })

  it('编辑 TRC20 留空保留密钥，仅明确选择公共查询才清除', async () => {
    const provider = providerFactory({
      provider_key: 'usdt_trc20', name: 'TRC20 直收',
      config: { walletAddress: 'TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE', apiBase: 'https://api.trongrid.io' },
      supported_types: ['usdt_trc20'], payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })
    const dialog = wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }
    dialog.loadProvider(provider)
    await nextTick()
    expect(wrapper.find('[name="provider-apiBase"]').exists()).toBe(false)
    await wrapper.get('form').trigger('submit.prevent')
    let payload = wrapper.emitted('save')?.at(-1)?.[0] as { config: Record<string, string> }
    expect(payload.config).not.toHaveProperty('apiKey')

    await wrapper.get('[name="provider-apiKey"]').setValue('replacement-key')
    await wrapper.get('[name="clear-trc20-api-key"]').setValue(true)
    expect(wrapper.get('[name="provider-apiKey"]').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit.prevent')
    payload = wrapper.emitted('save')?.at(-1)?.[0] as { config: Record<string, string> }
    expect(payload.config.apiKey).toBe('')

    await wrapper.get('[name="clear-trc20-api-key"]').setValue(false)
    await wrapper.get('form').trigger('submit.prevent')
    payload = wrapper.emitted('save')?.at(-1)?.[0] as { config: Record<string, string> }
    expect(payload.config.apiKey).toBe('replacement-key')

    await wrapper.get('[name="clear-trc20-api-key"]').setValue(true)
    dialog.loadProvider(provider)
    await nextTick()
    expect((wrapper.get('[name="clear-trc20-api-key"]').element as HTMLInputElement).checked).toBe(false)
    await wrapper.get('form').trigger('submit.prevent')
    payload = wrapper.emitted('save')?.at(-1)?.[0] as { config: Record<string, string> }
    expect(payload.config).not.toHaveProperty('apiKey')
    wrapper.unmount()
  })

  it('保留 EasyPay 原有 usdt_trc20 自定义映射及退款设置', async () => {
    const provider = providerFactory({ provider_key: 'easypay', name: '旧聚合支付', supported_types: ['usdt_trc20'], refund_enabled: true, config: { pid: 'pid-1', apiBase: 'https://legacy.example.com', customMethods: JSON.stringify([{ type: 'usdt_trc20', upstreamType: 'trc20', displayName: '旧 USDT' }]) } })
    const wrapper = mountDialog({ editing: provider })
    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ provider_key: 'easypay', supported_types: ['usdt_trc20'], refund_enabled: true, config: { customMethods: provider.config.customMethods } })
  })
  it('shows no payment guide for providers without a flow guide', () => {
    const wrapper = mountDialog()

    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.alipayGuideSummary'])
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.wxpayGuideSummary'])
    expect(wrapper.find('button[title="View payment guide"]').exists()).toBe(false)
  })

  it.each([
    ['alipay', 'admin.settings.payment.alipayGuideSummary'],
    ['wxpay', 'admin.settings.payment.wxpayGuideSummary'],
    ['airwallex', 'admin.settings.payment.airwallexGuideSummary'],
  ])('shows the payment guide summary for %s', async (providerKey, summaryKey) => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset(providerKey)
    await nextTick()

    expect(wrapper.text()).toContain(messages[summaryKey])
    expect(wrapper.find('button[title="View payment guide"]').exists()).toBe(true)
  })

  it('shows Airwallex webhook event and API version guidance with the webhook URL', async () => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset('airwallex')
    await nextTick()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.airwallexWebhookHint'])
    expect(wrapper.text()).toContain('/api/v1/payment/webhook/airwallex')
  })

  it('shows Stripe webhook API version guidance with the integrated SDK version', async () => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset('stripe')
    await nextTick()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.stripeWebhookHint'])
    expect(wrapper.text()).toContain(`Use Stripe API version ${STRIPE_SDK_API_VERSION}.`)
    expect(wrapper.text()).toContain('/api/v1/payment/webhook/stripe')
  })

  it('emits an empty Airwallex accountId when the admin clears it', async () => {
    const provider = providerFactory({
      config: {
        clientId: 'cid_123',
        apiBase: 'https://api.airwallex.com/api/v1',
        countryCode: 'CN',
        currency: 'CNY',
        accountId: 'acct_123',
      },
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()

    const accountIdInput = wrapper
      .findAll('input[type="text"]')
      .find(input => (input.element as HTMLInputElement).value === 'acct_123')
    if (!accountIdInput) throw new Error('accountId input not found')

    await accountIdInput.setValue('')
    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> }
    expect(payload.config.accountId).toBe('')
  })

  it.each(['epay', 'usdt.trc20'])('serializes EasyPay upstream type %s and adds the local type to supported_types', async (upstreamType) => {
    const provider = providerFactory({
      provider_key: 'easypay',
      name: 'EasyPay',
      config: {
        pid: 'pid-1',
        apiBase: 'https://pay.example.com',
        notifyUrl: 'https://example.com/api/v1/payment/webhook/easypay',
        returnUrl: 'https://example.com/payment/result',
      },
      supported_types: ['alipay', 'wxpay'],
      payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()

    await wrapper.find('button.btn-sm').trigger('click')
    await nextTick()

    const inputs = wrapper.findAll('input[type="text"]')
    const customTypeInputs = inputs.filter(input => (input.element as HTMLInputElement).placeholder === 'credit_card')
    const ldcTypeInput = customTypeInputs[0]
    const upstreamTypeInput = customTypeInputs[1]
    const displayNameInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === '信用卡')
    if (!ldcTypeInput || !upstreamTypeInput || !displayNameInput) {
      throw new Error('custom method inputs not found')
    }

    await ldcTypeInput.setValue('ldc')
    await upstreamTypeInput.setValue(upstreamType)
    await displayNameInput.setValue('LDC')
    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as {
      config: Record<string, string>
      supported_types: string[]
    }
    expect(JSON.parse(payload.config.customMethods)).toEqual([{ type: 'ldc', upstreamType, displayName: 'LDC' }])
    expect(payload.supported_types).toEqual(['alipay', 'wxpay', 'ldc'])
  })

  it.each([
    ['alipay_hk', 'hkpay'],
    ['usdt.trc20', 'usdt.trc20'],
    ['usdt_trc20', 'usdt/trc20'],
  ])('rejects invalid EasyPay mapping %s to %s', async (type, upstreamType) => {
    const provider = providerFactory({
      provider_key: 'easypay',
      name: 'EasyPay',
      config: {
        pid: 'pid-1',
        apiBase: 'https://pay.example.com',
        notifyUrl: 'https://example.com/api/v1/payment/webhook/easypay',
        returnUrl: 'https://example.com/payment/result',
      },
      supported_types: ['alipay', 'wxpay'],
      payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()

    await wrapper.find('button.btn-sm').trigger('click')
    await nextTick()

    const inputs = wrapper.findAll('input[type="text"]')
    const customTypeInputs = inputs.filter(input => (input.element as HTMLInputElement).placeholder === 'credit_card')
    const typeInput = customTypeInputs[0]
    const upstreamTypeInput = customTypeInputs[1]
    const displayNameInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === '信用卡')
    if (!typeInput || !upstreamTypeInput || !displayNameInput) {
      throw new Error('custom method inputs not found')
    }

    await typeInput.setValue(type)
    await upstreamTypeInput.setValue(upstreamType)
    await displayNameInput.setValue('Custom payment')
    await wrapper.find('form').trigger('submit.prevent')

    expect(wrapper.emitted('save')).toBeUndefined()
  })
})
