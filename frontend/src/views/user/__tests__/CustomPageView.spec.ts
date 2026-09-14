import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import CustomPageView from '../CustomPageView.vue'

const { state } = vi.hoisted(() => ({
  state: {
    route: { params: { id: 'docs' } },
    appStore: {
      publicSettingsLoaded: true,
      cachedPublicSettings: {
        custom_menu_items: [] as Array<{
          id: string
          url: string
          label?: string
          icon_svg?: string
          visibility?: string
          sort_order?: number
          hide_open_button?: boolean
          [key: string]: unknown
        }>
      },
      fetchPublicSettings: vi.fn()
    },
    authStore: {
      isAdmin: false,
      user: { id: 7 },
      token: 'test-token' as string | null
    },
    adminSettingsStore: {
      customMenuItems: []
    },
    rawEmbedUrl: false
  }
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' }
}))

vi.mock('vue-router', () => ({
  useRoute: () => state.route
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' }
  })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => state.appStore
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => state.authStore
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => state.adminSettingsStore
}))

vi.mock('@/api/client', () => ({
  buildApiUrl: (path: string) => `/api/v1${path}`
}))

vi.mock('@/utils/embedded-url', () => ({
  buildEmbeddedUrl: (
    url: string,
    userId?: number,
    token?: string | null,
    theme = 'light',
    lang?: string
  ) => {
    // 本地权限属性测试需要验证原始地址；上游行为测试则验证鉴权参数。
    if (state.rawEmbedUrl) return url
    const embedded = new URL(url)
    if (userId) embedded.searchParams.set('user_id', String(userId))
    if (token) embedded.searchParams.set('token', token)
    embedded.searchParams.set('theme', theme)
    if (lang) embedded.searchParams.set('lang', lang)
    embedded.searchParams.set('ui_mode', 'embedded')
    return embedded.toString()
  },
  detectTheme: () => 'light'
}))

let notifyResize: () => void = () => undefined
const wrappers: ReturnType<typeof mount>[] = []

function resetEmbedPage() {
  state.route.params.id = 'docs'
  state.appStore.publicSettingsLoaded = true
  state.appStore.cachedPublicSettings.custom_menu_items = [
    { id: 'docs', url: 'https://example.com/docs' }
  ]
  state.authStore.isAdmin = false
  state.authStore.user = { id: 7 }
  state.authStore.token = 'test-token'
  state.rawEmbedUrl = false
}

function mountPage() {
  const wrapper = mount(CustomPageView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true
      }
    }
  })
  wrappers.push(wrapper)
  return wrapper
}

function mountEmbed() {
  const wrapper = mountPage()
  const shell = wrapper.get('.custom-embed-shell').element
  const button = wrapper.get<HTMLAnchorElement>('.custom-open-fab').element
  const size = { width: 800, height: 600 }
  let capturedPointer: number | null = null
  Object.defineProperties(shell, {
    clientWidth: { get: () => size.width },
    clientHeight: { get: () => size.height }
  })
  Object.defineProperties(button, {
    offsetWidth: { value: 100 },
    offsetHeight: { value: 32 },
    offsetLeft: { get: () => Number.parseFloat(button.style.left || '688') },
    offsetTop: { get: () => Number.parseFloat(button.style.top || '12') },
    setPointerCapture: { value: vi.fn((id: number) => { capturedPointer = id }) },
    hasPointerCapture: { value: (id: number) => capturedPointer === id },
    releasePointerCapture: { value: vi.fn(() => { capturedPointer = null }) }
  })
  return { wrapper, button, size }
}

async function pointer(button: HTMLElement, type: string, x: number, y: number, extra = {}) {
  const event = new MouseEvent(type, {
    clientX: x,
    clientY: y,
    bubbles: true,
    cancelable: true,
    ...extra
  })
  Object.defineProperties(event, {
    pointerId: { value: 1 },
    isPrimary: { value: true }
  })
  button.dispatchEvent(event)
  await nextTick()
}

function click(button: HTMLElement, detail = 1) {
  const event = new MouseEvent('click', { bubbles: true, cancelable: true, detail })
  button.dispatchEvent(event)
  return event
}

