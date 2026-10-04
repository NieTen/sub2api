<template>
  <CommunicationsLayout active="members">
    <div class="space-y-5">
    <div class="grid grid-cols-2 gap-1 rounded-xl border border-gray-200 bg-gray-50 p-1 sm:inline-grid dark:border-dark-700 dark:bg-dark-900/50" role="group" :aria-label="t('community.members.title')">
      <button v-for="item in tabs" :key="item" type="button" class="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 sm:px-6" :class="tab === item ? 'bg-white text-primary-700 shadow-sm ring-1 ring-gray-200/70 dark:bg-dark-700 dark:text-primary-300 dark:ring-dark-600' : 'text-gray-500 hover:bg-white/70 dark:text-dark-300 dark:hover:bg-dark-800'" :aria-pressed="tab === item" :data-test="'tab-' + item" @click="changeTab(item)"><Icon :name="item === 'members' ? 'users' : 'chat'" size="sm" aria-hidden="true" />{{ t(item === 'members' ? 'community.members.title' : 'community.chat.title') }}</button>
    </div>
    <TablePageLayout v-show="tab === 'members'" class="member-table-layout">
      <template #filters>
        <div class="space-y-4 text-gray-900 dark:text-gray-100">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('community.members.description') }}</p>
            <router-link to="/admin/community/settings" class="btn btn-secondary gap-2"><Icon name="cog" size="sm" aria-hidden="true" />{{ t('community.settings') }}</router-link>
          </div>
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
            <button v-for="item in summaryItems" :key="item.filter" type="button" :data-test="'summary-' + item.key" :aria-pressed="status === item.filter" class="min-w-0 cursor-pointer rounded-xl border p-4 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500" :class="status === item.filter ? 'border-primary-300 bg-primary-50 ring-1 ring-primary-200 dark:border-primary-700 dark:bg-primary-900/20 dark:ring-primary-800' : 'border-gray-200 bg-white hover:border-primary-200 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-dark-500 dark:hover:bg-dark-700'" @click="changeStatus(item.filter)">
              <span class="flex items-center justify-between gap-2 text-xs text-gray-600 dark:text-dark-300"><span>{{ t('community.members.' + item.key) }}</span><Icon v-if="status === item.filter" name="checkCircle" size="sm" class="shrink-0 text-primary-600 dark:text-primary-400" aria-hidden="true" /></span>
              <span class="mt-2 block break-all text-2xl font-semibold tabular-nums" :class="status === item.filter ? 'text-primary-700 dark:text-primary-300' : ''">{{ summary[item.key] }}</span>
            </button>
          </div>
          <p class="text-xs text-gray-500 dark:text-dark-300">{{ t('community.members.summaryHint') }}</p>
          <form class="flex flex-wrap items-end gap-3 rounded-xl bg-gray-50 p-3 dark:bg-dark-900/50" @submit.prevent="searchNow">
            <div class="min-w-0 basis-full sm:flex-1 sm:basis-64">
              <label for="community-member-search" class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('community.members.search') }}</label>
              <div class="relative">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" aria-hidden="true" />
                <input id="community-member-search" ref="searchInput" v-model="search" name="community-member-search" type="search" class="input min-w-0 pl-9 pr-11 [&::-webkit-search-cancel-button]:appearance-none" :placeholder="t('community.members.search')" @input="scheduleSearch" />
                <button v-if="search" type="button" class="absolute inset-y-0 right-0 inline-flex w-10 items-center justify-center rounded-r-lg text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white" :aria-label="t('common.clear')" data-test="clear-member-search" @click="clearSearch"><Icon name="x" size="sm" aria-hidden="true" /></button>
              </div>
            </div>
            <div class="min-w-0 flex-1 sm:w-44 sm:flex-none">
              <label for="community-member-status" class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('community.members.membershipStatus') }}</label>
              <Select id="community-member-status" :model-value="status" :options="statusOptions" :searchable="false" :aria-label="t('community.members.membershipStatus')" @update:model-value="changeStatus($event as CommunityMemberStatus | 'all')" />
            </div>
            <div class="flex gap-2">
              <button type="button" class="btn btn-secondary gap-2" :disabled="loading" :aria-busy="loading" data-test="refresh-members" @click="load"><Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />{{ t(loading ? 'community.refreshing' : 'community.refresh') }}</button>
              <button type="button" class="btn btn-secondary" :disabled="!search && status === 'all'" data-test="reset-member-filters" @click="resetFilters">{{ t('common.reset') }}</button>
            </div>
          </form>
          <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        </div>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="members" :loading="loading" row-key="user_id">
          <template #cell-email="{ row }">
            <div class="flex items-center gap-3">
            <CommunityAvatar :name="row.username || row.email" @select="selectMember(row)" />
            <div class="min-w-0">
              <button type="button" class="block max-w-full break-all rounded text-left font-medium text-primary-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-primary-300" :data-test="'member-detail-' + row.user_id" @click="selectMember(row)">{{ row.email }}</button>
              <p class="mt-1 break-words text-xs text-gray-500 [overflow-wrap:anywhere] dark:text-dark-300">#{{ row.user_id }} · {{ row.username || '—' }}</p>
            </div>
            </div>
          </template>
          <template #cell-telegram_user_id="{ row }">
            <div v-if="row.telegram_user_id" class="flex items-center gap-3">
            <CommunityAvatar :telegram-id="row.telegram_user_id" :name="row.telegram_name || row.telegram_username" @select="selectMember(row)" />
            <div class="min-w-0">
              <button type="button" class="max-w-full break-words rounded text-left font-medium text-gray-900 [overflow-wrap:anywhere] hover:text-primary-700 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-100 dark:hover:text-primary-300" @click="selectMember(row)">{{ row.telegram_name || '—' }}</button>
              <p v-if="row.telegram_username" class="mt-1 break-all text-xs text-gray-500 dark:text-dark-300">{{ '@' + row.telegram_username.replace(/^@/, '') }}</p>
              <p class="mt-1 font-mono text-xs text-gray-500 dark:text-dark-300">{{ row.telegram_user_id }}</p>
            </div>
            </div>
            <span v-else class="text-gray-400">{{ t('community.members.notBound') }}</span>
          </template>
          <template #cell-user_status="{ value }">
            <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-danger']">{{ ['active', 'disabled'].includes(value) ? t('community.members.' + value) : value }}</span>
          </template>
          <template #cell-status="{ row }">
            <span :class="['badge', row.status === 'banned' ? 'badge-danger' : row.status === 'joined' ? 'badge-success' : row.status === 'pending' ? 'badge-warning' : 'badge-gray']">{{ t('community.members.' + row.status) }}</span>
            <p v-if="row.status === 'banned'" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ t('community.members.noReentry') }}</p>
            <p v-if="row.invite_expires_at" class="mt-1 text-xs text-gray-500 dark:text-dark-300">{{ t('community.members.inviteExpires') }}: {{ date(row.invite_expires_at) }}</p>
          </template>
          <template #cell-joined_at="{ value }"><span class="text-sm text-gray-500 dark:text-dark-300">{{ date(value) }}</span></template>
          <template #empty><div class="space-y-2 p-8 text-center"><p class="font-medium">{{ t('community.members.empty') }}</p><p class="text-sm text-gray-500 dark:text-dark-300">{{ t('community.members.emptyHint') }}</p></div></template>
        </DataTable>
      </template>
      <template v-if="total > 0" #pagination>
        <Pagination :total="total" :page="page" :page-size="20" :show-page-size-selector="false" @update:page="changePage" />
      </template>
    </TablePageLayout>
    <CommunityMessagesPanel v-if="messagesOpened" v-show="tab === 'messages'" :active="tab === 'messages'" @select="selected = $event" />
    <CommunityMemberDialog :identity="selected" @close="selected = null" @changed="load" />
    </div>
  </CommunicationsLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import CommunicationsLayout from '@/components/support/CommunicationsLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import CommunityAvatar from '@/components/community/CommunityAvatar.vue'
