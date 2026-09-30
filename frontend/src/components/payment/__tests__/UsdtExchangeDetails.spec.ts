import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UsdtExchangeDetails from '../UsdtExchangeDetails.vue'
import type { UsdtExchangeSnapshot } from '@/types/payment'

vi.mock('vue-i18n', async () => ({ ...(await vi.importActual('vue-i18n')), useI18n: () => ({ t: (key: string) => key }) }))
const exchange: UsdtExchangeSnapshot = { rate: 7.123456, source: 'fallback', observed_at: '2026-09-30T12:00:00Z', cny_base_amount: 100, cny_pay_amount: 102, usdt_pay_amount: 14.32, pricing_mode: 'balance_cny', fallback_reason: '上游报价获取失败' }

describe('订单锁定费率', () => {
  it('列表直接显示兜底标识和锁定费率，详情展示人民币账单', () => {
    const compact = mount(UsdtExchangeDetails, { props: { exchange, currency: 'USDT', compact: true } })
    expect(compact.get('[data-test="usdt-rate-source"]').text()).toBe('payment.exchange.fallbackPayment')
    expect(compact.text()).toContain('1 USDT = ¥7.123456')
    const detail = mount(UsdtExchangeDetails, { props: { exchange, currency: 'USDT' } })
    expect(detail.text()).toContain('¥102.00')
    expect(detail.text()).toContain('¥100.00')
    expect(detail.text()).toContain('¥2.00')
    expect(detail.text()).toContain('上游报价获取失败')
    expect(detail.text()).toContain('payment.exchange.lockedHint')
  })

  it('旧 USDT 订单无快照时不套用当前汇率，普通订单不显示 USDT 标识', () => {
    const legacy = mount(UsdtExchangeDetails, { props: { currency: 'USDT' } })
    expect(legacy.text()).toContain('payment.exchange.legacyPricing')
    expect(legacy.text()).not.toContain('1 USDT')
    const normal = mount(UsdtExchangeDetails, { props: { currency: 'CNY' } })
    expect(normal.text()).toBe('')
  })
})
