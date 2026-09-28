import { apiClient } from './client'

export interface CommunityMembership {
  telegram_user_id: number
  telegram_username: string
  telegram_name: string
  status: 'pending' | 'joined' | 'left' | 'banned'
  joined_at?: string
}

export interface CommunityChallenge {
  id: string
  status: 'waiting' | 'claimed' | 'confirmed'
  expires_at: string
  bot_url?: string
  telegram_user_id?: number
  telegram_username?: string
  telegram_name?: string
}

export interface CommunityIdentityConfirmation {
  challenge_id: string
  telegram_user_id: number
}

export interface CommunityState {
  contact_url: string
  enabled: boolean
  require_paid_recharge: boolean
  eligible: boolean
  banned?: boolean
  show_join_prompt: boolean
  prompt_key?: string
  group_name: string
  bot_username: string
  membership: CommunityMembership | null
  challenge?: CommunityChallenge | null
  invite: { url: string; expires_at: string } | null
}

export interface CommunitySettings {
  enabled: boolean
  require_paid_recharge: boolean
  login_prompt_enabled: boolean
  contact_url: string
  group_chat_id: string
  group_name: string
  bot_username: string
}

export type CommunityMemberStatus = 'not_joined' | 'pending' | 'joined' | 'left' | 'banned'

export interface CommunityMember {
  user_id: number
  email: string
  username: string
  user_status: string
  telegram_user_id?: number
  telegram_username: string
  telegram_name: string
  status: CommunityMemberStatus
  joined_at?: string
  invite_expires_at?: string
}

export interface CommunityMembersPage {
  items: CommunityMember[]
  total: number
  page: number
  page_size: number
  summary: { total: number; joined: number; not_joined: number; pending: number; left: number; banned?: number }
}

export interface CommunityTelegramUser {
  telegram_user_id: number
  telegram_name: string
  telegram_username: string
  member: CommunityMember | null
  banned: boolean
  is_bot?: boolean
}

export interface CommunityChatMessage {
  id: number
  telegram_message_id: number
  telegram_user_id: number
  telegram_name: string
  telegram_username: string
  sender_kind: 'user' | 'chat' | 'unknown'
  text: string
  message_type: string
  created_at: string
  edited_at?: string
  file_name?: string
  mime_type?: string
  media_available: boolean
  file_size?: number
  is_bot?: boolean
  reply_to_message_id?: number
  admin_user_id?: number
  outgoing: boolean
}

export interface CommunityChatPage {
  items: CommunityChatMessage[]
  has_more: boolean
  latest_id: number
}

const avatarCache = new Map<number, { expires: number; blob: Promise<Blob> }>()

async function readCommunityBlob(path: string): Promise<Blob> {
  const response = await apiClient.get<Blob>(path, {
    responseType: 'blob',
    // 保留登录过期和权限拦截，其余附件错误由此处解析服务器的 JSON 提示。
    validateStatus: status => (status >= 200 && status < 300) || (status >= 400 && status !== 401 && status !== 403)
  })
  if (response.status >= 400) {
    let message = `请求失败（HTTP ${response.status}）`
    try {
      const data: unknown = JSON.parse(await response.data.text())
      if (typeof data === 'object' && data && 'message' in data && typeof data.message === 'string' && data.message.trim()) message = data.message
    } catch { /* 非 JSON 错误页面不作为附件或 HTML 展示。 */ }
    throw { status: response.status, message }
  }
  return response.data
}

export const communityAPI = {
  async get() {
    return (await apiClient.get<CommunityState>('/community')).data
  },
  async verification() {
    return (await apiClient.post<CommunityState>('/community/verification', {})).data
  },
  async invite(identity?: CommunityIdentityConfirmation) {
    return (await apiClient.post<CommunityState>('/community/invite', identity ?? {})).data
  },
  async members(page: number, search = '', status: CommunityMemberStatus | 'all' = 'all') {
    return (await apiClient.get<CommunityMembersPage>('/admin/community/members', {
      params: { page, page_size: 20, search, status }
    })).data
  },
  async telegramUser(id: number) {
    return (await apiClient.get<CommunityTelegramUser>(`/admin/community/telegram-users/${id}`)).data
  },
  async unbind(userID: number, ticketID: number) {
    return (await apiClient.post<{ user_id: number; telegram_user_id: number; ticket_id: number; unbound: boolean; banned: boolean }>(`/admin/community/members/${userID}/unbind`, { ticket_id: ticketID })).data
  },
  async avatar(id: number) {
    const cached = avatarCache.get(id)
    if (cached && cached.expires > Date.now()) return cached.blob
    const blob = readCommunityBlob(`/admin/community/telegram-users/${id}/avatar`)
    avatarCache.set(id, { expires: Date.now() + 5 * 60 * 1000, blob })
    if (avatarCache.size > 200) avatarCache.delete(avatarCache.keys().next().value!)
    try { return await blob } catch (cause) { avatarCache.delete(id); throw cause }
  },
  async messages(params: { before_id?: number; after_id?: number; limit?: number } = {}) {
    return (await apiClient.get<CommunityChatPage>('/admin/community/messages', { params: { limit: 50, ...params } })).data
  },
  async sendMessage(text: string, clientRequestID: string) {
    return (await apiClient.post<CommunityChatMessage>('/admin/community/messages', { text, client_request_id: clientRequestID }, { timeout: 60000 })).data
  },
  async media(id: number) {
    return readCommunityBlob(`/admin/community/messages/${id}/media`)
  },
  async settings() {
    return (await apiClient.get<CommunitySettings>('/admin/community/settings')).data
  },
  async saveSettings(settings: CommunitySettings) {
    // 服务端会在线核验机器人与群权限，给其 45 秒校验窗口留出响应时间。
    return (await apiClient.put<CommunitySettings>('/admin/community/settings', settings, { timeout: 60000 })).data
  }
}
