import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
import { formatPaymentAmount } from '@/components/payment/currency'
import AmountInput from '@/components/payment/AmountInput.vue'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
import type { CheckoutInfoResponse, MethodLimit, SubscriptionPlan, UsdtCnyQuote } from '@/types/payment'

const routeState = vi.hoisted(() => ({
  path: '/purchase',
  query: {} as Record<string, unknown>,
}))

const routerReplace = vi.hoisted(() => vi.fn())
const routerPush = vi.hoisted(() => vi.fn())
const routerResolve = vi.hoisted(() => vi.fn(() => ({ href: '/payment/stripe?mock=1' })))
const createOrder = vi.hoisted(() => vi.fn())
const refreshUser = vi.hoisted(() => vi.fn())
const fetchActiveSubscriptions = vi.hoisted(() => vi.fn().mockResolvedValue(undefined))
const showError = vi.hoisted(() => vi.fn())
const showInfo = vi.hoisted(() => vi.fn())
const showWarning = vi.hoisted(() => vi.fn())
const getCheckoutInfo = vi.hoisted(() => vi.fn())
const bridgeInvoke = vi.hoisted(() => vi.fn())
const translate = vi.hoisted(() => vi.fn((key: string) => key))
// Public settings live in a reactive holder so tests can flip feature flags after mount
// and exercise the watchers that react to them.
const appStoreState = vi.hoisted(() => ({
  setPublicSettings: (_value: Record<string, unknown> | undefined) => {},
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({
      replace: routerReplace,
      push: routerPush,
      resolve: routerResolve,
    }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: translate,
    }),
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: {
      username: 'demo-user',
      balance: 0,
    },
    refreshUser,
  }),
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    createOrder,
  }),
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: [],
    fetchActiveSubscriptions,
  }),
}))

vi.mock('@/stores', async () => {
  const { reactive } = await import('vue')
  const state = reactive({ cachedPublicSettings: undefined as Record<string, unknown> | undefined })
  appStoreState.setPublicSettings = (value) => {
    state.cachedPublicSettings = value
  }
  return {
    useAppStore: () => ({
      showError,
      showInfo,
      showWarning,
      get cachedPublicSettings() {
        return state.cachedPublicSettings
      },
    }),
  }
})

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getCheckoutInfo,
  },
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => true,
}))

function checkoutInfoFixture(overrides: Partial<CheckoutInfoResponse> = {}) {
  const wxpayMethod: MethodLimit = {
    daily_limit: 0,
    daily_used: 0,
    daily_remaining: 0,
    single_min: 0,
    single_max: 0,
    fee_rate: 0,
    available: true,
  }
  const data: CheckoutInfoResponse = {
    methods: {
      wxpay: wxpayMethod,
    },
    global_min: 0,
    global_max: 0,
    plans: [],
    balance_disabled: false,
    balance_recharge_multiplier: 1,
    subscription_usd_to_cny_rate: 0,
    recharge_fee_rate: 0,
    help_text: '',
    help_image_url: '',
    stripe_publishable_key: '',
  }

  return {
    data: { ...data, ...overrides },
  }
}

function checkoutInfoWithPlansFixture(options: {
  checkout?: Partial<CheckoutInfoResponse>
  method?: Partial<MethodLimit>
  plan?: Partial<SubscriptionPlan>
} = {}) {
  const base = checkoutInfoFixture(options.checkout).data
  const plan: SubscriptionPlan = {
    id: 7,
    group_id: 3,
    name: 'Starter',
    description: '',
    price: 128,
    original_price: 0,
    validity_days: 30,
    validity_unit: 'day',
    rate_multiplier: 1,
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    features: [],
    group_platform: 'openai',
    sort_order: 1,
    for_sale: true,
    group_name: 'OpenAI',
    ...options.plan,
  }

  return {
    data: {
      ...base,
      methods: {
        ...base.methods,
        wxpay: {
          ...base.methods.wxpay,
          ...options.method,
        },
      },
      plans: [plan],
    },
  }
}

function jsapiOrderFixture(resumeToken: string) {
  return {
    order_id: 123,
    amount: 88,
    pay_amount: 88,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    payment_type: 'wxpay',
    out_trade_no: 'sub2_jsapi_123',
    result_type: 'jsapi_ready' as const,
    resume_token: resumeToken,
    jsapi: {
      appId: 'wx123',
      timeStamp: '1712345678',
      nonceStr: 'nonce',
      package: 'prepay_id=wx123',
      signType: 'RSA',
      paySign: 'signed',
    },
  }
}

