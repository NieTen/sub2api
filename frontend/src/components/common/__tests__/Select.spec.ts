import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import Select from '../Select.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const originalInnerWidth = window.innerWidth
let unmountWrapper: (() => void) | undefined

const setViewportWidth = (width: number) => {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: width,
  })
}

const mockTriggerRect = (left: number, width: number) => {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    x: left,
    y: 20,
    top: 20,
    right: left + width,
    bottom: 60,
    left,
    width,
    height: 40,
    toJSON: () => ({}),
  })
}

const openSelect = async () => {
  const wrapper = mount(Select, {
    props: {
      modelValue: null,
      options: [
        {
          value: 'example',
          label: 'very-long-unbroken-option-value-that-must-not-overflow',
        },
      ],
    },
  })
  unmountWrapper = () => wrapper.unmount()

  await wrapper.get('button').trigger('click')
  await nextTick()

  return document.body.querySelector<HTMLElement>('.select-dropdown-portal')
}

afterEach(() => {
  unmountWrapper?.()
  unmountWrapper = undefined
  document.body.innerHTML = ''
  setViewportWidth(originalInnerWidth)
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('Select dropdown viewport constraints', () => {
  it('preserves the existing 200px minimum width when space is available', async () => {
    setViewportWidth(1024)
    mockTriggerRect(20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('20px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('996px')
  })

  it('moves a dropdown left to keep readable width near the right edge', async () => {
    setViewportWidth(320)
    mockTriggerRect(220, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('112px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('200px')
  })

  it('clamps a trigger left of the viewport to the safe padding', async () => {
    setViewportWidth(320)
    mockTriggerRect(-20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('8px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('304px')
  })

  it('clamps an offscreen-right trigger position to the viewport boundary', async () => {
    setViewportWidth(320)
    mockTriggerRect(400, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('112px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('200px')
  })

  it('limits menu height when neither side can fit all options', async () => {
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ top: 350, bottom: 390, left: 20, width: 200 } as DOMRect)
    const wrapper = mount(Select, { props: { modelValue: null, options: Array.from({ length: 30 }, (_, value) => ({value, label: String(value)})) } })
    unmountWrapper = () => wrapper.unmount()
    await wrapper.get('button').trigger('click')
    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')!
    expect(Number.parseFloat(dropdown.style.maxHeight)).toBeLessThanOrEqual(window.innerHeight - 390 - 12)
    expect(dropdown.querySelector('[role=listbox]')?.getAttribute('id')).toBe(wrapper.get('button').attributes('aria-controls'))
  })
})

describe('Select keyboard interaction', () => {
  it('skips disabled options and group titles, returns focus after Enter and allows keyboard clearing', async () => {
    const wrapper = mount(Select, { attachTo: document.body, props: { modelValue: 'beta', clearable: true, searchable: false, options: [
      { value: 'group', label: '分组', kind: 'group' }, { value: 'alpha', label: 'Alpha', disabled: true }, { value: 'beta', label: 'Beta' }, { value: 'gamma', label: 'Gamma' }
    ] } })
    unmountWrapper = () => wrapper.unmount()
    await wrapper.get('.select-trigger').trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    const list = document.body.querySelector<HTMLElement>('[role=listbox]')!
    expect(document.activeElement).toBe(list)
    list.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    list.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await nextTick()
    expect(wrapper.emitted('update:modelValue')).toEqual([['gamma']])
    expect(document.activeElement).toBe(wrapper.get('.select-trigger').element)
    const clear = wrapper.get('.select-clear')
    expect(clear.element.tagName).toBe('BUTTON')
    await clear.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([null])
    expect(document.activeElement).toBe(wrapper.get('.select-trigger').element)
  })

  it('returns to the trigger before tabbing away and closes when disabled', async () => {
    const wrapper = mount(Select, { attachTo: document.body, props: { modelValue: null, options: [{ value: 'a', label: 'A' }] } })
    unmountWrapper = () => wrapper.unmount()
    await wrapper.get('button').trigger('click')
    await nextTick()
    document.body.querySelector('[role=listbox]')!.dispatchEvent(new KeyboardEvent('keydown', {key: 'Tab', bubbles: true}))
    await nextTick()
    expect(document.activeElement).toBe(wrapper.get('button').element)
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('false')
    await wrapper.get('button').trigger('click')
    await wrapper.setProps({disabled: true})
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('false')
  })
})

describe('Select remote search', () => {
  const mountRemoteSelect = (props: Record<string, unknown> = {}) => {
    const wrapper = mount(Select, {
      props: {
        modelValue: null,
        remote: true,
        options: [
          { value: 'alpha', label: 'Alpha account' },
          { value: 'beta', label: 'Beta account' },
        ],
        ...props,
      },
    })
    unmountWrapper = () => wrapper.unmount()
    return wrapper
  }

  const openDropdown = async () => {
    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')
    expect(dropdown).not.toBeNull()
    return dropdown as HTMLElement
  }

  const typeSearchQuery = async (query: string) => {
    const dropdown = await openDropdown()
    const input = dropdown.querySelector<HTMLInputElement>('.select-search-input')
    expect(input).not.toBeNull()
    input!.value = query
    input!.dispatchEvent(new Event('input'))
    await nextTick()
  }

  it('emits debounced search events and skips local filtering in remote mode', async () => {
    vi.useFakeTimers()
    const wrapper = mountRemoteSelect()
    await wrapper.get('button').trigger('click')
    await nextTick()

    await typeSearchQuery('zzz')

    // 防抖窗口内不触发。
    expect(wrapper.emitted('search')).toBeUndefined()
    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toEqual([['zzz']])
    // 远程模式不做本地过滤：无命中的 query 下选项仍完整展示（由父组件更新 options）。
    const dropdown = await openDropdown()
    const labels = [...dropdown.querySelectorAll('.select-option-label')].map((el) => el.textContent)
    expect(labels).toContain('Alpha account')
    expect(labels).toContain('Beta account')
  })

  it('does not emit search when the dropdown closes and the query resets', async () => {
    vi.useFakeTimers()
    const wrapper = mountRemoteSelect()
    await wrapper.get('button').trigger('click')
    await nextTick()

    await typeSearchQuery('hidden')

    // 关闭下拉：排队中的防抖定时器应被取消，也不应因 query 重置而尾随 emit。
    await wrapper.get('button').trigger('click')
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toBeUndefined()
  })

  it('shows the loading text instead of empty text while loading with no options', async () => {
    const wrapper = mountRemoteSelect({ options: [], loading: true })
    await wrapper.get('button').trigger('click')
    await nextTick()

    const dropdown = await openDropdown()
    expect(dropdown.querySelector('.select-empty')?.textContent).toContain('common.loading')
  })

  it('keeps local filtering and emits nothing when remote is not set', async () => {
    vi.useFakeTimers()
    const wrapper = mount(Select, {
      props: {
        modelValue: null,
        searchable: true,
        options: [
          { value: 'alpha', label: 'Alpha account' },
          { value: 'beta', label: 'Beta account' },
        ],
      },
    })
    unmountWrapper = () => wrapper.unmount()
    await wrapper.get('button').trigger('click')
    await nextTick()

    await typeSearchQuery('alpha')
    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toBeUndefined()
    const dropdown = await openDropdown()
    const labels = [...dropdown.querySelectorAll('.select-option-label')].map((el) => el.textContent)
    expect(labels).toEqual(['Alpha account'])
  })
})
