import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getAllProxies,
  getAllGroups
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      getBatchUsage: vi.fn(),
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token',
    isSimpleMode: false
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const DataTableStub = {
  props: ['data'],
  template: `
    <div data-test="data-table">
      <div v-for="row in data" :key="row.id" :data-test="'account-row-' + row.id">
        <span data-test="account-name">{{ row.name }}</span>
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `
}

const AccountTableFiltersStub = {
  props: ['filters'],
  emits: ['update:filters'],
  methods: {
    setStatus(status: string) {
      this.$emit('update:filters', { ...this.filters, status })
    }
  },
  template: `
    <div>
      <button data-test="filter-active" @click="setStatus('active')">active</button>
      <button data-test="filter-overdrafting" @click="setStatus('overdrafting')">overdrafting</button>
    </div>
  `
}

const AccountUsageCellStub = {
  props: ['account'],
  emits: ['usage-loaded'],
  methods: {
    emitOverdraftUsage() {
      this.$emit('usage-loaded', {
        updated_at: '2026-08-20T00:00:00Z',
        five_hour: {
          utilization: 100,
          resets_at: '2026-08-20T05:00:00Z',
          remaining_seconds: 18000,
          overdraft_active: true
        },
        seven_day: null,
        seven_day_sonnet: null,
        codex_quota_overdraft: {
          status: 'passed',
          quota_window: '5h',
          cycle_key: '5h:2026-08-20',
          attempts: 1,
          limit: 1,
          started_at: '2026-08-20T00:00:00Z'
        }
      })
    },
    emitNormalUsage() {
      this.$emit('usage-loaded', {
        updated_at: '2026-08-20T00:00:00Z',
        five_hour: {
          utilization: 25,
          resets_at: '2026-08-20T05:00:00Z',
          remaining_seconds: 18000,
          overdraft_active: false
        },
        seven_day: {
          utilization: 30,
          resets_at: '2026-08-27T00:00:00Z',
          remaining_seconds: 604800,
          overdraft_active: false
        },
        seven_day_sonnet: null,
        codex_quota_overdraft: null
      })
    }
  },
  template: `
    <div>
      <button :data-test="'usage-overdraft-' + account.id" @click="emitOverdraftUsage">overdraft</button>
      <button :data-test="'usage-normal-' + account.id" @click="emitNormalUsage">normal</button>
    </div>
  `
}

const account = (id: number, name: string) => ({
  id,
  name,
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z'
})

const mountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: {
        template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
      },
      DataTable: DataTableStub,
      Pagination: true,
      ConfirmDialog: true,
      AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
      AccountTableFilters: AccountTableFiltersStub,
      AccountBulkActionsBar: true,
      AccountActionMenu: true,
      ImportDataModal: true,
      ReAuthAccountModal: true,
      AccountTestModal: true,
      AccountStatsModal: true,
      ScheduledTestsPanel: true,
      SyncFromCrsModal: true,
      TempUnschedStatusModal: true,
      ErrorPassthroughRulesModal: true,
      TLSFingerprintProfilesModal: true,
      CreateAccountModal: true,
      EditAccountModal: true,
      BulkEditAccountModal: true,
      PlatformTypeBadge: true,
      AccountCapacityCell: true,
      AccountStatusIndicator: true,
      AccountTodayStatsCell: true,
      AccountGroupsCell: true,
      AccountUsageCell: AccountUsageCellStub,
      UpstreamBillingRateCell: true,
      HelpTooltip: true,
      Icon: true
    }
  }
})

describe('admin AccountsView 透支状态本地过滤', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getUpstreamBillingProbeSettings.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()

    listAccounts.mockResolvedValue({
      items: [
        account(1, 'normal-openai'),
        account(2, 'overdraft-openai')
      ],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: true, interval_minutes: 30 })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  it('筛选正常账户时，在用量确认透支中后移除该账号', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="filter-active"]').trigger('click')
    await wrapper.get('[data-test="usage-overdraft-2"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="account-row-1"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="account-row-2"]').exists()).toBe(false)
  })

  it('筛选透支中时，仅保留用量确认正在透支的账号', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="filter-overdrafting"]').trigger('click')
    await wrapper.get('[data-test="usage-normal-1"]').trigger('click')
    await wrapper.get('[data-test="usage-overdraft-2"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="account-row-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="account-row-2"]').exists()).toBe(true)
  })
})
