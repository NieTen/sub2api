import { apiClient } from './client'

export interface SupportAttachment {
  id: number
  file_name: string
  mime_type: string
  size: number
  created_at?: string
}

export interface SupportMessage {
  id: number
  ticket_id: number
  sender_id: number
  sender_role: 'user' | 'admin'
  source: 'web' | 'telegram'
  content: string
  created_at: string
  attachments: SupportAttachment[]
}

export interface SupportTicket {
  id: number
  user_id: number
  subject: string
  status: 'open' | 'closed'
  user_email: string
  username: string
  created_at: string
  updated_at: string
  last_message_at: string
}

export interface SupportTicketDetail {
  ticket: SupportTicket
  messages: SupportMessage[]
  has_more: boolean
}

export interface SupportPage<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface SupportSettings {
  enabled: boolean
  admin_emails: string[]
  telegram_bot_token?: string
  telegram_bot_token_configured?: boolean
  telegram_chat_id: string
  telegram_allowed_user_ids: number[]
  telegram_webhook_secret?: string
  telegram_webhook_secret_configured?: boolean
  clear_telegram_bot_token?: boolean
  clear_telegram_webhook_secret?: boolean
  telegram_webhook_url?: string
}

export interface BulkEmailRecipientFilter {
  balance_condition: 'all' | 'positive' | 'non_positive' | 'greater_than'
  balance_threshold?: string
  recharge_condition: 'all' | 'recharged'
}

export interface BulkEmailBatch {
  id: number
  subject: string
  body: string
  status: string
  total_count: number
  sent_count: number
  failed_count: number
  created_at: string
  attachments?: SupportAttachment[]
  recipient_filter?: BulkEmailRecipientFilter | null
}

export interface BulkEmailRecipient {
  id: number
  user_id: number
  email: string
  status: string
  attempts: number
  last_error: string
  sent_at?: string
}

export interface BulkEmailDetail {
  batch: BulkEmailBatch
  recipients: SupportPage<BulkEmailRecipient>
}

const ticketBase = (admin: boolean) => admin ? '/admin/tickets' : '/tickets'

export const supportAPI = {
  async list(admin: boolean, page: number, status = '') {
    return (await apiClient.get<SupportPage<SupportTicket>>(ticketBase(admin), {
      params: { page, page_size: 20, status: status || undefined }
    })).data
  },
  async detail(admin: boolean, id: number, afterMessageId = 0) {
    return (await apiClient.get<SupportTicketDetail>(`${ticketBase(admin)}/${id}`, {
      params: { after_message_id: afterMessageId, limit: 50 }
    })).data
  },
  async create(subject: string, content: string, attachmentIds: number[]) {
    return (await apiClient.post<SupportTicketDetail>('/tickets', {
      subject, content, attachment_ids: attachmentIds
    })).data
  },
  async reply(admin: boolean, id: number, content: string, attachmentIds: number[]) {
    return (await apiClient.post<SupportMessage>(`${ticketBase(admin)}/${id}/messages`, {
      content, attachment_ids: attachmentIds
    })).data
  },
  async status(admin: boolean, id: number, status: SupportTicket['status']) {
    return (await apiClient.patch<SupportTicket>(`${ticketBase(admin)}/${id}/status`, { status })).data
  },
  async upload(admin: boolean, file: File) {
    const data = new FormData()
    data.append('file', file)
    return (await apiClient.post<SupportAttachment>(`${ticketBase(admin)}/attachments`, data, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })).data
  },
  async attachment(admin: boolean, id: number) {
    // 图片始终经带鉴权的客户端加载，不暴露公开附件地址。
    return (await apiClient.get<Blob>(`${ticketBase(admin)}/attachments/${id}`, { responseType: 'blob' })).data
  },
  async deleteAttachment(admin: boolean, id: number) {
    await apiClient.delete(`${ticketBase(admin)}/attachments/${id}`)
  },
  async settings() {
    return (await apiClient.get<SupportSettings>('/admin/support/settings')).data
  },
  async saveSettings(data: SupportSettings) {
    return (await apiClient.put<SupportSettings>('/admin/support/settings', data)).data
  }
}

export const bulkEmailAPI = {
  async list(page: number) {
    return (await apiClient.get<SupportPage<BulkEmailBatch>>('/admin/bulk-emails', {
      params: { page, page_size: 20 }
    })).data
  },
  async create(data: { subject: string; body: string; user_ids: number[]; all_active: boolean; attachment_ids: number[]; recipient_filter?: BulkEmailRecipientFilter }) {
    return (await apiClient.post<BulkEmailBatch>('/admin/bulk-emails', data)).data
  },
  async detail(id: number, page = 1) {
    return (await apiClient.get<BulkEmailDetail>(`/admin/bulk-emails/${id}`, {
      params: { page, page_size: 20 }
    })).data
  },
  async image(id: number, index: number) {
    return (await apiClient.get<Blob>(`/admin/bulk-emails/${id}/images/${index}`, { responseType: 'blob' })).data
  },
  async start(id: number) {
    return (await apiClient.post<BulkEmailBatch>(`/admin/bulk-emails/${id}/start`)).data
  },
  async retry(id: number) {
    return (await apiClient.post<BulkEmailBatch>(`/admin/bulk-emails/${id}/retry`)).data
  }
}

export function supportError(error: unknown, fallback: string): string {
  if (typeof error === 'object' && error && 'message' in error && typeof error.message === 'string') {
    return error.message
  }
  return fallback
}
