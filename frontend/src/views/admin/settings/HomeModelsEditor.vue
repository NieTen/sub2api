<template>
  <section class="card" data-testid="home-models-editor" aria-labelledby="home-models-title" :aria-busy="loading || saving || syncing">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="home-models-title" class="text-lg font-semibold text-gray-900 dark:text-white">首页模型与价格</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        首页只展示此处保存的模型。输入模型名称后自动补全系统价格，所有价格均可手动修改。
      </p>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
        此处配置首页展示价格；实际调用费用以渠道和分组计费设置为准。
      </p>
    </div>

    <div class="space-y-4 p-6">
      <p v-if="loading" role="status" class="text-sm text-gray-500 dark:text-gray-400">正在加载首页模型…</p>
      <div v-else-if="!loaded" class="space-y-3">
        <p role="alert" class="text-sm text-red-600 dark:text-red-400">{{ loadError }}</p>
        <button type="button" class="btn btn-secondary btn-sm" data-testid="retry-load" @click="loadModels">重新加载</button>
      </div>

      <template v-else>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-sm text-gray-500 dark:text-gray-400">共 {{ drafts.length }} 个模型，最多 {{ maxModels }} 个</p>
          <div class="flex flex-wrap gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || syncing || drafts.length === 0" data-testid="sync-prices" @click="syncPrices">{{ syncing ? '正在同步…' : '一键同步价格' }}</button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || syncing || drafts.length >= maxModels" data-testid="add-text" @click="addModel('text')">添加文本模型</button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || syncing || drafts.length >= maxModels" data-testid="add-image" @click="addModel('image')">添加图片模型</button>
          </div>
        </div>

        <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">
          自动补全只填写空价格；一键同步会更新当前列表中有系统报价的价格。未匹配的模型和系统未提供的价格保留手填值，保存后生效。
        </p>

        <p v-if="drafts.length === 0" class="rounded-lg border border-dashed border-gray-300 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          暂无模型，请添加需要展示的模型。未配置或保存空列表时，首页不展示模型卡片。
        </p>

        <datalist id="home-model-vendors">
          <option v-for="vendor in vendorSuggestions" :key="vendor" :value="vendor" />
        </datalist>

        <fieldset
          v-for="(draft, index) in drafts"
          :key="draft.key"
          :disabled="saving || syncing"
          class="min-w-0 rounded-lg border border-gray-200 p-4 dark:border-dark-600"
          data-testid="model-row"
        >
          <legend class="px-1 text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ index + 1 }}. {{ draft.type === 'text' ? '文本模型' : '图片模型' }}
          </legend>
          <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ draft.type === 'text' ? '价格单位：USD / 百万 tokens；缓存输入和 Flex 输入可留空。' : '价格单位：USD / 张；按输出分辨率配置。' }}
            </p>
            <div class="flex gap-2">
              <button type="button" class="btn btn-secondary btn-sm" :disabled="index === 0 || saving || syncing" :aria-label="`上移第 ${index + 1} 个模型`" data-testid="move-up" @click="moveModel(index, -1)">上移</button>
              <button type="button" class="btn btn-secondary btn-sm" :disabled="index === drafts.length - 1 || saving || syncing" :aria-label="`下移第 ${index + 1} 个模型`" data-testid="move-down" @click="moveModel(index, 1)">下移</button>
              <button type="button" class="btn btn-secondary btn-sm text-red-600 dark:text-red-400" :disabled="saving || syncing" :aria-label="`删除第 ${index + 1} 个模型`" data-testid="remove-model" @click="removeModel(index)">删除</button>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-4 md:grid-cols-2" @input="successMessage = ''">
            <label class="block text-sm text-gray-700 dark:text-gray-300">
              <span class="mb-2 block font-medium">模型名称 <span aria-hidden="true">*</span></span>
              <input v-model="draft.name" type="text" maxlength="200" required class="input font-mono" data-field="name" placeholder="例如 gpt-5.4" @input="invalidatePriceRequest(draft)" @change="fillDefaultPrices(draft)" @keydown.enter.prevent.stop="saveModels" />
            </label>
            <label class="block text-sm text-gray-700 dark:text-gray-300">
              <span class="mb-2 block font-medium">厂商 <span aria-hidden="true">*</span></span>
              <input v-model="draft.vendor" type="text" maxlength="40" required list="home-model-vendors" class="input" data-field="vendor" placeholder="选择或输入厂商名称" @keydown.enter.prevent.stop="saveModels" />
            </label>
            <label v-for="field in priceFields(draft.type)" :key="field.key" class="block text-sm text-gray-700 dark:text-gray-300">
              <span class="mb-2 block font-medium">{{ field.label }} <span v-if="field.required" aria-hidden="true">*</span></span>
              <input
                v-model="draft.prices[field.key]"
                type="number"
                min="0"
                step="any"
                :required="field.required"
                class="input"
                :data-field="field.key"
                :placeholder="field.required ? '请输入非负价格' : '留空表示未配置'"
                @keydown.enter.prevent.stop="saveModels"
              />
            </label>
          </div>
          <p v-if="draft.priceLoading || draft.priceStatus" class="mt-3 text-xs leading-5 text-gray-500 dark:text-gray-400" data-testid="price-status" aria-live="polite">
            {{ draft.priceLoading ? '正在获取系统价格…' : draft.priceStatus }}
          </p>
        </fieldset>

        <p v-if="saveError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ saveError }}</p>
        <p v-if="successMessage" role="status" class="text-sm text-green-600 dark:text-green-400">{{ successMessage }}</p>
      </template>

      <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
        <p class="text-xs text-gray-500 dark:text-gray-400">此区域独立保存。调整后点击“保存模型价格”即可生效。</p>
        <button type="button" class="btn btn-primary" :disabled="!loaded || loading || saving || syncing" data-testid="save-models" @click="saveModels">
          {{ saving ? '正在保存…' : '保存模型价格' }}
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getHomeModels, getHomeModelSystemPrices, saveHomeModels, type HomeModel, type HomeModelPriceQuery, type HomeModelSystemPrice } from '@/api/admin/homeModels'
import { extractApiErrorMessage } from '@/utils/apiError'

