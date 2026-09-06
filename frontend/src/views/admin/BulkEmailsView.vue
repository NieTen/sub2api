<template>
  <CommunicationsLayout active="mail">
    <div class="space-y-5 text-gray-900 dark:text-gray-100">
      <p class="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-gray-200 bg-white px-4 py-3 text-xs leading-5 text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400">
        {{ t('communications.mailSetupHint') }}
        <router-link to="/admin/settings" class="font-medium text-primary-600 underline dark:text-primary-400">{{ t('communications.configureMail') }}</router-link>
      </p>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="max-w-2xl text-sm text-gray-500 dark:text-dark-400">{{ t('bulkMail.description') }}</p>
        <div class="flex gap-2"><button class="btn btn-secondary" :disabled="loading" @click="loadBatches()">{{ t('support.refresh') }}</button><button class="btn btn-primary" :disabled="busy || uploading" @click="composing = !composing">{{ t(composing ? 'common.cancel' : 'bulkMail.new') }}</button></div>
      </div>

      <form v-if="composing" class="space-y-5 rounded-xl border border-primary-100 bg-white p-5 dark:border-dark-700 dark:bg-dark-800" @submit.prevent="createDraft">
        <h2 class="text-lg font-semibold">{{ t('bulkMail.new') }}</h2>
        <div class="grid gap-6 xl:grid-cols-2">
          <fieldset :disabled="busy" class="min-w-0 space-y-4">
            <label class="block"><span class="mb-1 block text-sm font-medium">{{ t('bulkMail.subject') }}</span><input v-model="subject" class="input" required maxlength="200" /></label>
            <SupportComposer v-model="body" v-model:attachments="attachments" :admin="true" :disabled="busy" :label="t('bulkMail.body')" @busy="uploading = $event" />
            <p class="text-xs text-gray-500">{{ t('bulkMail.bodyHint') }}</p>
          </fieldset>
          <fieldset :disabled="busy" class="min-w-0 space-y-3">
            <legend class="mb-3 text-sm font-semibold">{{ t('bulkMail.recipients') }}</legend>
            <div class="flex flex-wrap gap-4 text-sm"><label class="flex items-center gap-2"><input v-model="allActive" type="radio" :value="false" name="recipient-mode" />{{ t('bulkMail.selectedUsers') }}</label><label class="flex items-center gap-2"><input v-model="allActive" type="radio" :value="true" name="recipient-mode" />{{ t('bulkMail.allActive') }}</label></div>
            <div class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600">
              <h3 class="text-sm font-semibold">{{ t('bulkMail.filters.title') }}</h3>
              <div class="grid gap-3 sm:grid-cols-2">
                <div class="space-y-2">
                  <label class="block">
                    <span class="mb-1 block text-xs font-medium">{{ t('bulkMail.filters.balance') }}</span>
                    <select v-model="balanceCondition" name="balance-condition" class="input" @change="filterError = ''">
                      <option value="all">{{ t('bulkMail.filters.all') }}</option>
                      <option value="positive">{{ t('bulkMail.filters.positive') }}</option>
                      <option value="non_positive">{{ t('bulkMail.filters.nonPositive') }}</option>
                      <option value="greater_than">{{ t('bulkMail.filters.greaterThan') }}</option>
                    </select>
                  </label>
                  <label v-if="balanceCondition === 'greater_than'" class="block">
                    <span class="mb-1 block text-xs font-medium">{{ t('bulkMail.filters.amount') }}</span>
                    <input v-model="balanceThreshold" name="balance-threshold" type="text" inputmode="decimal" class="input" maxlength="21" pattern="[0-9]{1,12}(\.[0-9]{1,8})?" required :placeholder="'0.00'" @input="filterError = ''" />
                  </label>
                </div>
                <div class="space-y-2">
                  <label class="block">
                    <span class="mb-1 block text-xs font-medium">{{ t('bulkMail.filters.recharge') }}</span>
                    <select v-model="rechargeCondition" name="recharge-condition" class="input" @change="filterError = ''">
                      <option value="all">{{ t('bulkMail.filters.all') }}</option>
                      <option value="recharged">{{ t('bulkMail.filters.recharged') }}</option>
                    </select>
                  </label>
                </div>
              </div>
              <p v-if="balanceCondition === 'greater_than'" class="text-xs text-gray-500">{{ t('bulkMail.filters.amountHint') }}</p>
              <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('bulkMail.filters.help') }}</p>
              <p v-if="filterError" role="alert" data-test="filter-error" class="text-sm text-red-600 dark:text-red-400">{{ filterError }}</p>
            </div>
            <p v-if="allActive" class="rounded-lg bg-primary-50 p-4 text-sm leading-6 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">{{ t('bulkMail.allActiveHint') }}</p>
            <template v-else>
              <input v-model="userSearch" type="search" class="input" :placeholder="t('bulkMail.searchUsers')" :aria-label="t('bulkMail.searchUsers')" @input="searchUsers" />
              <p v-if="usersError" role="alert" class="text-sm text-red-600">{{ usersError }} <button type="button" class="underline" @click="loadUsers">{{ t('support.refresh') }}</button></p>
              <p v-if="usersLoading" class="p-3 text-sm text-gray-500">{{ t('common.loading') }}</p>
              <ul v-else class="max-h-64 divide-y divide-gray-100 overflow-y-auto rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-600">
                <li v-for="user in users" :key="user.id"><label class="flex cursor-pointer items-center gap-3 px-3 py-2 hover:bg-gray-50 dark:hover:bg-dark-700"><input type="checkbox" :checked="selectedUsers.has(user.id)" @change="toggleUser(user)" /><span class="min-w-0"><span class="block truncate text-sm">{{ user.email }}</span><span class="block truncate text-xs text-gray-500">{{ user.username || `#${user.id}` }}</span><span class="mt-1 block text-xs text-gray-500">{{ t('bulkMail.filters.currentBalance') }}: ${{ user.balance }}</span></span></label></li>
                <li v-if="!users.length" class="p-4 text-sm text-gray-500">{{ t('bulkMail.noUsers') }}</li>
              </ul>
              <div v-if="usersTotal > 20" class="flex items-center justify-between gap-2 text-xs"><button type="button" class="btn btn-secondary btn-sm" :disabled="userPage === 1 || usersLoading" @click="changeUserPage(userPage - 1)">{{ t('pagination.previous') }}</button><span>{{ userPage }} / {{ Math.ceil(usersTotal / 20) }}</span><button type="button" class="btn btn-secondary btn-sm" :disabled="userPage * 20 >= usersTotal || usersLoading" @click="changeUserPage(userPage + 1)">{{ t('pagination.next') }}</button></div>
              <p class="text-sm font-medium">{{ t('bulkMail.selectedCount', { count: selectedUsers.size }) }}</p>
              <div v-if="selectedUsers.size" class="flex max-h-24 flex-wrap gap-2 overflow-y-auto"><button v-for="user in selectedUsers.values()" :key="user.id" type="button" class="max-w-full truncate rounded-full bg-gray-100 px-2 py-1 text-xs dark:bg-dark-700" :aria-label="`${t('common.remove')}: ${user.email}`" @click="toggleUser(user)">{{ user.email }} ×</button></div>
            </template>
          </fieldset>
        </div>
        <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"><button class="btn btn-primary" :disabled="busy || uploading || !subject.trim() || (!body.trim() && !attachments.length) || (!allActive && !selectedUsers.size)">{{ t(busy ? 'bulkMail.sending' : 'bulkMail.review') }}</button></div>
      </form>

      <section class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <h2 class="border-b border-gray-100 p-5 text-base font-semibold dark:border-dark-700">{{ t('bulkMail.history') }}</h2>
        <p v-if="listError" role="alert" class="p-5 text-sm text-red-600">{{ listError }}</p>
        <p v-else-if="loading && !batches.length" class="p-8 text-center text-gray-500">{{ t('common.loading') }}</p>
        <div v-else-if="!batches.length" class="space-y-2 p-10 text-center"><p class="font-medium">{{ t('bulkMail.noBatches') }}</p><p class="text-sm text-gray-500">{{ t('bulkMail.noBatchesHint') }}</p></div>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/50"><tr><th class="px-5 py-3">{{ t('bulkMail.subject') }}</th><th class="px-4 py-3">{{ t('bulkMail.status') }}</th><th class="px-4 py-3">{{ t('bulkMail.progress') }}</th><th class="px-4 py-3">{{ t('bulkMail.total') }}</th><th class="px-4 py-3">{{ t('bulkMail.failed') }}</th></tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="batch in batches" :key="batch.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
              <td class="max-w-sm px-5 py-4"><router-link :to="`/admin/bulk-emails/${batch.id}`" class="block break-words font-medium text-primary-600 hover:underline dark:text-primary-400">{{ batch.subject }}</router-link><p class="mt-1 text-xs text-gray-500">#{{ batch.id }} · {{ date(batch.created_at) }}</p></td>
              <td class="px-4 py-4"><span :class="['badge', statusClass(batch.status)]">{{ statusLabel(batch.status) }}</span></td>
              <td class="min-w-32 px-4 py-4"><div class="mb-1 text-xs text-gray-500">{{ batch.sent_count }} / {{ batch.total_count }}</div><progress class="h-1.5 w-full accent-primary-500" :value="batch.sent_count + batch.failed_count" :max="batch.total_count || 1" :aria-label="t('bulkMail.progress')"></progress></td>
              <td class="px-4 py-4 tabular-nums">{{ batch.total_count }}</td><td class="px-4 py-4 tabular-nums" :class="batch.failed_count ? 'text-red-600' : 'text-gray-500'">{{ batch.failed_count }}</td>
            </tr></tbody>
          </table>
        </div>
        <Pagination v-if="total > 0" :page="page" :page-size="20" :total="total" :show-page-size-selector="false" @update:page="changePage" />
      </section>
    </div>

    <BaseDialog :show="!!selectedId" :title="t(detail?.batch.status === 'draft' ? 'bulkMail.reviewTitle' : 'bulkMail.details')" width="extra-wide" :show-close-button="!busy" :close-on-escape="!busy" @close="closeDetail">
      <p v-if="detailError" role="alert" class="mb-4 text-sm text-red-600">{{ detailError }} <button class="underline" @click="loadDetail()">{{ t('support.refresh') }}</button></p>
      <p v-if="detailLoading" class="p-8 text-center text-gray-500">{{ t('common.loading') }}</p>
      <div v-if="detail" class="space-y-5">
        <p v-if="detail.batch.status === 'draft'" class="rounded-lg bg-amber-50 p-4 text-sm font-medium text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">{{ t('bulkMail.reviewHint', { count: detail.batch.total_count }) }}</p>
        <div class="rounded-lg border border-gray-200 px-4 py-3 dark:border-dark-600">
          <p class="text-xs text-gray-500">{{ t('bulkMail.filters.summary') }}</p>
          <p data-test="recipient-filter-summary" class="mt-1 text-sm font-medium">{{ recipientFilterSummary(detail.batch.recipient_filter) }}</p>
        </div>
        <div class="flex flex-wrap gap-5 rounded-lg bg-gray-50 p-4 dark:bg-dark-900/50"><div><span class="text-xs text-gray-500">{{ t('bulkMail.total') }}</span><p class="text-lg font-semibold tabular-nums">{{ detail.batch.total_count }}</p></div><div><span class="text-xs text-gray-500">{{ t('bulkMail.sent') }}</span><p class="text-lg font-semibold tabular-nums text-green-600">{{ detail.batch.sent_count }}</p></div><div><span class="text-xs text-gray-500">{{ t('bulkMail.failed') }}</span><p class="text-lg font-semibold tabular-nums text-red-600">{{ detail.batch.failed_count }}</p></div><span :class="['badge ml-auto self-center', statusClass(detail.batch.status)]">{{ statusLabel(detail.batch.status) }}</span></div>
        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"><h3 class="break-words font-semibold">{{ detail.batch.subject }}</h3><p class="mt-3 whitespace-pre-wrap break-words text-sm leading-6 [overflow-wrap:anywhere]">{{ detail.batch.body }}</p><div v-if="detail.batch.attachments?.length" class="mt-4 flex flex-wrap gap-3"><SupportImage v-for="attachment in detail.batch.attachments" :key="attachment.id" :attachment="attachment" :admin="true" :bulk-id="detail.batch.id" /></div></div>
        <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600"><div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/50"><tr><th class="px-4 py-3">{{ t('bulkMail.email') }}</th><th class="px-4 py-3">{{ t('bulkMail.status') }}</th><th class="px-4 py-3">{{ t('bulkMail.attempts') }}</th><th class="px-4 py-3">{{ t('bulkMail.lastError') }}</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="recipient in detail.recipients.items" :key="recipient.id"><td class="px-4 py-3">{{ recipient.email }}</td><td class="px-4 py-3"><span :class="['badge', statusClass(recipient.status)]">{{ statusLabel(recipient.status) }}</span></td><td class="px-4 py-3 tabular-nums">{{ recipient.attempts }}</td><td class="max-w-sm break-words px-4 py-3 text-xs text-red-600">{{ recipient.last_error || '—' }}</td></tr></tbody></table></div><Pagination v-if="detail.recipients.total > 0" :page="recipientPage" :page-size="20" :total="detail.recipients.total" :show-page-size-selector="false" @update:page="changeRecipientPage" /></div>
        <p v-if="detail.batch.failed_count > 0" class="text-xs text-gray-500">{{ t('bulkMail.retryHint') }}</p>
      </div>
      <template #footer>
        <button class="btn btn-secondary" :disabled="busy" @click="closeDetail">{{ t(detail?.batch.status === 'draft' ? 'bulkMail.cancelReview' : 'common.close') }}</button>
        <button v-if="detail?.batch.status === 'draft'" class="btn btn-primary" :disabled="busy || !detail.batch.total_count" @click="startBatch">{{ t(busy ? 'bulkMail.sending' : 'bulkMail.send') }}</button>
        <button v-else-if="detail && detail.batch.failed_count > 0 && !activeStatus(detail.batch.status)" class="btn btn-primary" :disabled="busy" @click="retryBatch">{{ t(busy ? 'bulkMail.sending' : 'bulkMail.retry') }}</button>
      </template>
    </BaseDialog>
  </CommunicationsLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import CommunicationsLayout from '@/components/support/CommunicationsLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import SupportComposer from '@/components/support/SupportComposer.vue'
