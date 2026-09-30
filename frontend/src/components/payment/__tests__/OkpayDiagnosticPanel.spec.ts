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
      { mode: 'current', status: 'authenticated', reason: 'success', message: '当前请求认证通过 ' + id },
      { mode: 'php_reference', status: 'authenticated', reason: 'success', message: '兼容请求认证通过 ' + id },
      { mode: 'hmac_sha256', status: 'authenticated', reason: 'success', message: 'HMAC 请求认证通过 ' + id },
    ],
    conclusion,
  }
}

function mountPanel(id = 42) {
  return mount(OkpayDiagnosticPanel, { props: { providerId: id, providerName: '商户 ' + id }, global: { stubs: { Icon: true } } })
}

describe('已保存 OKPay 实例认证诊断', () => {
  beforeEach(() => diagnoseOkpayProvider.mockReset())

  it('用户主动点击后按顺序显示三种请求结果，只显示固定分类和安全状态代码', async () => {
    const response = resultFixture()
    response.checks[0] = { ...response.checks[0], status: 'rejected', reason: 'auth_failed', http_status: 200, business_code: '20001', message: '不应显示的原始 message' }
    diagnoseOkpayProvider.mockResolvedValue({ data: { ...response, checks: [...response.checks].reverse(), token: '不应显示的密钥', sign: '不应显示的签名', balance: '9876.54321', raw_response: '不应显示的上游正文' } })
    const wrapper = mountPanel()
    expect(diagnoseOkpayProvider).not.toHaveBeenCalled()
    expect(wrapper.find('input, textarea').exists()).toBe(false)
    expect(wrapper.text()).toContain('不创建充值订单')
    await wrapper.get('[data-test="diagnose-okpay"]').trigger('click')
    await flushPromises()
    expect(diagnoseOkpayProvider).toHaveBeenCalledTimes(1)
    expect(diagnoseOkpayProvider).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-test="okpay-check-current"]').text()).toContain('身份认证失败')
    expect(wrapper.get('[data-test="okpay-check-current"]').text()).toContain('HTTP 200')
    expect(wrapper.get('[data-test="okpay-check-current"]').text()).toContain('业务代码：20001')
    expect(wrapper.get('[data-test="okpay-check-php_reference"]').text()).toContain('Go 模拟 PHP')
    expect(wrapper.get('[data-test="okpay-check-hmac_sha256"]').text()).toContain('新版 HMAC-SHA256')
    expect(wrapper.findAll('[data-test^="okpay-check-"]').map(check => check.attributes('data-test'))).toEqual(['okpay-check-current', 'okpay-check-php_reference', 'okpay-check-hmac_sha256'])
    expect(wrapper.get('[data-test="okpay-diagnostic-conclusion"]').text()).toContain('不能证明创建支付订单正常')
    for (const hidden of ['不应显示的密钥', '不应显示的签名', '9876.54321', '不应显示的上游正文', '不应显示的原始 message']) expect(wrapper.text()).not.toContain(hidden)
    wrapper.unmount()
  })

  it.each([
    ['both_authenticated', '参与对照的请求均通过认证'],
    ['php_only_authenticated', 'Go 模拟的旧版 PHP 请求通过'],
    ['current_only_authenticated', '当前配置请求通过，Go 模拟的旧版 PHP 请求未通过'],
    ['hmac_only_authenticated', '新版 HMAC-SHA256 通过，旧版 MD5 被拒绝'],
    ['legacy_only_authenticated', '旧版 MD5 通过，新版 HMAC-SHA256 被拒绝'],
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

  it('兼容没有 reason 的旧版两组诊断，仍使用固定说明而不显示原始 message', async () => {
    const data = resultFixture()
    data.checks = data.checks.slice(0, 2).map(check => ({ ...check, reason: undefined, message: '旧响应不应显示的正文' }))
    diagnoseOkpayProvider.mockResolvedValue({ data })
    const wrapper = mountPanel()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-test^="okpay-check-"]')).toHaveLength(2)
    expect(wrapper.get('[data-test="okpay-check-current"]').text()).toContain('只读商户认证通过')
    expect(wrapper.text()).not.toContain('旧响应不应显示的正文')
    wrapper.unmount()
  })

  it.each([
    { field: 'business_code', value: 'token=private-secret' },
    { field: 'business_code', value: '123456789' },
    { field: 'business_code', value: '200\n' },
    { field: 'http_status', value: 200.5 },
    { field: 'http_status', value: 700 },
    { field: 'reason', value: 'private-secret' },
    { field: 'mode', value: 'private-secret' },
  ])('拒绝不在白名单或范围内的诊断字段 $field=$value', async ({ field, value }) => {
    const data = resultFixture()
    Object.assign(data.checks[0], { [field]: value })
    diagnoseOkpayProvider.mockResolvedValue({ data })
    const wrapper = mountPanel()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="okpay-diagnostic-result"]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('不匹配或格式无效')
    expect(wrapper.text()).not.toContain('private-secret')
    wrapper.unmount()
  })

  it.each(['重复模式', '超过三组', '缺少HMAC结果却返回HMAC结论'])('拒绝不完整或重复的协议对照：%s', async variant => {
    const data = resultFixture()
    if (variant === '重复模式') data.checks[2] = { ...data.checks[1] }
    if (variant === '超过三组') data.checks.push({ ...data.checks[2] })
    if (variant === '缺少HMAC结果却返回HMAC结论') {
      data.checks = data.checks.slice(0, 2)
      data.conclusion = 'hmac_only_authenticated'
    }
    diagnoseOkpayProvider.mockResolvedValue({ data })
    const wrapper = mountPanel()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="okpay-diagnostic-result"]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('不匹配或格式无效')
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
