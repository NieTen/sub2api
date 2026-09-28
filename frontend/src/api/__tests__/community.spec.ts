import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import { communityAPI } from '../community'

vi.mock('../client', () => ({ apiClient: { get: vi.fn(), post: vi.fn(), put: vi.fn() } }))

describe('社群邀请 API', () => {
  beforeEach(() => { vi.clearAllMocks(); vi.mocked(apiClient.post).mockResolvedValue({ data: {} }); vi.mocked(apiClient.get).mockResolvedValue({ data: {} }) })
  it('领取邀请直接发送空对象，不要求预先提供 Telegram 身份', async () => {
    await communityAPI.invite()
    expect(apiClient.post).toHaveBeenCalledWith('/community/invite', {})
  })
  it('主动发起身份核对，并仅在确认时提交挑战和 Telegram 身份', async () => {
    await communityAPI.verification()
    expect(apiClient.post).toHaveBeenCalledWith('/community/verification', {})
    await communityAPI.invite({ challenge_id: 'challenge-one', telegram_user_id: 5939067819 })
    expect(apiClient.post).toHaveBeenCalledWith('/community/invite', { challenge_id: 'challenge-one', telegram_user_id: 5939067819 })
  })
  it('后台成员查询传递分页、搜索和入群状态', async () => {
    await communityAPI.members(3, 'alice@example.com', 'not_joined')
    expect(apiClient.get).toHaveBeenCalledWith('/admin/community/members', { params: { page: 3, page_size: 20, search: 'alice@example.com', status: 'not_joined' } })
  })

  it('消息读取、发送和工单解绑仅访问管理员接口，保留发送幂等编号', async () => {
    await communityAPI.messages({ after_id: 42, limit: 50 })
    expect(apiClient.get).toHaveBeenLastCalledWith('/admin/community/messages', { params: { after_id: 42, limit: 50 } })
    await communityAPI.sendMessage('消息正文', 'request-id-for-retry')
    expect(apiClient.post).toHaveBeenLastCalledWith('/admin/community/messages', { text: '消息正文', client_request_id: 'request-id-for-retry' }, { timeout: 60000 })
    await communityAPI.unbind(7, 15)
    expect(apiClient.post).toHaveBeenLastCalledWith('/admin/community/members/7/unbind', { ticket_id: 15 })
  })

  it('相同用户头像复用授权请求，附件错误保留服务器提示', async () => {
    const blob = new Blob(['图片'], { type: 'image/png' })
    vi.mocked(apiClient.get).mockResolvedValue({ data: blob, status: 200 })
    const [first, second] = await Promise.all([communityAPI.avatar(987654), communityAPI.avatar(987654)])
    expect(first).toBe(blob)
    expect(second).toBe(blob)
    expect(apiClient.get).toHaveBeenCalledTimes(1)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/community/telegram-users/987654/avatar', expect.objectContaining({ responseType: 'blob' }))
    vi.mocked(apiClient.get).mockResolvedValue({ data: { text: async () => JSON.stringify({ message: '附件超过 20 MB，请在 Telegram 中查看' }) }, status: 413 })
    await expect(communityAPI.media(20)).rejects.toMatchObject({ status: 413, message: '附件超过 20 MB，请在 Telegram 中查看' })
  })
})
