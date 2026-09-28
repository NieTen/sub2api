<template>
  <section class="overflow-hidden rounded-xl border border-gray-200 bg-white text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100" :aria-label="t('community.chat.title')">
    <header class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 p-5 dark:border-dark-700">
      <div><h2 class="font-semibold">{{ t('community.chat.title') }}</h2><p class="mt-1 max-w-2xl text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('community.chat.description') }}</p></div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="refreshing || loading" data-test="refresh-messages" @click="refresh">{{ t(refreshing ? 'community.refreshing' : 'community.refresh') }}</button>
    </header>
    <p v-if="error" role="alert" class="mx-5 mt-4 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
    <div ref="timeline" class="h-[min(60vh,640px)] min-h-72 space-y-5 overflow-y-auto overscroll-contain p-4 sm:p-5" @scroll="checkBottom">
      <div v-if="hasMore" class="text-center"><button type="button" class="btn btn-secondary btn-sm" :disabled="loadingOlder" data-test="older-messages" @click="older">{{ t(loadingOlder ? 'common.loading' : 'community.chat.older') }}</button></div>
      <p v-if="loading" role="status" class="py-10 text-center text-sm text-gray-500">{{ t('common.loading') }}</p>
      <div v-else-if="!messages.length" class="space-y-2 py-14 text-center"><p class="font-medium">{{ t('community.chat.empty') }}</p><p class="text-sm text-gray-500">{{ t('community.chat.emptyHint') }}</p></div>
      <article v-for="message in messages" :key="message.id" :data-message-id="message.id" class="flex gap-3" :class="message.outgoing ? 'flex-row-reverse' : ''">
        <CommunityAvatar :telegram-id="message.sender_kind === 'user' ? message.telegram_user_id : undefined" :name="senderName(message)" @select="select(message)" />
        <div class="min-w-0 max-w-[calc(100%-52px)] sm:max-w-[85%]">
          <div class="mb-1 flex flex-wrap items-baseline gap-x-2 gap-y-1 text-xs" :class="message.outgoing ? 'justify-end' : ''">
            <button type="button" class="break-words text-left font-medium text-gray-700 hover:underline dark:text-dark-200" @click="select(message)">{{ senderName(message) }}</button>
            <span v-if="message.outgoing" class="text-primary-600 dark:text-primary-400">{{ t('community.chat.sentByBot') }}</span>
            <time :datetime="message.created_at" class="text-gray-400">{{ date(message.created_at) }}</time>
            <span v-if="message.edited_at" class="text-gray-400">{{ t('community.chat.edited') }}</span>
          </div>
          <div class="rounded-xl px-4 py-3" :class="message.outgoing ? 'bg-primary-50 dark:bg-primary-900/20' : 'bg-gray-50 dark:bg-dark-900/60'">
            <span v-if="message.message_type !== 'text'" class="mb-2 block text-xs font-medium text-gray-500">{{ typeLabel(message.message_type) }}</span>
            <p v-if="message.text" class="whitespace-pre-wrap break-words text-sm leading-6 [overflow-wrap:anywhere]">{{ message.text }}</p>
            <CommunityMessageMedia v-if="mediaTypes.includes(message.message_type) || message.media_available || message.file_name" :message="message" :label="typeLabel(message.message_type)" />
            <p v-else-if="!message.text" class="text-sm text-gray-500">{{ t('community.chat.noText') }}</p>
          </div>
        </div>
      </article>
    </div>
    <div v-if="unread > 0" class="border-t border-gray-100 px-5 py-2 text-center dark:border-dark-700"><button type="button" class="text-sm text-primary-600 dark:text-primary-400" @click="scrollBottom">{{ t('community.chat.newMessages', { count: unread }) }}</button></div>
    <form class="space-y-3 border-t border-gray-100 p-4 sm:p-5 dark:border-dark-700" @submit.prevent="send">
      <label class="block"><span class="mb-2 block text-sm font-medium">{{ t('community.chat.compose') }}</span><textarea v-model="draft" name="community-message" rows="3" maxlength="4096" :disabled="sending" class="input resize-y" :placeholder="t('community.chat.placeholder')" /></label>
      <p v-if="sendError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ sendError }}</p>
      <div class="flex flex-wrap items-center justify-between gap-3"><p class="text-xs text-gray-500">{{ t('community.chat.sendHint') }} · {{ draft.length }}/4096</p><button type="submit" class="btn btn-primary" :disabled="sending || !draft.trim()" data-test="send-message">{{ t(sending ? 'community.chat.sending' : 'community.chat.send') }}</button></div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import CommunityAvatar from './CommunityAvatar.vue'
