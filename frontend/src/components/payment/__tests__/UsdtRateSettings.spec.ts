import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UsdtRateSettings from '../UsdtRateSettings.vue'

const getConfig = vi.hoisted(() => vi.fn())
const updateConfig = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())
vi.mock('vue-i18n', async () => ({ ...(await vi.importActual('vue-i18n')), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { getConfig, updateConfig } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showSuccess }) }))

describe('USDT 兜底费率独立配置', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getConfig.mockResolvedValue({ data: { usdt_cny_fallback_rate: 0 } })
    updateConfig.mockResolvedValue({ data: {} })
  })

  it('未配置时保持空白，拒绝零值并仅保存有效兜底费率', async () => {
    const wrapper = mount(UsdtRateSettings)
    await flushPromises()
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
    await wrapper.get('input').setValue('0')
    await wrapper.get('button').trigger('click')
    expect(updateConfig).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.exchange.invalidFallback')
    await wrapper.get('input').setValue('7.123456')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(updateConfig).toHaveBeenCalledWith({ usdt_cny_fallback_rate: 7.123456 })
    expect(showSuccess).toHaveBeenCalled()
  })

  it.each(['101', '0.9', '7.1234567'])('拒绝范围或精度无效的费率 %s', async value => {
    const wrapper = mount(UsdtRateSettings)
    await flushPromises()
    await wrapper.get('input').setValue(value)
    await wrapper.get('button').trigger('click')
    expect(updateConfig).not.toHaveBeenCalled()
  })

  it('读取失败时提供重试且禁止用空数据覆盖配置', async () => {
    getConfig.mockRejectedValue(new Error('读取失败'))
    const wrapper = mount(UsdtRateSettings)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('读取失败')
    expect(wrapper.get('input').attributes('disabled')).toBeDefined()
    expect(updateConfig).not.toHaveBeenCalled()
  })
})
