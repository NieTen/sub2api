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
})
