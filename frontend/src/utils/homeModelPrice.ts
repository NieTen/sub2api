// 只清理浮点运算产生的极小尾差，不按固定小数位截断模型价格。
export function normalizeHomeModelPrice(value: number): number {
  if (!Number.isFinite(value) || value <= 0 || Number.isInteger(value)) return value
  const candidate = Number(value.toPrecision(12))
  const tolerance = Math.abs(value) * Number.EPSILON * 2
  return Math.abs(candidate - value) <= tolerance ? candidate : value
}
