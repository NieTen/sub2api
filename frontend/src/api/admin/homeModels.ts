import { apiClient } from '../client'

export interface HomeTextModel {
  name: string
  vendor: string
  type: 'text'
  input: number
  output: number
  cachedInput: number | null
  flexInput: number | null
}

export interface HomeImageModel {
  name: string
  vendor: string
  type: 'image'
  resolutionPrices: Record<'1K' | '2K' | '4K', number>
}

export type HomeModel = HomeTextModel | HomeImageModel

/** 读取首页展示模型；与渠道实际计费配置独立。 */
export async function getHomeModels(): Promise<HomeModel[]> {
  const { data } = await apiClient.get<HomeModel[]>('/admin/settings/home-models')
  return data
}

/** 按当前顺序保存模型，空数组表示清空首页展示列表。 */
export async function saveHomeModels(models: HomeModel[]): Promise<HomeModel[]> {
  const { data } = await apiClient.put<HomeModel[]>('/admin/settings/home-models', { models })
  return data
}
