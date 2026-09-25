import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

import AppLayout from '../AppLayout.vue'

const replayTour = vi.fn()
const setReplayCallback = vi.fn()

vi.mock('@/stores', () => ({
  useAppStore: () => ({ sidebarCollapsed: false }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { role: 'admin' } }),
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
})
