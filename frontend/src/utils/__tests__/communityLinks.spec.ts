import { describe, expect, it } from 'vitest'
import { safeCommunityContactURL, safeCommunityTelegramURL } from '../communityLinks'

describe('社群外部链接安全', () => {
  it.each(['javascript:alert(1)', 'data:text/html,<script>alert(1)</script>', '//evil.example/a', '/relative', 'https://name:secret@example.com', 'https://example.com/with space', 'https://example.com\\@evil.example', 'https://example.com/\npath'])('拒绝危险客服链接 %s', value => {
    expect(safeCommunityContactURL(value)).toBe('')
  })
  it('支持现有 HTTP/HTTPS 客服网页并保留查询参数', () => {
    expect(safeCommunityContactURL('https://help.example.com/chat?source=web')).toBe('https://help.example.com/chat?source=web')
    expect(safeCommunityContactURL('http://help.example.com/chat')).toBe('http://help.example.com/chat')
  })
  it.each(['http://t.me/test_bot', 'https://t.me.evil.example/test_bot', 'https://evil.example/+secret', 'https://t.me:8443/+secret', 'tg://resolve?domain=test_bot', 'https://t.me/'])('拒绝不符合 Telegram 域名和协议的链接 %s', value => {
    expect(safeCommunityTelegramURL(value)).toBe('')
  })
  it('允许官方群邀请链接', () => {
    expect(safeCommunityTelegramURL('https://t.me/+invite_abc')).toBe('https://t.me/+invite_abc')
  })
})
