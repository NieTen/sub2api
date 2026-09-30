import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PaymentQRCodeView from '../PaymentQRCodeView.vue'

const getOrder = vi.hoisted(() => vi.fn())
const toCanvas = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const push = vi.hoisted(() => vi.fn())
const address = 'TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE'

vi.mock('vue-i18n', async () => ({ ...(await vi.importActual('vue-i18n')), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push }), useRoute: () => ({ query: { order_id: '42', payment_type: 'usdt_trc20', qr: '查询参数中的非可信地址', payment_amount_exact: '1.000001', expires_at: '2099-01-01' } }) }))
vi.mock('@/api/payment', () => ({ paymentAPI: { getOrder, cancelOrder: vi.fn() } }))
vi.mock('@/stores/payment', () => ({ usePaymentStore: () => ({ pollOrderStatus: vi.fn() }) }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('qrcode', () => ({ default: { toCanvas } }))

function mountPage() {
  return mount(PaymentQRCodeView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })
}

describe('旧二维码页面 TRC20 支付', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    toCanvas.mockResolvedValue(undefined)
    getOrder.mockResolvedValue({ data: { id: 42, payment_type: 'usdt_trc20', status: 'PENDING', expires_at: '2099-01-01', pay_amount: 88, payment_network: 'TRC20', payment_address: address, payment_amount_exact: '88.000010' } })
  })
  afterEach(() => { vi.useRealTimers() })

  it('只从服务端订单读取地址、金额和到期时间', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(getOrder).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-test="trc20-exact-amount"]').text()).toBe('88.000010')
    expect(wrapper.get('[data-test="trc20-address"]').text()).toBe(address)
    expect(wrapper.get('[data-test="trc20-bill-amount"]').text()).toBe('USDT 88.00')
    expect(toCanvas).toHaveBeenCalledWith(expect.any(HTMLCanvasElement), address, expect.any(Object))
    expect(wrapper.text()).not.toContain('查询参数中的非可信地址')
    wrapper.unmount()
  })

  it('过期订单隐藏二维码和付款复制区', async () => {
    const { data } = await getOrder()
    getOrder.mockResolvedValue({ data: { ...data, status: 'EXPIRED' } })
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.find('canvas').exists()).toBe(false)
    expect(wrapper.find('[data-test="trc20-payment-details"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('payment.crypto.expiredHint')
    wrapper.unmount()
  })

  it('订单读取失败时不回退到查询参数里的付款地址', async () => {
    getOrder.mockRejectedValue(new Error('无权限'))
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.find('canvas').exists()).toBe(false)
    expect(showError).toHaveBeenCalled()
    expect(toCanvas).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
