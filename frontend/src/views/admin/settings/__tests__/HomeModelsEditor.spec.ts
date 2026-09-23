import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import HomeModelsEditor from '../HomeModelsEditor.vue'
import type { HomeModel, HomeTextModel } from '@/api/admin/homeModels'

const { getHomeModels, saveHomeModels } = vi.hoisted(() => ({
  getHomeModels: vi.fn(),
  saveHomeModels: vi.fn(),
}))

vi.mock('@/api/admin/homeModels', () => ({ getHomeModels, saveHomeModels }))

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

    expect(saveHomeModels).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain(error)
  })

  it('拒绝超出有限数字范围的价格', async () => {
    const wrapper = await mountEditor()
    await wrapper.get('[data-field="input"]').setValue('1e309')
    await wrapper.get('[data-testid="save-models"]').trigger('click')
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
})