function oauthOrderFixture() {
  return {
    order_id: 456,
    amount: 128,
    pay_amount: 128,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    payment_type: 'wxpay',
    result_type: 'oauth_required' as const,
    oauth: {
      authorize_url: '/api/v1/auth/oauth/wechat/payment/start?payment_type=wxpay&redirect=%2Fpurchase%3Ffrom%3Dwechat',
      appid: 'wx123',
      scope: 'snsapi_base',
      redirect_url: '/auth/wechat/payment/callback',
    },
  }
}

async function mountSubscriptionConfirm(options: Parameters<typeof checkoutInfoWithPlansFixture>[0] = {}) {
  vi.useRealTimers()
  routeState.path = '/purchase'
  routeState.query = {
    tab: 'subscription',
    group: '3',
  }
  routerReplace.mockReset().mockResolvedValue(undefined)
  routerPush.mockReset().mockResolvedValue(undefined)
  routerResolve.mockClear()
  createOrder.mockReset()
  refreshUser.mockReset()
  fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoWithPlansFixture(options))
  bridgeInvoke.mockReset()
  window.localStorage.clear()
  ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: {
          template: '<div><slot /></div>',
        },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

async function mountSubscriptionPlanList(planCount: number) {
  vi.useRealTimers()
  routeState.path = '/purchase'
  routeState.query = { tab: 'subscription' }
  routerReplace.mockReset().mockResolvedValue(undefined)
  routerPush.mockReset().mockResolvedValue(undefined)
  routerResolve.mockClear()
  createOrder.mockReset()
  refreshUser.mockReset()
  fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  const basePlan = checkoutInfoWithPlansFixture().data.plans[0]
  const plans = Array.from({ length: planCount }, (_, index) => ({
    ...basePlan,
    id: index + 1,
    name: `Plan ${index + 1}`,
  }))
  getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({ plans }))
  bridgeInvoke.mockReset()
  window.localStorage.clear()
  ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: {
          template: '<div><slot /></div>',
        },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

describe('PaymentView help text', () => {
  beforeEach(() => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    createOrder.mockReset()
    window.localStorage.clear()
  })

  async function mountHelp(help_text: string, help_image_url = '') {
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({ help_text, help_image_url }))
    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    return wrapper
  }

  it('renders headings, emphasis, links, and lists in payment help without starting checkout', async () => {
    const wrapper = await mountHelp('## Recharge help\n\n**Read first**\n\n- [Contact support](https://example.com/help)')
    const help = wrapper.get('.markdown-body')
    expect(help.get('h2').text()).toBe('Recharge help')
    expect(help.get('strong').text()).toBe('Read first')
    expect(help.get('li a').attributes('href')).toBe('https://example.com/help')
    expect(createOrder).not.toHaveBeenCalled()
  })

  it('removes scripts, event handlers, and unsafe URLs from rendered help', async () => {
    const wrapper = await mountHelp([
      '<script>alert(1)</script>',
      '<img src="https://example.com/help.png" onerror="alert(1)">',
      '[Unsafe](javascript:alert%281%29)',
      '[Support](https://example.com/help)',
    ].join('\n\n'))
    const help = wrapper.get('.markdown-body')
    expect(help.find('script').exists()).toBe(false)
    expect(help.get('img').attributes('onerror')).toBeUndefined()
    expect(help.findAll('a').map(link => link.attributes('href'))).toEqual([undefined, 'https://example.com/help'])
  })

  it('keeps plain-text soft line breaks and the separate help image preview', async () => {
    const wrapper = await mountHelp('First line\nSecond line', 'https://example.com/help.png')
    const help = wrapper.get('.markdown-body')
    expect(help.get('p').text()).toBe('First line\nSecond line')
    expect(help.find('br').exists()).toBe(false)
    await wrapper.get('img').trigger('click')
    expect(wrapper.findAll('img')).toHaveLength(2)
    expect(wrapper.findAll('img')[1].attributes('src')).toBe('https://example.com/help.png')
  })

  it('keeps image-only help without an empty Markdown container', async () => {
    const wrapper = await mountHelp('', 'https://example.com/help.png')
    expect(wrapper.find('.markdown-body').exists()).toBe(false)
    expect(wrapper.get('img').attributes('src')).toBe('https://example.com/help.png')
  })
})

