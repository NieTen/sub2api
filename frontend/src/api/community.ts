import { apiClient } from './client'

export interface CommunityMembership {
  telegram_user_id: number
  telegram_username: string
  telegram_name: string
  status: 'pending' | 'joined' | 'left'
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

export type CommunityMemberStatus = 'not_joined' | 'pending' | 'joined' | 'left'

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
  summary: { total: number; joined: number; not_joined: number; pending: number; left: number }
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
  async settings() {
    return (await apiClient.get<CommunitySettings>('/admin/community/settings')).data
  },
  async saveSettings(settings: CommunitySettings) {
    // 服务端会在线核验机器人与群权限，给其 45 秒校验窗口留出响应时间。
    return (await apiClient.put<CommunitySettings>('/admin/community/settings', settings, { timeout: 60000 })).data
  }
}
