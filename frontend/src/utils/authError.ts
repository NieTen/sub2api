interface APIErrorLike {
  detail?: unknown
  message?: unknown
  error?: unknown
  response?: {
    data?: {
      detail?: unknown
      message?: unknown
      error?: unknown
    }
  }
}

function extractErrorMessage(error: unknown): string {
  const err = (error || {}) as APIErrorLike
  const errorText = (value: unknown): unknown =>
    value && typeof value === 'object' ? (value as { message?: unknown }).message : value
  // 优先保留接口原文，避免 Axios 的 HTTP 状态摘要遮住具体业务错误。
  const candidates = [
    err.response?.data?.detail,
    err.response?.data?.message,
    errorText(err.response?.data?.error),
    err.detail,
    errorText(err.error),
    err.message,
    error
  ]
  return candidates.find((value): value is string => typeof value === 'string' && value.trim().length > 0) || ''
}

export function buildAuthErrorMessage(
  error: unknown,
  options: {
    fallback: string | (() => string)
  }
): string {
  const { fallback } = options
  const message = extractErrorMessage(error)
  // 只有接口没有可显示的原文时，才查询既有错误码提示或通用提示。
  return message || (typeof fallback === 'function' ? fallback() : fallback)
}
