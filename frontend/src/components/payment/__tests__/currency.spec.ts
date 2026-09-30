import { describe, expect, it } from 'vitest'
import { currencySymbol, formatOrderPaymentAmount, formatPaymentAmount, normalizePaymentCurrency } from '../currency'

describe('formatPaymentAmount', () => {
  it('优先原样显示新订单两位实际金额并兼容旧订单六位金额', () => {
    expect(normalizePaymentCurrency(' usdt ')).toBe('USDT')
    expect(currencySymbol('USDT')).toBe('USDT ')
    expect(formatPaymentAmount(12.34, 'USDT', 'en-US')).toBe('USDT 12.34')
    expect(formatOrderPaymentAmount({ pay_amount: 12.34, currency: 'USDT', payment_amount_exact: '12.35' })).toBe('USDT 12.35')
    expect(formatOrderPaymentAmount({ pay_amount: 12.34, currency: 'USDT', payment_amount_exact: '12.40' })).toBe('USDT 12.40')
    expect(formatOrderPaymentAmount({ pay_amount: 12.34, currency: 'USDT', payment_amount_exact: '12.340001' })).toBe('USDT 12.340001')
    expect(formatOrderPaymentAmount({ pay_amount: 12.34, currency: 'USDT', payment_amount_exact: '12.340010' })).toBe('USDT 12.340010')
    expect(formatOrderPaymentAmount({ pay_amount: 12.34, currency: 'USDT' })).toBe('USDT 12.34')
  })
  it('uses the currency default fraction digits', () => {
    expect(formatPaymentAmount(100, 'JPY', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'KRW', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'HKD', 'en-US')).toContain('.00')
  })
})

describe('currencySymbol', () => {
  it('maps common payment currencies and falls back safely', () => {
    expect(currencySymbol('USD')).toBe('$')
    expect(currencySymbol('cny')).toBe('¥')
    expect(currencySymbol('EUR')).toBe('€')
    expect(currencySymbol('')).toBe('¥')
    expect(currencySymbol('XYZ')).toBe('XYZ')
  })
})