describe('PaymentView USDT 充值倍率', () => {
  it('充值和订阅入口都透传 OKPay 图标', async () => {
    const icon_url = 'https://images.example/okpay.png'
    const method = { ...checkoutInfoFixture().data.methods.wxpay, currency: 'USDT', input_currency: 'CNY', usdt_exchange: rateQuote(), icon_url }
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockResolvedValue(checkoutInfoFixture({ methods: { okpay: method } }))
    const recharge = shallowMount(PaymentView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Teleport: true, Transition: false } } })
    await flushPromises()
    expect(recharge.getComponent(PaymentMethodSelector).props('methods')).toEqual(expect.arrayContaining([expect.objectContaining({ type: 'okpay', icon_url })]))
    recharge.unmount()
    const subscription = await mountSubscriptionConfirm({ checkout: { methods: { okpay: method } } })
    expect(subscription.getComponent(PaymentMethodSelector).props('methods')).toEqual(expect.arrayContaining([expect.objectContaining({ type: 'okpay', icon_url })]))
    subscription.unmount()
  })

  it('原生 USDT 按人民币输入并沿用全局到账倍率，其他通道保持原逻辑', async () => {
    routeState.path = '/purchase'
    routeState.query = {}
    window.localStorage.clear()
    const method = checkoutInfoFixture().data.methods.wxpay
    getCheckoutInfo.mockResolvedValue(checkoutInfoFixture({ balance_recharge_multiplier: 0.14, methods: { okpay: { ...method, currency: 'USDT', input_currency: 'CNY', usdt_exchange: rateQuote() }, wxpay: { ...method, currency: 'CNY' } } }))
    const wrapper = shallowMount(PaymentView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Teleport: true, Transition: false } } })
    await flushPromises()
    wrapper.getComponent(PaymentMethodSelector).vm.$emit('select', 'okpay')
    await flushPromises()
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 100)
    await flushPromises()
    expect(wrapper.getComponent(AmountInput).props('currency')).toBe('CNY')
    expect(wrapper.text()).toContain('$14.00')
    expect(wrapper.text()).toContain('USDT 13.89')
    wrapper.getComponent(PaymentMethodSelector).vm.$emit('select', 'wxpay')
    await flushPromises()
    expect(wrapper.text()).toContain('$14.00')
    expect(wrapper.text()).toContain('payment.rechargeRatePreview')
    wrapper.unmount()
  })
})

function rateQuote(overrides: Partial<UsdtCnyQuote> = {}): UsdtCnyQuote {
  return { rate: 7.2, source: 'okx', observed_at: new Date().toISOString(), fetched_at: new Date().toISOString(), sample_count: 10, sample_prices: [7.2], aggregation: 'median_first_10_sell', ...overrides }
}

async function mountUsdtRecharge(method: Partial<MethodLimit> = {}, checkout: Partial<CheckoutInfoResponse> = {}, paymentType = 'usdt_trc20') {
  routeState.path = '/purchase'
  routeState.query = {}
  window.localStorage.clear()
  getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({ balance_recharge_multiplier: 0.14, methods: { [paymentType]: { ...checkoutInfoFixture().data.methods.wxpay, currency: 'USDT', input_currency: 'CNY', usdt_exchange: rateQuote(), ...method } }, ...checkout }))
  const wrapper = shallowMount(PaymentView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, UsdtExchangePreview: false, Teleport: true, Transition: false } } })
  await flushPromises()
  return wrapper
}

