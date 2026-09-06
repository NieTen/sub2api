const PREFIX = 'vip-community-prompt:'

export function vipCommunityPromptKey(userId: number, promptKey: string): string {
  return `${PREFIX}${userId}:${promptKey}`
}

export function wasVipCommunityPromptDismissed(key: string): boolean {
  try { return sessionStorage.getItem(key) === '1' } catch { return false }
}

export function rememberVipCommunityPrompt(key: string): void {
  try { sessionStorage.setItem(key, '1') } catch { /* 存储不可用时仍允许关闭弹窗。 */ }
}

export function clearVipCommunityPrompts(userId: number): void {
  try {
    const prefix = `${PREFIX}${userId}:`
    for (let index = sessionStorage.length - 1; index >= 0; index--) {
      const key = sessionStorage.key(index)
      if (key?.startsWith(prefix)) sessionStorage.removeItem(key)
    }
  } catch { /* 存储不可用不得阻止退出登录。 */ }
}
