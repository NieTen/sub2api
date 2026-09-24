import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AuthLayout from '@/components/layout/AuthLayout.vue'

const { fetchPublicSettingsMock, setLocaleMock, localeMock, appStoreMock } = vi.hoisted(() => ({
  fetchPublicSettingsMock: vi.fn(),
  setLocaleMock: vi.fn(),
  localeMock: { value: 'zh' },
  appStoreMock: {
    siteName: 'Sub2API',
    siteLogo: '',
    siteVersion: '',
    contactInfo: '',
    apiBaseUrl: '',
    docUrl: '',
    cachedPublicSettings: {
      site_subtitle: 'Subscription to API Conversion Platform',
      registration_enabled: true,
      backend_mode_enabled: false
    },
    publicSettingsLoaded: true,
    fetchPublicSettings: (...args: unknown[]) => fetchPublicSettingsMock(...args)
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStoreMock
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: localeMock
  })
}))

vi.mock('@/i18n', () => ({
  setLocale: (...args: unknown[]) => setLocaleMock(...args)
}))

const linkStubs = {
  RouterLink: {
    props: ['to'],
    template: '<a :href="to"><slot /></a>'
  }
}

describe('AuthLayout', () => {
  beforeEach(() => {
    fetchPublicSettingsMock.mockReset()
    fetchPublicSettingsMock.mockResolvedValue({})
    setLocaleMock.mockReset()
    setLocaleMock.mockResolvedValue(undefined)
    localeMock.value = 'zh'
    appStoreMock.siteName = 'Sub2API'
    appStoreMock.docUrl = ''
    appStoreMock.cachedPublicSettings.registration_enabled = true
    appStoreMock.cachedPublicSettings.backend_mode_enabled = false
  })

  it('renders the split layout with aside and footer slots', async () => {
    const wrapper = mount(AuthLayout, {
      props: {
        mode: 'split'
      },
      global: { stubs: linkStubs },
      slots: {
        aside: '<div data-testid="auth-aside">侧栏内容</div>',
        default: '<div data-testid="auth-content">表单内容</div>',
        footer: '<div data-testid="auth-footer">底部内容</div>'
      }
    })

    await flushPromises()

    expect(fetchPublicSettingsMock).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="auth-aside"]').text()).toBe('侧栏内容')
    expect(wrapper.get('[data-testid="auth-content"]').text()).toBe('表单内容')
    expect(wrapper.get('[data-testid="auth-footer"]').text()).toBe('底部内容')
    expect(wrapper.text()).toContain('Sub2API')
    expect(wrapper.get('.auth-form-column footer').text()).toContain('Sub2API')
  })

  it('keeps the centered layout available by default', async () => {
    const wrapper = mount(AuthLayout, {
      global: { stubs: linkStubs },
      slots: {
        default: '<div data-testid="auth-content">中心内容</div>'
      }
    })

    await flushPromises()

    expect(wrapper.get('[data-testid="auth-content"]').text()).toBe('中心内容')
    expect(wrapper.find('[data-testid="auth-aside"]').exists()).toBe(false)
    expect(wrapper.find('.auth-header').exists()).toBe(false)
  })

  it('使用配置品牌，导航与首页一致仅显示首页和模型广场', () => {
    appStoreMock.siteName = '开发者服务'
    const wrapper = mount(AuthLayout, {
      props: { mode: 'split', activePage: 'login' },
      global: { stubs: linkStubs }
    })

    expect(wrapper.get('.auth-brand').attributes('href')).toBe('/home')
    expect(wrapper.get('.auth-brand').text()).toBe('开发者服务')
    expect(wrapper.get('.auth-navigation a[href="/home#pricing"]').exists()).toBe(true)
    expect(wrapper.findAll('.auth-navigation a').map(link => link.attributes('href'))).toEqual(['/home', '/home#pricing'])
    expect(wrapper.get('a[href="/login"]').attributes('aria-current')).toBe('page')
  })

  it.each([
    'https://docs.example.com/start',
    '/docs/start',
    'javascript:alert(1)'
  ])('配置文档链接时也不显示快速开始入口：%s', (url) => {
    appStoreMock.docUrl = url
    const wrapper = mount(AuthLayout, {
      props: { mode: 'split' },
      global: { stubs: linkStubs }
    })

    expect(wrapper.findAll('.auth-navigation a').map(link => link.attributes('href'))).toEqual(['/home', '/home#pricing'])
  })

  it('hides registration when disabled and supports an explicit loaded view setting', () => {
    appStoreMock.cachedPublicSettings.registration_enabled = false
    const wrapper = mount(AuthLayout, {
      props: { mode: 'split' },
      global: { stubs: linkStubs }
    })

    expect(wrapper.find('a[href="/register"]').exists()).toBe(false)
    const enabledWrapper = mount(AuthLayout, {
      props: { mode: 'split', activePage: 'register', showRegistration: true },
      global: { stubs: linkStubs }
    })
    expect(enabledWrapper.get('a[href="/register"]').attributes('aria-current')).toBe('page')
  })

  it('hides registration in backend mode even when a view enables the entry', () => {
    appStoreMock.cachedPublicSettings.backend_mode_enabled = true
    const wrapper = mount(AuthLayout, {
      props: { mode: 'split', showRegistration: true },
      global: { stubs: linkStubs }
    })

    expect(wrapper.find('a[href="/register"]').exists()).toBe(false)
  })

  it.each([
    ['zh', 'en'],
    ['en', 'zh']
  ])('switches language through the shared locale loader from %s to %s', async (current, next) => {
    localeMock.value = current
    const wrapper = mount(AuthLayout, {
      props: { mode: 'split' },
      global: { stubs: linkStubs }
    })

    await wrapper.get('.auth-language-button').trigger('click')
    await flushPromises()

    expect(setLocaleMock).toHaveBeenCalledOnce()
    expect(setLocaleMock).toHaveBeenCalledWith(next)
    expect(wrapper.get('.auth-language-button').attributes('disabled')).toBeUndefined()
  })
})
