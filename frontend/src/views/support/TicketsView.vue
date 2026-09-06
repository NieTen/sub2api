<template>
  <component :is="admin ? CommunicationsLayout : AppLayout" :active="admin ? 'tickets' : undefined">
    <div class="space-y-5 text-gray-900 dark:text-gray-100">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <p class="max-w-2xl text-sm text-gray-500 dark:text-dark-400">{{ t(admin ? 'support.adminDescription' : 'support.description') }}</p>
        <div class="flex gap-2">
          <router-link v-if="admin" to="/admin/communications" class="btn btn-secondary">{{ t('support.settings') }}</router-link>
          <button v-else type="button" class="btn btn-primary" :disabled="busy || uploading" @click="creating = true">{{ t('support.newTicket') }}</button>
        </div>
      </div>

      <div class="grid items-start gap-5 lg:grid-cols-[minmax(260px,340px)_minmax(0,1fr)]">
        <section class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" :class="selectedId ? 'hidden lg:block' : ''" :aria-label="t(admin ? 'support.adminTitle' : 'support.title')">
          <div class="flex items-center gap-2 border-b border-gray-100 p-4 dark:border-dark-700">
            <select v-model="statusFilter" class="input flex-1" :aria-label="t('bulkMail.status')" @change="changeFilter">
              <option value="">{{ t('support.allStatus') }}</option><option value="open">{{ t('support.open') }}</option><option value="closed">{{ t('support.closed') }}</option>
            </select>
            <button type="button" class="btn btn-secondary" :disabled="loadingList" :aria-label="t('support.refresh')" @click="loadList()">↻</button>
          </div>
          <div v-if="listError" role="alert" class="p-4 text-sm text-red-600">{{ listError }}</div>
          <div v-else-if="loadingList && !tickets.length" class="p-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
          <div v-else-if="!tickets.length" class="space-y-2 p-8 text-center">
            <p class="font-medium">{{ t('support.noTickets') }}</p>
            <p class="text-sm text-gray-500">{{ t(admin ? 'support.emptyAdminHint' : 'support.emptyHint') }}</p>
          </div>
          <ul class="divide-y divide-gray-100 dark:divide-dark-700">
            <li v-for="ticket in tickets" :key="ticket.id">
              <router-link :to="`${basePath}/${ticket.id}`" class="block border-l-[3px] p-4 transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500 dark:hover:bg-dark-700" :class="ticket.id === selectedId ? 'border-primary-500 bg-primary-50/60 dark:bg-primary-900/10' : 'border-transparent'">
                <div class="mb-2 flex items-center justify-between gap-2">
                  <span class="font-mono text-xs text-gray-500">#{{ ticket.id }}</span>
                  <span :class="['badge', ticket.status === 'open' ? 'badge-success' : 'badge-gray']">{{ t(`support.${ticket.status}`) }}</span>
                </div>
                <p class="break-words text-sm font-semibold">{{ ticket.subject }}</p>
                <p v-if="admin" class="mt-1 truncate text-xs text-gray-500">{{ ticket.user_email || ticket.username }}</p>
                <time class="mt-2 block text-xs text-gray-500" :datetime="ticket.last_message_at">{{ date(ticket.last_message_at || ticket.updated_at) }}</time>
              </router-link>
            </li>
          </ul>
          <div v-if="total > 20" class="flex items-center justify-between border-t border-gray-100 p-3 text-sm dark:border-dark-700">
            <button class="btn btn-secondary btn-sm" :disabled="page <= 1 || loadingList" @click="changePage(page - 1)">{{ t('pagination.previous') }}</button>
            <span>{{ page }} / {{ Math.ceil(total / 20) }}</span>
            <button class="btn btn-secondary btn-sm" :disabled="page * 20 >= total || loadingList" @click="changePage(page + 1)">{{ t('pagination.next') }}</button>
          </div>
        </section>

        <section class="min-w-0 overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" :class="!selectedId ? 'hidden lg:block' : ''" :aria-label="t('support.content')">
          <div v-if="!selectedId" class="flex min-h-[460px] items-center justify-center p-8 text-sm text-gray-500">{{ t('support.selectTicket') }}</div>
          <template v-else>
            <div class="border-b border-gray-100 p-5 dark:border-dark-700">
              <router-link :to="basePath" class="mb-3 inline-block text-sm text-primary-600 lg:hidden">← {{ t('support.back') }}</router-link>
              <div v-if="detail" class="flex flex-wrap items-start justify-between gap-3">
                <div class="min-w-0 flex-1">
                  <div class="mb-2 flex items-center gap-2 text-xs text-gray-500"><span class="font-mono">#{{ detail.ticket.id }}</span><span :class="['badge', detail.ticket.status === 'open' ? 'badge-success' : 'badge-gray']">{{ t(`support.${detail.ticket.status}`) }}</span></div>
                  <h2 class="break-words text-lg font-semibold">{{ detail.ticket.subject }}</h2>
                  <p v-if="admin" class="mt-1 break-all text-sm text-gray-500">{{ detail.ticket.user_email }}</p>
                </div>
                <button class="btn btn-secondary btn-sm" :disabled="busy" @click="toggleStatus">{{ t(detail.ticket.status === 'closed' ? 'support.reopen' : 'support.close') }}</button>
              </div>
            </div>
            <p v-if="detailError" role="alert" class="p-5 text-sm text-red-600">{{ detailError }} <button class="underline" @click="loadDetail">{{ t('support.refresh') }}</button></p>
            <p v-if="loadingDetail" class="p-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</p>
            <template v-if="detail">
              <ol class="max-h-[65vh] space-y-5 overflow-y-auto p-5" aria-live="polite">
                <li v-for="message in detail.messages" :key="message.id" class="rounded-xl border p-4" :class="message.sender_role === 'admin' ? 'border-primary-100 bg-primary-50/60 dark:border-primary-900/40 dark:bg-primary-900/10' : 'border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-900/40'">
                  <div class="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500">
                    <span class="font-semibold text-gray-700 dark:text-gray-200">{{ t(`support.${message.sender_role}`) }}</span>
                    <span class="rounded border border-gray-200 px-1.5 py-0.5 dark:border-dark-600">{{ t(`support.${message.source}`) }}</span>
                    <time :datetime="message.created_at">{{ date(message.created_at) }}</time>
                  </div>
                  <p v-if="message.content" class="whitespace-pre-wrap break-words text-sm leading-6 [overflow-wrap:anywhere]">{{ message.content }}</p>
                  <div v-if="message.attachments?.length" class="mt-3 flex flex-wrap gap-3"><SupportImage v-for="attachment in message.attachments" :key="attachment.id" :attachment="attachment" :admin="admin" /></div>
                </li>
              </ol>
              <button v-if="detail.has_more" class="btn btn-secondary mx-5 mb-5" :disabled="loadingMore" @click="loadMore()">{{ t(loadingMore ? 'common.loading' : 'support.loadMore') }}</button>
              <div class="border-t border-gray-100 p-5 dark:border-dark-700">
                <p v-if="detail.ticket.status === 'closed'" class="text-sm text-gray-500">{{ t('support.closedHint') }}</p>
                <form v-else class="space-y-4" @submit.prevent="sendReply">
                  <SupportComposer :key="detail.ticket.id" v-model="reply" v-model:attachments="replyAttachments" :admin="admin" :disabled="busy" @busy="uploading = $event" />
                  <div class="flex justify-end"><button class="btn btn-primary" :disabled="busy || uploading || (!reply.trim() && !replyAttachments.length)">{{ t(busy ? 'support.sending' : 'support.send') }}</button></div>
                </form>
              </div>
            </template>
          </template>
        </section>
      </div>
    </div>

    <BaseDialog :show="creating" :title="t('support.newTicket')" width="wide" :close-on-escape="!busy && !uploading" :show-close-button="!busy && !uploading" @close="creating = false">
      <form id="create-support-ticket" class="space-y-4" @submit.prevent="createTicket">
        <label class="block"><span class="mb-1 block text-sm font-medium">{{ t('support.subject') }}</span><input v-model="subject" :disabled="busy" class="input" required maxlength="200" :placeholder="t('support.subjectPlaceholder')" /></label>
        <SupportComposer v-if="creating" v-model="initialContent" v-model:attachments="initialAttachments" :disabled="busy" @busy="uploading = $event" />
      </form>
      <template #footer><button class="btn btn-secondary" :disabled="busy || uploading" @click="creating = false">{{ t('common.cancel') }}</button><button form="create-support-ticket" class="btn btn-primary" :disabled="busy || uploading || !subject.trim() || (!initialContent.trim() && !initialAttachments.length)">{{ t(busy ? 'support.sending' : 'support.create') }}</button></template>
    </BaseDialog>
  </component>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import CommunicationsLayout from '@/components/support/CommunicationsLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import SupportComposer from '@/components/support/SupportComposer.vue'
