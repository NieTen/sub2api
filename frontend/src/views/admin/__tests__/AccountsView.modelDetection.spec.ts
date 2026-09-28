import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import AccountsView from '../AccountsView.vue'
import ManualModelDetectionDialog from '@/components/admin/account/ManualModelDetectionDialog.vue'
const { list, summaries } = vi.hoisted(() => ({ list: vi.fn(), summaries: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: {
  accounts: { list, listWithEtag: vi.fn(), getBatchTodayStats: vi.fn().mockResolvedValue({ stats: {} }), getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: true, interval_minutes: 30 }) },
  proxies: { getAll: vi.fn().mockResolvedValue([]) }, groups: { getAll: vi.fn().mockResolvedValue([]) }
} }))
vi.mock('@/api/admin/modelDetection', async original => ({ ...await original<typeof import('@/api/admin/modelDetection')>(), modelDetectionAPI: { summaries } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ token: 'test', isSimpleMode: false }) }))
vi.mock('vue-i18n', async original => ({ ...await original<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))

function render() {
  return mount(AccountsView, { global: { stubs: {
    AppLayout: { template: '<div><slot /></div>' }, TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
    DataTable: { props: ['columns', 'data'], template: '<div><div v-for="row in data" :key="row.id" :data-row="row.id"><slot v-if="columns.some(column => column.key === \'model_detection\')" name="cell-model_detection" :row="row" /></div></div>' },
    AccountTableActions: { template: '<div><slot name="after" /></div>' }, AccountTableFilters: true, AccountBulkActionsBar: true, Pagination: true, ConfirmDialog: true,
    AccountActionMenu: true, ImportDataModal: true, ReAuthAccountModal: true, AccountTestModal: true, AccountStatsModal: true, ScheduledTestsPanel: true,
    SyncFromCrsModal: true, TempUnschedStatusModal: true, ErrorPassthroughRulesModal: true, TLSFingerprintProfilesModal: true, CreateAccountModal: true,
    EditAccountModal: true, BulkEditAccountModal: true, PlatformTypeBadge: true, AccountCapacityCell: true, AccountStatusIndicator: true,
    AccountTodayStatsCell: true, AccountGroupsCell: true, AccountUsageCell: true, HelpTooltip: true, Icon: true, Teleport: true,
    ManualModelDetectionDialog: true, RouterLink: RouterLinkStub
  } } })
}
describe('账号列表模型检测列', () => {
  beforeEach(() => {
    vi.clearAllMocks(); localStorage.clear()
    list.mockResolvedValue({ items: [{ id: 17, name: '账号一' }, { id: 18, name: '账号二' }], total: 2, page: 1, page_size: 20, pages: 1 })
    summaries.mockResolvedValue([{ account_id: 17, plan_count: 1, running_count: 0, latest_run: { status: 'completed', verdict: 'suspected_drop', score: 40, model_id: 'model-a' } }])
  })
  it('批量查询摘要，每个账号均可直接手动检测，历史带入对应账号', async () => {
    const wrapper = render()
    await flushPromises()
    expect(summaries).toHaveBeenCalledTimes(1)
    expect(summaries).toHaveBeenCalledWith([17, 18], expect.any(AbortSignal))
    expect(wrapper.get('[data-row="17"]').text()).toContain('modelDetection.verdict.suspected_drop')
    expect(wrapper.get('[data-row="18"]').text()).toContain('modelDetection.notTested')
    expect(wrapper.findAll('button').filter(button => button.text() === 'modelDetection.run')).toHaveLength(2)
    await wrapper.get('[data-row="18"] button').trigger('click')
    expect(wrapper.findComponent(ManualModelDetectionDialog).props('account')).toEqual({ id: 18, name: '账号二' })
    expect(wrapper.findComponent(ManualModelDetectionDialog).props('show')).toBe(true)
    expect(wrapper.findAllComponents(RouterLinkStub).map(link => link.props('to'))).toContainEqual({ path: '/admin/model-detection', query: { account_id: 18 } })
    wrapper.unmount()
  })
  it('保留原列显隐偏好，隐藏检测列时不请求摘要', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['model_detection']))
    localStorage.setItem('account-hidden-columns-version', 'scheduler-score-hidden-by-default')
    const wrapper = render()
    await flushPromises()
    expect(summaries).not.toHaveBeenCalled()
    expect(wrapper.get('[data-row="18"]').text()).toBe('')
    wrapper.unmount()
  })
})
