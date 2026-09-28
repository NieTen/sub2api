import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import { modelDetectionAPI, detectionScore, detectionResultKey, detectionPlanInput, newDetectionPlan } from '../modelDetection'
vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(), post: vi.fn() } }))

describe('模型检测 API', () => {
  beforeEach(() => { vi.clearAllMocks(); vi.mocked(apiClient.get).mockResolvedValue({ data: [] }) })
  it('当前页多个账号只请求一次批量摘要，并传递取消信号', async () => {
    const controller = new AbortController()
    await modelDetectionAPI.summaries([7, 8, 9, 7], controller.signal)
    expect(apiClient.get).toHaveBeenCalledTimes(1)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/model-detection/summaries', { params: { account_ids: '7,8,9' }, signal: controller.signal })
  })
  it('超过单次上限的大分页按每 200 个账号分批，空页不请求', async () => {
    await modelDetectionAPI.summaries([])
    expect(apiClient.get).not.toHaveBeenCalled()
    await modelDetectionAPI.summaries(Array.from({ length: 450 }, (_, index) => index + 1))
    expect(apiClient.get).toHaveBeenCalledTimes(3)
    expect(vi.mocked(apiClient.get).mock.calls.map(([, config]) => config?.params.account_ids.split(',').length)).toEqual([200, 200, 50])
  })
  it('未完成或失败的检测不会错误显示正常结论，分数不出现长小数', () => {
    expect(detectionResultKey({ status: 'error', verdict: 'normal' } as never)).toBe('modelDetection.status.error')
    expect(detectionResultKey({ status: 'completed', verdict: 'suspected_drop' } as never)).toBe('modelDetection.verdict.suspected_drop')
    expect(detectionScore(33.3333333333333)).toBe('33.3')
    expect(detectionScore(null)).toBe('—')
    expect(detectionScore(0)).toBe('0')
  })
  it('批量手动检测只发送选择的模型，不允许另行覆盖参考模型', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({ data: { total: 2, success: 2, failed: 0, results: [] } })
    await modelDetectionAPI.runAccountModels(7, ['model-a', 'model-b'])
    expect(apiClient.post).toHaveBeenCalledWith('/admin/model-detection/accounts/7/runs', { model_ids: ['model-a', 'model-b'] })
  })
  it('编辑计划只提交可编辑配置，历史参考和能力字段不会回写', () => {
    const input = detectionPlanInput({ ...newDetectionPlan(7), model_id: ' model-a ', reference_model: 'old-reference', baseline_score: 100 } as never)
    expect(input.model_id).toBe('model-a')
    expect(input).not.toHaveProperty('reference_model')
    expect(input).not.toHaveProperty('baseline_score')
  })
})
