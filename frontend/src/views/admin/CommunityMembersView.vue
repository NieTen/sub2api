<template>
  <CommunicationsLayout active="members">
    <div class="space-y-5">
    <div class="flex gap-1 rounded-xl border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-800" :aria-label="t('community.members.title')">
      <button v-for="item in tabs" :key="item" type="button" class="rounded-lg px-5 py-2.5 text-sm font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500" :class="tab === item ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 hover:bg-gray-50 dark:text-dark-400 dark:hover:bg-dark-700'" :aria-pressed="tab === item" :data-test="'tab-' + item" @click="changeTab(item)">{{ t(item === 'members' ? 'community.members.title' : 'community.chat.title') }}</button>
    </div>
    <TablePageLayout v-show="tab === 'members'">
      <template #filters>
        <div class="space-y-4 text-gray-900 dark:text-gray-100">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('community.members.description') }}</p>
            <router-link to="/admin/community/settings" class="btn btn-secondary">{{ t('community.settings') }}</router-link>
          </div>
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
            <button v-for="item in summaryItems" :key="item.filter" type="button" :data-test="'summary-' + item.key" class="rounded-xl border bg-white p-4 text-left dark:bg-dark-800" :class="status === item.filter ? 'border-primary-500 ring-1 ring-primary-500' : 'border-gray-200 dark:border-dark-700'" @click="changeStatus(item.filter)">
              <span class="text-xs text-gray-500">{{ t('community.members.' + item.key) }}</span>
              <span class="mt-1 block text-xl font-semibold tabular-nums">{{ summary[item.key] }}</span>
            </button>
          </div>
          <p class="text-xs text-gray-500">{{ t('community.members.summaryHint') }}</p>
          <form class="flex flex-wrap gap-3" @submit.prevent="searchNow">
            <input v-model="search" name="community-member-search" type="search" class="input min-w-48 flex-1" :placeholder="t('community.members.search')" :aria-label="t('community.members.search')" @input="scheduleSearch" />
            <select v-model="status" name="community-member-status" class="input w-44" :aria-label="t('community.members.membershipStatus')" @change="changeStatus(status)">
              <option value="all">{{ t('community.members.all') }}</option>
              <option value="joined">{{ t('community.members.joined') }}</option>
              <option value="not_joined">{{ t('community.members.not_joined') }}</option>
              <option value="pending">{{ t('community.members.pending') }}</option>
              <option value="left">{{ t('community.members.left') }}</option>
              <option value="banned">{{ t('community.members.banned') }}</option>
            </select>
            <button type="button" class="btn btn-secondary" :disabled="loading" data-test="refresh-members" @click="load">{{ t('community.refresh') }}</button>
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
              <p class="break-all font-medium">{{ row.email }}</p>
              <p class="mt-1 text-xs text-gray-500">#{{ row.user_id }} · {{ row.username || '—' }}</p>
            </div>
            </div>
          </template>
          <template #cell-telegram_user_id="{ row }">
            <div v-if="row.telegram_user_id" class="flex items-center gap-3">
            <CommunityAvatar :telegram-id="row.telegram_user_id" :name="row.telegram_name || row.telegram_username" @select="selectMember(row)" />
            <div class="min-w-0">
              <p class="break-words">{{ row.telegram_name || '—' }}</p>
              <p v-if="row.telegram_username" class="mt-1 break-all text-xs text-gray-500">{{ '@' + row.telegram_username.replace(/^@/, '') }}</p>
              <p class="mt-1 font-mono text-xs text-gray-500">{{ row.telegram_user_id }}</p>
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
            <p v-if="row.invite_expires_at" class="mt-1 text-xs text-gray-500">{{ t('community.members.inviteExpires') }}: {{ date(row.invite_expires_at) }}</p>
          </template>
          <template #cell-joined_at="{ value }"><span class="text-sm text-gray-500">{{ date(value) }}</span></template>
          <template #empty><div class="space-y-2 p-8 text-center"><p class="font-medium">{{ t('community.members.empty') }}</p><p class="text-sm text-gray-500">{{ t('community.members.emptyHint') }}</p></div></template>
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
const status = ref<CommunityMemberStatus | 'all'>('all')
const loading = ref(false)
const error = ref('')
let generation = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined

const summaryItems = [
  { key: 'total', filter: 'all' }, { key: 'joined', filter: 'joined' },
  { key: 'not_joined', filter: 'not_joined' }, { key: 'pending', filter: 'pending' }, { key: 'left', filter: 'left' }, { key: 'banned', filter: 'banned' }
] as const
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
