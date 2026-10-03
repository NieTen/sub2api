import { onScopeDispose, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { fetchKeySetupModels, type KeySetupModelsError } from '@/api/keySetupModels'

export interface KeySetupModelsOptions {
  keyId: MaybeRefOrGetter<number | null | undefined>
  platform: MaybeRefOrGetter<string | null | undefined>
  enabled?: MaybeRefOrGetter<boolean>
}

const MODEL_LIST_TIMEOUT_MS = 15000

export function useKeySetupModels(options: KeySetupModelsOptions) {
  const models = ref<string[]>([])
  const loading = ref(false)
  const error = ref<KeySetupModelsError | null>(null)
  // 可直接绑定输入框；自定义名称不要求存在于目录中。
  const selectedModel = ref('')
  let controller: AbortController | null = null
  let timeout: ReturnType<typeof setTimeout> | undefined
  let requestId = 0
  let disposed = false

  const enabled = () => options.enabled === undefined || toValue(options.enabled)

  function cancelRequest(): void {
    requestId += 1
    controller?.abort()
    controller = null
    clearTimeout(timeout)
    timeout = undefined
  }

  async function refresh(): Promise<void> {
    cancelRequest()
    models.value = []
    error.value = null
    loading.value = false
    const keyId = toValue(options.keyId)
    const platform = toValue(options.platform)?.trim()
    if (disposed || !enabled() || !keyId || !Number.isSafeInteger(keyId) || keyId <= 0 || !platform) return

    const currentRequestId = requestId
    const currentController = new AbortController()
    controller = currentController
    let timedOut = false
    const isCurrent = () => !disposed && currentRequestId === requestId
    loading.value = true
    timeout = setTimeout(() => {
      if (!isCurrent()) return
      timedOut = true
      currentController.abort()
      loading.value = false
      error.value = 'unavailable'
    }, MODEL_LIST_TIMEOUT_MS)

    try {
      const result = await fetchKeySetupModels(keyId, platform, currentController.signal)
      // 即使请求层忽略取消，过期响应也不能更新另一个密钥的界面。
      if (!isCurrent() || currentController.signal.aborted) return
      models.value = result
      error.value = result.length ? null : 'empty'
      if (!selectedModel.value.trim() && result.length) selectedModel.value = result[0]
    } catch (cause) {
      if (!isCurrent() || (currentController.signal.aborted && !timedOut)) return
      error.value = cause instanceof Error && cause.message === 'key_changed' ? 'key_changed' : 'unavailable'
    } finally {
      if (isCurrent()) {
        loading.value = false
        controller = null
        clearTimeout(timeout)
        timeout = undefined
      }
    }
  }

  watch(
    () => [toValue(options.keyId), toValue(options.platform), enabled()] as const,
    () => {
      selectedModel.value = ''
      void refresh()
    },
    { immediate: true, flush: 'sync' }
  )

  onScopeDispose(() => {
    disposed = true
    cancelRequest()
    loading.value = false
  })

  return { models, loading, error, refresh, selectedModel }
}