describe('PaymentView 人民币换算与 USDT 限额', () => {
  afterEach(() => vi.useRealTimers())

  it.each(['usdt_trc20', 'okpay'])('%s 按原始人民币命中折扣，折后加手续费再换算并按实际 USDT 校验限额', async paymentType => {
    const tiers = [{ min_amount: 10, bonus_percent: 20 }]
    const wrapper = await mountUsdtRecharge({ single_max: 1.14 }, {
      recharge_bonus_tiers: tiers,
      recharge_bonus_mode: 'discount',
      recharge_fee_rate: 2.5,
    }, paymentType)
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 10)
    await flushPromises()

    expect(wrapper.getComponent(AmountInput).props()).toMatchObject({
      currency: 'CNY', bonusTiers: tiers, bonusMode: 'discount', multiplier: 0.14,
    })
    expect(wrapper.get('[data-testid="recharge-discount-row"]').text()).toContain('-¥2.00')
    expect(wrapper.text()).toContain('¥0.20')
    expect(wrapper.text()).toContain('¥8.20')
    expect(wrapper.get('[data-test="usdt-preview-pay-amount"]').text()).toBe('USDT 1.14')
    expect(wrapper.get('[data-testid="recharge-credited-row"]').text()).toContain('$1.40')
    expect(wrapper.find('[data-testid="recharge-bonus-row"]').exists()).toBe(false)
    expect(translate).toHaveBeenCalledWith('payment.rechargeRatePreview', { currency: 'CNY', usd: '0.14' })
    expect(wrapper.getComponent(PaymentMethodSelector).props('methods')).toEqual([
      expect.objectContaining({ type: paymentType, available: true }),
    ])
    const button = wrapper.findAll('button').find(item => item.text().includes('payment.createOrder'))!
    expect(button.attributes('disabled')).toBeUndefined()
    // 仅模拟请求，不创建真实订单；服务端必须收到原始输入，避免重复打折。
    createOrder.mockReset().mockRejectedValue(new Error('本地测试停止下单'))
    await button.trigger('click')
    await flushPromises()
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({ amount: 10, payment_type: paymentType, order_type: 'balance' }))
    wrapper.unmount()
  })

  it('赠金提高 USD 到账额度而不降低人民币本金、手续费和 USDT 实付', async () => {
    const wrapper = await mountUsdtRecharge({}, {
      recharge_bonus_tiers: [{ min_amount: 10, bonus_percent: 20 }],
      recharge_bonus_mode: 'bonus',
      recharge_fee_rate: 2.5,
    })
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 10)
    await flushPromises()
    expect(wrapper.text()).toContain('¥0.25')
    expect(wrapper.get('[data-test="usdt-preview-pay-amount"]').text()).toBe('USDT 1.43')
    expect(wrapper.get('[data-testid="recharge-bonus-row"]').text()).toContain('+$0.28')
    expect(wrapper.get('[data-testid="recharge-credited-row"]').text()).toContain('$1.68')
    expect(wrapper.find('[data-testid="recharge-discount-row"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('充值折扣不会降低订阅套餐价格', async () => {
    const wrapper = await mountSubscriptionConfirm({
      method: { currency: 'USDT', input_currency: 'CNY', usdt_exchange: rateQuote() },
      checkout: { subscription_usd_to_cny_rate: 7.2, recharge_bonus_tiers: [{ min_amount: 0, bonus_percent: 50 }], recharge_bonus_mode: 'discount' },
      plan: { price: 10 },
    })
    expect(wrapper.text()).toContain('¥72.00')
    expect(wrapper.text()).toContain('USDT 10.00')
    expect(wrapper.find('[data-testid="recharge-discount-row"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('折后金额符合渠道上限时，不用上限隐藏原始金额快捷按钮', async () => {
    const wrapper = await mountUsdtRecharge({ currency: 'CNY', input_currency: undefined, usdt_exchange: undefined, single_max: 80 }, {
      recharge_bonus_tiers: [{ min_amount: 100, bonus_percent: 20 }], recharge_bonus_mode: 'discount',
    })
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 100)
    await flushPromises()
    expect(wrapper.getComponent(AmountInput).props('max')).toBe(0)
    expect(wrapper.get('[data-testid="recharge-discount-row"]').text()).toContain('-¥20.00')
    const button = wrapper.findAll('button').find(item => item.text().includes('payment.createOrder'))!
    expect(button.attributes('disabled')).toBeUndefined()
    expect(button.text()).toContain('¥80.00')
    wrapper.unmount()
  })

  it('不同币种的候选渠道独立计算折扣精度，不能沿用当前 JPY 渠道的整数金额', async () => {
    const baseMethod = checkoutInfoFixture().data.methods.wxpay
    const wrapper = await mountUsdtRecharge({}, {
      recharge_bonus_tiers: [{ min_amount: 10, bonus_percent: 15 }], recharge_bonus_mode: 'discount',
      methods: {
        stripe: { ...baseMethod, currency: 'JPY' },
        okpay: { ...baseMethod, currency: 'USDT', input_currency: 'CNY', usdt_exchange: rateQuote(), single_max: 1.2 },
      },
    })
    wrapper.getComponent(PaymentMethodSelector).vm.$emit('select', 'stripe')
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 10)
    await flushPromises()
    expect(wrapper.getComponent(AmountInput).props('currency')).toBe('JPY')
    expect(wrapper.getComponent(PaymentMethodSelector).props('methods')).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'okpay', available: true }),
    ]))
    wrapper.getComponent(PaymentMethodSelector).vm.$emit('select', 'okpay')
    await flushPromises()
    expect(wrapper.get('[data-test="usdt-preview-pay-amount"]').text()).toBe('USDT 1.19')
    expect(wrapper.findAll('button').find(item => item.text().includes('payment.createOrder'))!.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('7 元 1% 手续费是 0.07 元，在 7.07 费率下应付恰好 1U', async () => {
    const wrapper = await mountUsdtRecharge({ usdt_exchange: rateQuote({ rate: 7.07 }), single_max: 1 }, { recharge_fee_rate: 1 })
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 7)
    await flushPromises()
    expect(wrapper.text()).toContain('¥0.07')
    expect(wrapper.get('[data-test="usdt-preview-pay-amount"]').text()).toBe('USDT 1.00')
    const button = wrapper.findAll('button').find(item => item.text().includes('payment.createOrder'))!
    expect(button.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('人民币本金乘到账倍率遇到半分时按后端规则四舍五入', async () => {
    const wrapper = await mountUsdtRecharge({}, { balance_recharge_multiplier: 1.005 })
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 1)
    await flushPromises()
    expect(wrapper.text()).toContain('$1.01')
    wrapper.unmount()
  })

  it('10 元加 2.5% 平台手续费按 7.2 换算为 1.43U，到账额度仍按本金计算', async () => {
    const wrapper = await mountUsdtRecharge({ usdt_exchange: rateQuote({ source: 'fallback', aggregation: 'configured_fallback' }) }, { recharge_fee_rate: 2.5 })
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 10)
    await flushPromises()
    expect(wrapper.text()).toContain('payment.exchange.fallbackPayment')
    expect(wrapper.text()).toContain('¥10.25')
    expect(wrapper.get('[data-test="usdt-preview-pay-amount"]').text()).toBe('USDT 1.43')
    expect(wrapper.text()).toContain('$1.40')
    expect(wrapper.getComponent(AmountInput).props()).toMatchObject({ currency: 'CNY', min: 0, max: 0 })
    wrapper.unmount()
  })

  it.each([
    { amount: 20, single_min: 10, single_max: 100, disabled: true },
    { amount: 200, single_min: 10, single_max: 100, disabled: false },
    { amount: 800, single_min: 10, single_max: 100, disabled: true },
  ])('按 USDT 应付判断实例限额：$amount 元，禁用=$disabled', async sample => {
    const wrapper = await mountUsdtRecharge({ single_min: sample.single_min, single_max: sample.single_max })
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', sample.amount)
    await flushPromises()
    const button = wrapper.findAll('button').find(item => item.text().includes('payment.createOrder'))!
    expect(button.attributes('disabled') !== undefined).toBe(sample.disabled)
    wrapper.unmount()
  })

  it('旧聚合同名方式没有显式人民币输入字段时不套用新的换算', async () => {
    const wrapper = await mountUsdtRecharge({ input_currency: undefined, usdt_exchange: undefined, currency: 'CNY' })
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 100)
    await flushPromises()
    expect(wrapper.find('[data-test="usdt-exchange-preview"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('¥100.00')
    expect(wrapper.text()).not.toContain('USDT 13.89')
    wrapper.unmount()
  })

  it('报价超过三十分钟会刷新，刷新失败时不继续展示或提交旧金额', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-30T12:00:00Z'))
    const wrapper = await mountUsdtRecharge({ usdt_exchange: rateQuote({ fetched_at: '2026-09-30T11:31:00Z', observed_at: '2026-09-30T11:31:00Z' }) }, {
      recharge_bonus_tiers: [{ min_amount: 10, bonus_percent: 20 }], recharge_bonus_mode: 'discount',
    })
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 10)
    await flushPromises()
    getCheckoutInfo.mockRejectedValue(new Error('暂时不可用'))
    await vi.advanceTimersByTimeAsync(60_000)
    await flushPromises()
    expect(getCheckoutInfo).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-test="usdt-preview-pay-amount"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('payment.exchange.unavailable')
    const button = wrapper.findAll('button').find(item => item.text().includes('payment.createOrder'))!
    expect(button.attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it.each([{ subscriptionRate: 7.2, expected: 'USDT 10.00', cny: '¥72.00' }, { subscriptionRate: 0, expected: 'USDT 1.39', cny: '¥10.00' }])('订阅按现有人民币基价换算，套餐币种标签不改变计价：$subscriptionRate', async sample => {
    const wrapper = await mountSubscriptionConfirm({ method: { currency: 'USDT', input_currency: 'CNY', usdt_exchange: rateQuote() }, checkout: { subscription_usd_to_cny_rate: sample.subscriptionRate }, plan: { price: 10, currency: 'NZD' } })
    expect(wrapper.text()).toContain(sample.expected)
    expect(wrapper.text()).toContain(sample.cny)
    wrapper.unmount()
  })
})

describe('PaymentView subscription plan grid', () => {
  it.each([3, 4, 6])('keeps %i plans on the existing mobile/tablet/desktop grid', async (planCount) => {
    const wrapper = await mountSubscriptionPlanList(planCount)
    const cards = wrapper.findAllComponents(SubscriptionPlanCard)

    expect(cards).toHaveLength(planCount)
    expect([...(cards[0].element.parentElement?.classList ?? [])]).toEqual(expect.arrayContaining([
      'grid',
      'grid-cols-1',
      'sm:grid-cols-2',
      'lg:grid-cols-3',
    ]))
  })
})

describe('PaymentView recharge rate preview', () => {
  it('uses the selected payment method currency in both locale templates', async () => {
    translate.mockClear()
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      balance_recharge_multiplier: 0.5,
      methods: {
        stripe: {
          ...checkoutInfoFixture().data.methods.wxpay,
          currency: 'USD',
        },
      },
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 10)
    await flushPromises()

    expect(translate).toHaveBeenCalledWith('payment.rechargeRatePreview', {
      currency: 'USD',
      usd: '0.50',
    })
    expect(en.payment.rechargeRatePreview).toBe('Current rate: 1 {currency} = {usd} USD')
    expect(zh.payment.rechargeRatePreview).toBe('当前倍率：1 {currency} = {usd} USD')
  })
})

