import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UserOrdersView from '../UserOrdersView.vue'

vi.mock('vue-i18n', async () => ({ ...(await vi.importActual('vue-i18n')), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('@/api/payment', () => ({ paymentAPI: {
  getMyOrders: vi.fn().mockResolvedValue({ data: { total: 2, items: [
    { id: 1, amount: 100, status: 'COMPLETED', payment_type: 'usdt_trc20', provider_instance_id: 'native', refund_supported: false },
    { id: 2, amount: 100, status: 'COMPLETED', payment_type: 'usdt_trc20', provider_instance_id: 'legacy' },
  ] } }),
  getRefundEligibleProviders: vi.fn().mockResolvedValue({ data: { provider_instance_ids: ['native', 'legacy'] } }),
} }))

describe('用户订单退款能力', () => {
  it('按订单退款能力隐藏原生入口，保留旧 EasyPay 同名自定义通道', async () => {
    const wrapper = mount(UserOrdersView, { global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      OrderTable: { props: ['orders'], template: '<div><div v-for="row in orders" :key="row.id" :data-order="row.id"><slot name="actions" :row="row" /></div></div>' },
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
      Icon: true, Select: true, Pagination: true,
    } } })
    await flushPromises()
    expect(wrapper.get('[data-order="1"]').text()).not.toContain('payment.orders.requestRefund')
    expect(wrapper.get('[data-order="2"]').text()).toContain('payment.orders.requestRefund')
    await wrapper.get('[data-order="2"] button').trigger('click')
    expect(wrapper.text()).toContain('#2')
    wrapper.unmount()
  })
})
