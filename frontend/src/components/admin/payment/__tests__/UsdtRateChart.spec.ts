import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UsdtRateChart from '../UsdtRateChart.vue'
import type { UsdtCnyQuote } from '@/types/payment'

const getUsdtRates = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { getUsdtRates } }))
vi.mock('vue-i18n', async () => ({ ...(await vi.importActual('vue-i18n')), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-chartjs', () => ({ Line: { name: 'RateLine', props: ['data', 'options'], template: '<div data-test="rate-line" />' } }))

function quote(source: UsdtCnyQuote['source'], time: string, rate: number): UsdtCnyQuote {
  return { source, rate, fetched_at: time, observed_at: time, sample_prices: source === 'okx' ? [rate] : [], sample_count: source === 'okx' ? 10 : 0, aggregation: source === 'okx' ? 'median_first_10_sell' : 'configured_fallback' }
}
function mountChart() {
  return mount(UsdtRateChart, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Icon: true, LoadingSpinner: true } } })
}

describe('USDT 最近三天费率图', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-09-30T12:00:00Z')); getUsdtRates.mockReset() })
  afterEach(() => vi.useRealTimers())

  it('真实成功样本与兜底点分色，不填补失败区间或生成历史数据', async () => {
    const history = [quote('okx', '2026-09-30T10:00:00Z', 7.2), quote('fallback', '2026-09-30T10:30:00Z', 7.3), quote('unavailable', '2026-09-30T11:00:00Z', 0), quote('okx', '2026-09-30T11:30:00Z', 7.25)]
    getUsdtRates.mockResolvedValue({ data: { current: quote('fallback', '2026-09-30T12:00:00Z', 7.3), history, error: '当前实时行情获取失败，使用兜底' } })
    const wrapper = mountChart()
    await flushPromises()
    expect(wrapper.text()).toContain('payment.exchange.fallbackPayment')
    expect(wrapper.get('[data-test="current-usdt-rate"]').text()).toBe('1 USDT = ¥7.3')
    expect(wrapper.get('[data-test="usdt-current-error"]').text()).toContain('使用兜底')
    const chart = wrapper.getComponent({ name: 'RateLine' })
    const datasets = chart.props('data').datasets
    expect(datasets[0].data.map((point: { y: number | null }) => point.y)).toEqual([7.2, null, null, 7.25])
    expect(datasets[1].data.map((point: { y: number | null }) => point.y)).toEqual([null, 7.3, null, null])
    expect(datasets[1].borderColor).toBe('#d97706')
    expect(datasets[0].borderColor).toBe('#0d9488')
    expect(datasets[0].data).toHaveLength(4)
    expect(chart.props('options').scales.x.max - chart.props('options').scales.x.min).toBe(72 * 60 * 60 * 1000)
    wrapper.unmount()
  })

  it('暂无历史时显示明确空态，即使当前有兜底值也不生成三天走势', async () => {
    getUsdtRates.mockResolvedValue({ data: { current: quote('fallback', '2026-09-30T12:00:00Z', 7.3), history: [] } })
    const wrapper = mountChart()
    await flushPromises()
    expect(wrapper.find('[data-test="rate-line"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="usdt-history-empty"]').text()).toContain('payment.exchange.noHistory')
    wrapper.unmount()
  })

  it('获取失败显示错误说明，过期历史报价不当成当前费率', async () => {
    getUsdtRates.mockResolvedValueOnce({ data: { current: quote('okx', '2026-09-30T11:29:00Z', 7.2), history: [] } }).mockRejectedValue(new Error('网络错误'))
    const wrapper = mountChart()
    await flushPromises()
    expect(wrapper.get('[data-test="current-usdt-rate"]').text()).toBe('—')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('payment.exchange.loadFailed')
    wrapper.unmount()
  })
})