describe('PaymentView subscription confirmation amounts', () => {
  it('shows converted CNY pay amount using the subscription rate, not the balance multiplier', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        balance_recharge_multiplier: 0.14,
        subscription_usd_to_cny_rate: 7.15,
      },
      method: {
        currency: 'CNY',
      },
      plan: {
        price: 9.99,
        original_price: 12.99,
      },
    })

    const text = wrapper.text()
    const convertedPrice = formatPaymentAmount(71.43, 'CNY')
    const convertedOriginalPrice = formatPaymentAmount(92.88, 'CNY')

    expect(text).toContain(convertedPrice)
    expect(text).toContain(convertedOriginalPrice)
    expect(text).not.toContain(formatPaymentAmount(9.99, 'CNY'))
    // 换算必须使用订阅汇率（×7.15），而不是余额倍率（÷0.14 = 71.36）
    expect(text).not.toContain(formatPaymentAmount(71.36, 'CNY'))
    expect(wrapper.findAll('button').some(button => button.text().includes(convertedPrice))).toBe(true)
  })

  it('keeps plan price when the subscription rate is not configured or payment currency is not CNY', async () => {
    // opt-in 回归锁：即使余额倍率已配置，未配置订阅汇率时 CNY 订阅仍按 price 直付
    const cnyWrapper = await mountSubscriptionConfirm({
      checkout: {
        balance_recharge_multiplier: 0.14,
        subscription_usd_to_cny_rate: 0,
      },
      method: {
        currency: 'CNY',
      },
      plan: {
        price: 7.99,
      },
    })

    expect(cnyWrapper.text()).toContain(formatPaymentAmount(7.99, 'CNY'))
    expect(cnyWrapper.text()).not.toContain(formatPaymentAmount(57.07, 'CNY'))
    expect(cnyWrapper.text()).not.toContain(formatPaymentAmount(57.13, 'CNY'))

    const usdWrapper = await mountSubscriptionConfirm({
      checkout: {
        subscription_usd_to_cny_rate: 7.15,
      },
      method: {
        currency: 'USD',
      },
      plan: {
        price: 7.99,
        original_price: 9.99,
      },
    })

    expect(usdWrapper.text()).toContain(formatPaymentAmount(7.99, 'USD'))
    expect(usdWrapper.text()).toContain(formatPaymentAmount(9.99, 'USD'))
  })

  it('adds fee rate after CNY rate conversion to match backend pay_amount', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        subscription_usd_to_cny_rate: 7.15,
        recharge_fee_rate: 2.5,
      },
      method: {
        currency: 'CNY',
      },
      plan: {
        price: 9.99,
      },
    })

    const text = wrapper.text()
    const convertedPrice = formatPaymentAmount(71.43, 'CNY')
    const fee = formatPaymentAmount(1.79, 'CNY')
    const total = formatPaymentAmount(73.22, 'CNY')

    expect(text).toContain(convertedPrice)
    expect(text).toContain(fee)
    expect(text).toContain(total)
    expect(wrapper.findAll('button').some(button => button.text().includes(total))).toBe(true)
  })
})

