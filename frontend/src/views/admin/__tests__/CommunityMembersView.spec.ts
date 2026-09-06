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
vi.mock('@/api/community', () => ({ communityAPI: { members: vi.fn() } }))

const data: CommunityMembersPage = {
  items: [
    { user_id: 1, email: 'new@example.com', username: 'new-user', user_status: 'active', telegram_name: '', telegram_username: '', status: 'not_joined' },
    { user_id: 2, email: 'joined@example.com', username: 'joined-user', user_status: 'disabled', telegram_user_id: 123456789, telegram_name: '<img src=x>', telegram_username: 'telegram_user', status: 'joined', joined_at: '2026-09-05T13:00:00Z' }
  ],
  total: 70, page: 1, page_size: 20,
  summary: { total: 70, joined: 10, not_joined: 60, pending: 5, left: 2 }
}
const render = () => mount(CommunityMembersView, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })

describe('社群成员列表', () => {
  beforeEach(() => { vi.clearAllMocks(); vi.useFakeTimers(); vi.mocked(communityAPI.members).mockResolvedValue(data) })
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

  it.each(['joined', 'not_joined', 'pending', 'left'])('分页后切换 %s 筛选会回到第一页', async status => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('[data-test="next-page"]').trigger('click')
    await flushPromises()
    expect(communityAPI.members).toHaveBeenLastCalledWith(2, '', 'all')
    await wrapper.find('select[name="community-member-status"]').setValue(status)
    await flushPromises()
    expect(communityAPI.members).toHaveBeenLastCalledWith(1, '', status)
    expect(wrapper.find('[data-test="summary-total"]').text()).toContain('70')
    wrapper.unmount()
  })

  it('搜索回到第一页并保留入群筛选，统计采用搜索后的服务器结果', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.find('select[name="community-member-status"]').setValue('not_joined')
    await flushPromises()
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
})
