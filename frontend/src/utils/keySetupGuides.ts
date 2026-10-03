export type KeySetupOS = 'macos' | 'windows' | 'linux'

export interface KeySetupText {
  zh: string
  en: string
}

export interface KeySetupGuideStep {
  description: KeySetupText
  command?: string
  shellLabel?: KeySetupText
}

export interface KeySetupGuide {
  client: string
  prerequisites: KeySetupText[]
  install: KeySetupGuideStep
  restart: KeySetupText[]
  verify: KeySetupGuideStep
  launch: KeySetupGuideStep
  links: { label: KeySetupText; url: string }[]
  notices: KeySetupText[]
}

const text = (zh: string, en: string): KeySetupText => ({ zh, en })
const link = (zh: string, en: string, url: string) => ({ label: text(zh, en), url })

// 安装资料只包含公开命令，不接收密钥或站点地址；旧弹窗的 Shell 标识也保持兼容。
function normalizeOS(os: string): KeySetupOS | null {
  if (os === 'windows' || os === 'powershell' || os === 'cmd') return 'windows'
  if (os === 'macos') return 'macos'
  if (os === 'linux' || os === 'unix') return 'linux'
  return null
}

export function getKeySetupGuide(client: string, os: string): KeySetupGuide | null {
  const targetOS = normalizeOS(os)
  if (!targetOS) return null
  const windows = targetOS === 'windows'
  const shellLabel = windows ? text('PowerShell', 'PowerShell') : text('终端 · Bash / Zsh', 'Terminal · Bash / Zsh')
  const restart = [text('安装完成后打开新终端；保存配置后退出旧会话，再启动客户端。', 'Open a new terminal after installation. After saving the configuration, exit the old session and start the client again.')]
  const npmPrerequisite = text('先安装 Node.js 与 npm，并确认终端能运行 node 和 npm。', 'Install Node.js and npm first, and confirm both commands are available in your terminal.')
  const installDescription = text('在自己的电脑上执行安装命令。已安装的客户端可跳过此步。', 'Run the installer on your own computer. Skip this step if the client is already installed.')
  const verifyDescription = text('显示版本号表示客户端已安装；接入是否成功还需在新会话中发送一条测试消息。', 'A version number confirms installation. Send a test message in a new session to check the connection.')
  const projectLaunch = text('先进入需要使用的项目目录，再执行启动命令。', 'Open your project directory, then run the launch command.')
  const guide = (install: KeySetupGuideStep, verify: KeySetupGuideStep, launch: KeySetupGuideStep): KeySetupGuide => ({
    client, prerequisites: [], install, restart: [...restart], verify, launch, links: [], notices: []
  })
  const cli = (install: string | undefined, verify: string, launch: string) => guide(
    { description: installDescription, command: install, shellLabel },
    { description: verifyDescription, command: verify, shellLabel },
    { description: projectLaunch, command: launch, shellLabel }
  )

  if (client === 'claude') {
    const result = cli(windows ? 'irm https://claude.ai/install.ps1 | iex' : 'curl -fsSL https://claude.ai/install.sh | bash', 'claude --version', 'claude')
    result.prerequisites = [text('原生安装不要求 Node.js。Windows 可选装 Git for Windows 以使用 Bash 工具。', 'The native installer does not require Node.js. Git for Windows is optional and enables the Bash tool on Windows.')]
    result.links = [link('Claude Code 官方安装说明', 'Official Claude Code installation', 'https://code.claude.com/docs/en/setup')]
    return result
  }

  if (client === 'codex' || client === 'codex-ws') {
    const result = cli('npm install -g @openai/codex@latest', 'codex --version', 'codex')
    result.prerequisites = [npmPrerequisite]
    result.links = [
      link('Codex CLI 官方说明', 'Official Codex CLI guide', 'https://learn.chatgpt.com/docs/codex/cli'),
      link('官方 npm 安装与版本检查示例', 'Official npm installation and version check', 'https://developers.openai.com/cookbook/examples/codex/using_goals_in_codex')
    ]
    if (client === 'codex-ws') result.notices.push(text('WebSocket 模式使用同一个 Codex CLI，只需使用对应连接配置，无需安装另一个客户端。', 'WebSocket mode uses the same Codex CLI. Apply the matching connection configuration; no separate client is needed.'))
    return result
  }

  if (client === 'codex-app') {
    const result = guide(
      { description: text('通过官方桌面应用说明下载适合当前系统的安装包。', 'Download the installer for your operating system through the official desktop app guide.') },
      { description: text('完全退出后重新打开应用，进入 Codex 并新建会话，确认使用刚配置的供应商和模型。', 'Fully quit and reopen the app, open Codex, and create a new session using the configured provider and model.') },
      { description: text('从系统应用列表启动桌面应用，打开 Codex 并选择项目。', 'Launch the desktop app from your system application list, open Codex, and select a project.') }
    )
    result.restart = [text('保存配置或导入供应商后，完全退出并重新打开桌面应用。', 'After saving the configuration or importing a provider, fully quit and reopen the desktop app.')]
    result.links = [link('官方桌面应用下载与入门', 'Official desktop app download and setup', 'https://learn.chatgpt.com/docs/quickstart')]
    result.notices = [text('桌面应用和 Codex CLI 是不同入口；安装 CLI 不代表已经安装桌面应用。', 'The desktop app and Codex CLI are separate entry points. Installing the CLI does not install the desktop app.')]
    return result
  }

  if (client === 'gemini') {
    const result = cli('npm install -g @google/gemini-cli', 'gemini --version', 'gemini')
    result.prerequisites = [text('需要 Node.js 20 或更高版本以及 npm。', 'Requires Node.js 20 or later and npm.')]
    result.links = [
      link('Gemini CLI 官方安装说明', 'Official Gemini CLI installation', 'https://geminicli.com/docs/get-started/installation/'),
      link('版本检查与常见问题', 'Version check and FAQ', 'https://geminicli.com/docs/resources/faq/')
    ]
    return result
  }

  if (client === 'opencode') {
    const result = cli('npm install -g opencode-ai', 'opencode --version', 'opencode')
    result.prerequisites = [npmPrerequisite]
    result.links = [
      link('OpenCode 官方安装说明', 'Official OpenCode installation', 'https://opencode.ai/docs/'),
      link('OpenCode 命令参考', 'OpenCode CLI reference', 'https://opencode.ai/docs/cli/')
    ]
    if (windows) result.notices.push(text('也可按官方指南使用 WSL；安装、配置和启动须在同一环境内完成。', 'WSL is another option in the official guide. Install, configure, and run the client in the same environment.'))
    return result
  }

  if (client === 'grok') {
    const result = cli(windows ? undefined : 'curl -fsSL https://x.ai/cli/install.sh | bash', 'grok inspect', 'grok')
    if (windows) result.install.description = text('打开官方安装说明并选择 Windows (PowerShell)，按该页命令安装 Grok Build。', 'Open the official installation guide, select Windows (PowerShell), and install Grok Build with the command shown there.')
    result.verify.description = text('检查 Grok 实际读取的配置来源，再启动新会话并选择已配置模型。', 'Check which configuration Grok actually loads, then start a new session and select the configured model.')
    result.links = [link('Grok Build 官方安装与自定义模型说明', 'Official Grok Build installation and custom models', 'https://docs.x.ai/build/overview')]
    return result
  }

  if (client === 'openclaw') {
    const result = cli(windows ? '& ([scriptblock]::Create((iwr -useb https://openclaw.ai/install.ps1))) -NoOnboard' : 'curl -fsSL https://openclaw.ai/install.sh | bash -s -- --no-onboard', 'openclaw --version', 'openclaw onboard')
    result.prerequisites = [text('官方安装器会检查并准备兼容的 Node.js；这里先安装，配置步骤稍后进行。', 'The official installer checks and prepares a compatible Node.js runtime. This command installs first; configuration follows below.')]
    result.launch.description = text('首次使用运行引导并检查本页配置的供应商；完成设置后，可运行 openclaw 打开终端界面。', 'Run onboarding on first use and check the provider configured here. Once setup is complete, run openclaw to open the terminal interface.')
    result.links = [
      link('OpenClaw 官方安装说明', 'Official OpenClaw installation', 'https://docs.openclaw.ai/install'),
      link('自定义供应商配置', 'Custom provider configuration', 'https://docs.openclaw.ai/gateway/config-tools/custom-providers'),
      link('首次启动与模型验证', 'First launch and model verification', 'https://docs.openclaw.ai/start/onboarding-overview')
    ]
    result.notices = [text('引导中的模型验证会发送真实请求。确认接口和模型后再继续。', 'Model verification during onboarding sends a real request. Check the endpoint and model before continuing.')]
    return result
  }

  if (client === 'hermes') {
    const result = cli(windows ? 'iex (irm https://hermes-agent.nousresearch.com/install.ps1)' : 'curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash', 'hermes doctor', 'hermes')
    result.prerequisites = [windows
      ? text('使用原生 PowerShell 安装；官方安装器会准备所需运行环境。', 'Install from native PowerShell. The official installer prepares the required runtime.')
      : text('需要 Git、curl、tar 和 SHA-256 工具；安装器会准备所需运行环境。', 'Requires Git, curl, tar, and SHA-256 utilities. The installer prepares the required runtime.')]
    if (targetOS === 'macos') result.prerequisites.push(text('当前官方 macOS 安装仅支持 Apple Silicon；Intel Mac 请先查阅支持矩阵。', 'Current official macOS installations support Apple Silicon only. Check the support matrix before installing on an Intel Mac.'))
    result.verify.description = text('检查运行环境与配置，再启动 Hermes 发送测试消息；也可运行 hermes model 检查自定义供应商。', 'Check runtime and configuration health, then start Hermes and send a test message. Use hermes model to inspect the custom provider.')
    result.links = [
      link('Hermes 官方安装说明', 'Official Hermes installation', 'https://hermes-agent.nousresearch.com/docs/getting-started/installation'),
      link('Hermes 自定义接口配置', 'Hermes custom endpoint configuration', 'https://hermes-agent.nousresearch.com/docs/integrations/providers')
    ]
    return result
  }

  if (client === 'claude-desktop') {
    const result = guide(
      { description: text('从官方入口安装 Claude Desktop，并安装支持 Desktop 供应商的 CC Switch。', 'Install Claude Desktop from the official download page and CC Switch with Desktop provider support.') },
      { description: text('在 CC Switch 的 Claude Desktop 面板检查供应商和模型映射，启用后重启 Desktop，再发起新对话。', 'Check the provider and model mapping in CC Switch’s Claude Desktop panel, enable it, restart Desktop, and start a new conversation.') },
      { description: text('从系统应用列表启动 Claude Desktop。需要模型映射时，保持 CC Switch 及其 Desktop 本地路由运行。', 'Launch Claude Desktop from your system application list. If model mapping is required, keep CC Switch and its Desktop local routing running.') }
    )
    result.restart = [text('在 CC Switch 的 Claude Desktop 面板点击「+」，添加自定义供应商并填写本页提供的配置。保存并启用后，完全退出并重新打开 Claude Desktop。', 'In CC Switch’s Claude Desktop panel, click “+”, add a custom provider, and enter the configuration provided here. Save and enable it, then fully quit and reopen Claude Desktop.')]
    result.links = [
      link('Claude 官方下载', 'Official Claude download', 'https://claude.com/download'),
      link('CC Switch 下载', 'Download CC Switch', 'https://github.com/farion1231/cc-switch/releases/latest'),
      link('CC Switch Desktop 供应商指南', 'CC Switch Desktop provider guide', 'https://github.com/farion1231/cc-switch/blob/v3.20.4/docs/user-manual/zh/2-providers/2.6-claude-desktop.md')
    ]
    result.notices = [
      text('这是 CC Switch 的第三方供应商配置流程。Claude Desktop 官方登录模式不使用本页 API Key。', 'This flow configures a third-party provider through CC Switch. Claude Desktop’s official sign-in mode does not use this page’s API key.'),
      text('「将 Claude Code 中已有的供应商导入」只在 Desktop 供应商列表为空时显示。本页使用「+」添加流程，不需要该入口，也不需要删除已有供应商。', '“Import existing providers from Claude Code” only appears when the Desktop provider list is empty. This guide uses “+” to add a provider and does not require that prompt or removing existing providers.')
    ]
    return result
  }

  return null
}