import SupportImage from '@/components/support/SupportImage.vue'
import { supportAPI, supportError, type SupportAttachment, type SupportTicket, type SupportTicketDetail } from '@/api/support'
import { useAppStore } from '@/stores/app'

const props = withDefaults(defineProps<{ admin?: boolean }>(), { admin: false })
const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const app = useAppStore()
const basePath = computed(() => props.admin ? '/admin/tickets' : '/tickets')
const selectedId = computed(() => Number(route.params.id) || 0)
const tickets = ref<SupportTicket[]>([])
const detail = ref<SupportTicketDetail | null>(null)
const statusFilter = ref('')
const page = ref(1)
const total = ref(0)
const loadingList = ref(false)
const loadingDetail = ref(false)
const loadingMore = ref(false)
const listError = ref('')
const detailError = ref('')
const busy = ref(false)
const uploading = ref(false)
const creating = ref(false)
const subject = ref('')
const initialContent = ref('')
const initialAttachments = ref<SupportAttachment[]>([])
const reply = ref('')
const replyAttachments = ref<SupportAttachment[]>([])
let listGeneration = 0
let detailGeneration = 0
let poll: ReturnType<typeof setInterval> | undefined

const date = (value: string) => value ? new Date(value).toLocaleString(locale.value) : '—'

