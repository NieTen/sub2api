import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { announcementsAPI } from '@/api'
import { useAnnouncementStore } from '@/stores/announcements'
import type { UserAnnouncement } from '@/types'

vi.mock('@/api', () => ({ announcementsAPI: { list: vi.fn(), markRead: vi.fn() } }))
const announcement: UserAnnouncement = { id: 1, title: '公告', content: '正文', notify_mode: 'popup', created_at: '', updated_at: '' }

describe('公告弹窗互斥状态', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    vi.clearAllMocks()
    vi.mocked(announcementsAPI.markRead).mockResolvedValue({ message: 'ok' })
  })
  afterEach(() => { vi.useRealTimers() })

  it('请求期间、公告队列间 300 毫秒和最后关闭动画期间持续阻塞其他弹窗', async () => {
    vi.mocked(announcementsAPI.list).mockResolvedValue([announcement, { ...announcement, id: 2 }])
    const store = useAnnouncementStore()
    const loading = store.fetchAnnouncements()
    expect(store.popupBlocking).toBe(true)
    await loading
    expect(store.currentPopup?.id).toBe(1)
    await store.dismissPopup()
    expect(store.currentPopup).toBeNull()
    expect(store.popupBlocking).toBe(true)
    await vi.advanceTimersByTimeAsync(300)
    expect(store.currentPopup?.id).toBe(2)
    await store.dismissPopup()
    expect(store.popupBlocking).toBe(true)
    await vi.advanceTimersByTimeAsync(300)
    expect(store.popupBlocking).toBe(false)
  })

  it('退出后清除待展示定时器，旧账号请求不能恢复公告', async () => {
    let finish: ((value: UserAnnouncement[]) => void) | undefined
    vi.mocked(announcementsAPI.list).mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const store = useAnnouncementStore()
    const loading = store.fetchAnnouncements()
    store.reset()
    finish?.([announcement])
    await loading
    await vi.advanceTimersByTimeAsync(300)
    expect(store.currentPopup).toBeNull()
    expect(store.popupBlocking).toBe(false)
  })
})
