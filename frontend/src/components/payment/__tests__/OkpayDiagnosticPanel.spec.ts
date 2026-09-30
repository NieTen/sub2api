import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OkpayDiagnosticPanel from '../OkpayDiagnosticPanel.vue'
import ProviderCard from '../ProviderCard.vue'
import zhSettings from '@/i18n/locales/zh/admin/settings'
import type { OkpayDiagnosticConclusion, OkpayDiagnosticResult, ProviderInstance } from '@/types/payment'

const diagnoseOkpayProvider = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { diagnoseOkpayProvider } }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      const name = key.replace('admin.settings.payment.okpayDiagnostic.', '')
      const copy = zhSettings.settings.payment.okpayDiagnostic
      return copy[name as keyof typeof copy] ?? key
    },
  }),
}))

function resultFixture(id = 42, conclusion: OkpayDiagnosticConclusion = 'both_authenticated'): OkpayDiagnosticResult {
  return {
    provider_instance_id: id,
    provider_name: '已保存商户 ' + id,
    checks: [
      { mode: 'current', status: 'authenticated', message: '当前请求认证通过 ' + id },
      { mode: 'php_reference', status: 'authenticated', message: '兼容请求认证通过 ' + id },
    ],
    conclusion,
  }
}

function mountPanel(id = 42) {
  return mount(OkpayDiagnosticPanel, { props: { providerId: id, providerName: '商户 ' + id }, global: { stubs: { Icon: true } } })
}

describe('已保存 OKPay 实例认证诊断', () => {
  beforeEach(() => diagnoseOkpayProvider.mockReset())

  it('用户主动点击后显示两种请求结果，不显示响应中的额外敏感数据', async () => {
    const response = resultFixture()
    diagnoseOkpayProvider.mockResolvedValue({ data: { ...response, checks: [...response.checks].reverse(), token: '不应显示的密钥', sign: '不应显示的签名', balance: '9876.54321', raw_response: '不应显示的上游正文' } })
    const wrapper = mountPanel()
    expect(diagnoseOkpayProvider).not.toHaveBeenCalled()
    expect(wrapper.find('input, textarea').exists()).toBe(false)
    expect(wrapper.text()).toContain('不创建充值订单')
    await wrapper.get('[data-test="diagnose-okpay"]').trigger('click')
    await flushPromises()
    expect(diagnoseOkpayProvider).toHaveBeenCalledTimes(1)
    expect(diagnoseOkpayProvider).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-test="okpay-check-current"]').text()).toContain('当前请求认证通过 42')
    expect(wrapper.get('[data-test="okpay-check-php_reference"]').text()).toContain('兼容请求认证通过 42')
    expect(wrapper.get('[data-test="okpay-diagnostic-conclusion"]').text()).toContain('不能证明创建支付订单正常')
    for (const hidden of ['不应显示的密钥', '不应显示的签名', '9876.54321', '不应显示的上游正文']) expect(wrapper.text()).not.toContain(hidden)
    wrapper.unmount()
  })

  it.each([
    ['both_authenticated', '两种请求均通过认证'],
    ['php_only_authenticated', 'PHP 示例兼容请求通过，当前请求未通过'],
    ['current_only_authenticated', '当前请求通过，PHP 示例兼容请求未通过'],
    ['both_rejected', '不能仅凭此结果判定密钥错误'],
    ['inconclusive', '诊断未能得出一致结论'],
  ] as const)('准确解释 %s 结论', async (conclusion, expected) => {
    const data = resultFixture(42, conclusion)
    data.checks[0].status = conclusion === 'both_authenticated' || conclusion === 'current_only_authenticated' ? 'authenticated' : conclusion === 'inconclusive' ? 'request_failed' : 'rejected'
    data.checks[1].status = conclusion === 'both_authenticated' || conclusion === 'php_only_authenticated' ? 'authenticated' : conclusion === 'inconclusive' ? 'invalid_response' : 'rejected'
    diagnoseOkpayProvider.mockResolvedValue({ data })
    const wrapper = mountPanel()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="okpay-diagnostic-conclusion"]').text()).toContain(expected)
    wrapper.unmount()
  })

  it('执行期间禁用重复请求，切换实例后不接收旧响应或改变新请求的加载状态', async () => {
    let finishFirst!: (response: { data: OkpayDiagnosticResult }) => void
    let finishSecond!: (response: { data: OkpayDiagnosticResult }) => void
    diagnoseOkpayProvider.mockImplementationOnce(() => new Promise(resolve => { finishFirst = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { finishSecond = resolve }))
    const wrapper = mountPanel()
    await wrapper.get('button').trigger('click')
    await wrapper.get('button').trigger('click')
    expect(diagnoseOkpayProvider).toHaveBeenCalledTimes(1)
    expect(wrapper.get('button').attributes('disabled')).toBeDefined()
    await wrapper.setProps({ providerId: 43, providerName: '商户 43' })
    await wrapper.get('button').trigger('click')
    finishFirst({ data: resultFixture(42) })
    await flushPromises()
    expect(wrapper.find('[data-test="okpay-diagnostic-result"]').exists()).toBe(false)
    expect(wrapper.get('button').attributes('disabled')).toBeDefined()
    finishSecond({ data: resultFixture(43) })
    await flushPromises()
    expect(wrapper.text()).toContain('商户 43')
    expect(wrapper.text()).not.toContain('认证通过 42')
    expect(wrapper.get('button').attributes('disabled')).toBeUndefined()
    await wrapper.setProps({ providerId: 44, providerName: '商户 44' })
    expect(wrapper.find('[data-test="okpay-diagnostic-result"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('拒绝其他实例的响应并使用安全 API 错误提示，随后允许重试', async () => {
    diagnoseOkpayProvider.mockResolvedValueOnce({ data: resultFixture(99) })
      .mockRejectedValueOnce({ status: 403, message: '没有诊断权限' })
      .mockResolvedValueOnce({ data: resultFixture() })
    const wrapper = mountPanel()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('不匹配或格式无效')
    expect(wrapper.find('[data-test="okpay-diagnostic-result"]').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('没有诊断权限')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="okpay-diagnostic-result"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it.each([['okpay', 42, true], ['okpay', 0, false], ['usdt_trc20', 42, false]] as const)('卡片入口只提供给已保存 OKPay：%s / %s', (providerKey, id, visible) => {
    const provider: ProviderInstance = { id, provider_key: providerKey, name: '商户', config: {}, supported_types: [providerKey], enabled: false, payment_mode: 'redirect', refund_enabled: false, allow_user_refund: false, limits: '', sort_order: 0 }
    const wrapper = mount(ProviderCard, { props: { provider, enabled: false, availableTypes: [] }, global: { stubs: { Icon: true, ToggleSwitch: true } } })
    expect(wrapper.find('[data-test="diagnose-okpay"]').exists()).toBe(visible)
    if (visible) expect(wrapper.get('[data-test="diagnose-okpay"]').attributes('disabled')).toBeUndefined()
    expect(diagnoseOkpayProvider).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
