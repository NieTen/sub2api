import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, reactive } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import BulkEmailsView from '../BulkEmailsView.vue'
import { bulkEmailAPI, type BulkEmailRecipientFilter } from '@/api/support'
import { list as listUsers } from '@/api/admin/users'
import type { AdminUser } from '@/types'

const state = reactive({ params: {} as Record<string, string> })
const push = vi.fn(async (path: string) => { state.params = path.match(/\/(\d+)$/) ? { id: path.split('/').pop()! } : {} })
vi.mock('vue-router', () => ({ useRoute: () => state, useRouter: () => ({ push }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key, te: () => true, locale: { value: 'zh' } }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/common/Pagination.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/api/admin/users', () => ({ list: vi.fn(async () => ({ items: [], total: 0 })) }))
vi.mock('@/api/support', () => ({
  bulkEmailAPI: { list: vi.fn(), create: vi.fn(), detail: vi.fn(), start: vi.fn(), retry: vi.fn() },
  supportError: (_error: unknown, fallback: string) => fallback
}))

const draft = { id: 7, subject: '<script>test</script>', body: '<img src=x onerror=alert(1)>', status: 'draft', total_count: 12, sent_count: 0, failed_count: 0, created_at: '2026-09-05T12:00:00Z' }
const Composer = defineComponent({
  name: 'SupportComposer', props: ['modelValue', 'attachments'], emits: ['update:modelValue', 'update:attachments'],
  template: '<div><textarea data-test="body" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" /><button type="button" data-test="image" @click="$emit(\'update:attachments\', [{ id: 8, file_name: \'x.png\', mime_type: \'image/png\', size: 3 }])">图片</button></div>'
})
const render = () => mount(BulkEmailsView, {
  global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    BaseDialog: { props: ['show'], template: '<section v-if="show" data-test="dialog"><slot /><slot name="footer" /></section>' },
    RouterLink: { template: '<a><slot /></a>' }, SupportComposer: Composer, SupportImage: true, Pagination: true
  } }
})

async function compose(allActive = true) {
  const wrapper = render()
  await flushPromises()
  await wrapper.findAll('button').find(button => button.text() === 'bulkMail.new')!.trigger('click')
  await wrapper.find('input[maxlength="200"]').setValue('账户通知')
  await wrapper.find('[data-test="body"]').setValue('请查看您的账户。')
  if (allActive) await wrapper.findAll('input[type="radio"]')[1].setValue(true)
  await flushPromises()
  return wrapper
}

describe('批量邮件确认流程', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.params = {}
    vi.mocked(bulkEmailAPI.list).mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    vi.mocked(bulkEmailAPI.create).mockResolvedValue(draft)
    vi.mocked(bulkEmailAPI.detail).mockResolvedValue({ batch: draft, recipients: { items: [], total: 12, page: 1, page_size: 20 } })
    vi.mocked(bulkEmailAPI.start).mockResolvedValue({ ...draft, status: 'queued' })
    vi.mocked(listUsers).mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
  })

  it('创建草稿不发送，明确确认后才调用 start，并且预览不会执行 HTML', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'bulkMail.new')!.trigger('click')
    await wrapper.find('input[maxlength="200"]').setValue(draft.subject)
    await wrapper.find('[data-test="body"]').setValue(draft.body)
    await wrapper.findAll('input[type="radio"]')[1].setValue(true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(bulkEmailAPI.create).toHaveBeenCalledWith({ subject: draft.subject, body: draft.body, all_active: true, user_ids: [], attachment_ids: [], recipient_filter: { balance_condition: 'all', recharge_condition: 'all' } })
    expect(bulkEmailAPI.start).not.toHaveBeenCalled()
    const dialog = wrapper.find('[data-test="dialog"]')
    expect(dialog.text()).toContain('12')
    expect(dialog.find('script').exists()).toBe(false)
    expect(dialog.find('img').exists()).toBe(false)
    await dialog.findAll('button').find(button => button.text() === 'bulkMail.send')!.trigger('click')
    await flushPromises()
    expect(bulkEmailAPI.start).toHaveBeenCalledTimes(1)
    expect(bulkEmailAPI.start).toHaveBeenCalledWith(7)
    wrapper.unmount()
  })

  it('允许只附图片的邮件进入草稿审核', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'bulkMail.new')!.trigger('click')
    await wrapper.find('input[maxlength="200"]').setValue('图片通知')
    await wrapper.find('[data-test="image"]').trigger('click')
    await wrapper.findAll('input[type="radio"]')[1].setValue(true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(bulkEmailAPI.create).toHaveBeenCalledWith(expect.objectContaining({ body: '', attachment_ids: [8] }))
    expect(bulkEmailAPI.start).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([
    { name: '有余额', balance: 'positive', recharge: 'all', amount: undefined },
    { name: '没余额（包含欠费）', balance: 'non_positive', recharge: 'all', amount: undefined },
    { name: '成功支付充值过', balance: 'all', recharge: 'recharged', amount: undefined },
    { name: '余额严格大于金额', balance: 'greater_than', recharge: 'all', amount: '100.00000001' }
  ])('$name 条件进入草稿请求', async ({ balance, recharge, amount }) => {
    const wrapper = await compose()
    await wrapper.find('select[name="balance-condition"]').setValue(balance)
    await wrapper.find('select[name="recharge-condition"]').setValue(recharge)
    if (amount) await wrapper.find('input[name="balance-threshold"]').setValue(amount)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(bulkEmailAPI.create).toHaveBeenCalledWith(expect.objectContaining({
      all_active: true,
      recipient_filter: { balance_condition: balance, recharge_condition: recharge, ...(amount ? { balance_threshold: amount } : {}) }
    }))
    expect(bulkEmailAPI.start).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('指定用户可组合余额和支付充值条件，按原始字符串提交高精度金额', async () => {
    vi.mocked(listUsers).mockResolvedValue({
      items: [{ id: 19, email: 'test@example.com', username: 'test', balance: 100, total_recharged: 999 } as AdminUser],
      total: 1, page: 1, page_size: 20, pages: 1
    })
    const wrapper = await compose(false)
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.find('select[name="balance-condition"]').setValue('greater_than')
    await wrapper.find('input[name="balance-threshold"]').setValue('999999999999.12345678')
    await wrapper.find('select[name="recharge-condition"]').setValue('recharged')
    expect(wrapper.text()).not.toContain('999')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(bulkEmailAPI.create).toHaveBeenCalledWith(expect.objectContaining({
      all_active: false,
      user_ids: [19],
      recipient_filter: { balance_condition: 'greater_than', balance_threshold: '999999999999.12345678', recharge_condition: 'recharged' }
    }))
    wrapper.unmount()
  })

  it.each(['0', '000000000001.50000000'])('合法阈值 %s 不经浮点数转换', async (amount) => {
    const wrapper = await compose()
    await wrapper.find('select[name="balance-condition"]').setValue('greater_than')
    await wrapper.find('input[name="balance-threshold"]').setValue(amount)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(bulkEmailAPI.create).toHaveBeenCalledWith(expect.objectContaining({
      recipient_filter: { balance_condition: 'greater_than', balance_threshold: amount, recharge_condition: 'all' }
    }))
    wrapper.unmount()
  })

  it.each(['', ' ', '-1', '-0', ' 1', '1 ', '.1', '1.', '1e3', 'NaN', '1.123456789', '1000000000000', '+1'])('非法阈值 %s 阻止创建草稿', async (amount) => {
    const wrapper = await compose()
    await wrapper.find('select[name="balance-condition"]').setValue('greater_than')
    await wrapper.find('input[name="balance-threshold"]').setValue(amount)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(bulkEmailAPI.create).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="filter-error"]').text()).toContain('bulkMail.filters.invalidAmount')
    expect(wrapper.find('[data-test="filter-error"]').text()).toContain('bulkMail.filters.balance')
    wrapper.unmount()
  })

  it.each(['all', 'positive', 'non_positive'])('切换到 %s 后不发送过时金额，也不受过时非法金额阻塞', async (condition) => {
    const wrapper = await compose()
    await wrapper.find('select[name="balance-condition"]').setValue('greater_than')
    await wrapper.find('input[name="balance-threshold"]').setValue('-3')
    await wrapper.find('select[name="balance-condition"]').setValue(condition)
    expect(wrapper.find('input[name="balance-threshold"]').exists()).toBe(false)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(bulkEmailAPI.create).toHaveBeenCalledWith(expect.objectContaining({
      recipient_filter: { balance_condition: condition, recharge_condition: 'all' }
    }))
    wrapper.unmount()
  })

  it.each(['draft', 'completed'])('%s 详情展示已保存条件及金额原文', async (status) => {
    const filter: BulkEmailRecipientFilter = { balance_condition: 'greater_than', balance_threshold: '123456789012.12345678', recharge_condition: 'recharged' }
    vi.mocked(bulkEmailAPI.detail).mockResolvedValue({ batch: { ...draft, status, recipient_filter: filter }, recipients: { items: [], total: 12, page: 1, page_size: 20 } })
    state.params = { id: '7' }
    const wrapper = render()
    await flushPromises()
    const summary = wrapper.find('[data-test="recipient-filter-summary"]').text()
    expect(summary).toContain('bulkMail.filters.balanceGreater')
    expect(summary).toContain('123456789012.12345678')
    expect(summary).toContain('bulkMail.filters.and')
    expect(summary).toContain('bulkMail.filters.rechargedSummary')
    wrapper.unmount()
  })

  it.each([undefined, null, {}])('旧批次缺少筛选条件时显示不限', async (filter) => {
    vi.mocked(bulkEmailAPI.detail).mockResolvedValue({ batch: { ...draft, recipient_filter: filter as BulkEmailRecipientFilter | null | undefined }, recipients: { items: [], total: 12, page: 1, page_size: 20 } })
    state.params = { id: '7' }
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="recipient-filter-summary"]').text()).toBe('bulkMail.filters.unrestricted')
    wrapper.unmount()
  })
})
