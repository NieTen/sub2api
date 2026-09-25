import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

import AppLayout from '../AppLayout.vue'

const replayTour = vi.fn()
const setReplayCallback = vi.fn()
const authStore = vi.hoisted(() => ({ user: { role: 'admin' } }))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ sidebarCollapsed: false }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/composables/useOnboardingTour', () => ({
  useOnboardingTour: () => ({ replayTour }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ setReplayCallback }),
}))

describe('AppLayout sidebar overlay', () => {
  beforeEach(() => {
    document
      .querySelectorAll('#sub2api-sidebar-overlay-script')
      .forEach((node) => node.remove())
    setReplayCallback.mockClear()
    replayTour.mockClear()
    authStore.user.role = 'admin'
  })

  it('does not inject a standalone sidebar overlay script', () => {
    mount(AppLayout, {
      global: {
        stubs: {
          AppSidebar: { template: '<aside />' },
          AppHeader: { template: '<header />' },
        },
      },
      slots: {
        default: '<div data-testid="slot" />',
      },
    })

    expect(
      document.querySelectorAll('#sub2api-sidebar-overlay-script'),
    ).toHaveLength(0)
    expect(setReplayCallback).toHaveBeenCalledWith(replayTour)
  })

  it('stays script-free on remount', () => {
    mount(AppLayout, {
      global: {
        stubs: {
          AppSidebar: { template: '<aside />' },
          AppHeader: { template: '<header />' },
        },
      },
    })
    mount(AppLayout, {
      global: {
        stubs: {
          AppSidebar: { template: '<aside />' },
          AppHeader: { template: '<header />' },
        },
      },
    })

    expect(
      document.querySelectorAll('#sub2api-sidebar-overlay-script'),
    ).toHaveLength(0)
  })

  it('只为客户概览启用紧凑工作台，默认及管理员布局保持原样', () => {
    const options = {
      props: { variant: 'workspace' as const },
      global: { stubs: { AppSidebar: true, AppHeader: true } },
      slots: { default: '<div data-testid="slot" />' },
    }
    const admin = mount(AppLayout, options)
    expect(admin.find('.workspace-layout').exists()).toBe(false)
    admin.unmount()

    authStore.user.role = 'user'
    const workspace = mount(AppLayout, options)
    expect(workspace.find('.workspace-layout').exists()).toBe(true)
    expect(workspace.findComponent({ name: 'AppSidebar' }).props('workspace')).toBe(true)
    expect(workspace.findComponent({ name: 'AppHeader' }).props('compact')).toBe(true)
    expect(workspace.find('[data-testid="slot"]').exists()).toBe(true)
    workspace.unmount()

    const standard = mount(AppLayout, { global: options.global })
    expect(standard.find('.workspace-layout').exists()).toBe(false)
    standard.unmount()
  })
})
