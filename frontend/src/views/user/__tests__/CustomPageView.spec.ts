import { nextTick, ref } from 'vue'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

import CustomPageView from '../CustomPageView.vue'

const route = {
  params: {
    id: 'sample-page',
  },
}

const appStore = {
  cachedPublicSettings: {
    custom_menu_items: [
      {
        id: 'sample-page',
        label: '示例页面',
        icon_svg: '<svg />',
        url: 'https://example.com/embed',
        visibility: 'user',
        sort_order: 1,
      },
    ],
  },
  publicSettingsLoaded: true,
  fetchPublicSettings: vi.fn(),
}

const authStore = {
  isAdmin: false,
  token: 'token-123',
  user: {
    id: 42,
  },
}

const adminSettingsStore = {
  customMenuItems: [],
}

vi.mock('vue-router', () => ({
  useRoute: () => route,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: ref('zh'),
    }),
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => adminSettingsStore,
}))

vi.mock('@/utils/embedded-url', () => ({
  buildEmbeddedUrl: (url: string) => url,
  detectTheme: () => 'light',
}))

describe('CustomPageView iframe permissions', () => {
  beforeEach(() => {
    document.documentElement.className = ''
  })

  it('renders custom embed iframe with direct permission attributes', async () => {
    const wrapper = mount(CustomPageView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
        },
      },
    })

    await nextTick()

    const iframe = wrapper.get('iframe')
    expect(iframe.attributes('src')).toBe('https://example.com/embed')
    expect(iframe.attributes('allow')).toBe('microphone; fullscreen')
    expect(iframe.attributes('allowfullscreen')).toBe('')
  })
})
