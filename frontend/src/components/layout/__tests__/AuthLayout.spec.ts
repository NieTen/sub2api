import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AuthLayout from '@/components/layout/AuthLayout.vue'

const { fetchPublicSettingsMock } = vi.hoisted(() => ({
  fetchPublicSettingsMock: vi.fn()
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    siteName: 'Sub2API',
    siteLogo: '',
    siteVersion: '',
    contactInfo: '',
    apiBaseUrl: '',
    docUrl: '',
    cachedPublicSettings: {
      site_subtitle: 'Subscription to API Conversion Platform'
    },
    publicSettingsLoaded: true,
    fetchPublicSettings: (...args: unknown[]) => fetchPublicSettingsMock(...args)
  })
}))

describe('AuthLayout', () => {
  beforeEach(() => {
    fetchPublicSettingsMock.mockReset()
    fetchPublicSettingsMock.mockResolvedValue({})
  })

  it('renders the split layout with aside and footer slots', async () => {
    const wrapper = mount(AuthLayout, {
      props: {
        mode: 'split'
      },
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
  })

  it('keeps the centered layout available by default', async () => {
    const wrapper = mount(AuthLayout, {
      slots: {
        default: '<div data-testid="auth-content">中心内容</div>'
      }
    })

    await flushPromises()

    expect(wrapper.get('[data-testid="auth-content"]').text()).toBe('中心内容')
    expect(wrapper.find('[data-testid="auth-aside"]').exists()).toBe(false)
  })
})
