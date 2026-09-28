import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { reactive } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelDetectionView from '../ModelDetectionView.vue'
import { modelDetectionAPI, newDetectionPlan, type ModelDetectionPlan, type ModelDetectionRun } from '@/api/admin/modelDetection'
import { list, getById, getAvailableModels } from '@/api/admin/accounts'

const { route, replace, success } = vi.hoisted(() => ({ route: { query: {} as Record<string, string> }, replace: vi.fn(), success: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ replace }) }))
vi.mock('vue-i18n', async original => ({ ...await original<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: success }) }))
vi.mock('@/api/client', () => ({ apiClient: {} }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/api/admin/accounts', () => ({ list: vi.fn(), getById: vi.fn(), getAvailableModels: vi.fn() }))
vi.mock('@/api/admin/modelDetection', async (original) => ({
  ...await original<typeof import('@/api/admin/modelDetection')>(),
  modelDetectionAPI: { overview: vi.fn(), catalog: vi.fn(), history: vi.fn(), create: vi.fn(), update: vi.fn(), run: vi.fn(), resetBaseline: vi.fn(), detail: vi.fn() }
}))
const plan: ModelDetectionPlan = { ...newDetectionPlan(17), id: 3, account_name: '测试账号', model_id: 'model-a', baseline_score: 90, baseline_run_id: 1, baseline_generation: 1, baseline_version: 'v1', last_run_at: null, next_run_at: null, created_at: '', updated_at: '' }
const run: ModelDetectionRun = { id: 8, plan_id: 3, account_id: 17, account_name: '测试账号', model_id: 'model-a', status: 'completed', verdict: 'suspected_drop', score: 60, baseline_score: 90, drop_points: 30, fingerprint: { status: 'unavailable', reference_model: '', message: '参考库未覆盖该模型' }, error_message: '', suite_version: 'v1', progress: 4, requests_total: 4, trigger: 'manual', started_at: null, finished_at: '2026-09-28T12:00:00Z', created_at: '2026-09-28T12:00:00Z', plan_snapshot: plan }
function render() {
  return mount(ModelDetectionView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    BaseDialog: { props: ['show', 'title'], template: '<section v-if="show" :data-dialog="title"><slot /></section>' },
    ConfirmDialog: true, ManualModelDetectionDialog: true, RouterLink: RouterLinkStub
  } } })
}

describe('管理员模型检测页面', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    route.query = reactive({})
    vi.mocked(list).mockResolvedValue({ items: [{ id: 17, name: '测试账号' }], total: 1 } as never)
    vi.mocked(getById).mockResolvedValue({ id: 17, name: '测试账号' } as never)
    vi.mocked(getAvailableModels).mockResolvedValue([{ id: 'model-a', display_name: '模型 A' }] as never)
    vi.mocked(modelDetectionAPI.catalog).mockResolvedValue({ reference_models: [], suite_version: 'v1', requests_per_run: 4 })
    vi.mocked(modelDetectionAPI.overview).mockResolvedValue({ active: [], recent: [run], plans: [plan], stats: { total: 1, normal: 0, suspected_drop: 1, error: 0, average_score: 60 } })
    vi.mocked(modelDetectionAPI.history).mockResolvedValue({ items: [run], next_before_id: null })
    vi.mocked(modelDetectionAPI.create).mockResolvedValue(plan)
    vi.mocked(modelDetectionAPI.update).mockResolvedValue(plan)
  })
  it('加载总览只读取检测记录，不自动发起模型请求；区分能力变化与指纹', async () => {
    const wrapper = render()
    await flushPromises()
    expect(modelDetectionAPI.run).not.toHaveBeenCalled()
    expect(modelDetectionAPI.create).not.toHaveBeenCalled()
    expect(modelDetectionAPI.history).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('modelDetection.explanation')
    expect(wrapper.text()).toContain('modelDetection.verdict.suspected_drop')
    expect(wrapper.text()).toContain('60')
    wrapper.unmount()
  })
  it('新增每天定时计划默认暂停，保留用户输入的时区与时间', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'modelDetection.createPlan')!.trigger('click')
    const dialog = wrapper.get('[data-dialog="modelDetection.createPlan"]')
    expect((dialog.get('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)
    await dialog.findAll('select')[0].setValue(17)
    await dialog.get('input[list="plan-detection-models"]').setValue('custom-model')
    await dialog.findAll('select')[1].setValue('daily')
    await dialog.get('input[type="time"]').setValue('13:45')
    await dialog.get('input[placeholder="Asia/Shanghai"]').setValue('Asia/Tokyo')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(modelDetectionAPI.create).toHaveBeenCalledWith(expect.objectContaining({ account_id: 17, model_id: 'custom-model', schedule_type: 'daily', daily_time: '13:45', timezone: 'Asia/Tokyo', enabled: false }))
    expect(modelDetectionAPI.run).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('从账号列带入账号，并用游标加载更早的历史记录', async () => {
    route.query.account_id = '17'
    vi.mocked(modelDetectionAPI.history).mockResolvedValueOnce({ items: [run], next_before_id: 8 }).mockResolvedValueOnce({ items: [{ ...run, id: 7 }], next_before_id: null })
    const wrapper = render()
    await flushPromises()
    expect(modelDetectionAPI.history).toHaveBeenCalledWith(17, undefined)
    await wrapper.findAll('button').find(button => button.text() === 'modelDetection.more')!.trigger('click')
    await flushPromises()
    expect(modelDetectionAPI.history).toHaveBeenLastCalledWith(17, 8)
    expect(wrapper.findAll('button').filter(button => button.text() === 'modelDetection.detail')).toHaveLength(3)
    wrapper.unmount()
  })
  it('详情保留回答和原始错误，恶意回答只作为文本展示', async () => {
    vi.mocked(modelDetectionAPI.detail).mockResolvedValue({ ...run, error_message: '上游模型不可用', details: [{ kind: 'quality', prompt: '测试题', response: '<img src=x onerror=alert(1)>', evaluation: { score: 60 } }] })
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'modelDetection.detail')!.trigger('click')
    await flushPromises()
    const dialog = wrapper.get('[data-dialog="modelDetection.details"]')
    expect(dialog.text()).toContain('<img src=x onerror=alert(1)>')
    expect(dialog.find('img').exists()).toBe(false)
    expect(dialog.text()).toContain('参考库未覆盖该模型')
    expect(dialog.text()).toContain('上游模型不可用')
    wrapper.unmount()
  })
  it('无效时区阻止提交并保持表单可修改', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'modelDetection.createPlan')!.trigger('click')
    const dialog = wrapper.get('[data-dialog="modelDetection.createPlan"]')
    await dialog.findAll('select')[0].setValue(17)
    await dialog.get('input[list="plan-detection-models"]').setValue('model-a')
    await dialog.get('input[placeholder="Asia/Shanghai"]').setValue('invalid/timezone')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(modelDetectionAPI.create).not.toHaveBeenCalled()
    expect(dialog.get('[role="alert"]').text()).toBe('modelDetection.invalidForm')
    wrapper.unmount()
  })
})
