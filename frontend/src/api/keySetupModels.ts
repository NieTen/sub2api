import { apiClient } from './client'
import { buildGatewayUrl, getAPIBaseURL } from './url'
import type { ApiKey } from '@/types'

export type KeySetupModelsError = 'unavailable' | 'empty' | 'key_changed'

function modelListURL(platform: string): string {
  const path = platform === 'antigravity' ? '/antigravity/v1/models' : '/v1/models'
  const apiBase = getAPIBaseURL().replace(/\/+$/, '')
  // 保留部署子路径，查询地址只来自当前应用配置，不使用用户填写的接入地址。
  return /\/api\/v1$/i.test(apiBase)
    ? `${apiBase.slice(0, -'/api/v1'.length)}${path}`
    : buildGatewayUrl(path)
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw new DOMException('请求已取消', 'AbortError')
}

function readModelIDs(payload: unknown): string[] {
  if (!payload || typeof payload !== 'object' || !Array.isArray((payload as { data?: unknown }).data)) {
    throw new Error('unavailable')
  }
  const models: string[] = []
  const seen = new Set<string>()
  for (const item of (payload as { data: unknown[] }).data) {
    if (!item || typeof item !== 'object') continue
    const id = (item as { id?: unknown }).id
    if (typeof id !== 'string') continue
    const model = id.trim()
    const hasControlCharacter = Array.from(model).some(character => character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127)
    if (!model || model.length > 256 || hasControlCharacter || seen.has(model)) continue
    seen.add(model)
    models.push(model)
  }
  return models
}

/** 读取本站密钥对应的模型目录，不执行推理，也不表示模型已通过实际调用验证。 */
export async function fetchKeySetupModels(
  keyId: number,
  platform: string,
  signal?: AbortSignal
): Promise<string[]> {
  if (!Number.isSafeInteger(keyId) || keyId <= 0 || !platform.trim()) throw new Error('unavailable')
  throwIfAborted(signal)

  let key: ApiKey
  try {
    // 现有接口校验登录用户与密钥的所有权，秘密值仅在此次请求内存中使用。
    const response = await apiClient.get<ApiKey>(`/keys/${keyId}`, { signal })
    key = response.data
  } catch {
    throwIfAborted(signal)
    throw new Error('unavailable')
  }
  throwIfAborted(signal)
  if (!key || key.id !== keyId || key.group?.platform !== platform) throw new Error('key_changed')
  if (key.status !== 'active' || !key.key?.trim()) throw new Error('unavailable')

  try {
    const response = await fetch(modelListURL(platform), {
      method: 'GET',
      headers: { Accept: 'application/json', Authorization: `Bearer ${key.key}` },
      cache: 'no-store',
      credentials: 'omit',
      redirect: 'error',
      signal
    })
    if (!response.ok) throw new Error('unavailable')
    const payload: unknown = await response.json()
    throwIfAborted(signal)
    return readModelIDs(payload)
  } catch {
    throwIfAborted(signal)
    // 不向调用方传递可能包含密钥、上游响应或请求头的原始错误。
    throw new Error('unavailable')
  }
}