describe('CustomPageView', () => {
  beforeEach(() => {
    resetEmbedPage()
    document.documentElement.className = ''
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { notifyResize = callback }
      observe() {}
      disconnect() {}
    })
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    vi.unstubAllGlobals()
    notifyResize = () => undefined
  })

  describe('custom page open button', () => {
    it.each([undefined, false, true])('honors the per-menu hide button setting %s while keeping the iframe', (hidden) => {
      Object.assign(state.appStore.cachedPublicSettings.custom_menu_items[0], { hide_open_button: hidden })
      const wrapper = mountPage()
      expect(wrapper.find('.custom-open-fab').exists()).toBe(hidden !== true)
      expect(wrapper.get('iframe').attributes('src')).toContain('https://example.com/docs')
    })

    it('preserves the embedded URL, secure link attributes, and normal clicks with small pointer movements', async () => {
      const { wrapper, button } = mountEmbed()
      expect(button.href).toBe(wrapper.get('iframe').attributes('src'))
      expect(button.href).toContain('user_id=7')
      expect(button.href).toContain('token=test-token')
      expect(button.target).toBe('_blank')
      expect(button.rel).toBe('noopener noreferrer')
      await pointer(button, 'pointerdown', 700, 24)
      await pointer(button, 'pointermove', 702, 25)
      await pointer(button, 'pointerup', 702, 25)
      expect(button.style.left).toBe('')
      expect(click(button).defaultPrevented).toBe(false)
      expect(click(button, 0).defaultPrevented).toBe(false)
    })

    it('captures the pointer across iframe content and suppresses only the click following a drag', async () => {
      const { button } = mountEmbed()
      await pointer(button, 'pointerdown', 700, 24)
      expect(button.setPointerCapture).toHaveBeenCalledWith(1)
      await pointer(button, 'pointermove', 200, 124)
      expect(button.style.left).toBe('188px')
      expect(button.style.top).toBe('112px')
      await pointer(button, 'pointerup', 200, 124)
      expect(button.releasePointerCapture).toHaveBeenCalledWith(1)
      expect(click(button).defaultPrevented).toBe(true)
      expect(click(button).defaultPrevented).toBe(false)
      await pointer(button, 'pointerdown', 200, 124)
      await pointer(button, 'pointermove', 220, 124)
      await pointer(button, 'pointerup', 220, 124)
      expect(click(button, 0).defaultPrevented).toBe(false)
    })

    it('keeps the button inside each boundary and reachable when the container shrinks', async () => {
      const { button, size } = mountEmbed()
      await pointer(button, 'pointerdown', 700, 24)
      await pointer(button, 'pointermove', -1000, -1000)
      expect([button.style.left, button.style.top]).toEqual(['0px', '0px'])
      await pointer(button, 'pointermove', 2000, 2000)
      expect([button.style.left, button.style.top]).toEqual(['700px', '568px'])
      await pointer(button, 'pointerup', 2000, 2000)
      size.width = 300
      size.height = 200
      notifyResize()
      await nextTick()
      expect([button.style.left, button.style.top]).toEqual(['200px', '168px'])
    })

    it('stops moving on cancellation or lost pointer capture and permits the next normal click', async () => {
      const { button } = mountEmbed()
      for (const endEvent of ['pointercancel', 'lostpointercapture']) {
        await pointer(button, 'pointerdown', 700, 24)
        await pointer(button, 'pointermove', 500, 124)
        await pointer(button, endEvent, 500, 124)
        const position = button.style.cssText
        await pointer(button, 'pointermove', 400, 224)
        expect(button.style.cssText).toBe(position)
        await pointer(button, 'pointerdown', 500, 124)
        await pointer(button, 'pointerup', 500, 124)
        expect(click(button).defaultPrevented).toBe(false)
      }
    })

    it('leaves secondary mouse button gestures alone', async () => {
      const { button } = mountEmbed()
      await pointer(button, 'pointerdown', 700, 24, { button: 2 })
      await pointer(button, 'pointermove', 500, 124)
      expect(button.setPointerCapture).not.toHaveBeenCalled()
      expect(button.style.left).toBe('')
    })

    it('keeps Markdown pages separate from the embedded-page controls', async () => {
      state.appStore.cachedPublicSettings.custom_menu_items = [{ id: 'docs', url: 'md:guide' }]
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, text: async () => '# Guide' }))
      const wrapper = mountPage()
      await flushPromises()
      expect(wrapper.find('.custom-open-fab').exists()).toBe(false)
      expect(wrapper.find('iframe').exists()).toBe(false)
      expect(wrapper.get('.markdown-page-content h1').text()).toBe('Guide')
    })
  })

  describe('iframe permissions', () => {
    it('renders custom embed iframe with direct permission attributes', async () => {
      state.route.params.id = 'sample-page'
      state.appStore.cachedPublicSettings.custom_menu_items = [{
        id: 'sample-page',
        label: '示例页面',
        icon_svg: '<svg />',
        url: 'https://example.com/embed',
        visibility: 'user',
        sort_order: 1
      }]
      state.authStore.user = { id: 42 }
      state.authStore.token = 'token-123'
      state.rawEmbedUrl = true

      const wrapper = mountPage()
      await nextTick()

      const iframe = wrapper.get('iframe')
      expect(iframe.attributes('src')).toBe('https://example.com/embed')
      expect(iframe.attributes('allow')).toBe('microphone; fullscreen')
      expect(iframe.attributes('allowfullscreen')).toBe('')
    })
  })
})