import CommunityMemberDialog from '@/components/community/CommunityMemberDialog.vue'
import CommunityMessagesPanel from '@/components/community/CommunityMessagesPanel.vue'
import { communityAPI, type CommunityMember, type CommunityMembersPage, type CommunityMemberStatus, type CommunityTelegramUser } from '@/api/community'
import { supportError } from '@/api/support'

const { t, locale } = useI18n()
const members = ref<CommunityMember[]>([])
const summary = ref<CommunityMembersPage['summary']>({ total: 0, joined: 0, not_joined: 0, pending: 0, left: 0, banned: 0 })
const tabs = ['members', 'messages'] as const
const tab = ref<typeof tabs[number]>('members')
const messagesOpened = ref(false)
const selected = ref<(CommunityTelegramUser & { sender_kind?: string }) | null>(null)
const total = ref(0)
const page = ref(1)
const search = ref('')
const searchInput = ref<HTMLInputElement | null>(null)
const status = ref<CommunityMemberStatus | 'all'>('all')
const loading = ref(false)
const error = ref('')
let generation = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined

const summaryItems = [
  { key: 'total', filter: 'all' }, { key: 'joined', filter: 'joined' },
  { key: 'not_joined', filter: 'not_joined' }, { key: 'pending', filter: 'pending' }, { key: 'left', filter: 'left' }, { key: 'banned', filter: 'banned' }
] as const
const statusOptions = computed(() => summaryItems.map(item => ({ value: item.filter, label: t('community.members.' + item.filter) })))
const columns = computed(() => [
  { key: 'email', label: t('community.members.siteIdentity') },
  { key: 'telegram_user_id', label: t('community.members.telegramIdentity') },
  { key: 'user_status', label: t('community.members.userStatus') },
  { key: 'status', label: t('community.members.membershipStatus') },
  { key: 'joined_at', label: t('community.members.joinedAt') }
])
const date = (value?: string) => value ? new Date(value).toLocaleString(locale.value) : '—'

