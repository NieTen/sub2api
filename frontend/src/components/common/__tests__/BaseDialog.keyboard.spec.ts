import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import BaseDialog from '../BaseDialog.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key === 'common.close' ? '关闭' : key }) }))

type Props = InstanceType<typeof BaseDialog>['$props']
const wrappers: VueWrapper[] = []

async function openDialog(title: string, props: Partial<Props> = {}, content?: string) {
  const wrapper = mount(BaseDialog, {
    attachTo: document.body,
    props: { show: true, title, trapFocus: true, showCloseButton: false, ...props },
    slots: { default: content ?? '<button data-first>第一个操作</button><button data-last>最后一个操作</button>' },
    global: { stubs: { Icon: true } }
  })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}

function dialog(title: string): HTMLElement {
  return Array.from(document.querySelectorAll<HTMLElement>('[role="dialog"]')).find(element => (
    document.getElementById(element.getAttribute('aria-labelledby') || '')?.textContent?.trim() === title
  ))!
}

function press(key: string, shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key, shiftKey, bubbles: true, cancelable: true })
  ;(document.activeElement || document).dispatchEvent(event)
  return event
}

describe('BaseDialog 多弹窗键盘与焦点', () => {
  beforeEach(() => {
    // jsdom 无布局，明确模拟可见元素以验证真实 Tab 处理与焦点移动。
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([{}] as unknown as DOMRectList)
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    document.body.innerHTML = ''
    document.body.classList.remove('modal-open')
    vi.restoreAllMocks()
  })

  it('同层仅最后一个 DOM 弹窗响应 Escape，关闭后下层恢复响应', async () => {
    const first = await openDialog('底层')
    const second = await openDialog('顶层')
    expect(press('Escape').defaultPrevented).toBe(true)
    expect(second.emitted('close')).toHaveLength(1)
    expect(first.emitted('close')).toBeUndefined()
    await second.setProps({ show: false })
    press('Escape')
    expect(first.emitted('close')).toHaveLength(1)
  })

  it('zIndex 优先于打开顺序，低层打开与关闭均不抢夺高层焦点', async () => {
    const high = await openDialog('高层', { zIndex: 80 })
    const highLast = dialog('高层').querySelector<HTMLButtonElement>('[data-last]')!
    highLast.focus()
    const low = await openDialog('后打开的低层', { zIndex: 50 })
    expect(document.activeElement).toBe(highLast)
    press('Escape')
    expect(high.emitted('close')).toHaveLength(1)
    expect(low.emitted('close')).toBeUndefined()
    await low.setProps({ show: false })
    expect(document.activeElement).toBe(highLast)
    await low.setProps({ show: true, zIndex: 90 })
    press('Escape')
    expect(low.emitted('close')).toHaveLength(1)
    expect(high.emitted('close')).toHaveLength(1)
  })

  it('同层 DOM 重排后按实际显示顺序判断，不使用旧打开顺序', async () => {
    const first = await openDialog('原底层')
    const second = await openDialog('原顶层')
    document.body.appendChild(dialog('原底层'))
    press('Escape')
    expect(first.emitted('close')).toHaveLength(1)
    expect(second.emitted('close')).toBeUndefined()
  })

  it('Tab 和 Shift+Tab 仅在顶层首尾循环，下层不会把中间焦点拉走', async () => {
    await openDialog('底层')
    await openDialog('顶层')
    const top = dialog('顶层')
    const first = top.querySelector<HTMLButtonElement>('[data-first]')!
    const last = top.querySelector<HTMLButtonElement>('[data-last]')!
    first.focus()
    expect(press('Tab').defaultPrevented).toBe(false)
    expect(document.activeElement).toBe(first)
    last.focus()
    expect(press('Tab').defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(first)
    expect(press('Tab', true).defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(last)
  })

  it('顶层禁止 Escape 时不会穿透关闭底层，也不会全局强制焦点锁', async () => {
    const bottom = await openDialog('底层')
    const top = await openDialog('禁止关闭', { closeOnEscape: false, trapFocus: false })
    press('Escape')
    expect(top.emitted('close')).toBeUndefined()
    expect(bottom.emitted('close')).toBeUndefined()
    const last = dialog('禁止关闭').querySelector<HTMLButtonElement>('[data-last]')!
    last.focus()
    expect(press('Tab').defaultPrevented).toBe(false)
    expect(document.activeElement).toBe(last)
  })

  it('顶层关闭后恢复下层触发按钮，最后关闭才返回页面按钮', async () => {
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    outside.focus()
    const bottom = await openDialog('底层')
    const trigger = dialog('底层').querySelector<HTMLButtonElement>('[data-last]')!
    trigger.focus()
    const top = await openDialog('顶层')
    await top.setProps({ show: false })
    expect(document.activeElement).toBe(trigger)
    expect(document.body.classList.contains('modal-open')).toBe(true)
    await bottom.setProps({ show: false })
    expect(document.activeElement).toBe(outside)
  })

  it('没有可聚焦控件时焦点留在顶层面板，不落到背景', async () => {
    await openDialog('只读说明', {}, '<p>没有操作按钮</p>')
    const panel = dialog('只读说明').querySelector<HTMLElement>('.modal-content')!
    expect(document.activeElement).toBe(panel)
    expect(press('Tab').defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(panel)
  })

  it('长标题仍关联到弹窗，关闭按钮有中文名称并独立触发关闭', async () => {
    const title = '很长的文件名'.repeat(50)
    const wrapper = await openDialog(title, { showCloseButton: true })
    const overlay = dialog(title)
    expect(overlay).toBeDefined()
    const close = overlay.querySelector<HTMLButtonElement>('button[aria-label="关闭"]')!
    expect(close.type).toBe('button')
    close.click()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