async function loadList(silent = false) {
  const generation = ++listGeneration
  if (!silent) loadingList.value = true
  try {
    const data = await supportAPI.list(props.admin, page.value, statusFilter.value)
    if (generation !== listGeneration) return
    tickets.value = data.items || []
    total.value = data.total
    listError.value = ''
  } catch (error) {
    if (generation === listGeneration && !silent) listError.value = supportError(error, t('support.loadFailed'))
  } finally {
    if (generation === listGeneration) loadingList.value = false
  }
}

function changeFilter() { page.value = 1; void loadList() }
function changePage(value: number) { page.value = value; void loadList() }

async function loadDetail() {
  const generation = ++detailGeneration
  detail.value = null
  detailError.value = ''
  reply.value = ''
  replyAttachments.value = []
  uploading.value = false
  loadingMore.value = false
  if (!selectedId.value) return
  loadingDetail.value = true
  try {
    const data = await supportAPI.detail(props.admin, selectedId.value)
    if (generation === detailGeneration) detail.value = data
  } catch (error) {
    if (generation === detailGeneration) detailError.value = supportError(error, t('support.loadFailed'))
  } finally {
    if (generation === detailGeneration) loadingDetail.value = false
  }
}

async function loadMore(silent = false) {
  if (!detail.value || loadingMore.value) return
  const generation = detailGeneration
  const current = detail.value
  loadingMore.value = true
  try {
    const data = await supportAPI.detail(props.admin, current.ticket.id, current.messages[current.messages.length - 1]?.id || 0)
    if (generation !== detailGeneration || !detail.value) return
    const seen = new Set(detail.value.messages.map(message => message.id))
    detail.value.messages.push(...(data.messages || []).filter(message => !seen.has(message.id)))
    detail.value.ticket = data.ticket
    detail.value.has_more = data.has_more
    detailError.value = ''
  } catch (error) {
    if (!silent && generation === detailGeneration) detailError.value = supportError(error, t('support.loadFailed'))
  } finally {
    if (generation === detailGeneration) loadingMore.value = false
  }
}