describe('PaymentView payment recovery', () => {
  beforeEach(() => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    routerReplace.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset().mockResolvedValue(undefined)
    routerResolve.mockClear()
    createOrder.mockReset()
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    bridgeInvoke.mockReset()
    window.localStorage.clear()
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined
  })

  it('restores a custom EasyPay method as the selected payment method', async () => {
    getCheckoutInfo.mockResolvedValue(checkoutInfoFixture({
      methods: {
        wxpay: checkoutInfoFixture().data.methods.wxpay,
        ldc: {
          daily_limit: 0,
          daily_used: 0,
          daily_remaining: 0,
          single_min: 0,
          single_max: 0,
          fee_rate: 0,
          available: true,
          display_name: 'LDC Pay',
        },
      },
    }))
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 888,
      amount: 66,
      qrCode: 'ldc-qr',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'ldc',
      payUrl: 'https://pay.example.com/ldc',
      outTradeNo: 'sub2_ldc_888',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 66,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: '',
      createdAt: Date.now(),
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: {
            template: '<div><slot /></div>',
          },
          PaymentStatusPanel: {
            template: '<button data-test="payment-done" @click="$emit(\'done\')" />',
          },
          PaymentMethodSelector: {
            props: ['selected'],
            template: '<div data-test="method-selector">{{ selected }}</div>',
          },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()
    await wrapper.find('[data-test="payment-done"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="method-selector"]').text()).toBe('ldc')
  })
})

