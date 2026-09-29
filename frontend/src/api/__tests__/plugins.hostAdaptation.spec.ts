import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import plugins from '../admin/plugins'

vi.mock('../client', () => ({ apiClient: { get: vi.fn(), post: vi.fn(), put: vi.fn() } }))

describe('插件宿主适配接口契约', () => {
  beforeEach(() => vi.resetAllMocks())

  it('独立开关只提交 enabled，不夹带插件配置、启用状态或灰度', async () => {
    const installation = { id: 7, host_adaptation_enabled: true, state: 'disabled', bindings: [] }
    vi.mocked(apiClient.put).mockResolvedValue({ data: installation })
    await expect(plugins.setHostAdaptation(7, true)).resolves.toEqual(installation)
    expect(apiClient.put).toHaveBeenCalledWith('/admin/plugins/7/host-adaptation', { enabled: true })
    expect(apiClient.post).not.toHaveBeenCalled()
  })

  it('目录使用插件专用完整资源响应，保留未分组账号和代理协议', async () => {
    const resources = {
      accounts: [{ id: 1, name: '未分组账号', group_ids: [] }, { id: 2, name: '多个分组', group_ids: [3, 4] }],
      groups: [{ id: 3, name: '分组三' }, { id: 4, name: '分组四' }],
      proxies: [{ id: 5, name: '代理', protocol: 'socks5', host: 'proxy.example', port: 1080 }],
    }
    vi.mocked(apiClient.get).mockResolvedValue({ data: resources })
    await expect(plugins.resources(7)).resolves.toEqual(resources)
    expect(apiClient.get).toHaveBeenCalledTimes(1)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/plugins/7/resources')
  })

  it('动作保留宿主拒绝结果和请求编号，不自行重试', async () => {
    const action = { type: 'model_test', request_id: 'action-unique', account_id: 9 }
    const result = { accepted: false, message: '当前插件尚未运行' }
    vi.mocked(apiClient.post).mockResolvedValue({ data: result })
    await expect(plugins.action(7, action)).resolves.toEqual(result)
    expect(apiClient.post).toHaveBeenCalledWith('/admin/plugins/7/actions', action)
    expect(apiClient.post).toHaveBeenCalledTimes(1)
  })

  it('权限错误保留后端原因，不回退到管理员账号或代理列表', async () => {
    const failure = { status: 403, message: '宿主适配未开启' }
    vi.mocked(apiClient.get).mockRejectedValue(failure)
    await expect(plugins.resources(7)).rejects.toEqual(failure)
    expect(apiClient.get).toHaveBeenCalledTimes(1)
  })
})
