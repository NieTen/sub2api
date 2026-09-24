import type { HomeModel } from '@/api/admin/homeModels'
import { normalizeHomeModelPrice } from '@/utils/homeModelPrice'

// 保留参考首页的厂商标志与中英文文案，价格统一读取后台配置。
export const brandVendors: Record<string, { label: string; viewBox: string; path: string }> = {
        openai: {
          label: 'OPENAI',viewBox: '0 0 24 24',
          path: 'M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073zM13.26 22.43a4.476 4.476 0 0 1-2.876-1.04l.141-.081 4.779-2.758a.795.795 0 0 0 .392-.681v-6.737l2.02 1.168a.071.071 0 0 1 .038.052v5.583a4.504 4.504 0 0 1-4.494 4.494zM3.6 18.304a4.47 4.47 0 0 1-.535-3.014l.142.085 4.783 2.759a.771.771 0 0 0 .78 0l5.843-3.369v2.332a.08.08 0 0 1-.033.062L9.74 19.95a4.5 4.5 0 0 1-6.14-1.646zM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.766.766 0 0 0 .388.676l5.815 3.355-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855l-5.833-3.387L15.119 7.2a.076.076 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.676 8.105v-5.678a.79.79 0 0 0-.407-.667zm2.01-3.023l-.141-.085-4.774-2.782a.776.776 0 0 0-.785 0L9.409 9.23V6.897a.066.066 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.68 4.66zm-12.64 4.135l-2.02-1.164a.08.08 0 0 1-.038-.057V6.075a4.5 4.5 0 0 1 7.375-3.453l-.142.08L8.704 5.46a.795.795 0 0 0-.393.681zm1.097-2.365l2.602-1.5 2.607 1.5v2.999l-2.597 1.5-2.607-1.5z'
        },
        anthropic: {
          label: 'ANTHROPIC',viewBox: '0 0 16 16',
          path: 'm3.127 10.604 3.135-1.76.053-.153-.053-.085H6.11l-.525-.032-1.791-.048-1.554-.065-1.505-.08-.38-.081L0 7.832l.036-.234.32-.214.455.04 1.009.069 1.513.105 1.097.064 1.626.17h.259l.036-.105-.089-.065-.068-.064-1.566-1.062-1.695-1.121-.887-.646-.48-.327-.243-.306-.104-.67.435-.48.585.04.15.04.593.456 1.267.981 1.654 1.218.242.202.097-.068.012-.049-.109-.181-.9-1.626-.96-1.655-.428-.686-.113-.411a2 2 0 0 1-.068-.484l.496-.674L4.446 0l.662.089.279.242.411.94.666 1.48 1.033 2.014.302.597.162.553.06.17h.105v-.097l.085-1.134.157-1.392.154-1.792.052-.504.25-.605.497-.327.387.186.319.456-.045.294-.19 1.23-.37 1.93-.243 1.29h.142l.161-.16.654-.868 1.097-1.372.484-.545.565-.601.363-.287h.686l.505.751-.226.775-.707.895-.585.759-.839 1.13-.524.904.048.072.125-.012 1.897-.403 1.024-.186 1.223-.21.553.258.06.263-.218.536-1.307.323-1.533.307-2.284.54-.028.02.032.04 1.029.098.44.024h1.077l2.005.15.525.346.315.424-.053.323-.807.411-3.631-.863-.872-.218h-.12v.073l.726.71 1.331 1.202 1.667 1.55.084.383-.214.302-.226-.032-1.464-1.101-.565-.497-1.28-1.077h-.084v.113l.295.432 1.557 2.34.08.718-.112.234-.404.141-.444-.08-.911-1.28-.94-1.44-.759-1.291-.093.053-.448 4.821-.21.246-.484.186-.403-.307-.214-.496.214-.98.258-1.28.21-1.016.19-1.263.112-.42-.008-.028-.092.012-.953 1.307-1.448 1.957-1.146 1.227-.274.109-.477-.247.045-.44.266-.39 1.586-2.018.956-1.25.617-.723-.004-.105h-.036l-4.212 2.736-.75.096-.324-.302.04-.496.154-.162 1.267-.871z'
        },
        gemini: {
          label: 'GEMINI',viewBox: '0 0 24 24',
          path: 'M12 2l1.89 7.2L21 12l-7.11 2.8L12 22l-1.89-7.2L3 12l7.11-2.8L12 2z'
        },
        grok: {
          label: 'GROK',viewBox: '0 0 24 24',
          path: 'M9.27 15.29l7.978-5.897c.391-.29.95-.177 1.137.272.98 2.369.542 5.215-1.41 7.169-1.951 1.954-4.667 2.382-7.149 1.406l-2.711 1.257c3.889 2.661 8.611 2.003 11.562-.953 2.341-2.344 3.066-5.539 2.388-8.42l.006.007c-.983-4.232.242-5.924 2.75-9.383.06-.082.12-.164.179-.248l-3.301 3.305v-.01L9.267 15.292M7.623 16.723c-2.792-2.67-2.31-6.801.071-9.184 1.761-1.763 4.647-2.483 7.166-1.425l2.705-1.25a7.808 7.808 0 00-1.829-1A8.975 8.975 0 005.984 5.83c-2.533 2.536-3.33 6.436-1.962 9.764 1.022 2.487-.653 4.246-2.34 6.022-.599.63-1.199 1.259-1.682 1.925l7.62-6.815'
        }
      }

