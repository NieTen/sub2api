import type { MethodLimit, UsdtCnyQuote, UsdtExchangeSnapshot } from '@/types/payment'

export function isUsdtCnyMethod(method?: MethodLimit): boolean {
  // 以服务端显式输入币种识别原生渠道，避免改变旧聚合通道的同名付款方式。
  return method?.input_currency === 'CNY' && method.currency === 'USDT'
}

export function usableUsdtQuote(quote?: UsdtCnyQuote | null): quote is UsdtCnyQuote {
  return !!quote && (quote.source === 'okx' || quote.source === 'fallback') && Number.isFinite(quote.rate) && quote.rate > 0
}

export function freshUsdtQuote(quote: UsdtCnyQuote | null | undefined, now = Date.now()): boolean {
  if (!usableUsdtQuote(quote)) return false
  const time = Date.parse(quote.source === 'okx' ? quote.fetched_at || quote.observed_at : quote.observed_at)
  return Number.isFinite(time) && time <= now + 60_000 && now - time < 30 * 60_000
}

export function convertCnyToUsdt(cnyAmount: number, rate: number): number {
  if (!Number.isFinite(cnyAmount) || cnyAmount < 0 || !Number.isFinite(rate) || rate <= 0) return 0
  // 前端用于报价预览，订单最终金额由服务端锁定；消除整分边界的浮点误差后向上保留两位。
  return Math.ceil(cnyAmount / rate * 100 - 1e-9) / 100
}

export function calculateCnyFee(amount: number, feeRate: number): number {
  if (!Number.isFinite(amount) || amount <= 0 || !Number.isFinite(feeRate) || feeRate <= 0) return 0
  return Math.ceil(amount * feeRate - 1e-9) / 100
}

export function roundPaymentValue(value: number, fractionDigits = 2): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** fractionDigits
  return Math.round((value + Number.EPSILON * Math.max(1, Math.abs(value))) * factor) / factor
}

export function formatUsdtRate(rate: number): string {
  return Number.isFinite(rate) && rate > 0 ? rate.toFixed(6).replace(/\.?0+$/, '') : '—'
}

export function isUsdtExchangeSnapshot(value: unknown): value is UsdtExchangeSnapshot {
  if (!value || typeof value !== 'object') return false
  const snapshot = value as Partial<UsdtExchangeSnapshot>
  return Number.isFinite(snapshot.rate) && Number(snapshot.rate) > 0
    && (snapshot.source === 'okx' || snapshot.source === 'fallback')
    && typeof snapshot.observed_at === 'string'
    && Number.isFinite(snapshot.cny_base_amount) && Number.isFinite(snapshot.cny_pay_amount)
    && Number.isFinite(snapshot.usdt_pay_amount)
    && ['balance_cny', 'subscription_cny', 'subscription_legacy_cny'].includes(snapshot.pricing_mode || '')
}
