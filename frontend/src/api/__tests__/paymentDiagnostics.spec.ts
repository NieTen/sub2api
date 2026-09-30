import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import { adminPaymentAPI } from '../admin/payment'

vi.mock('../client', () => ({ apiClient: { post: vi.fn() } }))

describe('OKPay 认证诊断接口', () => {
  beforeEach(() => vi.resetAllMocks())

  it('仅提交已保存实例编号，不传请求体、凭据或订单数据', async () => {
    const response = { data: { provider_instance_id: 42, checks: [], conclusion: 'inconclusive' } }
    vi.mocked(apiClient.post).mockResolvedValue(response)
    await expect(adminPaymentAPI.diagnoseOkpayProvider(42)).resolves.toEqual(response)
    expect(apiClient.post).toHaveBeenCalledTimes(1)
    expect(apiClient.post).toHaveBeenCalledWith('/admin/payment/providers/42/diagnose-okpay', undefined, { timeout: 40000 })
  })

  it('保留 API 错误且不重试或回退到创建订单', async () => {
    const error = { status: 403, message: '没有诊断权限' }
    vi.mocked(apiClient.post).mockRejectedValue(error)
    await expect(adminPaymentAPI.diagnoseOkpayProvider(42)).rejects.toEqual(error)
    expect(apiClient.post).toHaveBeenCalledTimes(1)
  })
})
