import { apiClient } from '../client'

export interface ModelDetectionPlanInput {
  account_id: number
  model_id: string
  enabled: boolean
  schedule_type: 'interval' | 'daily'
  interval_minutes: number
  daily_time: string
  timezone: string
  reference_model: string
  drop_threshold: number
  max_results: number
}

export interface ModelDetectionPlan extends ModelDetectionPlanInput {
  id: number
  account_name: string
  baseline_score: number | null
  baseline_version: string
  baseline_run_id: number | null
  baseline_generation: number
  last_run_at: string | null
  next_run_at: string | null
  created_at: string
  updated_at: string
}

export interface ModelDetectionDetail {
  kind: string
  prompt: string
  response: string
  expected_count?: number
  evaluation?: Record<string, unknown>
}

export interface ModelDetectionRun {
  id: number
  plan_id: number
  account_id: number
  account_name: string
  model_id: string
  status: 'queued' | 'running' | 'completed' | 'error' | 'inconclusive'
  verdict: string
  score: number | null
  baseline_score: number | null
  drop_points: number | null
  fingerprint: { status: string; reference_model: string; analysis?: Record<string, unknown>; message?: string }
  details?: ModelDetectionDetail[]
  error_message: string
  suite_version: string
  progress: number
  requests_total: number
  trigger: string
  started_at: string | null
  finished_at: string | null
  created_at: string
  plan_snapshot: ModelDetectionPlan
}

export interface ModelDetectionSummary {
  account_id: number
  plan_count: number
  running_count: number
  latest_run: ModelDetectionRun | null
}

export interface ModelDetectionOverview {
  active: ModelDetectionRun[]
  recent: ModelDetectionRun[]
  stats: { total: number; normal: number; suspected_drop: number; error: number; average_score: number | null }
  plans: ModelDetectionPlan[]
}

export interface ModelDetectionCatalog {
  reference_models: { id: string; display_name: string; family: string }[]
  suite_version: string
  requests_per_run: number
}

const prefix = '/admin/model-detection'

export const modelDetectionAPI = {
  async catalog() { return (await apiClient.get<ModelDetectionCatalog>(`${prefix}/catalog`)).data },
  async overview() { return (await apiClient.get<ModelDetectionOverview>(`${prefix}/overview`)).data },
  async plans(accountId?: number) {
    return (await apiClient.get<ModelDetectionPlan[]>(`${prefix}/plans`, { params: { account_id: accountId } })).data ?? []
  },
  async create(input: ModelDetectionPlanInput) { return (await apiClient.post<ModelDetectionPlan>(`${prefix}/plans`, input)).data },
  async update(id: number, input: ModelDetectionPlanInput) { return (await apiClient.put<ModelDetectionPlan>(`${prefix}/plans/${id}`, input)).data },
  async run(id: number) { return (await apiClient.post<ModelDetectionRun>(`${prefix}/plans/${id}/run`)).data },
  async runAccount(accountId: number, modelId: string, referenceModel = '') {
    return (await apiClient.post<ModelDetectionRun>(`${prefix}/accounts/${accountId}/run`, { model_id: modelId, reference_model: referenceModel })).data
  },
  async resetBaseline(id: number) { return (await apiClient.post<ModelDetectionPlan>(`${prefix}/plans/${id}/reset-baseline`)).data },
  async detail(id: number) { return (await apiClient.get<ModelDetectionRun>(`${prefix}/runs/${id}`)).data },
  async history(accountId: number, beforeId?: number) {
    return (await apiClient.get<{ items: ModelDetectionRun[]; next_before_id: number | null }>(`${prefix}/accounts/${accountId}/history`, { params: { limit: 20, before_id: beforeId } })).data
  },
  async summaries(accountIds: number[], signal?: AbortSignal) {
    if (!accountIds.length) return []
    // 后端每次接收最多 200 个账号，大分页仍按批处理而非每行请求。
    const ids = [...new Set(accountIds)]
    const batches: number[][] = []
    for (let index = 0; index < ids.length; index += 200) batches.push(ids.slice(index, index + 200))
    const responses = await Promise.all(batches.map(batch => apiClient.get<ModelDetectionSummary[]>(`${prefix}/summaries`, { params: { account_ids: batch.join(',') }, signal })))
    return responses.flatMap(response => response.data ?? [])
  }
}

export function newDetectionPlan(accountId = 0): ModelDetectionPlanInput {
  return { account_id: accountId, model_id: '', enabled: false, schedule_type: 'interval', interval_minutes: 60, daily_time: '09:00', timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai', reference_model: '', drop_threshold: 20, max_results: 100 }
}

export function detectionResultKey(run: ModelDetectionRun): string {
  if (run.status !== 'completed') return `modelDetection.status.${run.status}`
  if (['baseline', 'normal', 'suspected_drop'].includes(run.verdict)) return `modelDetection.verdict.${run.verdict}`
  return 'modelDetection.status.completed'
}

export function detectionScore(score: number | null | undefined): string {
  return score == null || !Number.isFinite(score) ? '—' : Number(score.toFixed(1)).toString()
}
