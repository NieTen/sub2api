// 与后端一致，仅允许 HTTPS 图片或站内绝对路径，不将图标地址作为 HTML 插入。
export function normalizePaymentMethodIconUrl(value: string | undefined): string {
  const raw = value?.trim() || ''
  const hasControl = Array.from(raw).some(character => {
    const code = character.charCodeAt(0)
    return code <= 31 || (code >= 127 && code <= 159)
  })
  if (!raw || raw.length > 2048 || hasControl || raw.includes('\\') || /%(?![0-9a-f]{2})/i.test(raw)) return ''
  try {
    const relative = raw.startsWith('/') && !raw.startsWith('//')
    if (!relative && !/^https:\/\//i.test(raw)) return ''
    if (!relative && !raw.replace(/^https:\/\//i, '').split(/[/?#]/)[0]) return ''
    if (/^https:\/\/[^/?#]*@/i.test(raw)) return ''
    const parsed = new URL(raw, 'https://payment-icon.invalid')
    if (parsed.protocol !== 'https:' || parsed.username || parsed.password || parsed.hash) return ''
    return raw
  } catch {
    return ''
  }
}