export const brandMessages = {
        zh: { home:'首页',pricing:'模型广场',login:'登录',register:'注册',console:'控制台',subtitle:'众智 API',tagline:'让 AI 连接更稳定',getKey:'获取密钥',modelsAvailable:'可用模型',availability:'服务可用性',latency:'平均延迟',pool:'自建号池',poolDesc:'自有官方账号池，适合 OpenAI、Claude、Codex 等模型稳定中转。',speed:'极速响应',speedDesc:'多节点部署与高速优化通道，兼容 OpenAI API，降低调用延迟。',reliable:'高可用保障',reliableDesc:'多渠道冗余与故障切换，减少 AI API 调用链路的单点异常。',billing:'透明计费',billingDesc:'适配 NewAPI、Cherry Studio、Claude Code、Codex CLI，调用记录清晰可查。',all:'全部',textModels:'文本模型',imageModels:'图片模型',input:'输入',cachedInput:'缓存输入',flexInput:'Flex 输入',output:'输出',tokenUnit:'每百万 tokens',imageGeneration:'图片生成',perImage:'每张图片',resolutionPricing:'按输出分辨率计价',rights:'保留所有权利。',toDark:'切换到深色模式',toLight:'切换到浅色模式',openMenu:'打开菜单',closeMenu:'关闭菜单',loadingModels:'正在加载模型价格…',loadFailed:'模型数据加载失败',loadFailedDesc:'暂时无法获取模型价格，请稍后重试。',retry:'重新加载',emptyModels:'暂无模型数据'},
        en: { home:'Home',pricing:'Model plaza',login:'Log in',register:'Sign up',console:'Console',subtitle:'Zhongzhi API',tagline:'Make every AI connection more reliable',getKey:'Get API key',modelsAvailable:'Available models',availability:'Service availability',latency:'Average latency',pool:'Managed account pool',poolDesc:'First-party account pools for reliable OpenAI, Claude and Codex routing.',speed:'Fast response',speedDesc:'Multi-node deployment and optimized routes reduce API latency.',reliable:'High availability',reliableDesc:'Channel redundancy and failover reduce single points of failure.',billing:'Transparent billing',billingDesc:'Works with NewAPI, Cherry Studio, Claude Code and Codex CLI, with clear usage records.',all:'All',textModels:'Text models',imageModels:'Image models',input:'Input',cachedInput:'Cached input',flexInput:'Flex input',output:'Output',tokenUnit:'per 1M tokens',imageGeneration:'Image generation',perImage:'per image',resolutionPricing:'Priced by output resolution',rights:'All rights reserved.',toDark:'Switch to dark mode',toLight:'Switch to light mode',openMenu:'Open menu',closeMenu:'Close menu',loadingModels:'Loading model prices…',loadFailed:'Failed to load model data',loadFailedDesc:'Model prices are temporarily unavailable. Please try again.',retry:'Reload',emptyModels:'No model data available'}
      } as const
export type BrandMessageKey = keyof typeof brandMessages.zh

export function getBrandVendor(vendor: string) {
  const key = vendor.trim().toLowerCase()
  return Object.prototype.hasOwnProperty.call(brandVendors, key) ? brandVendors[key] : null
}

export function formatHomePrice(value: number | null): string {
  if (value === null) return '—'
  if (value === 0) return '$0'
  const price = normalizeHomeModelPrice(value)
  const formatted = price.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 20, useGrouping: false })
  // 超出格式化精度的极小价格保留原数值表示，避免丢失有效数字或显示为免费。
  return '$' + (Number(formatted) === price ? formatted : price.toString())
}

function isPrice(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

export function isValidHomeModel(value: unknown): value is HomeModel {
  if (!value || typeof value !== 'object') return false
  const model = value as Record<string, unknown>
  if (typeof model.name !== 'string' || !model.name.trim() || typeof model.vendor !== 'string' || !model.vendor.trim()) return false
  if (model.type === 'text') {
    return isPrice(model.input) && isPrice(model.output) &&
      (model.cachedInput === null || isPrice(model.cachedInput)) &&
      (model.flexInput === null || isPrice(model.flexInput))
  }
  if (model.type === 'image' && model.resolutionPrices && typeof model.resolutionPrices === 'object') {
    const prices = model.resolutionPrices as Record<string, unknown>
    return ['1K', '2K', '4K'].every(size => isPrice(prices[size]))
  }
  return false
}