async function createTicket() {
  if (busy.value || uploading.value) return
  if (!subject.value.trim() || (!initialContent.value.trim() && !initialAttachments.value.length)) {
    app.showError(t('support.required')); return
  }
  busy.value = true
  try {
    const data = await supportAPI.create(subject.value.trim(), initialContent.value.trim(), initialAttachments.value.map(item => item.id))
    creating.value = false
    subject.value = ''
    initialContent.value = ''
    initialAttachments.value = []
    page.value = 1
    statusFilter.value = ''
    app.showSuccess(t('support.created'))
    await router.push(`${basePath.value}/${data.ticket.id}`)
    await loadList()
  } catch (error) {
    app.showError(supportError(error, t('support.saveFailed')))
  } finally { busy.value = false }
}

async function sendReply() {
  if (!detail.value || busy.value || uploading.value || (!reply.value.trim() && !replyAttachments.value.length)) return
  const id = detail.value.ticket.id
  const generation = detailGeneration
  busy.value = true
  try {
    const message = await supportAPI.reply(props.admin, id, reply.value.trim(), replyAttachments.value.map(item => item.id))
    if (generation === detailGeneration) {
      reply.value = ''
      replyAttachments.value = []
      // 按游标补齐到本次回复，避免历史分页尚未加载时出现消息丢失的错觉。
      while (detail.value && generation === detailGeneration) {
        const messages = detail.value.messages
        const after = messages[messages.length - 1]?.id || 0
        if (after >= message.id) break
        const data = await supportAPI.detail(props.admin, id, after)
        if (generation !== detailGeneration || !detail.value) break
        const seen = new Set(detail.value.messages.map(item => item.id))
        detail.value.messages.push(...(data.messages || []).filter(item => !seen.has(item.id)))
        detail.value.ticket = data.ticket
        detail.value.has_more = data.has_more
        if (!data.has_more || !data.messages?.length) break
      }
    }
    app.showSuccess(t('support.sent'))
    await loadList(true)
  } catch (error) { app.showError(supportError(error, t('support.saveFailed'))) }
  finally { busy.value = false }
}

async function toggleStatus() {
  if (!detail.value || busy.value) return
  const current = detail.value.ticket
  const generation = detailGeneration
  busy.value = true
  try {
    const ticket = await supportAPI.status(props.admin, current.id, current.status === 'open' ? 'closed' : 'open')
    if (generation === detailGeneration && detail.value) detail.value.ticket = ticket
    app.showSuccess(t('support.statusSaved'))
    await loadList(true)
  } catch (error) { app.showError(supportError(error, t('support.saveFailed'))) }
  finally { busy.value = false }
}

watch([selectedId, () => props.admin], () => { void loadDetail() }, { immediate: true })
watch(() => props.admin, () => { page.value = 1; statusFilter.value = ''; void loadList() })
onMounted(() => {
  void loadList()
  poll = setInterval(() => {
    if (document.hidden || busy.value || loadingDetail.value) return
    if (detail.value && !detail.value.has_more) void loadMore(true)
    void loadList(true)
  }, 10000)
})
onBeforeUnmount(() => {
  listGeneration++
  detailGeneration++
  if (poll) clearInterval(poll)
})
</script>