import CommunityMessageMedia from './CommunityMessageMedia.vue'
import { communityAPI, type CommunityChatMessage, type CommunityTelegramUser } from '@/api/community'
import { supportError } from '@/api/support'
const emit = defineEmits<{ select: [identity: CommunityTelegramUser & { sender_kind: string }] }>()
const props = withDefaults(defineProps<{ active?: boolean }>(), { active: true })
const { t, locale } = useI18n()
const messages = ref<CommunityChatMessage[]>([])
const timeline = ref<HTMLElement | null>(null)
const loading = ref(true)
const refreshing = ref(false)
const loadingOlder = ref(false)
const hasMore = ref(false)
const error = ref('')
const draft = ref('')
const sending = ref(false)
const sendError = ref('')
const unread = ref(0)
const mediaTypes = ['photo', 'video', 'animation', 'audio', 'voice', 'document', 'sticker', 'video_note']
const knownTypes = ['text', ...mediaTypes, 'contact', 'location', 'venue', 'poll', 'dice', 'service', 'unsupported']
let disposed = false
let latestID = 0
let atBottom = true
let timer: ReturnType<typeof setInterval> | undefined
let pendingSend: { text: string; id: string } | null = null
const date = (value: string) => new Date(value).toLocaleString(locale.value)
const typeLabel = (type: string) => t('community.chat.types.' + (knownTypes.includes(type) ? type : 'unsupported'))
const senderName = (message: CommunityChatMessage) => message.telegram_name || message.telegram_username || t('community.chat.unknownSender')
function select(message: CommunityChatMessage) {
  emit('select', { telegram_user_id: message.sender_kind === 'user' ? message.telegram_user_id : 0, telegram_name: senderName(message), telegram_username: message.telegram_username, member: null, banned: false, is_bot: message.is_bot, sender_kind: message.sender_kind })
}
function merge(items: CommunityChatMessage[]) {
  const map = new Map(messages.value.map(item => [item.id, item]))
  const added = items.filter(item => !map.has(item.id)).length
  for (const item of items) map.set(item.id, item)
  messages.value = [...map.values()].sort((a, b) => a.id - b.id)
  return added
}
function checkBottom() {
  const element = timeline.value
  atBottom = !element || element.scrollHeight - element.scrollTop - element.clientHeight < 64
  if (atBottom) unread.value = 0
}
async function scrollBottom() {
  await nextTick()
  if (timeline.value) timeline.value.scrollTop = timeline.value.scrollHeight
  unread.value = 0
  atBottom = true
}
async function initial() {
  try {
    const data = await communityAPI.messages()
    if (disposed) return
    merge(data.items)
    latestID = data.latest_id
    hasMore.value = data.has_more
    await scrollBottom()
  } catch (cause) { if (!disposed) error.value = supportError(cause, t('community.chat.loadFailed')) }
  finally { if (!disposed) loading.value = false }
}
async function refresh() {
  if (loading.value || refreshing.value || disposed) return
  if (!messages.value.length && latestID === 0) { loading.value = true; error.value = ''; await initial(); return }
  refreshing.value = true
  error.value = ''
  try {
    let more = true
    while (more && !disposed) {
      const data = await communityAPI.messages({ after_id: latestID })
      if (disposed) return
      const added = merge(data.items)
      const nextID = data.items.length ? Math.max(...data.items.map(item => item.id)) : latestID
      more = data.has_more && nextID > latestID
      latestID = Math.max(latestID, nextID)
      if (atBottom) await scrollBottom()
      else unread.value += added
    }
  } catch (cause) { if (!disposed) error.value = supportError(cause, t('community.chat.loadFailed')) }
  finally { if (!disposed) refreshing.value = false }
}
async function older() {
  if (loadingOlder.value || !messages.value.length || !hasMore.value) return
  loadingOlder.value = true
  error.value = ''
  const previousHeight = timeline.value?.scrollHeight || 0
  const previousTop = timeline.value?.scrollTop || 0
  try {
    const data = await communityAPI.messages({ before_id: messages.value[0].id })
    if (disposed) return
    merge(data.items)
    hasMore.value = data.has_more
    await nextTick()
    if (timeline.value) timeline.value.scrollTop = previousTop + timeline.value.scrollHeight - previousHeight
  } catch (cause) { if (!disposed) error.value = supportError(cause, t('community.chat.loadFailed')) }
  finally { if (!disposed) loadingOlder.value = false }
}
async function send() {
  const text = draft.value.trim()
  if (!text || text.length > 4096 || sending.value) return
  if (pendingSend?.text !== text) {
    const id = typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : [...crypto.getRandomValues(new Uint8Array(16))].map(value => value.toString(16).padStart(2, '0')).join('')
    pendingSend = { text, id }
  }
  sending.value = true
  sendError.value = ''
  try {
    const message = await communityAPI.sendMessage(text, pendingSend.id)
    if (disposed) return
    merge([message])
    // 增量游标只由读取响应推进，避免发送期间漏掉其他用户的消息。
    draft.value = ''
    pendingSend = null
    await scrollBottom()
  } catch (cause) {
    if (!disposed) {
      sendError.value = supportError(cause, t('community.chat.sendFailed'))
      // Telegram 明确拒绝时允许新请求重试；结果不确定时保留原请求编号。
      if (typeof cause === 'object' && cause && 'reason' in cause && cause.reason === 'COMMUNITY_CHAT_SEND_FAILED') pendingSend = null
    }
  }
  finally { if (!disposed) sending.value = false }
}
watch(() => props.active, active => { if (active) void refresh() })
onMounted(() => { void initial(); timer = setInterval(() => { if (props.active && document.visibilityState === 'visible') void refresh() }, 5000) })
onBeforeUnmount(() => { disposed = true; if (timer) clearInterval(timer) })
</script>
