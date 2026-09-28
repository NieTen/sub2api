import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import CommunityMessageMedia from '../CommunityMessageMedia.vue'
import { communityAPI, type CommunityChatMessage } from '@/api/community'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/client', () => ({ apiClient: {} }))
vi.mock('@/api/community', () => ({ communityAPI: { media: vi.fn() } }))
const message: CommunityChatMessage = { id: 7, telegram_message_id: 7, telegram_user_id: 123, telegram_name: '示例', telegram_username: '', sender_kind: 'user', message_type: 'photo', text: '图片说明', created_at: '', outgoing: false, media_available: true }
describe('群消息附件', () => {
  beforeEach(() => { vi.clearAllMocks(); vi.stubGlobal('URL', { createObjectURL: vi.fn(() => 'blob:local-image'), revokeObjectURL: vi.fn() }) })
  afterEach(() => { vi.unstubAllGlobals() })
  it.each([['image/png', 'img'], ['audio/ogg', 'audio'], ['video/mp4', 'video']])('%s 附件点击后加载，卸载释放本地地址', async (type, element) => {
    vi.mocked(communityAPI.media).mockResolvedValue(new Blob(['本地测试'], { type }))
    const wrapper = mount(CommunityMessageMedia, { props: { message, label: '附件' } })
    expect(communityAPI.media).not.toHaveBeenCalled()
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(communityAPI.media).toHaveBeenCalledWith(7)
    expect(wrapper.find(element).attributes('src')).toBe('blob:local-image')
    wrapper.unmount()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:local-image')
  })
  it('文档只提供下载，不嵌入执行 HTML', async () => {
    vi.mocked(communityAPI.media).mockResolvedValue(new Blob(['<script>alert(1)</script>'], { type: 'text/html' }))
    const wrapper = mount(CommunityMessageMedia, { props: { message: { ...message, message_type: 'document', file_name: '示例.html' }, label: '文档' } })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('a').attributes('download')).toBe('示例.html')
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('script').exists()).toBe(false)
    wrapper.unmount()
  })
  it.each(['voice', 'audio', 'document'])('application/ogg 的 %s 消息依据类型选择播放器或下载', async messageType => {
    vi.mocked(communityAPI.media).mockResolvedValue(new Blob(['OggS'], { type: 'application/ogg' }))
    const wrapper = mount(CommunityMessageMedia, { props: { message: { ...message, message_type: messageType }, label: 'Ogg 附件' } })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('audio').exists()).toBe(messageType !== 'document')
    expect(wrapper.find('a').attributes('href')).toBe('blob:local-image')
    wrapper.unmount()
  })
  it('无法下载时说明限制，失败时原样展示后端消息', async () => {
    const wrapper = mount(CommunityMessageMedia, { props: { message: { ...message, media_available: false }, label: '图片' } })
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).toContain('community.chat.mediaUnavailable')
    await wrapper.setProps({ message })
    vi.mocked(communityAPI.media).mockRejectedValue({ message: '附件已被移除' })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toBe('附件已被移除')
    wrapper.unmount()
  })
})
