import { enableAutoUnmount, flushPromises, mount, type DOMWrapper, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import HomeModelsEditor from '../HomeModelsEditor.vue'
import type { HomeModel, HomeTextModel } from '@/api/admin/homeModels'

const { getHomeModels, saveHomeModels, getHomeModelSystemPrices } = vi.hoisted(() => ({
  getHomeModels: vi.fn(),
  saveHomeModels: vi.fn(),
  getHomeModelSystemPrices: vi.fn(),
}))

vi.mock('@/api/admin/homeModels', () => ({ getHomeModels, saveHomeModels, getHomeModelSystemPrices }))

enableAutoUnmount(afterEach)

interface SystemPrice {
  name: string
  type: HomeModel['type']
  found: boolean
  input?: number
  output?: number
  cachedInput: number | null
  flexInput: number | null
  resolutionPrices?: Record<'1K' | '2K' | '4K', number>
  reason?: string
}

function systemTextPrice(name = 'gpt-test', overrides: Partial<SystemPrice> = {}): SystemPrice {
  return { name, type: 'text', found: true, input: 5, output: 30, cachedInput: 0.5, flexInput: 2.5, ...overrides }
}

function pendingPrices() {
  let resolve!: (prices: SystemPrice[]) => void
  let reject!: (error: Error) => void
  const promise = new Promise<SystemPrice[]>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function fieldValue(wrapper: DOMWrapper<Element> | VueWrapper, field: string) {
  return (wrapper.get(`[data-field="${field}"]`).element as HTMLInputElement).value
}

function textModel(name = 'gpt-test'): HomeTextModel {
  return { name, vendor: 'OpenAI', type: 'text', input: 1, output: 2, cachedInput: null, flexInput: null }
}

async function mountEditor(models: HomeModel[] = [textModel()]) {
  getHomeModels.mockResolvedValueOnce(models)
  const wrapper = mount(HomeModelsEditor)
  await flushPromises()
  return wrapper
}

describe('首页模型与价格编辑器', () => {
  beforeEach(() => {
    getHomeModels.mockReset()
    saveHomeModels.mockReset().mockImplementation(async (models: HomeModel[]) => models)
    getHomeModelSystemPrices.mockReset().mockImplementation(async (models: { name: string; type: HomeModel['type'] }[]) =>
      models.map(model => ({ ...model, found: false, cachedInput: null, flexInput: null, reason: '未找到系统价格' })),
    )
  })

  it('加载已有配置，保留零价格并将可选空价格保存为 null', async () => {
    const wrapper = await mountEditor([{ ...textModel(), cachedInput: 0.3, flexInput: 0.5 }])

    expect(wrapper.text()).toContain('USD / 百万 tokens')
    expect(wrapper.text()).toContain('实际调用费用以渠道和分组计费设置为准')
    await wrapper.get('[data-field="input"]').setValue('0')
    await wrapper.get('[data-field="output"]').setValue('0')
    await wrapper.get('[data-field="cachedInput"]').setValue('')
    await wrapper.get('[data-field="flexInput"]').setValue('')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()

    expect(saveHomeModels).toHaveBeenCalledWith([{ ...textModel(), input: 0, output: 0 }])
    expect(wrapper.get('[role="status"]').text()).toContain('模型价格已保存，刷新首页即可查看')
  })

  it('已保存的浮点尾数回读时显示简洁价格，再次保存不带计算误差', async () => {
    const wrapper = await mountEditor([
      { ...textModel(), input: 0.09999999999999999, output: 0.19999999999999998, cachedInput: 0.049999999999999996 },
      {
        name: 'image-test', vendor: 'OpenAI', type: 'image',
        resolutionPrices: { '1K': 0.09999999999999999, '2K': 0.19999999999999998, '4K': 0.049999999999999996 },
      },
    ])
    const rows = wrapper.findAll('[data-testid="model-row"]')

    expect(['input', 'output', 'cachedInput', 'flexInput'].map(field => fieldValue(rows[0], field))).toEqual(['0.1', '0.2', '0.05', ''])
    expect(['1K', '2K', '4K'].map(field => fieldValue(rows[1], field))).toEqual(['0.1', '0.2', '0.05'])
    expect(getHomeModelSystemPrices).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(saveHomeModels).toHaveBeenCalledWith([
      { ...textModel(), input: 0.1, output: 0.2, cachedInput: 0.05 },
      { name: 'image-test', vendor: 'OpenAI', type: 'image', resolutionPrices: { '1K': 0.1, '2K': 0.2, '4K': 0.05 } },
    ])
  })

  it('一键同步得到的浮点尾数也显示为简洁价格，微小 Flex 价格保持精度', async () => {
    getHomeModelSystemPrices.mockResolvedValueOnce([
      systemTextPrice('gpt-test', {
        input: 0.09999999999999999, output: 0.19999999999999998, cachedInput: 0.049999999999999996, flexInput: 0.000000125,
      }),
      {
        name: 'image-test', type: 'image', found: true, cachedInput: null, flexInput: null,
        resolutionPrices: { '1K': 0.09999999999999999, '2K': 0.19999999999999998, '4K': 0.049999999999999996 },
      },
    ])
    const wrapper = await mountEditor([
      textModel(),
      { name: 'image-test', vendor: 'OpenAI', type: 'image', resolutionPrices: { '1K': 1, '2K': 2, '4K': 3 } },
    ])
    await wrapper.get('[data-testid="sync-prices"]').trigger('click')
    await flushPromises()
    const rows = wrapper.findAll('[data-testid="model-row"]')

    expect(['input', 'output', 'cachedInput'].map(field => fieldValue(rows[0], field))).toEqual(['0.1', '0.2', '0.05'])
    expect(Number(fieldValue(rows[0], 'flexInput'))).toBe(0.000000125)
    expect(['1K', '2K', '4K'].map(field => fieldValue(rows[1], field))).toEqual(['0.1', '0.2', '0.05'])
    expect(saveHomeModels).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(saveHomeModels).toHaveBeenCalledWith([
      { ...textModel(), input: 0.1, output: 0.2, cachedInput: 0.05, flexInput: 0.000000125 },
      { name: 'image-test', vendor: 'OpenAI', type: 'image', resolutionPrices: { '1K': 0.1, '2K': 0.2, '4K': 0.05 } },
    ])
  })

  it('已保存和手动输入的真实高精度价格不被截断，零价与 null 保持原意', async () => {
    const wrapper = await mountEditor([{
      ...textModel(), input: 0.000000125, output: 0.1234567890123, cachedInput: 0, flexInput: null,
    }])
    expect(Number(fieldValue(wrapper, 'input'))).toBe(0.000000125)
    expect(fieldValue(wrapper, 'output')).toBe('0.1234567890123')
    expect(fieldValue(wrapper, 'cachedInput')).toBe('0')
    expect(fieldValue(wrapper, 'flexInput')).toBe('')

    await wrapper.get('[data-field="input"]').setValue('0.1234567890123')
    await wrapper.get('[data-field="output"]').setValue('0.000000125')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()

    expect(saveHomeModels).toHaveBeenCalledWith([{
      ...textModel(), input: 0.1234567890123, output: 0.000000125, cachedInput: 0, flexInput: null,
    }])
    expect(fieldValue(wrapper, 'input')).toBe('0.1234567890123')
    expect(Number(fieldValue(wrapper, 'output'))).toBe(0.000000125)
  })

  it.each([
    ['input', '-1', '输入价格必须是大于或等于 0 的有效数字'],
    ['output', '', '输出价格必填'],
    ['cachedInput', '-0.1', '缓存输入价格必须是大于或等于 0 的有效数字'],
    ['flexInput', '-1', 'Flex 输入价格必须是大于或等于 0 的有效数字'],
    ['name', '  ', '模型名称必填'],
    ['vendor', '  ', '厂商必填'],
  ])('拒绝无效字段 %s=%s', async (field, value, error) => {
    const wrapper = await mountEditor()
    await wrapper.get(`[data-field="${field}"]`).setValue(value)
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()

    expect(saveHomeModels).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain(error)
  })

  it('拒绝超出有限数字范围的价格', async () => {
    const wrapper = await mountEditor()
    await wrapper.get('[data-field="input"]').setValue('1e309')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(saveHomeModels).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toMatch(/输入价格(必须|必填)/)
  })

  it('添加图片模型并保存三个分辨率价格，允许自定义厂商', async () => {
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-image"]').trigger('click')
    expect(wrapper.text()).toContain('USD / 张')
    await wrapper.get('[data-field="name"]').setValue(' custom-image ')
    await wrapper.get('[data-field="vendor"]').setValue(' 自定义厂商 ')
    await wrapper.get('[data-field="1K"]').setValue('0')
    await wrapper.get('[data-field="2K"]').setValue('0.0125')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(saveHomeModels).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('4K 价格必填')

    await wrapper.get('[data-field="4K"]').setValue('0.025')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(saveHomeModels).toHaveBeenCalledWith([{
      name: 'custom-image', vendor: '自定义厂商', type: 'image',
      resolutionPrices: { '1K': 0, '2K': 0.0125, '4K': 0.025 },
    }])
  })

  it('同类型模型名称不可重复，重名的不同类型可以保存', async () => {
    const wrapper = await mountEditor([textModel('same')])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    const second = wrapper.findAll('[data-testid="model-row"]')[1]
    await second.get('[data-field="name"]').setValue(' same ')
    await second.get('[data-field="input"]').setValue('1')
    await second.get('[data-field="output"]').setValue('2')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    expect(saveHomeModels).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('重复的模型名称')

    await second.get('[data-testid="remove-model"]').trigger('click')
    await wrapper.get('[data-testid="add-image"]').trigger('click')
    const image = wrapper.findAll('[data-testid="model-row"]')[1]
    await image.get('[data-field="name"]').setValue('same')
    for (const size of ['1K', '2K', '4K']) await image.get(`[data-field="${size}"]`).setValue('1')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(saveHomeModels).toHaveBeenCalledWith([
      textModel('same'),
      { name: 'same', vendor: 'OpenAI', type: 'image', resolutionPrices: { '1K': 1, '2K': 1, '4K': 1 } },
    ])
  })

  it('按照上移下移和删除后的顺序保存，删除全部模型可保存空数组', async () => {
    const wrapper = await mountEditor([textModel('one'), textModel('two'), textModel('three')])
    await wrapper.findAll('[data-testid="model-row"]')[2].get('[data-testid="move-up"]').trigger('click')
    await wrapper.findAll('[data-testid="model-row"]')[0].get('[data-testid="move-down"]').trigger('click')
    await wrapper.findAll('[data-testid="model-row"]')[2].get('[data-testid="remove-model"]').trigger('click')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(saveHomeModels).toHaveBeenLastCalledWith([textModel('three'), textModel('one')])

    for (const row of wrapper.findAll('[data-testid="model-row"]')) await row.get('[data-testid="remove-model"]').trigger('click')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(saveHomeModels).toHaveBeenLastCalledWith([])
    expect(wrapper.findAll('[data-testid="model-row"]')).toHaveLength(0)
    expect(wrapper.get('[role="status"]').text()).toContain('已保存')
  })

  it('首次加载失败时不能保存，重试成功后才允许编辑已有数据', async () => {
    getHomeModels.mockRejectedValueOnce(new Error('读取失败')).mockResolvedValueOnce([textModel('stored')])
    const wrapper = mount(HomeModelsEditor)
    expect(wrapper.get('[data-testid="save-models"]').attributes('disabled')).toBeDefined()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('读取失败')
    expect(wrapper.get('[data-testid="save-models"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    expect(saveHomeModels).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="retry-load"]').trigger('click')
    await flushPromises()
    expect(getHomeModels).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="save-models"]').attributes('disabled')).toBeUndefined()
    expect((wrapper.get('[data-field="name"]').element as HTMLInputElement).value).toBe('stored')
  })

  it('保存失败保留编辑内容，重试保存成功且阻止重复请求', async () => {
    const wrapper = await mountEditor()
    await wrapper.get('[data-field="input"]').setValue('7.5')
    saveHomeModels.mockRejectedValueOnce(new Error('保存失败'))
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('保存失败')
    expect((wrapper.get('[data-field="input"]').element as HTMLInputElement).value).toBe('7.5')

    let resolveSave!: (models: HomeModel[]) => void
    saveHomeModels.mockImplementationOnce(() => new Promise<HomeModel[]>(resolve => { resolveSave = resolve }))
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    expect(wrapper.get('[data-testid="save-models"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    expect(saveHomeModels).toHaveBeenCalledTimes(2)
    resolveSave([{ ...textModel(), input: 7.5 }])
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toContain('已保存')
  })

  it('空配置不自动增加系统模型，保留添加入口及空列表说明', async () => {
    const wrapper = await mountEditor([])

    expect(wrapper.findAll('[data-testid="model-row"]')).toHaveLength(0)
    expect(wrapper.text()).toContain('暂无模型')
    expect(wrapper.text()).toContain('首页')
    expect(wrapper.get('[data-testid="add-text"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="sync-prices"]').attributes('disabled')).toBeDefined()
    expect(getHomeModelSystemPrices).not.toHaveBeenCalled()
    expect(saveHomeModels).not.toHaveBeenCalled()
  })

  it('加载已保存的手动报价不查询或覆盖系统价格', async () => {
    const stored = { ...textModel('manual'), input: 0, output: 19.8, cachedInput: 0, flexInput: 4.25 }
    const wrapper = await mountEditor([stored])

    expect(getHomeModelSystemPrices).not.toHaveBeenCalled()
    await wrapper.get('[data-field="vendor"]').setValue('自定义厂商')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()

    expect(getHomeModelSystemPrices).not.toHaveBeenCalled()
    expect(getHomeModels).toHaveBeenCalledOnce()
    expect(saveHomeModels).toHaveBeenCalledWith([{ ...stored, vendor: '自定义厂商' }])
  })

  it('新增模型确认名称后自动补空，保留已手填的零价', async () => {
    getHomeModelSystemPrices.mockResolvedValueOnce([systemTextPrice('gpt-5.4')])
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-field="input"]').setValue('0')
    await wrapper.get('[data-field="cachedInput"]').setValue('0')
    await wrapper.get('[data-field="name"]').setValue(' gpt-5.4 ')
    await flushPromises()

    expect(getHomeModelSystemPrices).toHaveBeenCalledWith([{ name: 'gpt-5.4', type: 'text' }])
    expect(fieldValue(wrapper, 'input')).toBe('0')
    expect(fieldValue(wrapper, 'output')).toBe('30')
    expect(fieldValue(wrapper, 'cachedInput')).toBe('0')
    expect(fieldValue(wrapper, 'flexInput')).toBe('2.5')
    expect(wrapper.get('[data-testid="price-status"]').text()).not.toBe('')
    expect(saveHomeModels).not.toHaveBeenCalled()
  })

  it('自动补全可获得图片尺寸价，且保留手动图片价格', async () => {
    getHomeModelSystemPrices.mockResolvedValueOnce([{
      name: 'known-image', type: 'image', found: true, cachedInput: null, flexInput: null,
      resolutionPrices: { '1K': 0.1, '2K': 0.15, '4K': 0.2 },
    }])
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-image"]').trigger('click')
    await wrapper.get('[data-field="2K"]').setValue('0')
    await wrapper.get('[data-field="name"]').setValue('known-image')
    await flushPromises()

    expect(fieldValue(wrapper, '1K')).toBe('0.1')
    expect(fieldValue(wrapper, '2K')).toBe('0')
    expect(fieldValue(wrapper, '4K')).toBe('0.2')
  })

  it('自动查询期间允许手动改价，返回结果只填仍为空的字段', async () => {
    const request = pendingPrices()
    getHomeModelSystemPrices.mockReturnValueOnce(request.promise)
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-field="name"]').setValue('gpt-test')

    expect(wrapper.get('[data-testid="model-row"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-field="input"]').setValue('7.25')
    await wrapper.get('[data-field="flexInput"]').setValue('0')
    request.resolve([systemTextPrice()])
    await flushPromises()

    expect(fieldValue(wrapper, 'input')).toBe('7.25')
    expect(fieldValue(wrapper, 'output')).toBe('30')
    expect(fieldValue(wrapper, 'flexInput')).toBe('0')
    await wrapper.get('[data-field="output"]').setValue('8.75')
    expect(fieldValue(wrapper, 'output')).toBe('8.75')
  })

  it('模型改名后丢弃旧响应，避免把前一个模型的价格写入新模型', async () => {
    const oldRequest = pendingPrices()
    const newRequest = pendingPrices()
    getHomeModelSystemPrices.mockReturnValueOnce(oldRequest.promise).mockReturnValueOnce(newRequest.promise)
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-field="name"]').setValue('old-model')
    await wrapper.get('[data-field="name"]').setValue('new-model')

    oldRequest.resolve([systemTextPrice('old-model', { input: 99 })])
    await flushPromises()
    expect(fieldValue(wrapper, 'input')).toBe('')

    newRequest.resolve([systemTextPrice('new-model', { input: 3.25 })])
    await flushPromises()
    expect(fieldValue(wrapper, 'name')).toBe('new-model')
    expect(fieldValue(wrapper, 'input')).toBe('3.25')
  })

  it('只触发输入而未确认改名时，旧模型响应也不能写回', async () => {
    const request = pendingPrices()
    getHomeModelSystemPrices.mockReturnValueOnce(request.promise)
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-field="name"]').setValue('old-model')
    const nameInput = wrapper.get('[data-field="name"]')
    ;(nameInput.element as HTMLInputElement).value = 'typing-new-model'
    await nameInput.trigger('input')

    request.resolve([systemTextPrice('old-model')])
    await flushPromises()
    expect(fieldValue(wrapper, 'input')).toBe('')
    expect(fieldValue(wrapper, 'output')).toBe('')
    expect(getHomeModelSystemPrices).toHaveBeenCalledOnce()
  })

  it('删除仍在查询的模型后，旧响应不会污染新增的同名行', async () => {
    const oldRequest = pendingPrices()
    const newRequest = pendingPrices()
    getHomeModelSystemPrices.mockReturnValueOnce(oldRequest.promise).mockReturnValueOnce(newRequest.promise)
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-field="name"]').setValue('same')
    await wrapper.get('[data-testid="remove-model"]').trigger('click')
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-field="name"]').setValue('same')

    oldRequest.resolve([systemTextPrice('same', { input: 99 })])
    await flushPromises()
    expect(wrapper.findAll('[data-testid="model-row"]')).toHaveLength(1)
    expect(fieldValue(wrapper, 'input')).toBe('')

    newRequest.resolve([systemTextPrice('same', { input: 6 })])
    await flushPromises()
    expect(fieldValue(wrapper, 'input')).toBe('6')
  })

  it('一键同步仅查询当前配置且保留顺序，未匹配与系统缺失的人工价格不变', async () => {
    const first = { ...textModel('first'), cachedInput: 9, flexInput: 8 }
    const unknown = { ...textModel('custom'), input: 0, output: 12, cachedInput: 0, flexInput: 7 }
    const image: HomeModel = { name: 'picture', type: 'image', vendor: '自定义图片厂商', resolutionPrices: { '1K': 1, '2K': 2, '4K': 3 } }
    getHomeModelSystemPrices.mockResolvedValueOnce([
      systemTextPrice('first', { input: 0, output: 6, cachedInput: null, flexInput: null }),
      { name: 'custom', type: 'text', found: false, cachedInput: null, flexInput: null, reason: '未匹配该模型' },
      { name: 'picture', type: 'image', found: true, cachedInput: null, flexInput: null, resolutionPrices: { '1K': 0.1, '2K': 0.2, '4K': 0.3 } },
    ])
    const wrapper = await mountEditor([first, unknown, image])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-testid="sync-prices"]').trigger('click')
    await flushPromises()

    expect(getHomeModelSystemPrices).toHaveBeenCalledWith([
      { name: 'first', type: 'text' }, { name: 'custom', type: 'text' }, { name: 'picture', type: 'image' },
    ])
    const rows = wrapper.findAll('[data-testid="model-row"]')
    expect(rows.map(row => fieldValue(row, 'name'))).toEqual(['first', 'custom', 'picture', ''])
    expect(fieldValue(rows[0], 'input')).toBe('0')
    expect(fieldValue(rows[0], 'output')).toBe('6')
    expect(fieldValue(rows[0], 'cachedInput')).toBe('9')
    expect(fieldValue(rows[0], 'flexInput')).toBe('8')
    expect(fieldValue(rows[1], 'input')).toBe('0')
    expect(fieldValue(rows[1], 'output')).toBe('12')
    expect(fieldValue(rows[1], 'cachedInput')).toBe('0')
    expect(fieldValue(rows[1], 'flexInput')).toBe('7')
    expect(rows[1].get('[data-testid="price-status"]').text()).toMatch(/未匹配|未找到/)
    expect(fieldValue(rows[2], 'vendor')).toBe('自定义图片厂商')
    expect(fieldValue(rows[2], '4K')).toBe('0.3')
    expect(saveHomeModels).not.toHaveBeenCalled()
    expect(getHomeModels).toHaveBeenCalledOnce()
  })

  it('同步请求期间禁止编辑、保存及重复同步，结束后恢复', async () => {
    const request = pendingPrices()
    getHomeModelSystemPrices.mockReturnValueOnce(request.promise)
    const wrapper = await mountEditor()
    await wrapper.get('[data-testid="sync-prices"]').trigger('click')

    for (const testId of ['sync-prices', 'save-models', 'add-text', 'add-image', 'model-row']) {
      expect(wrapper.get(`[data-testid="${testId}"]`).attributes('disabled')).toBeDefined()
    }
    await wrapper.get('[data-testid="sync-prices"]').trigger('click')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    expect(getHomeModelSystemPrices).toHaveBeenCalledOnce()
    expect(saveHomeModels).not.toHaveBeenCalled()

    request.resolve([systemTextPrice()])
    await flushPromises()
    expect(wrapper.get('[data-testid="save-models"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="model-row"]').attributes('disabled')).toBeUndefined()
  })

  it('系统同步失败保留全部人工报价，允许再次同步', async () => {
    getHomeModelSystemPrices.mockRejectedValueOnce(new Error('系统定价读取失败'))
    const wrapper = await mountEditor([{ ...textModel(), cachedInput: 0.3, flexInput: 0 }])
    await wrapper.get('[data-field="input"]').setValue('8.25')
    await wrapper.get('[data-testid="sync-prices"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('系统定价读取失败')
    expect(fieldValue(wrapper, 'input')).toBe('8.25')
    expect(fieldValue(wrapper, 'output')).toBe('2')
    expect(fieldValue(wrapper, 'cachedInput')).toBe('0.3')
    expect(fieldValue(wrapper, 'flexInput')).toBe('0')
    expect(wrapper.get('[data-testid="sync-prices"]').attributes('disabled')).toBeUndefined()
    expect(saveHomeModels).not.toHaveBeenCalled()

    getHomeModelSystemPrices.mockResolvedValueOnce([systemTextPrice()])
    await wrapper.get('[data-testid="sync-prices"]').trigger('click')
    await flushPromises()
    expect(fieldValue(wrapper, 'input')).toBe('5')
    expect(getHomeModelSystemPrices).toHaveBeenCalledTimes(2)
  })

  it('自动查询失败不覆盖手填值，仍可完成手动配置并保存', async () => {
    getHomeModelSystemPrices.mockRejectedValueOnce(new Error('自动获取价格失败'))
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-field="input"]').setValue('0')
    await wrapper.get('[data-field="name"]').setValue('custom-model')
    await flushPromises()

    expect(wrapper.get('[data-testid="price-status"]').text()).toContain('自动获取价格失败')
    expect(fieldValue(wrapper, 'input')).toBe('0')
    expect(fieldValue(wrapper, 'output')).toBe('')
    await wrapper.get('[data-field="output"]').setValue('1.25')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
    await flushPromises()
    expect(saveHomeModels).toHaveBeenCalledWith([{ ...textModel('custom-model'), input: 0, output: 1.25 }])
  })

  it('离开编辑器后返回的旧请求不影响重新打开的已保存配置', async () => {
    const request = pendingPrices()
    getHomeModelSystemPrices.mockReturnValueOnce(request.promise)
    const oldWrapper = await mountEditor([])
    await oldWrapper.get('[data-testid="add-text"]').trigger('click')
    await oldWrapper.get('[data-field="name"]').setValue('gpt-test')
    oldWrapper.unmount()
    const wrapper = await mountEditor([{ ...textModel(), input: 18 }])

    request.resolve([systemTextPrice()])
    await flushPromises()
    expect(fieldValue(wrapper, 'input')).toBe('18')
    expect(getHomeModelSystemPrices).toHaveBeenCalledOnce()
    expect(saveHomeModels).not.toHaveBeenCalled()
  })

  it('保存会等待名称确认触发的自动查询，然后提交补齐后的价格', async () => {
    const request = pendingPrices()
    getHomeModelSystemPrices.mockReturnValueOnce(request.promise)
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-field="name"]').setValue('gpt-test')
    await wrapper.get('[data-testid="save-models"]').trigger('click')

    expect(saveHomeModels).not.toHaveBeenCalled()
    request.resolve([systemTextPrice()])
    await flushPromises()
    expect(saveHomeModels).toHaveBeenCalledWith([{
      name: 'gpt-test', vendor: 'OpenAI', type: 'text', input: 5, output: 30, cachedInput: 0.5, flexInput: 2.5,
    }])
  })

  it('名称尚未失焦时按回车，也会补齐必填价格再保存', async () => {
    getHomeModelSystemPrices.mockResolvedValueOnce([systemTextPrice()])
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    const nameInput = wrapper.get('[data-field="name"]')
    ;(nameInput.element as HTMLInputElement).value = 'gpt-test'
    await nameInput.trigger('input')
    expect(getHomeModelSystemPrices).not.toHaveBeenCalled()

    await nameInput.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(getHomeModelSystemPrices).toHaveBeenCalledOnce()
    expect(saveHomeModels).toHaveBeenCalledWith([{
      name: 'gpt-test', vendor: 'OpenAI', type: 'text', input: 5, output: 30, cachedInput: 0.5, flexInput: 2.5,
    }])
  })

  it('一键同步后到达的旧自动响应不会改写同步结果或补入过期可选价', async () => {
    const oldRequest = pendingPrices()
    getHomeModelSystemPrices.mockReturnValueOnce(oldRequest.promise).mockResolvedValueOnce([
      systemTextPrice('gpt-test', { input: 4, output: 8, cachedInput: null, flexInput: null }),
    ])
    const wrapper = await mountEditor([])
    await wrapper.get('[data-testid="add-text"]').trigger('click')
    await wrapper.get('[data-field="name"]').setValue('gpt-test')
    await wrapper.get('[data-testid="sync-prices"]').trigger('click')
    await flushPromises()

    oldRequest.resolve([systemTextPrice()])
    await flushPromises()
    expect(fieldValue(wrapper, 'input')).toBe('4')
    expect(fieldValue(wrapper, 'output')).toBe('8')
    expect(fieldValue(wrapper, 'cachedInput')).toBe('')
    expect(fieldValue(wrapper, 'flexInput')).toBe('')
    expect(saveHomeModels).not.toHaveBeenCalled()
  })

  it('批量结果含非法价格时保留整份草稿，避免只同步部分模型', async () => {
    getHomeModelSystemPrices.mockResolvedValueOnce([
      systemTextPrice('first'), systemTextPrice('second', { input: -1 }),
    ])
    const wrapper = await mountEditor([textModel('first'), textModel('second')])
    await wrapper.get('[data-testid="sync-prices"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('系统价格响应格式异常')
    const rows = wrapper.findAll('[data-testid="model-row"]')
    expect(rows.map(row => fieldValue(row, 'input'))).toEqual(['1', '1'])
    expect(rows.map(row => fieldValue(row, 'output'))).toEqual(['2', '2'])
    expect(saveHomeModels).not.toHaveBeenCalled()
  })
})