import SupportImage from '@/components/support/SupportImage.vue'
import { bulkEmailAPI, supportError, type BulkEmailBatch, type BulkEmailDetail, type BulkEmailRecipientFilter, type SupportAttachment } from '@/api/support'
import { list as listUsers } from '@/api/admin/users'
import type { AdminUser } from '@/types'
import { useAppStore } from '@/stores/app'

const { t, te, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const app = useAppStore()
const selectedId = computed(() => Number(route.params.id) || 0)
const batches = ref<BulkEmailBatch[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const listError = ref('')
const composing = ref(false)
const subject = ref('')
const body = ref('')
const attachments = ref<SupportAttachment[]>([])
const allActive = ref(false)
const balanceCondition = ref<BulkEmailRecipientFilter['balance_condition']>('all')
const rechargeCondition = ref<BulkEmailRecipientFilter['recharge_condition']>('all')
const balanceThreshold = ref('')
const filterError = ref('')
const selectedUsers = ref(new Map<number, AdminUser>())
const users = ref<AdminUser[]>([])
const userSearch = ref('')
const userPage = ref(1)
const usersTotal = ref(0)
const usersLoading = ref(false)
const usersError = ref('')
const detail = ref<BulkEmailDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
const recipientPage = ref(1)
const busy = ref(false)
const uploading = ref(false)
let listGeneration = 0
let usersGeneration = 0
let detailGeneration = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined
let poll: ReturnType<typeof setInterval> | undefined

const date = (value: string) => value ? new Date(value).toLocaleString(locale.value) : '—'
const activeStatus = (status: string) => ['queued', 'sending', 'processing', 'running'].includes(status)
const statusLabel = (status: string) => te(`bulkMail.statuses.${status}`) ? t(`bulkMail.statuses.${status}`) : status
const statusClass = (status: string) => ['failed', 'partial_failed'].includes(status) ? 'badge-danger' : ['completed', 'sent'].includes(status) ? 'badge-success' : activeStatus(status) ? 'badge-warning' : 'badge-gray'

function recipientFilterSummary(filter?: BulkEmailRecipientFilter | null) {
  const parts: string[] = []
  if (filter?.balance_condition === 'positive') parts.push(t('bulkMail.filters.positive'))
  if (filter?.balance_condition === 'non_positive') parts.push(t('bulkMail.filters.nonPositive'))
  if (filter?.balance_condition === 'greater_than') parts.push(t('bulkMail.filters.balanceGreater', { amount: filter.balance_threshold }))
  if (filter?.recharge_condition === 'recharged') parts.push(t('bulkMail.filters.rechargedSummary'))
  return parts.length ? parts.join(` ${t('bulkMail.filters.and')} `) : t('bulkMail.filters.unrestricted')
}

function recipientFilter(): BulkEmailRecipientFilter | null {
  // decimal(20,8) 阈值全程保持字符串，避免 JavaScript 浮点数改变筛选边界。
  const validAmount = /^\d{1,12}(\.\d{1,8})?$/
  filterError.value = ''
  if (balanceCondition.value === 'greater_than' && !validAmount.test(balanceThreshold.value)) {
    filterError.value = t('bulkMail.filters.invalidAmount', { field: t('bulkMail.filters.balance') })
    return null
  }
  return {
    balance_condition: balanceCondition.value,
    recharge_condition: rechargeCondition.value,
    // 切换为不限或预设条件后，不发送之前输入的金额。
    ...(balanceCondition.value === 'greater_than' ? { balance_threshold: balanceThreshold.value } : {})
  }
}

async function loadBatches(silent = false) {
  const generation = ++listGeneration
  if (!silent) loading.value = true
  try {
    const data = await bulkEmailAPI.list(page.value)
    if (generation !== listGeneration) return
    batches.value = data.items || []
    total.value = data.total
    listError.value = ''
  } catch (error) { if (!silent && generation === listGeneration) listError.value = supportError(error, t('bulkMail.loadFailed')) }
  finally { if (generation === listGeneration) loading.value = false }
}

async function loadUsers() {
  const generation = ++usersGeneration
  usersLoading.value = true
  usersError.value = ''
  try {
    const data = await listUsers(userPage.value, 20, { status: 'active', search: userSearch.value.trim(), include_subscriptions: false })
    if (generation !== usersGeneration) return
    users.value = data.items || []
    usersTotal.value = data.total
  } catch (error) { if (generation === usersGeneration) usersError.value = supportError(error, t('support.loadFailed')) }
  finally { if (generation === usersGeneration) usersLoading.value = false }
}

function searchUsers() {
  if (searchTimer) clearTimeout(searchTimer)
  usersGeneration++
  searchTimer = setTimeout(() => { userPage.value = 1; void loadUsers() }, 300)
}
function toggleUser(user: AdminUser) {
  const next = new Map(selectedUsers.value)
  if (next.has(user.id)) next.delete(user.id)
  else next.set(user.id, user)
  selectedUsers.value = next
}
function changeUserPage(value: number) { userPage.value = value; void loadUsers() }
function changePage(value: number) { page.value = value; void loadBatches() }
function changeRecipientPage(value: number) { recipientPage.value = value; void loadDetail() }
function closeDetail() { if (!busy.value) void router.push('/admin/bulk-emails') }

async function loadDetail(silent = false) {
  if (!selectedId.value) return
  const generation = ++detailGeneration
  if (!silent) detailLoading.value = true
  try {
    const data = await bulkEmailAPI.detail(selectedId.value, recipientPage.value)
    if (generation !== detailGeneration) return
    detail.value = data
    detailError.value = ''
  } catch (error) { if (!silent && generation === detailGeneration) detailError.value = supportError(error, t('bulkMail.loadFailed')) }
  finally { if (generation === detailGeneration) detailLoading.value = false }
}

async function createDraft() {
  if (busy.value || uploading.value) return
  if (!subject.value.trim() || (!body.value.trim() && !attachments.value.length) || (!allActive.value && !selectedUsers.value.size)) {
    app.showError(t('bulkMail.required')); return
  }
  const filter = recipientFilter()
  if (!filter) return
  busy.value = true
  try {
    const batch = await bulkEmailAPI.create({ subject: subject.value.trim(), body: body.value.trim(), all_active: allActive.value, user_ids: allActive.value ? [] : [...selectedUsers.value.keys()], attachment_ids: attachments.value.map(item => item.id), recipient_filter: filter })
    subject.value = ''
    body.value = ''
    attachments.value = []
    selectedUsers.value = new Map()
    balanceCondition.value = 'all'
    rechargeCondition.value = 'all'
    balanceThreshold.value = ''
    composing.value = false
    uploading.value = false
    app.showSuccess(t('bulkMail.draftSaved'))
    await router.push(`/admin/bulk-emails/${batch.id}`)
    page.value = 1
    await loadBatches()
  } catch (error) { app.showError(supportError(error, t('support.saveFailed'))) }
  finally { busy.value = false }
}

async function startBatch() {
  if (!detail.value || busy.value || detail.value.batch.status !== 'draft' || !detail.value.batch.total_count) return
  busy.value = true
  try {
    await bulkEmailAPI.start(detail.value.batch.id)
    app.showSuccess(t('bulkMail.started'))
    await Promise.all([loadDetail(), loadBatches(true)])
  } catch (error) { app.showError(supportError(error, t('support.saveFailed'))) }
  finally { busy.value = false }
}

async function retryBatch() {
  if (!detail.value || busy.value) return
  busy.value = true
  try {
    await bulkEmailAPI.retry(detail.value.batch.id)
    app.showSuccess(t('bulkMail.retryStarted'))
    await Promise.all([loadDetail(), loadBatches(true)])
  } catch (error) { app.showError(supportError(error, t('support.saveFailed'))) }
  finally { busy.value = false }
}

watch(selectedId, () => {
  detailGeneration++
  detail.value = null
  detailError.value = ''
  recipientPage.value = 1
  void loadDetail()
}, { immediate: true })
watch(composing, (value) => { if (value && !users.value.length) void loadUsers() })
onMounted(() => {
  void loadBatches()
  poll = setInterval(() => {
    if (document.hidden || busy.value || detailLoading.value || loading.value) return
    if (detail.value && activeStatus(detail.value.batch.status)) void loadDetail(true)
    if (batches.value.some(batch => activeStatus(batch.status))) void loadBatches(true)
  }, 5000)
})
onBeforeUnmount(() => {
  listGeneration++; usersGeneration++; detailGeneration++
  if (poll) clearInterval(poll)
  if (searchTimer) clearTimeout(searchTimer)
})
</script>
