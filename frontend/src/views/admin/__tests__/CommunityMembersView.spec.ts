import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import CommunityMembersView from '../CommunityMembersView.vue'
import { communityAPI, type CommunityMembersPage } from '@/api/community'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/layout/TablePageLayout.vue', () => ({ default: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' } }))
vi.mock('@/components/common/DataTable.vue', () => ({ default: {
  props: ['data', 'columns', 'loading'],
  template: `<table><tbody><tr v-for="row in data" :key="row.user_id"><td v-for="column in columns" :key="column.key"><slot :name="'cell-' + column.key" :row="row" :value="row[column.key]">{{ row[column.key] }}</slot></td></tr></tbody></table>`
} }))
vi.mock('@/components/common/Pagination.vue', () => ({ default: {
  props: ['page', 'total'], emits: ['update:page'],
  template: `<button data-test="next-page" @click="$emit('update:page', page + 1)">Next</button>`
} }))
vi.mock('@/api/support', () => ({ supportError: (_error: unknown, fallback: string) => fallback }))
vi.mock('@/api/community', () => ({ communityAPI: { members: vi.fn(), telegramUser: vi.fn(), avatar: vi.fn() } }))

const data: CommunityMembersPage = {
  items: [
    { user_id: 1, email: 'new@example.com', username: 'new-user', user_status: 'active', telegram_name: '', telegram_username: '', status: 'not_joined' },
    { user_id: 2, email: 'joined@example.com', username: 'joined-user', user_status: 'disabled', telegram_user_id: 123456789, telegram_name: '<img src=x>', telegram_username: 'telegram_user', status: 'joined', joined_at: '2026-09-05T13:00:00Z' }
  ],
  total: 70, page: 1, page_size: 20,
  summary: { total: 70, joined: 10, not_joined: 60, pending: 5, left: 2 }
}
const render = () => mount(CommunityMembersView, { attachTo: document.body, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
const selectStatus = async (wrapper: ReturnType<typeof render>, status: string) => {
  await wrapper.get('#community-member-status').trigger('click')
  const option = wrapper.findAll('[role="option"]').find(item => item.text() === 'community.members.' + status)
  expect(option).toBeDefined()
  await option!.trigger('click')
  await flushPromises()
}

describe('社群成员列表', () => {
  beforeEach(() => { vi.clearAllMocks(); vi.useFakeTimers(); vi.mocked(communityAPI.members).mockResolvedValue(data); vi.mocked(communityAPI.avatar).mockRejectedValue({ status: 404 }) })
  afterEach(() => { vi.useRealTimers() })

  it('展示未入群网站用户与实际 Telegram 身份，统计使用搜索全集而非当前页', async () => {
    const wrapper = render()
    await flushPromises()
    expect(communityAPI.members).toHaveBeenCalledWith(1, '', 'all')
    expect(wrapper.text()).toContain('new@example.com')
    expect(wrapper.text()).toContain('community.members.notBound')
    expect(wrapper.text()).toContain('123456789')
    expect(wrapper.text()).toContain('@telegram_user')
    expect(wrapper.text()).toContain('community.members.disabled')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[data-test="summary-total"]').text()).toContain('70')
    expect(wrapper.find('[data-test="summary-not_joined"]').text()).toContain('60')
    wrapper.unmount()
  })

  it.each(['joined', 'not_joined', 'pending', 'left', 'banned'])('分页后切换 %s 筛选会回到第一页', async status => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('[data-test="next-page"]').trigger('click')
    await flushPromises()
    expect(communityAPI.members).toHaveBeenLastCalledWith(2, '', 'all')
    await selectStatus(wrapper, status)
    expect(communityAPI.members).toHaveBeenLastCalledWith(1, '', status)
    expect(wrapper.find('[data-test="summary-total"]').text()).toContain('70')
    wrapper.unmount()
  })

  it('搜索回到第一页并保留入群筛选，统计采用搜索后的服务器结果', async () => {
    const wrapper = render()
    await flushPromises()
    await selectStatus(wrapper, 'not_joined')
    await wrapper.find('[data-test="next-page"]').trigger('click')
    await flushPromises()
    vi.mocked(communityAPI.members).mockResolvedValue({ ...data, items: [], total: 2, summary: { total: 3, joined: 1, not_joined: 2, pending: 1, left: 0 } })
    await wrapper.find('input[name="community-member-search"]').setValue(' Alice ')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(communityAPI.members).toHaveBeenLastCalledWith(1, 'Alice', 'not_joined')
    expect(wrapper.find('[data-test="summary-total"]').text()).toContain('3')
    expect(wrapper.find('[data-test="summary-joined"]').text()).toContain('1')
    wrapper.unmount()
  })

  it('点击已入群统计可直接筛选，卸载后不执行待触发搜索', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('[data-test="summary-joined"]').trigger('click')
    await flushPromises()
    expect(communityAPI.members).toHaveBeenLastCalledWith(1, '', 'joined')
    await wrapper.find('input[name="community-member-search"]').setValue('pending')
    wrapper.unmount()
    const count = vi.mocked(communityAPI.members).mock.calls.length
    await vi.advanceTimersByTimeAsync(500)
    expect(communityAPI.members).toHaveBeenCalledTimes(count)
  })

  it('网站与 Telegram 头像都能打开详情，Telegram 详情使用最新绑定', async () => {
    const identity = { telegram_user_id: 123456789, telegram_name: 'Telegram 用户', telegram_username: 'telegram_user', banned: false, member: data.items[1] }
    vi.mocked(communityAPI.telegramUser).mockResolvedValue(identity)
    const wrapper = render()
    await flushPromises()
    const avatars = wrapper.findAll('button[aria-label="community.members.openDetail"]')
    expect(avatars).toHaveLength(3)
    await avatars[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').text()).toContain('new@example.com')
    expect(communityAPI.telegramUser).not.toHaveBeenCalled()
    await wrapper.find('[data-test="close-member-detail"]').trigger('click')
    await avatars[2].trigger('click')
    await flushPromises()
    expect(communityAPI.telegramUser).toHaveBeenCalledWith(123456789)
    expect(wrapper.find('[role="dialog"]').text()).toContain('joined@example.com')
    wrapper.unmount()
  })

  it('清除搜索保留当前状态并取消待执行搜索，重置恢复全部成员', async () => {
    const wrapper = render()
    await flushPromises()
    await selectStatus(wrapper, 'joined')
    await wrapper.get('input[name="community-member-search"]').setValue('尚未提交的搜索')
    await wrapper.get('[data-test="clear-member-search"]').trigger('click')
    await flushPromises()
    expect(communityAPI.members).toHaveBeenLastCalledWith(1, '', 'joined')
    expect(document.activeElement).toBe(wrapper.get('input[name="community-member-search"]').element)
    const count = vi.mocked(communityAPI.members).mock.calls.length
    await vi.advanceTimersByTimeAsync(400)
    expect(communityAPI.members).toHaveBeenCalledTimes(count)
    await wrapper.get('[data-test="reset-member-filters"]').trigger('click')
    await flushPromises()
    expect(communityAPI.members).toHaveBeenLastCalledWith(1, '', 'all')
    expect(wrapper.get('#community-member-status').text()).toContain('community.members.all')
    expect(wrapper.get('[data-test="reset-member-filters"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('点击账户名称打开现有详情，不依赖头像入口', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-test="member-detail-1"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('new@example.com')
    expect(communityAPI.telegramUser).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
