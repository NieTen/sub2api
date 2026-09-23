export type SidebarPageMode = 'dashboard' | 'guide'

export interface SidebarPageOverlayOptions {
  target: HTMLElement
  navigate: (route: string) => void
  request: (path: string) => Promise<unknown>
}

export interface SidebarPageOverlay {
  render: (mode: SidebarPageMode) => void
  destroy: () => void
}

// 页面实例随 Vue 组件挂载与卸载，后台请求复用现有认证客户端。
export function createSidebarPageOverlay(options: SidebarPageOverlayOptions): SidebarPageOverlay
