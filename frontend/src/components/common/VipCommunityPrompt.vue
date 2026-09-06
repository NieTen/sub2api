<template>
  <BaseDialog :show="visible" :title="t('community.vipPromptTitle')" width="narrow" @close="dismiss">
    <p class="text-sm leading-6 text-gray-600 dark:text-dark-300">{{ t('community.vipPromptBody', { group: state?.group_name || t('community.vipGroup') }) }}</p>
    <template #footer>
      <button type="button" class="btn btn-secondary" data-test="vip-later" @click="dismiss">{{ t('community.vipPromptLater') }}</button>
      <button type="button" class="btn btn-primary" data-test="vip-join" @click="join">{{ t('community.vipPromptAction') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { communityAPI, type CommunityState } from '@/api/community'
import { useAuthStore } from '@/stores/auth'
import { useAnnouncementStore } from '@/stores/announcements'
import { useAdminComplianceStore } from '@/stores/adminCompliance'
import { rememberVipCommunityPrompt, vipCommunityPromptKey, wasVipCommunityPromptDismissed } from '@/utils/vipCommunityPrompt'

const props = withDefaults(defineProps<{ blocked?: boolean }>(), { blocked: false })
const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const announcements = useAnnouncementStore()
const compliance = useAdminComplianceStore()
const state = ref<CommunityState | null>(null)
const dismissed = ref(false)
const stateUserId = ref<number | null>(null)
let generation = 0

const userId = computed(() => auth.isAuthenticated ? auth.user?.id ?? null : null)
const storageKey = computed(() => userId.value !== null && state.value?.prompt_key
  ? vipCommunityPromptKey(userId.value, state.value.prompt_key) : '')
const visible = computed(() => userId.value !== null && stateUserId.value === userId.value
  && state.value?.enabled === true && state.value.require_paid_recharge === true
  && state.value.eligible === true && state.value.show_join_prompt === true
  && state.value.membership?.status !== 'joined' && !!storageKey.value && !dismissed.value
  && !props.blocked && !announcements.popupBlocking
  && !(auth.isAdmin && (compliance.loading || compliance.shouldShow)))

function dismiss(): void {
  if (!visible.value) return
  rememberVipCommunityPrompt(storageKey.value)
  dismissed.value = true
}

function join(): void {
  if (!visible.value) return
  dismiss()
  // 弹窗只导航到领取页面，个人邀请必须由用户在页面内主动领取。
  void router.push('/community')
}

watch(userId, async (id) => {
  const current = ++generation
  state.value = null
  stateUserId.value = null
  dismissed.value = false
  if (id === null) return
  try {
    const data = await communityAPI.get()
    // 退出或切换账号后，旧账号的响应不可写入当前弹窗。
    if (current !== generation || userId.value !== id) return
    state.value = data
    stateUserId.value = id
    dismissed.value = !!storageKey.value && wasVipCommunityPromptDismissed(storageKey.value)
  } catch { /* 提醒加载失败时静默跳过，不影响登录和正常页面。 */ }
}, { immediate: true, flush: 'sync' })

onBeforeUnmount(() => { generation++ })
</script>
