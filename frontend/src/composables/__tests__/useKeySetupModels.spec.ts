import { effectScope, ref, type EffectScope } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useKeySetupModels } from '../useKeySetupModels'

const { fetchModels } = vi.hoisted(() => ({ fetchModels: vi.fn() }))
vi.mock('@/api/keySetupModels', () => ({ fetchKeySetupModels: fetchModels }))

const scopes: EffectScope[] = []
function setup(enabled = true) {
  const keyId = ref<number | null>(7)
  const platform = ref('openai')
  const active = ref(enabled)
  const scope = effectScope()
  scopes.push(scope)
  const state = scope.run(() => useKeySetupModels({ keyId, platform, enabled: active }))!
  return { keyId, platform, active, scope, ...state }
}

describe('密钥配置模型列表状态', () => {
  beforeEach(() => vi.resetAllMocks())
  afterEach(() => {
    scopes.splice(0).forEach(scope => scope.stop())
    vi.useRealTimers()
  })

  it('只在弹窗启用时加载，关闭后清理列表和选择', async () => {
    fetchModels.mockResolvedValue(['gpt-example'])
    const state = setup(false)
    expect(fetchModels).not.toHaveBeenCalled()
    state.active.value = true
    await flushPromises()
    expect(state.models.value).toEqual(['gpt-example'])
    expect(state.selectedModel.value).toBe('gpt-example')
    expect(state.error.value).toBeNull()
    state.active.value = false
    expect(state.models.value).toEqual([])
    expect(state.selectedModel.value).toBe('')
  })

  it('更换密钥后立即取消旧请求并忽略延迟到达的旧结果', async () => {
    const resolve: Array<(models: string[]) => void> = []
    fetchModels.mockImplementation(() => new Promise<string[]>(done => resolve.push(done)))
    const state = setup()
    const oldSignal: AbortSignal = fetchModels.mock.calls[0][2]
    state.selectedModel.value = 'old-custom'
    state.keyId.value = 8
    expect(oldSignal.aborted).toBe(true)
    expect(state.selectedModel.value).toBe('')
    expect(state.models.value).toEqual([])
    resolve[1](['new-model'])
    await flushPromises()
    resolve[0](['old-model'])
    await flushPromises()
    expect(state.models.value).toEqual(['new-model'])
    expect(state.selectedModel.value).toBe('new-model')
  })

  it('同一密钥的平台变化也会隔离状态', async () => {
    fetchModels.mockResolvedValueOnce(['openai-model']).mockResolvedValueOnce(['claude-model'])
    const state = setup()
    await flushPromises()
    state.platform.value = 'anthropic'
    expect(state.selectedModel.value).toBe('')
    await flushPromises()
    expect(fetchModels).toHaveBeenLastCalledWith(7, 'anthropic', expect.any(AbortSignal))
    expect(state.models.value).toEqual(['claude-model'])
  })

  it('目录结果不会覆盖加载期间输入的自定义模型', async () => {
    let resolve!: (models: string[]) => void
    fetchModels.mockImplementation(() => new Promise<string[]>(done => { resolve = done }))
    const state = setup()
    state.selectedModel.value = 'custom-model'
    resolve(['listed-model'])
    await flushPromises()
    expect(state.selectedModel.value).toBe('custom-model')
  })

  it('查询失败或空目录仍允许输入和保留自定义模型', async () => {
    fetchModels.mockRejectedValueOnce(new Error('包含秘密的任意原始错误')).mockResolvedValueOnce([])
    const state = setup()
    await flushPromises()
    expect(state.error.value).toBe('unavailable')
    state.selectedModel.value = 'custom-model'
    await state.refresh()
    expect(state.error.value).toBe('empty')
    expect(state.selectedModel.value).toBe('custom-model')
  })

  it('连续刷新时旧失败不能覆盖新成功', async () => {
    let rejectOld!: (error: Error) => void
    fetchModels.mockImplementationOnce(() => new Promise<string[]>((_resolve, reject) => { rejectOld = reject }))
      .mockResolvedValueOnce(['current-model'])
    const state = setup()
    await state.refresh()
    rejectOld(new Error('旧错误'))
    await flushPromises()
    expect(state.models.value).toEqual(['current-model'])
    expect(state.error.value).toBeNull()
    expect(state.loading.value).toBe(false)
  })

  it('离开组件会取消请求，晚到响应不能写回状态', async () => {
    let resolve!: (models: string[]) => void
    fetchModels.mockImplementation(() => new Promise<string[]>(done => { resolve = done }))
    const state = setup()
    const signal: AbortSignal = fetchModels.mock.calls[0][2]
    state.scope.stop()
    expect(signal.aborted).toBe(true)
    resolve(['late-model'])
    await flushPromises()
    expect(state.models.value).toEqual([])
    expect(state.loading.value).toBe(false)
  })

  it('请求超时停止加载且保持自定义模型可用', async () => {
    vi.useFakeTimers()
    fetchModels.mockImplementation(() => new Promise(() => {}))
    const state = setup()
    state.selectedModel.value = 'custom-model'
    const signal: AbortSignal = fetchModels.mock.calls[0][2]
    await vi.advanceTimersByTimeAsync(15000)
    expect(signal.aborted).toBe(true)
    expect(state.loading.value).toBe(false)
    expect(state.error.value).toBe('unavailable')
    expect(state.selectedModel.value).toBe('custom-model')
  })

  it('密钥分组已变化时返回明确状态，不误报目录已加载', async () => {
    fetchModels.mockRejectedValue(new Error('key_changed'))
    const state = setup()
    await flushPromises()
    expect(state.error.value).toBe('key_changed')
    expect(state.models.value).toEqual([])
  })
})
