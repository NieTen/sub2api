<template>
  <CommunicationsLayout active="members">
    <TablePageLayout>
      <template #filters>
        <div class="space-y-4 text-gray-900 dark:text-gray-100">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('community.members.description') }}</p>
            <router-link to="/admin/community/settings" class="btn btn-secondary">{{ t('community.settings') }}</router-link>
          </div>
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
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
            </select>
            <button type="button" class="btn btn-secondary" :disabled="loading" data-test="refresh-members" @click="load">{{ t('community.refresh') }}</button>
          </form>
          <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        </div>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="members" :loading="loading" row-key="user_id">
          <template #cell-email="{ row }">
            <div class="min-w-0">
              <p class="break-all font-medium">{{ row.email }}</p>
              <p class="mt-1 text-xs text-gray-500">#{{ row.user_id }} · {{ row.username || '—' }}</p>
            </div>
          </template>
          <template #cell-telegram_user_id="{ row }">
            <div v-if="row.telegram_user_id" class="min-w-0">
              <p class="break-words">{{ row.telegram_name || '—' }}</p>
              <p v-if="row.telegram_username" class="mt-1 break-all text-xs text-gray-500">{{ '@' + row.telegram_username.replace(/^@/, '') }}</p>
              <p class="mt-1 font-mono text-xs text-gray-500">{{ row.telegram_user_id }}</p>
            </div>
            <span v-else class="text-gray-400">{{ t('community.members.notBound') }}</span>
          </template>
          <template #cell-user_status="{ value }">
            <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-danger']">{{ ['active', 'disabled'].includes(value) ? t('community.members.' + value) : value }}</span>
          </template>
          <template #cell-status="{ row }">
            <span :class="['badge', row.status === 'joined' ? 'badge-success' : row.status === 'pending' ? 'badge-warning' : 'badge-gray']">{{ t('community.members.' + row.status) }}</span>
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
  </CommunicationsLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import CommunicationsLayout from '@/components/support/CommunicationsLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { communityAPI, type CommunityMember, type CommunityMembersPage, type CommunityMemberStatus } from '@/api/community'
import { supportError } from '@/api/support'

const { t, locale } = useI18n()
const members = ref<CommunityMember[]>([])
const summary = ref<CommunityMembersPage['summary']>({ total: 0, joined: 0, not_joined: 0, pending: 0, left: 0 })
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
  { key: 'not_joined', filter: 'not_joined' }, { key: 'pending', filter: 'pending' }, { key: 'left', filter: 'left' }
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
    summary.value = data.summary
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

onMounted(load)
onBeforeUnmount(() => { generation++; if (searchTimer) clearTimeout(searchTimer) })
</script>
