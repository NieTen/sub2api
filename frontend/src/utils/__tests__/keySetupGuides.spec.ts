import { describe, expect, it } from 'vitest'
import { getKeySetupGuide } from '@/utils/keySetupGuides'

const clients = ['codex-app', 'claude', 'claude-desktop', 'openclaw', 'hermes', 'opencode', 'codex', 'gemini', 'grok', 'codex-ws']
const systems = ['macos', 'windows', 'linux']

describe('客户端安装指南', () => {
  it.each(clients)('%s 在三个系统都有完整的双语步骤和 HTTPS 文档入口', client => {
    for (const os of systems) {
      const guide = getKeySetupGuide(client, os)!
      expect(guide.client).toBe(client)
      const messages = [
        ...guide.prerequisites, ...guide.restart, ...guide.notices,
        guide.install.description, guide.verify.description, guide.launch.description,
        ...guide.links.map(item => item.label)
      ]
      for (const message of messages) {
        expect(message.zh.trim()).not.toBe('')
        expect(message.en.trim()).not.toBe('')
      }
      expect(guide.links.length).toBeGreaterThan(0)
      for (const item of guide.links) {
        const url = new URL(item.url)
        expect(url.protocol).toBe('https:')
        expect(url.username + url.password + url.search).toBe('')
      }
    }
  })

  it('未知客户端或系统不生成猜测的指南', () => {
    expect(getKeySetupGuide('unknown', 'windows')).toBeNull()
    expect(getKeySetupGuide('claude', 'unknown')).toBeNull()
  })

  it('Windows 仅展示 PowerShell 命令，并兼容旧弹窗的 shell 标识', () => {
    for (const shell of ['windows', 'cmd', 'powershell']) {
      const guide = getKeySetupGuide('claude', shell)!
      expect(guide.install.command).toBe('irm https://claude.ai/install.ps1 | iex')
      expect(guide.install.shellLabel?.en).toBe('PowerShell')
    }
    expect(getKeySetupGuide('claude', 'unix')?.install.command).toContain('| bash')
  })

  it('Codex WS 共用已确认的 CLI 安装方式', () => {
    const normal = getKeySetupGuide('codex', 'windows')!
    const websocket = getKeySetupGuide('codex-ws', 'windows')!
    expect(websocket.install).toEqual(normal.install)
    expect(websocket.verify.command).toBe('codex --version')
    expect(websocket.launch.command).toBe('codex')
    expect(websocket.notices[0]?.zh).toContain('同一个 Codex CLI')
  })

  it('桌面端保留图形下载和重新启动流程，不把 CLI 安装当作桌面安装', () => {
    for (const client of ['codex-app', 'claude-desktop']) {
      const guide = getKeySetupGuide(client, 'windows')!
      expect(guide.install.command).toBeUndefined()
      expect(guide.launch.command).toBeUndefined()
      expect(guide.restart[0]?.zh).toContain('退出')
    }
    const desktop = getKeySetupGuide('claude-desktop', 'macos')!
    expect(desktop.restart[0]?.zh).toContain('CC Switch')
    expect(desktop.notices[0]?.zh).toContain('官方登录模式不使用本页 API Key')
  })

  it('未核实的 Grok Windows 安装只提供官方说明，已核实的 Unix 命令可复制', () => {
    expect(getKeySetupGuide('grok', 'windows')?.install.command).toBeUndefined()
    expect(getKeySetupGuide('grok', 'windows')?.install.description.en).toContain('PowerShell')
    expect(getKeySetupGuide('grok', 'linux')?.install.command).toContain('https://x.ai/cli/install.sh')
  })

  it('原生 Hermes 支持 PowerShell，OpenClaw 安装步骤不提前执行引导', () => {
    expect(getKeySetupGuide('hermes', 'windows')?.install.command).toContain('install.ps1')
    expect(getKeySetupGuide('openclaw', 'windows')?.install.command).toContain('-NoOnboard')
    expect(getKeySetupGuide('openclaw', 'linux')?.install.command).toContain('--no-onboard')
    expect(getKeySetupGuide('hermes', 'macos')?.prerequisites.some(item => item.en.includes('Apple Silicon'))).toBe(true)
  })
})