type PriceKey = 'input' | 'output' | 'cachedInput' | 'flexInput' | '1K' | '2K' | '4K'
interface PriceField {
  key: PriceKey
  label: string
  required: boolean
}
interface HomeModelDraft {
  key: number
  name: string
  vendor: string
  type: HomeModel['type']
  prices: Record<PriceKey, string | number>
  priceLoading: boolean
  priceStatus: string
}

const maxModels = 500
const vendorSuggestions = ['OpenAI', 'ANTHROPIC', 'GEMINI', 'GROK']
const textPriceFields: PriceField[] = [
  { key: 'input', label: '输入价格', required: true },
  { key: 'output', label: '输出价格', required: true },
  { key: 'cachedInput', label: '缓存输入价格', required: false },
  { key: 'flexInput', label: 'Flex 输入价格', required: false },
]
const imagePriceFields: PriceField[] = [
  { key: '1K', label: '1K 价格', required: true },
  { key: '2K', label: '2K 价格', required: true },
  { key: '4K', label: '4K 价格', required: true },
]
const drafts = ref<HomeModelDraft[]>([])
const loading = ref(false)
const loaded = ref(false)
const saving = ref(false)
const syncing = ref(false)
const loadError = ref('')
const saveError = ref('')
const successMessage = ref('')
let nextKey = 0
let disposed = false
const priceRequestVersions = new Map<number, number>()
const pendingPriceRequests = new Map<number, Promise<void>>()

function priceFields(type: HomeModel['type']): PriceField[] {
  return type === 'text' ? textPriceFields : imagePriceFields
}

function emptyDraft(type: HomeModel['type']): HomeModelDraft {
  return {
    key: nextKey++, name: '', vendor: 'OpenAI', type,
    prices: { input: '', output: '', cachedInput: '', flexInput: '', '1K': '', '2K': '', '4K': '' },
    priceLoading: false,
    priceStatus: '',
  }
}

function toDraft(model: HomeModel): HomeModelDraft {
  const draft = emptyDraft(model.type)
  draft.name = model.name
  draft.vendor = model.vendor
  if (model.type === 'text') {
    draft.prices.input = model.input
    draft.prices.output = model.output
    draft.prices.cachedInput = model.cachedInput ?? ''
    draft.prices.flexInput = model.flexInput ?? ''
  } else {
    for (const size of ['1K', '2K', '4K'] as const) draft.prices[size] = model.resolutionPrices[size]
  }
  return draft
}

