import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import type { RouteRecordRaw } from 'vue-router'

type Guard = (to: Record<string, unknown>, from: Record<string, unknown>, next: ReturnType<typeof vi.fn>) => Promise<void>
const harness = vi.hoisted(() => ({ guard: null as Guard | null, routes: [] as RouteRecordRaw[] }))
const auth = vi.hoisted(() => ({ checkAuth: vi.fn(), isAuthenticated: false, isAdmin: false, isSimpleMode: false, hasPendingAuthSession: false }))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({})),
  createRouter: vi.fn((options: { routes: RouteRecordRaw[] }) => {
    harness.routes = options.routes
    return { beforeEach: (guard: Guard) => { harness.guard = guard }, afterEach: vi.fn(), onError: vi.fn() }
  })
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteName: 'Sub2API', backendModeEnabled: false, publicSettingsLoaded: true, cachedPublicSettings: {} }) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/stores/adminCompliance', () => ({ useAdminComplianceStore: () => ({ initialized: true }) }))
vi.mock('@/composables/useNavigationLoading', () => ({ useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn() }) }))
vi.mock('@/composables/useRoutePrefetch', () => ({ useRoutePrefetch: () => ({ triggerPrefetch: vi.fn() }) }))
vi.mock('@/api/setup', () => ({ getSetupStatus: vi.fn() }))
vi.mock('@/router/title', () => ({ resolveRouteDocumentTitle: () => 'Sub2API' }))

async function visit(path: string) {
  const route = harness.routes.find(item => item.path === path)
  expect(route).toBeDefined()
  const next = vi.fn()
  await harness.guard!({ path, fullPath: path, name: route!.name, meta: route!.meta, params: {} }, {}, next)
  return next
}

const adminPaths = ['/admin/communications', '/admin/support/settings', '/admin/bulk-emails/:id(\\d+)?', '/admin/tickets/:id(\\d+)?', '/admin/community/settings', '/admin/community/members']

describe('社群与机器人群发真实路由守卫', () => {
  beforeAll(async () => { await import('@/router') })
  beforeEach(() => { auth.isAuthenticated = false; auth.isAdmin = false })

  it.each(['/community', ...adminPaths])('匿名访问 %s 转到登录并保留返回地址', async path => {
    const next = await visit(path)
    expect(next).toHaveBeenCalledWith({ path: '/login', query: { redirect: path } })
  })

  it('已登录普通用户可以访问社群页面', async () => {
    auth.isAuthenticated = true
    expect(await visit('/community')).toHaveBeenCalledWith()
  })

  it.each(adminPaths)('普通用户不能进入 %s', async path => {
    auth.isAuthenticated = true
    expect(await visit(path)).toHaveBeenCalledWith('/dashboard')
  })

  it.each(adminPaths)('管理员可以进入 %s', async path => {
    auth.isAuthenticated = true
    auth.isAdmin = true
    expect(await visit(path)).toHaveBeenCalledWith()
  })
})
