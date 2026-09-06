import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SupportComposer from '../SupportComposer.vue'
import { supportAPI } from '@/api/support'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/support', () => ({
  supportAPI: { upload: vi.fn(), deleteAttachment: vi.fn() },
  supportError: (_error: unknown, fallback: string) => fallback
}))

const attachment = { id: 1, file_name: 'screenshot.png', mime_type: 'image/png', size: 3 }
const render = (attachments = [] as typeof attachment[]) => mount(SupportComposer, {
  props: { modelValue: '', attachments, admin: true },
  global: { stubs: { SupportImage: true } }
})

describe('工单图片编辑器', () => {
  beforeEach(() => vi.clearAllMocks())

  it('拒绝粘贴 SVG，不会上传可执行的图像内容', async () => {
    const wrapper = render()
    const file = new File(['<svg onload="alert(1)"/>'], 'image.svg', { type: 'image/svg+xml' })
    await wrapper.find('textarea').trigger('paste', { clipboardData: { items: [{ kind: 'file', type: file.type, getAsFile: () => file }] } })
    await flushPromises()
    expect(supportAPI.upload).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').text()).toBe('support.invalidImage')
    wrapper.unmount()
  })

  it('粘贴位图后通过管理员上传 API 添加附件', async () => {
    vi.mocked(supportAPI.upload).mockResolvedValue(attachment)
    const wrapper = render()
    const file = new File(['png'], 'image.png', { type: 'image/png' })
    await wrapper.find('textarea').trigger('paste', { clipboardData: { items: [{ kind: 'file', type: file.type, getAsFile: () => file }] } })
    await flushPromises()
    expect(supportAPI.upload).toHaveBeenCalledWith(true, file)
    expect(wrapper.emitted('update:attachments')?.[0]).toEqual([[attachment]])
    expect(wrapper.emitted('busy')).toEqual([[true], [false]])
    wrapper.unmount()
  })

  it('已有四张图片时拒绝继续粘贴', async () => {
    const wrapper = render([1, 2, 3, 4].map(id => ({ ...attachment, id })))
    const file = new File(['png'], 'image.png', { type: 'image/png' })
    await wrapper.find('textarea').trigger('paste', { clipboardData: { items: [{ kind: 'file', type: file.type, getAsFile: () => file }] } })
    expect(supportAPI.upload).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('support.tooManyImages')
    wrapper.unmount()
  })

  it('草稿图片在服务器删除成功后才从编辑器移除', async () => {
    let finish: (() => void) | undefined
    vi.mocked(supportAPI.deleteAttachment).mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const wrapper = render([attachment])
    await wrapper.find('button[aria-label^="support.removeImage"]').trigger('click')
    expect(supportAPI.deleteAttachment).toHaveBeenCalledWith(true, 1)
    expect(wrapper.emitted('update:attachments')).toBeUndefined()
    finish?.()
    await flushPromises()
    expect(wrapper.emitted('update:attachments')?.[0]).toEqual([[]])
    wrapper.unmount()
  })
})
