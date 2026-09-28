<template>
  <BaseDialog :show="identity !== null" :title="t('community.members.detail')" :close-on-escape="!unbinding" :show-close-button="!unbinding" @close="$emit('close')">
    <div v-if="identity" class="space-y-5">
      <div class="flex items-center gap-3">
        <CommunityAvatar :telegram-id="identity.telegram_user_id" :name="identity.telegram_name" @select="load" />
        <div class="min-w-0"><p class="break-words font-semibold">{{ detail?.telegram_name || identity.telegram_name || t('community.chat.unknownSender') }}</p><p v-if="detail?.telegram_username || identity.telegram_username" class="mt-1 break-all text-sm text-gray-500">{{ '@' + (detail?.telegram_username || identity.telegram_username).replace(/^@/, '') }}</p><span v-if="detail?.is_bot || identity.is_bot" class="badge badge-gray mt-1">{{ t('community.members.botAccount') }}</span></div>
      </div>
      <p v-if="loading" role="status" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <dl class="grid gap-4 text-sm sm:grid-cols-2">
        <div><dt class="text-xs text-gray-500">{{ t('community.telegramId') }}</dt><dd class="mt-1 font-mono">{{ identity.telegram_user_id || '—' }}</dd></div>
        <div><dt class="text-xs text-gray-500">{{ t('community.members.membershipStatus') }}</dt><dd class="mt-1"><span v-if="isBanned || member" :class="['badge', isBanned ? 'badge-danger' : member?.status === 'joined' ? 'badge-success' : 'badge-gray']">{{ t('community.members.' + (isBanned ? 'banned' : member?.status)) }}</span><span v-else>{{ t('community.members.notBound') }}</span></dd></div>
        <template v-if="member">
          <div class="sm:col-span-2"><dt class="text-xs text-gray-500">{{ t('community.members.siteIdentity') }}</dt><dd class="mt-1 break-all">{{ member.email }}<span class="mt-1 block text-xs text-gray-500">#{{ member.user_id }} · {{ member.username || '—' }}</span></dd></div>
          <div><dt class="text-xs text-gray-500">{{ t('community.members.userStatus') }}</dt><dd class="mt-1">{{ t('community.members.' + member.user_status) }}</dd></div>
          <div><dt class="text-xs text-gray-500">{{ t('community.members.joinedAt') }}</dt><dd class="mt-1">{{ member.joined_at ? new Date(member.joined_at).toLocaleString(locale) : '—' }}</dd></div>
        </template>
      </dl>
      <p v-if="isBanned" class="rounded-lg bg-red-50 p-3 text-sm leading-6 text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ t('community.members.bannedHint') }}</p>
      <p v-else-if="member?.status === 'left'" class="text-sm text-gray-500">{{ t('community.leftHint') }}</p>
      <p v-if="!identity.telegram_user_id" class="text-sm text-gray-500">{{ t(identity.sender_kind === 'chat' ? 'community.chat.anonymousHint' : 'community.members.notBound') }}</p>
      <form v-if="member?.telegram_user_id && !loading && !error" class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-700" @submit.prevent="unbind">
        <h4 class="text-sm font-semibold">{{ t('community.members.unbindTitle') }}</h4>
        <p class="text-sm leading-6 text-gray-500">{{ t('community.members.unbindHint') }}</p>
        <label class="block"><span class="mb-1 block text-sm">{{ t('community.members.ticketID') }}</span><input v-model="ticketID" name="unbind-ticket-id" inputmode="numeric" pattern="[1-9][0-9]*" required :disabled="unbinding" class="input" :placeholder="t('community.members.ticketPlaceholder')" /></label>
        <label class="flex items-start gap-2 text-sm leading-6"><input v-model="confirmed" name="unbind-confirmed" type="checkbox" class="mt-1 rounded" :disabled="unbinding" /><span>{{ t('community.members.unbindConfirm') }}</span></label>
        <p v-if="unbindError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ unbindError }}</p>
        <button type="submit" class="btn btn-secondary text-red-600 dark:text-red-400" :disabled="unbinding || !confirmed || !validTicketID">{{ t(unbinding ? 'community.members.unbinding' : 'community.members.unbind') }}</button>
      </form>
      <p v-if="success" role="status" class="rounded-lg bg-green-50 p-3 text-sm text-green-700 dark:bg-green-900/20 dark:text-green-300">{{ t('community.members.unbound') }}</p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import CommunityAvatar from './CommunityAvatar.vue'
import { communityAPI, type CommunityMember, type CommunityTelegramUser } from '@/api/community'
import { supportError } from '@/api/support'

const props = defineProps<{ identity: (CommunityTelegramUser & { sender_kind?: string }) | null }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const { t, locale } = useI18n()
const detail = ref<CommunityTelegramUser | null>(null)
const loading = ref(false)
const error = ref('')
const ticketID = ref('')
const confirmed = ref(false)
const unbinding = ref(false)
const unbindError = ref('')
const success = ref(false)
const member = computed<CommunityMember | null>(() => detail.value ? detail.value.member : (success.value ? null : props.identity?.member ?? null))
const isBanned = computed(() => (detail.value?.banned ?? props.identity?.banned) || member.value?.status === 'banned')
const validTicketID = computed(() => /^[1-9]\d*$/.test(ticketID.value) && Number.isSafeInteger(Number(ticketID.value)))
let generation = 0
async function load() {
  if (unbinding.value) return
  const current = ++generation
  detail.value = null
  error.value = ''
  loading.value = false
  if (!props.identity?.telegram_user_id) return
  loading.value = true
  try { const result = await communityAPI.telegramUser(props.identity.telegram_user_id); if (current === generation) detail.value = result }
  catch (cause) { if (current === generation) error.value = supportError(cause, t('community.members.detailFailed')) }
  finally { if (current === generation) loading.value = false }
}
async function unbind() {
  if (unbinding.value || !confirmed.value || !validTicketID.value || !member.value?.telegram_user_id) return
  const current = generation
  unbinding.value = true
  unbindError.value = ''
  try {
    const result = await communityAPI.unbind(member.value.user_id, Number(ticketID.value))
    if (current !== generation) return
    success.value = true
    if (detail.value) detail.value = { ...detail.value, member: null, banned: result.banned }
    emit('changed')
  } catch (cause) { if (current === generation) unbindError.value = supportError(cause, t('community.members.unbindFailed')) }
  finally { if (current === generation) unbinding.value = false }
}
watch(() => props.identity, () => { ticketID.value = ''; confirmed.value = false; unbindError.value = ''; unbinding.value = false; success.value = false; void load() }, { immediate: true })
onBeforeUnmount(() => { generation++ })
</script>