describe('PaymentView WeChat JSAPI flow', () => {
  beforeEach(() => {
    routeState.path = '/purchase'
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-123',
    }
    routerReplace.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset().mockResolvedValue(undefined)
    routerResolve.mockClear()
    createOrder.mockReset()
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture())
    bridgeInvoke.mockReset()
    window.localStorage.clear()
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = {
      invoke: bridgeInvoke,
    }
  })

  it('resets payment state and redirects to /payment/result after JSAPI reports success', async () => {
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-123'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => {
      callback({ err_msg: 'get_brand_wcpay_request:ok' })
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith({ path: '/purchase', query: {} })
    expect(routerPush).toHaveBeenCalledWith({
      path: '/payment/result',
      query: {
        order_id: '123',
        out_trade_no: 'sub2_jsapi_123',
        resume_token: 'resume-token-123',
      },
    })
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('resets payment state when JSAPI reports cancellation', async () => {
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-cancel'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => {
      callback({ err_msg: 'get_brand_wcpay_request:cancel' })
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(showInfo).toHaveBeenCalledWith('payment.qr.cancelled')
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('clears stale recovery state when JSAPI never becomes available', async () => {
    vi.useFakeTimers()
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-missing-bridge'))
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      'payment.errors.wechatJsapiUnavailable payment.errors.wechatOpenInWeChatHint',
    )
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
    expect(wrapper.html()).not.toContain('payment-status-panel-stub')
  })

  it('clears a stale recovery snapshot before handling wechat resume callback params', async () => {
    createOrder.mockRejectedValueOnce(new Error('resume failed'))
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 999,
      amount: 66,
      qrCode: 'stale-qr',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/stale',
      outTradeNo: 'stale-out-trade-no',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 66,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: '',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }))

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      wechat_resume_token: 'resume-token-123',
    }))
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('keeps subscription resume context for token-only WeChat callbacks', async () => {
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-subscription-7',
      payment_type: 'wxpay_direct',
      order_type: 'subscription',
      plan_id: '7',
    }
    getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())
    createOrder.mockResolvedValue(oauthOrderFixture())

    const originalLocation = window.location
    const locationState = {
      href: 'http://localhost/purchase',
      origin: 'http://localhost',
    }
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState,
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith({ path: '/purchase', query: {} })
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      payment_type: 'wxpay',
      order_type: 'subscription',
      plan_id: 7,
      wechat_resume_token: 'resume-subscription-7',
    }))
    expect(locationState.href).toContain('/api/v1/auth/oauth/wechat/payment/start?')
    expect(new URL(locationState.href, 'http://localhost').searchParams.get('redirect')).toBe(
      '/purchase?from=wechat&payment_type=wxpay&order_type=subscription&plan_id=7',
    )

    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    })
  })

  it('falls back to QR flow when mobile WeChat payment is unavailable', async () => {
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-h5',
      payment_type: 'wxpay_direct',
    }
    createOrder
      .mockRejectedValueOnce({ reason: 'WECHAT_H5_NOT_AUTHORIZED' })
      .mockResolvedValueOnce({
        order_id: 778,
        amount: 88,
        pay_amount: 88,
        fee_rate: 0,
        expires_at: '2099-01-01T00:10:00.000Z',
        payment_type: 'wxpay',
        qr_code: 'weixin://wxpay/bizpayurl?pr=fallback-native',
        out_trade_no: 'sub2_qr_778',
      })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(createOrder).toHaveBeenNthCalledWith(1, expect.objectContaining({
      payment_type: 'wxpay',
      is_mobile: true,
      wechat_resume_token: 'resume-token-h5',
    }))
    expect(createOrder).toHaveBeenNthCalledWith(2, expect.objectContaining({
      payment_type: 'wxpay',
      is_mobile: false,
      payment_source: 'hosted_redirect',
    }))
    expect(showWarning).toHaveBeenCalledWith('payment.errors.mobilePaymentFallbackToQr')
    expect(showError).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('weixin://wxpay/bizpayurl?pr=fallback-native')
  })
})

