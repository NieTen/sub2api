import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
import okpayIcon from '@/assets/icons/okpay.svg'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}))

describe('PaymentMethodSelector', () => {
  it('使用自定义 OKPay 图标，失败回退默认且更换地址后重新加载', async () => {
    const method = { type: 'okpay', icon_url: 'https://images.example/okpay.png', fee_rate: 0, available: true }
    const wrapper = mount(PaymentMethodSelector, { props: { selected: 'okpay', methods: [method] } })
    expect(wrapper.get('img').attributes('src')).toBe(method.icon_url)
    expect(wrapper.get('img').attributes('referrerpolicy')).toBe('no-referrer')
    await wrapper.get('img').trigger('error')
    expect(wrapper.get('img').attributes('src')).toBe(okpayIcon)
    await wrapper.get('img').trigger('error')
    expect(wrapper.get('img').attributes('src')).toBe(okpayIcon)
    await wrapper.setProps({ methods: [{ ...method, icon_url: '/images/updated.png' }] })
    expect(wrapper.get('img').attributes('src')).toBe('/images/updated.png')
    await wrapper.setProps({ methods: [{ ...method, icon_url: '' }] })
    expect(wrapper.get('img').attributes('src')).toBe(okpayIcon)
  })

  it.each(['javascript:alert(1)', 'data:image/svg+xml,test', '//images.example/icon.png', 'http://images.example/icon.png', 'https://user:pass@images.example/icon.png', 'https://@images.example/icon.png', '/\\images.example/icon.png'])('不加载无效自定义图标 %s', icon_url => {
    const wrapper = mount(PaymentMethodSelector, { props: { selected: 'okpay', methods: [{ type: 'okpay', icon_url, fee_rate: 0, available: true }] } })
    expect(wrapper.get('img').attributes('src')).toBe(okpayIcon)
  })

  it('原生 OKPay 和 TRC20 入口显示图标并能选择', async () => {
    const wrapper = mount(PaymentMethodSelector, { props: { selected: 'okpay', methods: [{ type: 'okpay', fee_rate: 0, available: true }, { type: 'usdt_trc20', fee_rate: 0, available: true }] } })
    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(2)
    expect(buttons.every(button => button.find('img').exists())).toBe(true)
    await buttons[1].trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual(['usdt_trc20'])
  })
  it('wraps large custom method collections without letting labels widen the selector', () => {
    const methods = Array.from({ length: 12 }, (_, index) => ({
      type: `custom_${index}`,
      display_name: `CUSTOM_PAYMENT_METHOD_${index}`,
      fee_rate: 0,
      available: true,
    }))

    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'custom_0',
        methods,
      },
    })

    const grid = wrapper.get('[data-testid="payment-method-grid"]')
    expect(grid.classes()).toEqual(expect.arrayContaining(['grid', 'sm:grid-cols-3', 'lg:grid-cols-4']))
    expect(grid.classes()).not.toContain('sm:flex')

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(methods.length)
    expect(buttons.every(button => button.classes().includes('min-w-0'))).toBe(true)
    expect(buttons.every((button, index) => button.attributes('title') === methods[index].display_name)).toBe(true)
    expect(wrapper.findAll('[data-testid="payment-method-label"]').every(label => label.classes().includes('truncate'))).toBe(true)
  })

  it('shows the configured display name for custom EasyPay methods', () => {
    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'ldc',
        methods: [{ type: 'ldc', display_name: 'LDC Pay', fee_rate: 0, available: true }],
      },
    })

    expect(wrapper.text()).toContain('LDC Pay')
    expect(wrapper.text()).not.toContain('ldc')
    expect(wrapper.text()).not.toContain('payment.methods.ldc')
  })

  it('uses the generic selected style for custom methods that contain built-in names', () => {
    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'card_alipay',
        methods: [{ type: 'card_alipay', display_name: 'Card Pay', fee_rate: 0, available: true }],
      },
    })

    const button = wrapper.get('button')
    expect(button.classes()).toContain('border-primary-500')
    expect(button.classes()).not.toContain('border-[#02A9F1]')
  })
})
