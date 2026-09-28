import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ModelDetectionModelPicker from '../ModelDetectionModelPicker.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const models = Array.from({ length: 21 }, (_, index) => ({ id: `model-${index + 1}`, display_name: `模型 ${index + 1}` }))
describe('检测模型多选', () => {
  it('搜索不清空已选模型，并可通过已选项移除', async () => {
    const wrapper = mount(ModelDetectionModelPicker, { props: { models, modelValue: ['model-1'] } })
    await wrapper.get('[data-testid="model-search"]').setValue('model-20')
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="selected-models"]').text()).toContain('model-1')
    await wrapper.get('[data-testid="selected-models"] button').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[]])
    wrapper.unmount()
  })
  it('添加自定义模型去空白去重，并保留当前选择', async () => {
    const wrapper = mount(ModelDetectionModelPicker, { props: { models, modelValue: ['model-1'] } })
    await wrapper.get('[data-testid="custom-model"]').setValue(' model-1 ')
    await wrapper.get('[data-testid="custom-model"]').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await wrapper.get('[data-testid="custom-model"]').setValue(' custom-reasoner ')
    await wrapper.get('[data-testid="custom-model"]').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['model-1', 'custom-reasoner']])
    wrapper.unmount()
  })
  it('达到二十个上限时禁止增加，仍可取消已选项', async () => {
    const wrapper = mount(ModelDetectionModelPicker, { props: { models, modelValue: models.slice(0, 20).map(model => model.id) } })
    expect(wrapper.get('input[value="model-21"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('input[value="model-1"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('input[value="model-1"]').setValue(false)
    expect((wrapper.emitted('update:modelValue')?.[0][0] as string[])).toHaveLength(19)
    wrapper.unmount()
  })
})