async function loadModels() {
  if (loading.value || saving.value || syncing.value) return
  loading.value = true
  loaded.value = false
  loadError.value = ''
  try {
    const models = await getHomeModels()
    if (!Array.isArray(models)) throw new Error('首页模型数据格式异常，请重试。')
    drafts.value = models.map(toDraft)
    loaded.value = true
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, '首页模型加载失败，请重试。')
  } finally {
    loading.value = false
  }
}

function clearFeedback() {
  saveError.value = ''
  successMessage.value = ''
}

function addModel(type: HomeModel['type']) {
  if (!loaded.value || saving.value || syncing.value || drafts.value.length >= maxModels) return
  drafts.value.push(emptyDraft(type))
  clearFeedback()
}

function moveModel(index: number, offset: number) {
  const target = index + offset
  if (saving.value || syncing.value || target < 0 || target >= drafts.value.length) return
  const [draft] = drafts.value.splice(index, 1)
  drafts.value.splice(target, 0, draft)
  clearFeedback()
}

function removeModel(index: number) {
  if (saving.value || syncing.value) return
  const draft = drafts.value[index]
  if (draft) invalidatePriceRequest(draft)
  drafts.value.splice(index, 1)
  clearFeedback()
}

function invalidatePriceRequest(draft: HomeModelDraft) {
  priceRequestVersions.set(draft.key, (priceRequestVersions.get(draft.key) ?? 0) + 1)
  pendingPriceRequests.delete(draft.key)
  draft.priceLoading = false
  draft.priceStatus = ''
}

function systemPrices(result: HomeModelSystemPrice): Partial<Record<PriceKey, number | null>> {
  return result.type === 'text'
    ? { input: result.input, output: result.output, cachedInput: result.cachedInput, flexInput: result.flexInput }
    : { ...result.resolutionPrices }
}

async function querySystemPrices(queries: HomeModelPriceQuery[]): Promise<HomeModelSystemPrice[]> {
  const results = await getHomeModelSystemPrices(queries)
  if (!Array.isArray(results) || results.length !== queries.length || results.some((result, index) => {
    const query = queries[index]
    if (!result || result.name !== query.name || result.type !== query.type || typeof result.found !== 'boolean') return true
    if (!result.found) return false
    const values = systemPrices(result)
    return priceFields(query.type).some(field => {
      const value = values[field.key]
      return value == null ? field.required : typeof value !== 'number' || !Number.isFinite(value) || value < 0
    })
  })) throw new Error('系统价格响应格式异常，原有价格已保留。')
  return results
}

function applySystemPrices(draft: HomeModelDraft, result: HomeModelSystemPrice, overwrite: boolean) {
  if (!result.found) {
    draft.priceStatus = `${result.reason || '未找到该模型的系统报价'}；原有价格已保留，可手动填写。`
    return
  }
  const values = systemPrices(result)
  const unavailable: string[] = []
  let filled = 0
  for (const field of priceFields(draft.type)) {
    const value = values[field.key]
    if (value == null) {
      unavailable.push(field.label)
    } else if (overwrite || String(draft.prices[field.key]).trim() === '') {
      draft.prices[field.key] = value
      filled++
    }
  }
  draft.priceStatus = overwrite ? '已同步系统价格，可继续手动修改。' : filled ? '已补全系统价格，手动填写的价格已保留。' : '已保留手动填写的价格。'
  if (unavailable.length) draft.priceStatus += ` 系统未提供${unavailable.join('、')}，保留手填值。`
}

function fillDefaultPrices(draft: HomeModelDraft): Promise<void> {
  if (!loaded.value || saving.value || syncing.value || disposed) return Promise.resolve()
  const name = draft.name.trim()
  if (!name || Array.from(name).length > 200) return Promise.resolve()
  invalidatePriceRequest(draft)
  const version = priceRequestVersions.get(draft.key)
  draft.priceLoading = true
  const current = () => !disposed && !syncing.value && drafts.value.some(row => row.key === draft.key) && draft.name.trim() === name && priceRequestVersions.get(draft.key) === version
  const request = querySystemPrices([{ name, type: draft.type }]).then(results => {
    // 请求完成时再次检查空值，避免覆盖请求期间输入的手动价格或已改名的模型。
    if (current()) applySystemPrices(draft, results[0], false)
  }).catch(error => {
    if (current()) draft.priceStatus = extractApiErrorMessage(error, '系统价格获取失败，原有价格已保留，可手动填写。')
  }).finally(() => {
    if (current()) draft.priceLoading = false
    if (pendingPriceRequests.get(draft.key) === request) pendingPriceRequests.delete(draft.key)
  })
  pendingPriceRequests.set(draft.key, request)
  return request
}