async function load() {
  const current = ++generation
  loading.value = true
  error.value = ''
  try {
    const data = await communityAPI.members(page.value, search.value.trim(), status.value)
    if (current !== generation) return
    members.value = data.items || []
    total.value = data.total
    // 统计直接使用服务端搜索后的全集结果，不以当前分页或状态筛选重新计算。
    summary.value = { ...data.summary, banned: data.summary.banned ?? 0 }
  } catch (cause) {
    if (current === generation) error.value = supportError(cause, t('community.members.loadFailed'))
  } finally { if (current === generation) loading.value = false }
}

function searchNow() {
  if (searchTimer) clearTimeout(searchTimer)
  page.value = 1
  void load()
}
function scheduleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  generation++
  searchTimer = setTimeout(searchNow, 300)
}
function clearSearch() {
  search.value = ''
  searchNow()
  searchInput.value?.focus()
}
function resetFilters() {
  search.value = ''
  status.value = 'all'
  searchNow()
}
function changeStatus(value: CommunityMemberStatus | 'all') {
  status.value = value
  searchNow()
}
function changePage(value: number) {
  if (searchTimer) clearTimeout(searchTimer)
  page.value = value
  void load()
}
function changeTab(value: typeof tabs[number]) {
  tab.value = value
  if (value === 'messages') messagesOpened.value = true
}
function selectMember(member: CommunityMember) {
  selected.value = { telegram_user_id: member.telegram_user_id || 0, telegram_name: member.telegram_name || member.username || member.email, telegram_username: member.telegram_username, member, banned: member.status === 'banned' }
}

onMounted(load)
onBeforeUnmount(() => { generation++; if (searchTimer) clearTimeout(searchTimer) })
</script>

<style scoped>
/* 成员统计与筛选较多，独立保留表格空间，避免中等桌面只剩一行。 */
@media (min-width: 1024px) {
  .member-table-layout { height: auto; }
  .member-table-layout :deep(.layout-section-scrollable) {
    flex: none;
    height: clamp(20rem, calc(100dvh - 34rem), 40rem);
  }
}
</style>
