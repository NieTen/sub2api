import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsSystemLogTable from '../OpsSystemLogTable.vue'
import LogRetentionSelect from '../LogRetentionSelect.vue'
import enLocale from '@/i18n/locales/en'
import zhLocale from '@/i18n/locales/zh'

const mockListSystemLogs = vi.fn()
const mockCleanupSystemLogs = vi.fn()
const mockGetSystemLogSinkHealth = vi.fn()
const mockGetRuntimeLogConfig = vi.fn()
const mockUpdateRuntimeLogConfig = vi.fn()
const mockShowError = vi.fn()
const mockCopyToClipboard = vi.fn()

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: mockCopyToClipboard }),
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listSystemLogs: (...args: any[]) => mockListSystemLogs(...args),
    cleanupSystemLogs: (...args: any[]) => mockCleanupSystemLogs(...args),
    getSystemLogSinkHealth: (...args: any[]) => mockGetSystemLogSinkHealth(...args),
    getRuntimeLogConfig: (...args: any[]) => mockGetRuntimeLogConfig(...args),
    updateRuntimeLogConfig: (...args: any[]) => mockUpdateRuntimeLogConfig(...args),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: any[]) => mockShowError(...args),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: {
    modelValue: {
      type: [String, Number],
      default: '',
    },
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

const PaginationStub = defineComponent({
  name: 'PaginationStub',
  template: '<div class="pagination-stub" />',
})

const runtimeConfig = {
  level: 'info',
  persist_access_logs: false,
  enable_sampling: false,
  sampling_initial: 100,
  sampling_thereafter: 100,
  caller: true,
  stacktrace_level: 'error',
  retention_days: 30,
  request_retention_days: 90,
}

const sinkHealth = {
  queue_depth: 0,
  queue_capacity: 5000,
  dropped_count: 0,
  write_failed_count: 0,
  written_count: 1,
  avg_write_delay_ms: 0,
}

describe('OpsSystemLogTable host support', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mockListSystemLogs.mockResolvedValue({
      items: [
        {
          id: 1,
          created_at: '2026-07-14T00:10:01Z',
          host: 'api-node-1',
          level: 'warn',
          component: 'app',
          message: 'request failed',
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    })
    mockCleanupSystemLogs.mockResolvedValue({ deleted: 1 })
    mockGetSystemLogSinkHealth.mockResolvedValue(sinkHealth)
    mockGetRuntimeLogConfig.mockResolvedValue(runtimeConfig)
    mockUpdateRuntimeLogConfig.mockImplementation(async config => config)
  })

  it('renders the host and sends it with list and cleanup filters', async () => {
    const wrapper = mount(OpsSystemLogTable, {
      global: {
        stubs: {
          Select: SelectStub,
          Pagination: PaginationStub,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('api-node-1')

    const hostLabel = wrapper.findAll('label').find((label) => label.text().includes('admin.ops.systemLogs.host'))
    expect(hostLabel).toBeDefined()
    await hostLabel!.find('input').setValue(' api-node-2 ')

    const searchButton = wrapper.findAll('button').find((button) => button.text() === 'admin.ops.systemLogs.search')
    expect(searchButton).toBeDefined()
    await searchButton!.trigger('click')
    await flushPromises()

    expect(mockListSystemLogs).toHaveBeenLastCalledWith(expect.objectContaining({ host: 'api-node-2' }))

    const cleanupButton = wrapper.findAll('button').find((button) => button.text() === 'admin.ops.systemLogs.cleanCurrentFilters')
    expect(cleanupButton).toBeDefined()
    await cleanupButton!.trigger('click')
    await flushPromises()

    expect(mockCleanupSystemLogs).toHaveBeenCalledWith(expect.objectContaining({ host: 'api-node-2' }))
  })

  it('keeps database access-log persistence opt-in', async () => {
    const wrapper = mount(OpsSystemLogTable, {
      global: {
        stubs: {
          Select: SelectStub,
          Pagination: PaginationStub,
        },
      },
    })
    await flushPromises()

    const label = wrapper.findAll('label').find((item) => item.text().includes('admin.ops.systemLogs.persistAccessLogs'))
    expect(label).toBeDefined()
    expect((label!.find('input').element as HTMLInputElement).checked).toBe(false)
  })

  it.each([
    ['zh', zhLocale],
    ['en', enLocale],
  ])('defines the Host translation for %s', (_name, locale) => {
    expect(locale.admin.ops.systemLogs.host).toBe('Host')
  })
})


describe('OKPay 调试日志详情', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetSystemLogSinkHealth.mockResolvedValue(sinkHealth)
    mockGetRuntimeLogConfig.mockResolvedValue(runtimeConfig)
    mockCopyToClipboard.mockResolvedValue(true)
  })

  afterEach(() => vi.restoreAllMocks())

  it.each([true, false])('桌面模式 %s 仅展示并复制白名单诊断字段，HTML 保持普通文本', async isDesktop => {
    vi.spyOn(window, 'matchMedia').mockImplementation(query => ({
      matches: isDesktop, media: query, onchange: null,
      addListener: vi.fn(), removeListener: vi.fn(),
      addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: () => false,
    }))
    const upstreamMessage = '<img src=x onerror=alert(1)><script>alert(1)</script>'
    const safeExtra = {
      provider: 'okpay', instance_id: '4', operation: 'payLink',
      signature_algorithm: 'hmac_sha256', transport: 'current', http_status: 200,
      duration_ms: 321, result: 'rejected', request_sent: true, response_bytes: 128,
      upstream_status: 'warning', business_code: '400', stage: 'upstream',
      request: {
        field_count: 3, fields: ['amount', 'coin', 'sign'], amount: '0.99', coin: 'USDT',
        status: '0', timestamp: '1790841600', name_bytes: 12, name_runes: 4,
        unique_id_bytes: 16, nonce_bytes: 32, sign_bytes: 64,
        return_url: { present: true, bytes: 100, https: true, host: 'site.example', path_bytes: 15, query_bytes: 20, query_params: 1, scheme: 'https', valid: true, validation_reason: '', has_userinfo: false, has_control_chars: false, source: 'request' },
        callback_url: { present: false, bytes: 0, https: false, host: '', path_bytes: 0, query_bytes: 0, query_params: 0, scheme: 'relative', valid: false, validation_reason: '', has_userinfo: false, has_control_chars: false, source: 'empty' },
      },
      upstream_messages: { msg: 'amount 必须大于 1 USDT', message: upstreamMessage },
    }
    mockListSystemLogs.mockResolvedValue({
      items: [{
        id: 1, created_at: '2026-10-01T12:00:00Z', request_id: 'req-okpay-test', host: 'api-node-1', level: 'warn',
        component: 'payment.okpay', message: 'OKPay debug',
        extra: {
          ...safeExtra, token: '应隐藏顶层凭据', error: '应隐藏通用错误', data: { balance: '应隐藏余额' },
          request: {
            ...safeExtra.request, token: '应隐藏请求凭据', sign: '应隐藏签名',
            fields: [...safeExtra.request.fields, '应隐藏任意字段', { token: '应隐藏字段对象' }],
            return_url: { ...safeExtra.request.return_url, url: '应隐藏完整地址', query: '应隐藏查询内容' },
          },
          upstream_messages: { ...safeExtra.upstream_messages, raw: '应隐藏原始响应' },
        },
      }], total: 1, page: 1, page_size: 20,
    })
    const wrapper = mount(OpsSystemLogTable, {
      global: { stubs: { Select: SelectStub, Pagination: PaginationStub } },
    })
    await flushPromises()
    expect(wrapper.find('table').exists()).toBe(isDesktop)
    const details = wrapper.get('[data-testid="okpay-debug-details"]')
    expect(details.element.tagName).toBe('DETAILS')
    expect((details.element as HTMLDetailsElement).open).toBe(false)
    expect(details.get('summary').text()).toBe('admin.ops.systemLogs.okpayDebugDetails')
    ;(details.element as HTMLDetailsElement).open = true
    const json = details.get('pre').text()
    expect(JSON.parse(json)).toEqual({ created_at: '2026-10-01T12:00:00Z', request_id: 'req-okpay-test', component: 'payment.okpay', ...safeExtra })
    expect(wrapper.text()).not.toContain('应隐藏')
    expect(details.find('img').exists()).toBe(false)
    expect(details.find('script').exists()).toBe(false)
    expect(details.get('pre').text()).toContain(upstreamMessage)
    await details.get('button').trigger('click')
    expect(mockCopyToClipboard).toHaveBeenCalledWith(json)
    wrapper.unmount()
  })

  it('显示本地地址校验失败及来源，只接受约定 URL 枚举和布尔值', async () => {
    const callbackSummary = {
      present: true, bytes: 60, https: false, host: 'site.example', scheme: 'http', valid: false,
      validation_reason: 'scheme_not_https', has_userinfo: false, has_control_chars: false, source: 'provider_config',
    }
    mockListSystemLogs.mockResolvedValue({
      items: [{
        id: 3, created_at: '2026-10-01T12:00:00Z', host: '', level: 'warn', component: 'payment.okpay', message: 'OKPay debug',
        extra: {
          stage: 'validation', request_sent: false, validation_field: 'callback_url',
          validation_reason: 'scheme_not_https', validation_message: '回调地址必须使用 HTTPS',
          request: {
            callback_url: { ...callbackSummary, raw_url: '应隐藏完整URL' },
            return_url: {
              scheme: 'https://应隐藏任意协议', validation_reason: '应隐藏任意原因', source: '应隐藏任意来源',
              valid: '应隐藏非布尔值', has_userinfo: 'false', has_control_chars: 1,
            },
          },
        },
      }], total: 1, page: 1, page_size: 20,
    })
    const wrapper = mount(OpsSystemLogTable, {
      global: { stubs: { Select: SelectStub, Pagination: PaginationStub } },
    })
    await flushPromises()
    const details = wrapper.get('[data-testid="okpay-debug-details"]')
    const json = details.get('pre').text()
    expect(JSON.parse(json)).toMatchObject({
      stage: 'validation', request_sent: false, validation_field: 'callback_url',
      validation_reason: 'scheme_not_https', validation_message: '回调地址必须使用 HTTPS',
      request: { callback_url: callbackSummary },
    })
    expect(JSON.parse(json).request).not.toHaveProperty('return_url')
    expect(wrapper.text()).not.toContain('应隐藏')
    await details.get('button').trigger('click')
    expect(mockCopyToClipboard).toHaveBeenCalledWith(json)
    wrapper.unmount()
  })

  it.each([
    ['app', 'OKPay debug'],
    ['payment.okpay', '普通支付日志'],
  ])('不为其他日志 %s / %s 展示专用详情', async (component, message) => {
    mockListSystemLogs.mockResolvedValue({
      items: [{ id: 2, created_at: '2026-10-01T12:00:00Z', host: '', level: 'warn', component, message, extra: { upstream_messages: { msg: '不应展开' } } }],
      total: 1, page: 1, page_size: 20,
    })
    const wrapper = mount(OpsSystemLogTable, {
      global: { stubs: { Select: SelectStub, Pagination: PaginationStub } },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="okpay-debug-details"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('不应展开')
    wrapper.unmount()
  })
})

describe('rolling log retention settings', () => {
  const mountTable = () => mount(OpsSystemLogTable, {
    global: { stubs: { Select: SelectStub, Pagination: PaginationStub } }
  })
  const saveButton = (wrapper: ReturnType<typeof mountTable>) => wrapper.findAll('button').find(button => button.text() === 'admin.ops.systemLogs.saveAndApply')!

  beforeEach(() => {
    vi.clearAllMocks()
    mockListSystemLogs.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    mockGetSystemLogSinkHealth.mockResolvedValue(sinkHealth)
    mockGetRuntimeLogConfig.mockResolvedValue(runtimeConfig)
    mockUpdateRuntimeLogConfig.mockImplementation(async config => config)
  })

  it('loads and saves separate operations and request retention windows', async () => {
    const wrapper = mountTable()
    await flushPromises()
    const fields = wrapper.findAllComponents(LogRetentionSelect)
    expect(fields.map(field => field.props('modelValue'))).toEqual([30, 90])
    fields[0]!.vm.$emit('update:modelValue', 7)
    fields[1]!.vm.$emit('update:modelValue', 180)
    await saveButton(wrapper).trigger('click')
    await flushPromises()
    expect(mockUpdateRuntimeLogConfig).toHaveBeenCalledWith(expect.objectContaining({ retention_days: 7, request_retention_days: 180 }))
    wrapper.unmount()
  })

  it('supports forever and rejects fractional or empty custom days', async () => {
    const wrapper = mountTable()
    await flushPromises()
    const requestField = wrapper.findAllComponents(LogRetentionSelect)[1]!
    requestField.vm.$emit('update:modelValue', 0)
    await saveButton(wrapper).trigger('click')
    await flushPromises()
    expect(mockUpdateRuntimeLogConfig).toHaveBeenCalledWith(expect.objectContaining({ request_retention_days: 0 }))
    mockUpdateRuntimeLogConfig.mockClear()
    for (const value of [1.5, Number.NaN, -1, 3651]) {
      requestField.vm.$emit('update:modelValue', value)
      await saveButton(wrapper).trigger('click')
    }
    expect(mockUpdateRuntimeLogConfig).not.toHaveBeenCalled()
    expect(mockShowError).toHaveBeenCalledWith('admin.ops.systemLogs.retentionDaysInvalid')
    wrapper.unmount()
  })

  it('does not overwrite saved retention with defaults after a load failure', async () => {
    mockGetRuntimeLogConfig.mockRejectedValue(new Error('unavailable'))
    const wrapper = mountTable()
    await flushPromises()
    expect(saveButton(wrapper).attributes('disabled')).toBeDefined()
    await saveButton(wrapper).trigger('click')
    expect(mockUpdateRuntimeLogConfig).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
