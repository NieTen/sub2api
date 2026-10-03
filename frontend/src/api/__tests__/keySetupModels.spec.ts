import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchKeySetupModels } from '../keySetupModels'

const { get, getAPIBaseURL } = vi.hoisted(() => ({ get: vi.fn(), getAPIBaseURL: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { get } }))
vi.mock('../url', () => ({ getAPIBaseURL, buildGatewayUrl: (path: string) => `https://current.example${path}` }))

const fetchMock = vi.fn()
const ownedKey = (platform = 'openai') => ({ id: 7, key: 'sk-local-private-key', status: 'active', group: { platform } })
const response = (payload: unknown) => ({ ok: true, json: async () => payload })

describe('密钥配置模型目录接口', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getAPIBaseURL.mockReturnValue('/api/v1')
    get.mockResolvedValue({ data: ownedKey() })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  it('先校验用户拥有密钥，再仅通过请求头查询本站目录', async () => {
    fetchMock.mockResolvedValue(response({ data: [{ id: 'gpt-example' }, { id: 'gpt-example' }, { id: ' another-model ' }, { id: '' }, { id: 'bad\nmodel' }] }))
    const controller = new AbortController()
    await expect(fetchKeySetupModels(7, 'openai', controller.signal)).resolves.toEqual(['gpt-example', 'another-model'])
    expect(get).toHaveBeenCalledWith('/keys/7', { signal: controller.signal })
    expect(fetchMock).toHaveBeenCalledWith('/v1/models', {
      method: 'GET', headers: { Accept: 'application/json', Authorization: 'Bearer sk-local-private-key' },
      cache: 'no-store', credentials: 'omit', redirect: 'error', signal: controller.signal
    })
    expect(String(fetchMock.mock.calls[0][0])).not.toContain('sk-local-private-key')
  })

  it('保留当前实例子路径并使用 Antigravity 专用目录', async () => {
    getAPIBaseURL.mockReturnValue('https://current.example/sub2api/api/v1')
    get.mockResolvedValue({ data: ownedKey('antigravity') })
    fetchMock.mockResolvedValue(response({ data: [{ id: 'gemini-example' }] }))
    await expect(fetchKeySetupModels(7, 'antigravity')).resolves.toEqual(['gemini-example'])
    expect(fetchMock.mock.calls[0][0]).toBe('https://current.example/sub2api/antigravity/v1/models')
  })

  it('密钥平台变化时拒绝继续请求，避免混用平台目录', async () => {
    get.mockResolvedValue({ data: ownedKey('anthropic') })
    await expect(fetchKeySetupModels(7, 'openai')).rejects.toThrow('key_changed')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('密钥归属查询失败时不使用旧密钥，也不回传原始错误', async () => {
    get.mockRejectedValue(new Error('sk-local-private-key 原始请求内容'))
    await expect(fetchKeySetupModels(7, 'openai')).rejects.toThrow(/^unavailable$/)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('拒绝无效编号和停用密钥', async () => {
    await expect(fetchKeySetupModels(-1, 'openai')).rejects.toThrow('unavailable')
    expect(get).not.toHaveBeenCalled()
    get.mockResolvedValue({ data: { ...ownedKey(), status: 'inactive' } })
    await expect(fetchKeySetupModels(7, 'openai')).rejects.toThrow('unavailable')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('模型列表失败或响应格式不符时返回固定错误，空目录可正常返回', async () => {
    fetchMock.mockResolvedValueOnce({ ok: false }).mockResolvedValueOnce(response({ message: 'not a model list' })).mockResolvedValueOnce(response({ data: [] }))
    await expect(fetchKeySetupModels(7, 'openai')).rejects.toThrow('unavailable')
    await expect(fetchKeySetupModels(7, 'openai')).rejects.toThrow('unavailable')
    await expect(fetchKeySetupModels(7, 'openai')).resolves.toEqual([])
  })

  it('在取得密钥前被取消时不发出后续目录请求', async () => {
    const controller = new AbortController()
    get.mockImplementation(async () => { controller.abort(); return { data: ownedKey() } })
    await expect(fetchKeySetupModels(7, 'openai', controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
