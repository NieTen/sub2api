import { sanitizeUrl } from './url'

export function safeCommunityContactURL(value?: string): string {
  if (!value) return ''
  for (const character of value) {
    const code = character.charCodeAt(0)
    if (code <= 32 || code === 127 || character === '\\') return ''
  }
  const safe = sanitizeUrl(value)
  if (!safe) return ''
  const parsed = new URL(safe)
  // 不展示含账号密码的外部链接，避免误认实际跳转站点。
  return parsed.username || parsed.password ? '' : safe
}

export function safeCommunityTelegramURL(value?: string): string {
  const safe = safeCommunityContactURL(value)
  if (!safe) return ''
  const parsed = new URL(safe)
  // 验证和邀请链接只跳转 Telegram 官方域名，拒绝伪装域名及自定义端口。
  return parsed.protocol === 'https:' && parsed.hostname === 't.me' && !parsed.port && parsed.pathname !== '/' ? safe : ''
}
