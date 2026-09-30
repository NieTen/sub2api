export const DEFAULT_PAYMENT_CURRENCY = 'CNY'

const PAYMENT_CURRENCY_SYMBOLS: Record<string, string> = {
  USD: '$',
  CNY: '¥',
  RMB: '¥',
  EUR: '€',
  GBP: '£',
  JPY: '¥',
  HKD: 'HK$',
  TWD: 'NT$',
  KRW: '₩',
  AUD: 'A$',
  CAD: 'C$',
  SGD: 'S$',
  NZD: 'NZ$',
  MOP: 'MOP$',
  MYR: 'RM',
  THB: '฿',
  PHP: '₱',
  INR: '₹',
  USDT: 'USDT ',
}

export function normalizePaymentCurrency(currency?: string | null): string {
  const normalized = String(currency || '').trim().toUpperCase()
  return normalized === 'USDT' || /^[A-Z]{3}$/.test(normalized) ? normalized : DEFAULT_PAYMENT_CURRENCY
}

export function currencySymbol(currency?: string | null): string {
  const normalized = normalizePaymentCurrency(currency)
  return PAYMENT_CURRENCY_SYMBOLS[normalized] || normalized
}

export function formatOrderPaymentAmount(order: { pay_amount: number; currency?: string; payment_amount_exact?: string }, locale?: string): string {
  // 链上识别金额必须原样展示，尾数不能转为浮点数或按业务订单精度四舍五入。
  if (normalizePaymentCurrency(order.currency) === 'USDT' && order.payment_amount_exact && /^\d+\.\d{6}$/.test(order.payment_amount_exact)) {
    return 'USDT ' + order.payment_amount_exact
  }
  return formatPaymentAmount(order.pay_amount, order.currency, locale)
}

function paymentCurrencyFractionDigits(currency: string): number {
  if (currency === 'USDT') return 2
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

export function formatPaymentAmount(amount: number, currency?: string | null, locale?: string): string {
  const normalized = normalizePaymentCurrency(currency)
  const fractionDigits = paymentCurrencyFractionDigits(normalized)
  // USDT 不属于 ISO 4217，按商户订单的两位精度显示，避免被误标为人民币。
  if (normalized === 'USDT') {
    return 'USDT ' + new Intl.NumberFormat(locale || undefined, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(Number.isFinite(amount) ? amount : 0)
  }
  try {
    return new Intl.NumberFormat(locale || undefined, {
      style: 'currency',
      currency: normalized,
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: fractionDigits,
      maximumFractionDigits: fractionDigits,
    }).format(Number.isFinite(amount) ? amount : 0)
  } catch {
    return `${normalized} ${(Number.isFinite(amount) ? amount : 0).toFixed(fractionDigits)}`
  }
}