async function syncPrices() {
  if (!loaded.value || loading.value || saving.value || syncing.value || disposed) return
  clearFeedback()
  const rows = drafts.value.filter(draft => draft.name.trim() && Array.from(draft.name.trim()).length <= 200)
  if (!rows.length) {
    saveError.value = '请先填写需要同步的模型名称。'
    return
  }
  syncing.value = true
  drafts.value.forEach(invalidatePriceRequest)
  try {
    const results = await querySystemPrices(rows.map(draft => ({ name: draft.name.trim(), type: draft.type })))
    if (disposed) return
    rows.forEach((draft, index) => applySystemPrices(draft, results[index], true))
    const matched = results.filter(result => result.found).length
    const skipped = drafts.value.length - matched
    successMessage.value = `已同步 ${matched} 个模型${skipped ? `，${skipped} 个模型未匹配或名称未填写，原有价格已保留` : ''}。请点击“保存模型价格”后生效。`
  } catch (error) {
    if (!disposed) saveError.value = extractApiErrorMessage(error, '系统价格同步失败，原有价格已保留，请重试。')
  } finally {
    syncing.value = false
  }
}

function serializeModels(): HomeModel[] {
  if (drafts.value.length > maxModels) throw new Error(`最多配置 ${maxModels} 个模型。`)
  const seen = new Set<string>()
  return drafts.value.map((draft, index) => {
    const prefix = `第 ${index + 1} 个模型：`
    const name = draft.name.trim()
    const vendor = draft.vendor.trim()
    if (!name || Array.from(name).length > 200) throw new Error(`${prefix}模型名称必填，且不能超过 200 个字符。`)
    if (!vendor || Array.from(vendor).length > 40) throw new Error(`${prefix}厂商必填，且不能超过 40 个字符。`)
    const duplicateKey = JSON.stringify([name, draft.type])
    if (seen.has(duplicateKey)) throw new Error(`${prefix}同类型中存在重复的模型名称“${name}”。`)
    seen.add(duplicateKey)

    const prices: Partial<Record<PriceKey, number | null>> = {}
    for (const field of priceFields(draft.type)) {
      const raw = String(draft.prices[field.key]).trim()
      if (raw === '') {
        if (field.required) throw new Error(`${prefix}${field.label}必填。`)
        prices[field.key] = null
      } else {
        const value = Number(raw)
        if (!Number.isFinite(value) || value < 0) throw new Error(`${prefix}${field.label}必须是大于或等于 0 的有效数字。`)
        prices[field.key] = value
      }
    }
    if (draft.type === 'text') {
      return { name, vendor, type: 'text', input: prices.input!, output: prices.output!, cachedInput: prices.cachedInput ?? null, flexInput: prices.flexInput ?? null }
    }
    return { name, vendor, type: 'image', resolutionPrices: { '1K': prices['1K']!, '2K': prices['2K']!, '4K': prices['4K']! } }
  })
}

async function saveModels() {
  // 初次加载成功前不允许保存，避免请求失败时用空列表覆盖已有配置。
  if (!loaded.value || loading.value || saving.value || syncing.value || disposed) return
  clearFeedback()
  // 点击保存会触发输入框 change；等待自动补全完成后再校验与保存。
  for (const draft of drafts.value) {
    if (!pendingPriceRequests.has(draft.key) && priceFields(draft.type).some(field => field.required && String(draft.prices[field.key]).trim() === '')) {
      void fillDefaultPrices(draft)
    }
  }
  saving.value = true
  await Promise.all([...pendingPriceRequests.values()])
  if (disposed) return
  let models: HomeModel[]
  try {
    models = serializeModels()
  } catch (error) {
    saveError.value = extractApiErrorMessage(error, '请检查模型名称和价格。')
    saving.value = false
    return
  }
  try {
    const saved = await saveHomeModels(models)
    if (!Array.isArray(saved)) throw new Error('保存响应格式异常，请重新保存。')
    drafts.value = saved.map(toDraft)
    successMessage.value = '模型价格已保存，刷新首页即可查看。'
  } catch (error) {
    saveError.value = extractApiErrorMessage(error, '模型价格保存失败，请重试。')
  } finally {
    saving.value = false
  }
}

onMounted(loadModels)
onBeforeUnmount(() => {
  disposed = true
  pendingPriceRequests.clear()
  priceRequestVersions.clear()
})
</script>
