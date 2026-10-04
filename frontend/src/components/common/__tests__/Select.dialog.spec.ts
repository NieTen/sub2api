import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import BaseDialog from '../BaseDialog.vue'
import Select from '../Select.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key === 'common.close' ? '关闭' : key === 'common.clear' ? '清除' : key })
}))

const wrappers: VueWrapper[] = []

async function openDialogSelect(searchable: boolean) {
  const onClose = vi.fn()
  const value = ref<string | number | boolean | null>('alpha')
  const wrapper = mount(defineComponent({
    setup: () => () => h(BaseDialog, {
      show: true,
      title: '选择供应商',
      trapFocus: true,
      showCloseButton: false,
      onClose
    }, {
      default: () => [
        h('button', { type: 'button', 'data-first': '' }, '前一个操作'),
        h(Select, {
          modelValue: value.value,
          'onUpdate:modelValue': (nextValue: string | number | boolean | null) => { value.value = nextValue },
          ariaLabel: '供应商',
          clearable: true,
          searchable,
          options: [{ value: 'alpha', label: '供应商甲' }, { value: 'beta', label: '供应商乙' }]
        }),
        h('button', { type: 'button', 'data-last': '' }, '后一个操作')
      ]
    })
  // 仅跳过过渡动画，保留真实 Teleport、Select 与 BaseDialog 键盘处理。
  }), { attachTo: document.body, global: { stubs: { Icon: true, transition: true } } })
  wrappers.push(wrapper)
  await flushPromises()

  const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
  const trigger = dialog.querySelector<HTMLButtonElement>('.select-trigger')!
  const first = dialog.querySelector<HTMLButtonElement>('[data-first]')!
  const last = dialog.querySelector<HTMLButtonElement>('[data-last]')!
  trigger.focus()
  trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true }))
  await flushPromises()

  const dropdown = document.querySelector<HTMLElement>('.select-dropdown-portal')!
  const keyboardTarget = dropdown.querySelector<HTMLElement>(searchable ? '[role="combobox"]' : '[role="listbox"]')!
  expect(dialog.contains(dropdown)).toBe(false)
  expect(document.activeElement).toBe(keyboardTarget)
  expect(trigger.getAttribute('aria-expanded')).toBe('true')
  return { dialog, trigger, first, last, onClose, value, keyboardTarget }
}

function press(target: HTMLElement, key: string, shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key, shiftKey, bubbles: true, cancelable: true })
  target.dispatchEvent(event)
  return event
}

describe('Select 在焦点约束对话框中的键盘交互', () => {
  beforeEach(() => {
    // jsdom 不计算布局，用可见尺寸启用 BaseDialog 的真实焦点约束分支。
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([{}] as unknown as DOMRectList)
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    document.body.innerHTML = ''
    document.body.classList.remove('modal-open')
    vi.restoreAllMocks()
  })

  it.each([false, true])('Escape 只关闭下拉，再次按下才关闭对话框（可搜索：%s）', async searchable => {
    const { dialog, trigger, onClose, keyboardTarget } = await openDialogSelect(searchable)
    expect(press(keyboardTarget, 'Escape').defaultPrevented).toBe(true)
    await flushPromises()

    expect(document.querySelector('.select-dropdown-portal')).toBeNull()
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(trigger)
    expect(onClose).not.toHaveBeenCalled()
    expect(dialog.isConnected).toBe(true)

    expect(press(trigger, 'Escape').defaultPrevented).toBe(true)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it.each([
    { searchable: false, shiftKey: false },
    { searchable: false, shiftKey: true },
    { searchable: true, shiftKey: false },
    { searchable: true, shiftKey: true }
  ])('Tab 先恢复原触发器，避免对话框误判菜单焦点在外部（%j）', async ({ searchable, shiftKey }) => {
    const { trigger, first, last, onClose, keyboardTarget } = await openDialogSelect(searchable)
    const event = press(keyboardTarget, 'Tab', shiftKey)

    // 断言组件处理后的焦点与事件；浏览器默认 Tab 前进/后退由浏览器回归验证。
    expect(event.defaultPrevented).toBe(false)
    expect(document.activeElement).toBe(trigger)
    expect(document.activeElement).not.toBe(first)
    expect(document.activeElement).not.toBe(last)
    await flushPromises()
    expect(document.querySelector('.select-dropdown-portal')).toBeNull()
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    expect(onClose).not.toHaveBeenCalled()
  })

  it('清除按钮独立可聚焦，清除后回到原触发器且不关闭对话框', async () => {
    const { dialog, trigger, onClose, value, keyboardTarget } = await openDialogSelect(false)
    press(keyboardTarget, 'Tab')
    await flushPromises()

    const clear = dialog.querySelector<HTMLButtonElement>('button[aria-label="清除 供应商"]')!
    expect(clear.type).toBe('button')
    expect(clear.disabled).toBe(false)
    expect(clear.tabIndex).toBe(0)
    clear.focus()
    expect(press(clear, 'Tab').defaultPrevented).toBe(false)
    expect(document.activeElement).toBe(clear)
    clear.click()
    await flushPromises()

    expect(value.value).toBeNull()
    expect(dialog.querySelector('.select-clear')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    expect(onClose).not.toHaveBeenCalled()
  })
})