describe('PaymentView subscription feature flag', () => {
  afterEach(() => {
    appStoreState.setPublicSettings(undefined)
  })

  function tabLabels(wrapper: Awaited<ReturnType<typeof mountSubscriptionPlanList>>) {
    return wrapper
      .findAll('button')
      .map((button) => button.text())
      .filter((text) => text === 'payment.tabTopUp' || text === 'payment.tabSubscribe')
  }

  it('keeps the top-up / subscribe switcher when subscription_enabled is absent (opt-out default)', async () => {
    const wrapper = await mountSubscriptionPlanList(2)

    expect(tabLabels(wrapper)).toEqual(['payment.tabTopUp', 'payment.tabSubscribe'])
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(2)
  })

  it('drops the subscribe tab, hides the switcher and ignores ?tab=subscription when subscriptions are disabled', async () => {
    appStoreState.setPublicSettings({ subscription_enabled: false })
    const wrapper = await mountSubscriptionPlanList(2)

    expect(tabLabels(wrapper)).toEqual([])
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(0)
    expect(wrapper.text()).toContain('payment.rechargeAccount')
  })

  it('shows an unavailable notice instead of a doomed top-up form when balance recharge is disabled too', async () => {
    appStoreState.setPublicSettings({ subscription_enabled: false })
    const wrapper = await mountSubscriptionConfirm({ checkout: { balance_disabled: true } })

    expect(tabLabels(wrapper)).toEqual([])
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(0)
    expect(wrapper.text()).not.toContain('payment.confirmSubscription')
    expect(wrapper.text()).not.toContain('payment.rechargeAccount')
    expect(wrapper.text()).toContain('payment.billingUnavailable')
    wrapper.unmount()
  })

  it('falls back from the subscribe tab to top-up when the flag flips off after mount', async () => {
    const wrapper = await mountSubscriptionPlanList(2)
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(2)

    appStoreState.setPublicSettings({ subscription_enabled: false })
    await flushPromises()

    expect(tabLabels(wrapper)).toEqual([])
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(0)
    expect(wrapper.text()).toContain('payment.rechargeAccount')
    wrapper.unmount()
  })

  it('enters the subscribe tab when a subscription-only site turns subscriptions back on', async () => {
    appStoreState.setPublicSettings({ subscription_enabled: false })
    const wrapper = await mountSubscriptionConfirm({ checkout: { balance_disabled: true } })
    expect(wrapper.text()).toContain('payment.billingUnavailable')

    appStoreState.setPublicSettings({ subscription_enabled: true })
    await flushPromises()

    expect(wrapper.text()).not.toContain('payment.billingUnavailable')
    expect(wrapper.text()).not.toContain('payment.rechargeAccount')
    expect(wrapper.findAllComponents(SubscriptionPlanCard).length).toBeGreaterThan(0)
    wrapper.unmount()
  })
})
