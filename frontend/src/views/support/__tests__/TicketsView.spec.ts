import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import TicketsView from '../TicketsView.vue'
import type { SupportTicketDetail } from '@/api/support'

const { detailMock, listMock } = vi.hoisted(() => ({ detailMock: vi.fn(), listMock: vi.fn() }))
const route = reactive({ params: { id: '1' } })
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/support/CommunicationsLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/api/support', () => ({
  supportAPI: { detail: detailMock, list: listMock },
  supportError: (_cause: unknown, fallback: string) => fallback
}))

const wrappers: VueWrapper[] = []
let originalScroll: PropertyDescriptor | undefined
const scrollMock = vi.fn()

function pendingDetail() {
  let resolve!: (value: SupportTicketDetail) => void
  const promise = new Promise<SupportTicketDetail>(complete => { resolve = complete })
  return { promise, resolve }
}

function response(id: number, subject = `工单 ${id}`): SupportTicketDetail {
  const time = '2026-10-03T12:00:00Z'
  return {
    ticket: { id, subject, user_id: 1, status: 'open', user_email: 'test@example.test', username: '测试用户', created_at: time, updated_at: time, last_message_at: time },
    messages: [{ id: 1, ticket_id: id, sender_id: 1, sender_role: 'user', source: 'web', content: '合成历史消息', created_at: time, attachments: [] }],
    has_more: false
  }
}

function render() {
  const wrapper = mount(TicketsView, {
    props: { admin: true },
    global: { stubs: {
      AppLayout: { template: '<main><slot /></main>' },
      CommunicationsLayout: { template: '<main><slot /></main>' },
      BaseDialog: { template: '<div />' },
      SupportComposer: { template: '<textarea />' },
      SupportImage: true,
      RouterLink: { template: '<a><slot /></a>' }
    } }
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('工单会话视口对齐', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    route.params.id = '1'
    listMock.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    originalScroll = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: scrollMock })
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    if (originalScroll) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', originalScroll)
    else Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView')
    vi.useRealTimers()
  })

  it('详情尚未返回时不滚动，渲染消息与回复区后才对齐会话', async () => {
    const pending = pendingDetail()
    detailMock.mockReturnValueOnce(pending.promise)
    const wrapper = render()
    await flushPromises()
    expect(scrollMock).not.toHaveBeenCalled()
    pending.resolve(response(1))
    await flushPromises()
    expect(wrapper.text()).toContain('合成历史消息')
    expect(wrapper.find('textarea').exists()).toBe(true)
    expect(scrollMock).toHaveBeenCalledTimes(1)
    expect(scrollMock).toHaveBeenLastCalledWith({ block: 'start', behavior: 'auto' })
  })

  it('快速切换 A→B→A 后迟到的旧响应不能滚动新会话', async () => {
    const firstA = pendingDetail()
    const secondB = pendingDetail()
    const latestA = pendingDetail()
    detailMock.mockReturnValueOnce(firstA.promise).mockReturnValueOnce(secondB.promise).mockReturnValueOnce(latestA.promise)
    const wrapper = render()
    await nextTick()
    route.params.id = '2'
    await nextTick()
    route.params.id = '1'
    await nextTick()
    firstA.resolve(response(1, '过期 A'))
    secondB.resolve(response(2, '过期 B'))
    await flushPromises()
    expect(scrollMock).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('过期 A')
    expect(wrapper.text()).not.toContain('过期 B')
    latestA.resolve(response(1, '当前 A'))
    await flushPromises()
    expect(wrapper.text()).toContain('当前 A')
    expect(scrollMock).toHaveBeenCalledTimes(1)
  })

  it('卸载后返回的详情不触发页面滚动', async () => {
    const pending = pendingDetail()
    detailMock.mockReturnValueOnce(pending.promise)
    const wrapper = render()
    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    pending.resolve(response(1))
    await flushPromises()
    expect(scrollMock).not.toHaveBeenCalled()
  })
})
