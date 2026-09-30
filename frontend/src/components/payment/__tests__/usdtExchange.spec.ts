import { describe, expect, it } from 'vitest'
import { calculateCnyFee, convertCnyToUsdt, freshUsdtQuote, isUsdtCnyMethod, isUsdtExchangeSnapshot, roundPaymentValue } from '../usdtExchange'
import type { MethodLimit, UsdtCnyQuote } from '@/types/payment'

describe('USDT 人民币换算', () => {
  it('含手续费人民币账单除以费率后向上保留两位，不会少付', () => {
    expect(calculateCnyFee(7, 1)).toBe(0.07)
    expect(calculateCnyFee(10, 2.5)).toBe(0.25)
    expect(convertCnyToUsdt(7 + calculateCnyFee(7, 1), 7.07)).toBe(1)
    expect(roundPaymentValue(1.005)).toBe(1.01)
    expect(roundPaymentValue(3 * 2.675)).toBe(8.03)
    expect(convertCnyToUsdt(101, 7)).toBe(14.43)
    expect(convertCnyToUsdt(70, 7)).toBe(10)
    expect(convertCnyToUsdt(0.01, 7.123456)).toBe(0.01)
    expect(convertCnyToUsdt(100, 0)).toBe(0)
    expect(convertCnyToUsdt(100, Number.NaN)).toBe(0)
  })

  it('以显式输入币种区分原生通道与旧 EasyPay 自定义方式', () => {
    expect(isUsdtCnyMethod({ currency: 'USDT', input_currency: 'CNY' } as MethodLimit)).toBe(true)
    expect(isUsdtCnyMethod({ currency: 'USDT' } as MethodLimit)).toBe(false)
    expect(isUsdtCnyMethod({ currency: 'CNY' } as MethodLimit)).toBe(false)
  })

  it('超过三十分钟、时间异常和不可用报价不能继续预览', () => {
    const now = Date.parse('2026-09-30T12:00:00Z')
    const quote: UsdtCnyQuote = { rate: 7, source: 'okx', fetched_at: '2026-09-30T11:31:00Z', observed_at: '2026-09-30T11:31:00Z', sample_count: 10, sample_prices: [7], aggregation: 'median_first_10_sell' }
    expect(freshUsdtQuote(quote, now)).toBe(true)
    expect(freshUsdtQuote({ ...quote, fetched_at: '2026-09-30T11:30:00Z' }, now)).toBe(false)
    expect(freshUsdtQuote({ ...quote, fetched_at: '无效' }, now)).toBe(false)
    expect(freshUsdtQuote({ ...quote, source: 'unavailable' }, now)).toBe(false)
    expect(freshUsdtQuote({ ...quote, source: 'fallback', observed_at: '2026-09-30T12:00:00Z' }, now)).toBe(true)
  })

  it('接受后端真实计价快照并拒绝缺金额或非法费率的恢复记录', () => {
    const snapshot = { rate: 7, source: 'fallback', observed_at: '2026-09-30T12:00:00Z', cny_base_amount: 100, cny_pay_amount: 101, usdt_pay_amount: 14.43, pricing_mode: 'balance_cny' }
    expect(isUsdtExchangeSnapshot(snapshot)).toBe(true)
    expect(isUsdtExchangeSnapshot({ ...snapshot, rate: 0 })).toBe(false)
    expect(isUsdtExchangeSnapshot({ ...snapshot, usdt_pay_amount: undefined })).toBe(false)
  })
})
