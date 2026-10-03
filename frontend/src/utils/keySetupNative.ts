import type { GroupPlatform } from '@/types'
import { normalizeKeySetupEndpoint } from '@/utils/keySetupClients'
import type { KeySetupOS, KeySetupText } from '@/utils/keySetupGuides'

export interface KeySetupNativeOptions {
  baseUrl: string
  apiKey: string
  model: string
  os: KeySetupOS
  platform?: GroupPlatform | null
}

export interface KeySetupNativeConfig {
  files: {
    path: string
    language: 'json' | 'yaml'
    content: string
    merge: true
  }[]
  instructions: KeySetupText[]
  verifyCommand: string
  launchCommand: string
}

// YAML 双引号标量兼容 JSON 转义，额外转义 YAML 会视作换行的 Unicode 字符。
function yamlString(value: string): string {
  return JSON.stringify(value).replace(/[\u0085\u2028\u2029]/g, char => `\\u${char.charCodeAt(0).toString(16).padStart(4, '0')}`)
}

/** 只生成供用户合并的配置片段，不执行命令、不写文件，也不把密钥放进 URL。 */
export function generateKeySetupNative(client: string, options: KeySetupNativeOptions): KeySetupNativeConfig | null {
  if (client !== 'openclaw' && client !== 'hermes') return null
  if (!['macos', 'windows', 'linux'].includes(options.os)) return null
  if (!options.apiKey.trim() || !options.model.trim()) return null

  let endpoint: string
  try {
    endpoint = normalizeKeySetupEndpoint(options.baseUrl, options.platform, client)
  } catch {
    return null
  }

  const windows = options.os === 'windows'
  const instructions: KeySetupText[] = [
    {
      zh: '先备份已有配置，然后合并下面的字段；保留其他供应商、模型和工具设置，不要直接覆盖整个文件。',
      en: 'Back up the existing configuration, then merge the fields below. Keep other providers, models, and tool settings; do not replace the entire file.'
    },
    {
      zh: '这是配置文件内容，不是终端命令。片段包含 API Key，请仅保存在自己的客户端配置中。',
      en: 'This is configuration file content, not a terminal command. It contains your API key; save it only in your own client configuration.'
    },
    {
      zh: '使用页面所选模型；模型必须属于当前密钥可用范围。保存后退出旧会话，再启动客户端。',
      en: 'Use the model selected on this page, which must be available to this API key. Save the file, exit the old session, and restart the client.'
    }
  ]

  if (client === 'openclaw') {
    // 自定义 OpenAI 兼容协议走本站现有 /v1/chat/completions，不猜测上游原生协议。
    const config = {
      models: {
        mode: 'merge',
        providers: {
          sub2api: {
            baseUrl: endpoint,
            apiKey: options.apiKey,
            api: 'openai-completions',
            models: [{ id: options.model, name: options.model }]
          }
        }
      },
      agents: { defaults: { model: { primary: `sub2api/${options.model}` } } }
    }
    instructions.push({
      zh: '若已自定义 OPENCLAW_CONFIG_PATH，请编辑实际配置文件。首次使用还需运行 openclaw onboard 完成本地引导。',
      en: 'If OPENCLAW_CONFIG_PATH is customized, edit that configuration file. On first use, run openclaw onboard to finish local setup.'
    })
    return {
      files: [{
        path: windows ? '%USERPROFILE%\\.openclaw\\openclaw.json' : '~/.openclaw/openclaw.json',
        language: 'json',
        content: JSON.stringify(config, null, 2),
        merge: true
      }],
      instructions,
      verifyCommand: 'openclaw models status',
      launchCommand: 'openclaw onboard'
    }
  }

  // 每个动态值作为独立 YAML 字符串序列化，禁止拼接成可执行 Shell。
  const content = [
    'model:',
    `  default: ${yamlString(options.model)}`,
    '  provider: "custom"',
    `  base_url: ${yamlString(endpoint)}`,
    `  api_key: ${yamlString(options.apiKey)}`
  ].join('\n')
  instructions.push({
    zh: '若已设置 HERMES_HOME，请编辑该目录中的 config.yaml；Windows 原生安装与 WSL 的配置目录不同。',
    en: 'If HERMES_HOME is set, edit config.yaml in that directory. Native Windows and WSL use different configuration directories.'
  })
  return {
    files: [{
      path: windows ? '%LOCALAPPDATA%\\hermes\\config.yaml' : '~/.hermes/config.yaml',
      language: 'yaml',
      content,
      merge: true
    }],
    instructions,
    verifyCommand: 'hermes doctor',
    launchCommand: 'hermes'
  }
}
