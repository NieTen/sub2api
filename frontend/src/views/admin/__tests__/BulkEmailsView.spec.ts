import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, reactive } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import BulkEmailsView from '../BulkEmailsView.vue'
import { bulkEmailAPI, type BulkEmailBatch, type BulkEmailRecipientFilter } from '@/api/support'
import { list as listUsers } from '@/api/admin/users'
import type { AdminUser } from '@/types'

const state = reactive({ params: {} as Record<string, string> })
const push = vi.fn(async (path: string) => { state.params = path.match(/\/(\d+)$/) ? { id: path.split('/').pop()! } : {} })
vi.mock('vue-router', () => ({ useRoute: () => state, useRouter: () => ({ push }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key, te: () => true, locale: { value: 'zh' } }) }))
const alerts = vi.hoisted(() => ({ showSuccess: vi.fn(), showError: vi.fn() }))
vi.mock('@/stores/app', () => ({ useAppStore: () => alerts }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/common/Pagination.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/api/admin/users', () => ({ list: vi.fn(async () => ({ items: [], total: 0 })) }))
vi.mock('@/api/support', () => ({
  bulkEmailAPI: { list: vi.fn(), create: vi.fn(), detail: vi.fn(), start: vi.fn(), retry: vi.fn(), pause: vi.fn(), resume: vi.fn() },
  supportError: (_error: unknown, fallback: string) => fallback
}))

const draft = { id: 7, subject: '<script>test</script>', body: '<img src=x onerror=alert(1)>', status: 'draft', total_count: 12, sent_count: 0, failed_count: 0, created_at: '2026-09-05T12:00:00Z' }
const pageOf = <T,>(items: T[]) => ({ items, total: items.length, page: 1, page_size: 20 })
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

  it.each([
    { status: 'needs_review', failed: 0, retryable: false },
    { status: 'needs_review', failed: 2, retryable: true },
    { status: 'sending', failed: 2, retryable: false }
  ])('$status 批次有 $failed 次确认失败时准确显示进度和重试入口', async ({ status, failed, retryable }) => {
    const batch = { ...draft, status, total_count: status === 'sending' ? 12 : 6 + failed, sent_count: 3, failed_count: failed, unconfirmed_count: 3 }
    const recipient = { id: 1, user_id: 9, email: 'recipient@example.com', status: 'unconfirmed', attempts: 258, last_error: '' }
    vi.mocked(bulkEmailAPI.list).mockResolvedValue({ items: [batch], total: 1, page: 1, page_size: 20 })
    vi.mocked(bulkEmailAPI.detail).mockResolvedValue({ batch, recipients: { items: [recipient], total: batch.total_count, page: 1, page_size: 20 } })
    state.params = { id: '7' }

    const wrapper = render()
    await flushPromises()
    const row = wrapper.get('tbody tr')
    expect(row.text()).toContain('bulkMail.reviewDeliveryHint')
    expect(row.text()).toContain(`${6 + failed} / ${batch.total_count}`)
    expect(row.get('progress').element.value).toBe(6 + failed)
    const dialog = wrapper.get('[data-test="dialog"]')
    expect(dialog.text()).toContain('bulkMail.reviewDeliveryHint')
    expect(dialog.get('tbody tr').text()).toContain('bulkMail.statuses.unconfirmed')
    const retry = dialog.findAll('button').find(button => button.text() === 'bulkMail.retry')
    expect(Boolean(retry)).toBe(retryable)
    if (retry) {
      await retry.trigger('click')
      await flushPromises()
      expect(bulkEmailAPI.retry).toHaveBeenCalledWith(7)
    }
    wrapper.unmount()
  })

  it('列表和详情按批次执行暂停和继续，防止重复提交', async () => {
    const batch = { ...draft, status: 'queued' }
    const completed = { ...draft, id: 8, status: 'completed' }
    vi.mocked(bulkEmailAPI.list).mockImplementation(async () => pageOf([{ ...batch }, completed]))
    vi.mocked(bulkEmailAPI.detail).mockImplementation(async () => ({ batch: { ...batch }, recipients: pageOf([]) }))
    let confirmPause!: () => void
    vi.mocked(bulkEmailAPI.pause).mockImplementationOnce(() => new Promise<void>(resolve => {
      confirmPause = () => { batch.status = 'paused'; resolve() }
    }))
    vi.mocked(bulkEmailAPI.resume).mockImplementationOnce(async () => { batch.status = 'queued' })
    state.params = { id: '7' }
    const wrapper = render()
    await flushPromises()
    const dialog = wrapper.get('[data-test="dialog"]')
    const rows = wrapper.findAll('section table')[0].findAll('tbody tr')
    expect(rows[1].find('button').exists()).toBe(false)
    const pause = rows[0].get('button')
    const pauseInDialog = dialog.findAll('button').find(b => b.text() === 'bulkMail.pause')!
    expect(pause.text()).toBe('bulkMail.pause')
    await Promise.all([pause.trigger('click'), pauseInDialog.trigger('click')])
    expect(bulkEmailAPI.pause).toHaveBeenCalledTimes(1)
    expect(bulkEmailAPI.pause).toHaveBeenCalledWith(7)
    expect(pauseInDialog.attributes('disabled')).toBeDefined()
    confirmPause()
    await flushPromises()
    expect(rows[0].get('button').text()).toBe('bulkMail.resume')
    expect(dialog.text()).toContain('bulkMail.statuses.paused')
    await dialog.findAll('button').find(b => b.text() === 'bulkMail.resume')!.trigger('click')
    await flushPromises()
    expect(bulkEmailAPI.resume).toHaveBeenCalledWith(7)
    expect(rows[0].get('button').text()).toBe('bulkMail.pause')
    wrapper.unmount()
  })

  it('暂停请求失败后获取最新列表，切换的详情保持当前批次', async () => {
    const batches = [{ ...draft, status: 'sending' }, { ...draft, id: 8, subject: '另一批邮件', status: 'queued' }]
    vi.mocked(bulkEmailAPI.list).mockImplementation(async () => pageOf(batches.map(b => ({ ...b }))))
    vi.mocked(bulkEmailAPI.detail).mockImplementation(async id => ({ batch: { ...batches.find(b => b.id === id)! }, recipients: pageOf([]) }))
    let rejectPause!: (reason: Error) => void
    vi.mocked(bulkEmailAPI.pause).mockImplementationOnce(() => new Promise<void>((_, reject) => { rejectPause = reject }))
    state.params = { id: '7' }
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-test="dialog"]').findAll('button').find(b => b.text() === 'bulkMail.pause')!.trigger('click')
    state.params = { id: '8' }
    await flushPromises()
    batches[0].status = 'completed'
    rejectPause(new Error('批次已完成'))
    await flushPromises()
    expect(alerts.showError).toHaveBeenCalledTimes(1)
    expect(wrapper.get('tbody tr').text()).toContain('bulkMail.statuses.completed')
    expect(wrapper.get('[data-test="dialog"]').text()).toContain('另一批邮件')
    expect(bulkEmailAPI.detail).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('暂停和详情刷新暂时失败后仍可按原批次再次暂停', async () => {
    const detail = { batch: { ...draft, status: 'sending' }, recipients: pageOf([]) }
    vi.mocked(bulkEmailAPI.detail).mockResolvedValueOnce(detail).mockRejectedValueOnce(new Error('刷新失败'))
      .mockResolvedValue({ ...detail, batch: { ...draft, status: 'paused' } })
    vi.mocked(bulkEmailAPI.pause).mockRejectedValueOnce(new Error('请求中断')).mockResolvedValueOnce()
    state.params = { id: '7' }
    const wrapper = render()
    await flushPromises()
    const dialog = wrapper.get('[data-test="dialog"]')
    const pause = () => dialog.findAll('button').find(b => b.text() === 'bulkMail.pause')!
    await pause().trigger('click')
    await flushPromises()
    expect(dialog.find('[role="alert"]').exists()).toBe(true)
    expect(pause().attributes('disabled')).toBeUndefined()
    await pause().trigger('click')
    await flushPromises()
    expect(bulkEmailAPI.pause).toHaveBeenCalledTimes(2)
    expect(bulkEmailAPI.pause).toHaveBeenLastCalledWith(7)
    expect(dialog.text()).toContain('bulkMail.statuses.paused')
    wrapper.unmount()
  })

  it('暂停期间继续轮询回执以及其他标签页的恢复操作', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    const visible = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    const batch: BulkEmailBatch = { ...draft, status: 'paused' }
    vi.mocked(bulkEmailAPI.list).mockImplementation(async () => pageOf([{ ...batch }]))
    vi.mocked(bulkEmailAPI.detail).mockImplementation(async () => ({ batch: { ...batch }, recipients: pageOf([]) }))
    state.params = { id: '7' }
    const wrapper = render()
    try {
      await flushPromises()
      batch.sent_count = 1
      await vi.advanceTimersByTimeAsync(5000)
      expect(wrapper.get('progress').element.value).toBe(1)
      batch.status = 'sending'
      await vi.advanceTimersByTimeAsync(5000)
      expect(wrapper.get('tbody tr button').text()).toBe('bulkMail.pause')
      expect(wrapper.get('[data-test="dialog"]').text()).toContain('bulkMail.statuses.sending')
    } finally {
      wrapper.unmount()
      visible.mockRestore()
      vi.useRealTimers()
    }
  })
})
