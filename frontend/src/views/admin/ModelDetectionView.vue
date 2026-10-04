<template>
  <AppLayout>
    <div class="space-y-5 text-gray-900 dark:text-gray-100">
      <header class="flex flex-wrap items-start justify-between gap-4">
        <div><h1 class="text-2xl font-semibold">{{ t('modelDetection.title') }}</h1><p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('modelDetection.description') }}</p></div>
        <div class="flex flex-wrap gap-2"><button class="btn btn-secondary" :disabled="loading" :aria-busy="loading" @click="refresh"><Icon name="refresh" size="sm" :class="{ 'motion-safe:animate-spin': loading }" aria-hidden="true" />{{ t(loading ? 'modelDetection.refreshing' : 'modelDetection.refresh') }}</button><button class="btn btn-primary" @click="openPlan()"><Icon name="plus" size="sm" aria-hidden="true" />{{ t('modelDetection.createPlan') }}</button></div>
      </header>
      <p class="max-w-5xl text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('modelDetection.explanation') }}</p>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-400">{{ error }}</p>
      <section class="panel">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 px-4 py-3 dark:border-dark-700"><h2 class="font-semibold">{{ t('modelDetection.active') }} <span class="ml-2 font-mono text-primary-600">{{ overview.active.length }}</span></h2><span class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.autoRefresh') }}</span></div>
        <ModelDetectionRunTable :runs="overview.active" :empty-text="t('modelDetection.noActive')" @detail="openDetail" />
      </section>
      <section class="panel">
        <div class="border-b border-gray-200 px-4 py-3 dark:border-dark-700"><h2 class="font-semibold">{{ t('modelDetection.recent') }}</h2><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.recentHint') }}</p></div>
        <dl class="grid grid-cols-2 gap-px bg-gray-100 sm:grid-cols-5 dark:bg-dark-700">
          <div v-for="stat in stats" :key="stat.label" class="bg-white px-4 py-3 dark:bg-dark-800"><dt class="text-xs text-gray-500 dark:text-gray-400">{{ stat.label }}</dt><dd class="mt-1 font-mono text-xl" :class="stat.alert ? 'text-red-600 dark:text-red-400' : ''">{{ stat.value }}</dd></div>
        </dl>
        <ModelDetectionRunTable :runs="overview.recent" @detail="openDetail" />
      </section>
      <section class="panel p-4">
        <div class="grid items-end gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
          <form class="flex min-w-0 items-end gap-2" @submit.prevent="searchAccounts"><label class="min-w-0 flex-1"><span class="mb-1 block text-sm font-medium">{{ t('modelDetection.searchAccount') }}</span><input v-model="accountSearch" class="input" /></label><button class="btn btn-secondary" :disabled="accountsLoading" :aria-busy="accountsLoading"><Icon :name="accountsLoading ? 'refresh' : 'search'" size="sm" :class="{ 'motion-safe:animate-spin': accountsLoading }" aria-hidden="true" />{{ t('modelDetection.search') }}</button></form>
          <label class="min-w-0"><span class="mb-1 block text-sm font-medium">{{ t('modelDetection.account') }}</span><select v-model.number="selectedAccountId" class="input" :title="selectedAccount?.name" @change="selectAccount"><option :value="0">{{ t('modelDetection.allAccounts') }}</option><option v-for="account in accounts" :key="account.id" :value="account.id">{{ account.name }} · #{{ account.id }}</option></select></label>
          <button class="btn btn-secondary" :disabled="!selectedAccount" @click="manualAccount = selectedAccount"><Icon name="play" size="sm" aria-hidden="true" />{{ t('modelDetection.run') }}</button>
        </div>
        <p v-if="accountsError" role="alert" class="mt-2 text-sm text-red-600">{{ accountsError }}</p>
      </section>
      <section class="panel">
        <h2 class="border-b border-gray-200 px-4 py-3 font-semibold dark:border-dark-700">{{ t('modelDetection.plans') }} <span class="ml-2 text-sm font-normal text-gray-500 dark:text-gray-400">{{ plans.length }}</span></h2>
        <p v-if="!plans.length" class="p-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('modelDetection.noPlans') }}</p>
        <div v-else class="overflow-x-auto"><table class="w-full whitespace-nowrap text-left text-sm"><thead class="bg-gray-50 text-xs text-gray-500 dark:text-gray-400 dark:bg-dark-900"><tr><th class="px-4 py-3">{{ t('modelDetection.account') }} / {{ t('modelDetection.model') }}</th><th class="px-4 py-3">{{ t('modelDetection.schedule') }}</th><th class="px-4 py-3">{{ t('modelDetection.baseline') }}</th><th class="px-4 py-3">{{ t('modelDetection.nextRun') }}</th><th class="px-4 py-3"><span class="sr-only">{{ t('modelDetection.editPlan') }}</span></th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="plan in plans" :key="plan.id" class="transition-colors hover:bg-gray-50/70 dark:hover:bg-dark-700/40">
          <td class="px-4 py-3"><span class="block max-w-64 truncate font-medium" :title="plan.account_name">{{ plan.account_name || `#${plan.account_id}` }}</span><span class="mt-1 block max-w-64 truncate font-mono text-xs text-gray-500 dark:text-gray-400" :title="plan.model_id">{{ plan.model_id }}</span></td>
          <td class="px-4 py-3"><span :class="plan.enabled ? 'text-green-700 dark:text-green-400' : 'text-gray-500 dark:text-gray-400'">{{ t(plan.enabled ? 'modelDetection.scheduled' : 'modelDetection.paused') }}</span><span class="mt-1 block max-w-64 truncate text-xs text-gray-500 dark:text-gray-400" :title="scheduleText(plan)">{{ scheduleText(plan) }}</span></td>
          <td class="px-4 py-3 font-mono">{{ detectionScore(plan.baseline_score) }}</td><td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">{{ plan.enabled && plan.next_run_at ? formatDateTime(plan.next_run_at) : '—' }}</td>
          <td class="px-4 py-3"><div class="flex items-center gap-1" :aria-busy="busyId === plan.id"><Icon v-if="busyId === plan.id" name="refresh" size="sm" class="shrink-0 text-primary-500 motion-safe:animate-spin" :aria-label="t('common.loading')" /><button class="action" :disabled="!!busyId || isActive(plan.id)" @click="run(plan)"><Icon name="play" size="sm" aria-hidden="true" />{{ t('modelDetection.run') }}</button><button class="action" :disabled="!!busyId" @click="togglePlan(plan)">{{ t(plan.enabled ? 'modelDetection.pause' : 'modelDetection.enable') }}</button><button class="action" :disabled="!!busyId" @click="openPlan(plan)"><Icon name="edit" size="sm" aria-hidden="true" />{{ t('modelDetection.edit') }}</button></div></td>
        </tr></tbody></table></div>
      </section>
      <section class="panel">
        <div class="flex items-center justify-between gap-3 border-b border-gray-200 px-4 py-3 dark:border-dark-700"><h2 class="min-w-0 font-semibold">{{ t('modelDetection.history') }}<span v-if="selectedAccount" class="mt-1 block truncate text-sm font-normal text-gray-500 dark:text-gray-400" :title="selectedAccount.name">{{ selectedAccount.name }}</span></h2><span v-if="historyLoading" role="status" class="shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</span></div>
        <p v-if="!selectedAccountId" class="p-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('modelDetection.chooseHistory') }}</p>
        <template v-else><p v-if="historyError" role="alert" class="p-4 text-sm text-red-600">{{ historyError }}</p><ModelDetectionRunTable :runs="history" @detail="openDetail" /><div v-if="nextBeforeId" class="border-t border-gray-100 p-3 text-center dark:border-dark-700"><button class="btn btn-secondary" :disabled="historyLoading" @click="loadHistory(true)">{{ t('modelDetection.more') }}</button></div></template>
      </section>
    </div>

    <BaseDialog :show="showPlan" :title="t(editingPlanId ? 'modelDetection.editPlan' : 'modelDetection.createPlan')" width="wide" :close-on-escape="!saving" :close-on-click-outside="!saving" :show-close-button="!saving" trap-focus @close="closePlan">
      <form id="model-detection-plan-form" class="min-w-0 space-y-5" @submit.prevent="savePlan">
        <p class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">{{ t('modelDetection.costHint', { count: (editingPlanId ? 1 : planModelIds.length) * (catalog?.requests_per_run || 4) }) }}</p>
        <div v-if="!editingPlanId" class="flex items-end gap-2"><label class="min-w-0 flex-1"><span class="field-label">{{ t('modelDetection.searchAccount') }}</span><input v-model="accountSearch" class="input" :disabled="saving" @keydown.enter.prevent="searchAccounts" /></label><button type="button" class="btn btn-secondary" :disabled="accountsLoading || saving" :aria-busy="accountsLoading" @click="searchAccounts"><Icon :name="accountsLoading ? 'refresh' : 'search'" size="sm" :class="{ 'motion-safe:animate-spin': accountsLoading }" aria-hidden="true" />{{ t('modelDetection.search') }}</button></div>
        <p v-if="accountsError" role="alert" class="text-sm text-red-600">{{ accountsError }}</p>
        <fieldset :disabled="saving" class="grid min-w-0 gap-4 sm:grid-cols-2">
          <label class="min-w-0" :class="{ 'sm:col-span-2': !editingPlanId }"><span class="field-label">{{ t('modelDetection.account') }}</span><select v-model.number="form.account_id" class="input" required :disabled="!!editingPlanId"><option :value="0" disabled>{{ t('modelDetection.chooseAccount') }}</option><option v-for="account in accounts" :key="account.id" :value="account.id">{{ account.name }} · #{{ account.id }}</option></select></label>
          <label v-if="editingPlanId"><span class="field-label">{{ t('modelDetection.model') }}</span><input v-model="form.model_id" class="input font-mono" readonly /><span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.editSingleModelHint') }}</span></label>
          <div v-else class="min-w-0 sm:col-span-2"><ModelDetectionModelPicker :key="form.account_id" v-model="planModelIds" :models="models" :loading="modelsLoading" :disabled="saving || !form.account_id" /><p v-if="modelsFailed" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.modelsUnavailable') }}</p></div>
          <label><span class="field-label">{{ t('modelDetection.schedule') }}</span><select v-model="form.schedule_type" class="input"><option value="interval">{{ t('modelDetection.interval') }}</option><option value="daily">{{ t('modelDetection.daily') }}</option></select></label>
          <label v-if="form.schedule_type === 'interval'"><span class="field-label">{{ t('modelDetection.minutes') }}</span><input v-model.number="form.interval_minutes" type="number" min="1" max="10080" step="1" required class="input" /></label>
          <label v-else><span class="field-label">{{ t('modelDetection.dailyTime') }}</span><input v-model="form.daily_time" type="time" required class="input" /></label>
          <label><span class="field-label">{{ t('modelDetection.timezone') }}</span><input v-model="form.timezone" class="input" required placeholder="Asia/Shanghai" /></label>
          <label><span class="field-label">{{ t('modelDetection.threshold') }}</span><input v-model.number="form.drop_threshold" type="number" min="0.1" max="100" step="0.1" required class="input" /><span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.thresholdHint') }}</span></label>

          <label><span class="field-label">{{ t('modelDetection.retention') }}</span><input v-model.number="form.max_results" type="number" min="100" max="1000" step="1" required class="input" /></label>
          <label class="flex items-center gap-2 sm:col-span-2"><input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded" />{{ t('modelDetection.enabled') }}</label>
        </fieldset>
        <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('modelDetection.automaticComparison') }}</p>
        <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('modelDetection.longRunningHint', { seconds: catalog?.request_timeout_seconds || 600 }) }}</p>
        <div v-if="planFailures.length" role="alert" class="rounded-lg bg-red-50 p-3 text-sm dark:bg-red-900/20"><p class="font-medium text-red-700 dark:text-red-300">{{ t('modelDetection.batchPartial', { success: planSuccessCount, failed: planFailures.length }) }}</p><ul class="mt-2 space-y-1 text-red-600 dark:text-red-400"><li v-for="failure in planFailures" :key="failure.model_id" class="break-words"><span class="font-mono">{{ failure.model_id }}</span>: {{ failure.error || failure.reason || t('modelDetection.actionFailed') }}</li></ul><p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.retryFailedOnly') }}</p></div>
        <p v-if="formError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ formError }}</p>
      </form>
      <template #footer><button type="button" class="btn btn-secondary" :disabled="saving" @click="closePlan">{{ t('modelDetection.cancel') }}</button><button type="submit" form="model-detection-plan-form" class="btn btn-primary" :disabled="saving || (!editingPlanId && !planModelIds.length)" :aria-busy="saving"><Icon v-if="saving" name="refresh" size="sm" class="motion-safe:animate-spin" aria-hidden="true" />{{ t(saving ? 'common.saving' : 'modelDetection.save') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="showDetail" :title="t('modelDetection.details')" width="wide" trap-focus @close="closeDetail">
      <p v-if="detailLoading" class="p-6 text-center text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</p>
      <p v-else-if="detailError" role="alert" class="text-sm text-red-600">{{ detailError }}</p>
      <div v-else-if="detail" class="min-w-0 space-y-5 text-sm [overflow-wrap:anywhere]">
        <div><p class="break-words font-semibold">{{ detail.account_name }} · {{ detail.model_id }}</p><p class="mt-1 text-gray-500 dark:text-gray-400">#{{ detail.id }} · {{ formatDateTime(detail.created_at) }} · {{ t(detectionResultKey(detail)) }}</p></div>
        <dl class="grid grid-cols-3 gap-3 rounded-lg bg-gray-50 p-4 dark:bg-dark-900"><div><dt class="text-gray-500 dark:text-gray-400">{{ t('modelDetection.score') }}</dt><dd class="mt-1 font-mono text-xl">{{ detectionScore(detail.score) }}</dd></div><div><dt class="text-gray-500 dark:text-gray-400">{{ t('modelDetection.baseline') }}</dt><dd class="mt-1 font-mono text-xl">{{ detectionScore(detail.baseline_score) }}</dd></div><div><dt class="text-gray-500 dark:text-gray-400">{{ t('modelDetection.difference') }}</dt><dd class="mt-1 font-mono text-xl">{{ detectionScore(detail.drop_points) }}</dd></div></dl>
        <p v-if="detail.error_message" role="alert" class="break-words rounded-lg bg-red-50 p-3 text-red-600 dark:bg-red-900/20 dark:text-red-400">{{ detail.error_message }}</p>
        <div><h3 class="font-semibold">{{ t('modelDetection.fingerprint') }}</h3><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.fingerprintHint') }}</p><dl class="mt-3 grid gap-3 sm:grid-cols-3"><div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.fingerprintResult') }}</dt><dd class="mt-1 font-medium">{{ fingerprintStatus(detail.fingerprint.status) }}</dd></div><div v-if="detail.fingerprint.analysis?.prediction_name"><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.closestModel') }}</dt><dd class="mt-1">{{ detail.fingerprint.analysis.prediction_name }}</dd></div><div v-if="typeof detail.fingerprint.analysis?.probability === 'number'"><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.candidateProbability') }}</dt><dd class="mt-1 font-mono">{{ detectionScore(detail.fingerprint.analysis.probability * 100) }}%</dd></div></dl><p v-if="detail.fingerprint.reference_model" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.reference') }}: <span class="font-mono">{{ detail.fingerprint.reference_model }}</span></p><p v-if="detail.fingerprint.message" class="mt-3 break-words text-xs text-gray-500 dark:text-gray-400">{{ detail.fingerprint.message }}</p><details v-if="detail.fingerprint.analysis" class="mt-3"><summary class="cursor-pointer text-xs text-primary-600">{{ t('modelDetection.fingerprintEvidence') }}</summary><pre class="evidence mt-2">{{ pretty(detail.fingerprint.analysis) }}</pre></details></div>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelDetection.suite') }}: {{ detail.suite_version }}</p>
        <p v-if="!detail.details?.length" class="text-gray-500 dark:text-gray-400">{{ t('modelDetection.noEvidence') }}</p>
        <details v-for="(item, index) in detail.details || []" :key="index" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"><summary class="cursor-pointer font-medium">{{ index + 1 }}. {{ t(item.kind === 'fingerprint' ? 'modelDetection.fingerprintProbe' : 'modelDetection.quality') }}</summary><div class="mt-4 space-y-3"><dl class="flex flex-wrap gap-x-5 gap-y-2 text-xs text-gray-500 dark:text-gray-400"><div v-if="item.status"><dt class="inline">{{ t('modelDetection.requestStatus') }}: </dt><dd class="inline">{{ t('modelDetection.status.' + item.status) }}</dd></div><div v-if="item.duration_ms != null"><dt class="inline">{{ t('modelDetection.duration') }}: </dt><dd class="inline font-mono">{{ t('modelDetection.seconds', { seconds: detectionScore(item.duration_ms / 1000) }) }}</dd></div><div v-if="item.timeout_seconds"><dt class="inline">{{ t('modelDetection.timeout') }}: </dt><dd class="inline font-mono">{{ t('modelDetection.seconds', { seconds: item.timeout_seconds }) }}</dd></div></dl><p v-if="item.status && item.status !== 'completed'" class="rounded-lg bg-amber-50 p-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">{{ t('modelDetection.incompleteEvidence') }}</p><p v-if="item.error_message" class="break-words text-xs text-red-600 dark:text-red-400">{{ item.error_message }}</p><div><h4 class="mb-1 font-medium">{{ t('modelDetection.prompt') }}</h4><pre class="evidence">{{ item.prompt }}</pre></div><div><h4 class="mb-1 font-medium">{{ t('modelDetection.response') }}</h4><pre class="evidence">{{ item.response || t('modelDetection.noResponse') }}</pre></div><div v-if="item.evaluation"><h4 class="mb-1 font-medium">{{ t('modelDetection.evaluation') }}</h4><pre class="evidence">{{ pretty(item.evaluation) }}</pre></div></div></details>
      </div>
    </BaseDialog>
    <ManualModelDetectionDialog :show="!!manualAccount" :account="manualAccount" @close="manualAccount = null" @queued="onQueued" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelDetectionModelPicker from '@/components/admin/account/ModelDetectionModelPicker.vue'
import ModelDetectionRunTable from '@/components/admin/account/ModelDetectionRunTable.vue'
import ManualModelDetectionDialog from '@/components/admin/account/ManualModelDetectionDialog.vue'
import { list as listAccounts, getById, getAvailableModels } from '@/api/admin/accounts'
import { modelDetectionAPI, newDetectionPlan, detectionPlanInput, detectionScore, detectionResultKey, type ModelDetectionBatchItem, type ModelDetectionCatalog, type ModelDetectionOverview, type ModelDetectionPlan, type ModelDetectionRun } from '@/api/admin/modelDetection'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { useAppStore } from '@/stores/app'
import type { ClaudeModel } from '@/types'

const { t } = useI18n()
const app = useAppStore()
const route = useRoute()
const router = useRouter()
const overview = ref<ModelDetectionOverview>({ active: [], recent: [], plans: [], stats: { total: 0, normal: 0, suspected_drop: 0, error: 0, average_score: null } })
const catalog = ref<ModelDetectionCatalog | null>(null)
const loading = ref(false)
const error = ref('')
const accountSearch = ref('')
const accounts = ref<{ id: number; name: string }[]>([])
const accountsLoading = ref(false)
const accountsError = ref('')
const queryAccountId = () => { const id = Number(route.query.account_id); return Number.isSafeInteger(id) && id > 0 ? id : 0 }
const selectedAccountId = ref(queryAccountId())
const selectedAccount = computed(() => accounts.value.find(account => account.id === selectedAccountId.value) || null)
const plans = computed(() => overview.value.plans.filter(plan => !selectedAccountId.value || plan.account_id === selectedAccountId.value))
const stats = computed(() => [
  { label: t('modelDetection.total'), value: overview.value.stats.total },
  { label: t('modelDetection.normal'), value: overview.value.stats.normal },
  { label: t('modelDetection.suspectedDrop'), value: overview.value.stats.suspected_drop, alert: overview.value.stats.suspected_drop > 0 },
  { label: t('modelDetection.errors'), value: overview.value.stats.error, alert: overview.value.stats.error > 0 },
  { label: t('modelDetection.average'), value: detectionScore(overview.value.stats.average_score) }
])
const history = ref<ModelDetectionRun[]>([])
const nextBeforeId = ref<number | null>(null)
const historyLoading = ref(false)
const historyError = ref('')
let historyRequest = 0
let accountRequest = 0
const showPlan = ref(false)
const editingPlanId = ref<number | null>(null)
const form = ref(newDetectionPlan())
const formError = ref('')
const planModelIds = ref<string[]>([])
const planFailures = ref<ModelDetectionBatchItem[]>([])
const planSuccessCount = ref(0)
const saving = ref(false)
const models = ref<ClaudeModel[]>([])
const modelsFailed = ref(false)
const modelsLoading = ref(false)
let modelRequest = 0
const busyId = ref<number | null>(null)
const manualAccount = ref<{ id: number; name: string } | null>(null)
const showDetail = ref(false)
const detail = ref<ModelDetectionRun | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
let detailRequest = 0
let detailPending = false
let timer: ReturnType<typeof setInterval> | undefined
let disposed = false
const pretty = (value: unknown) => JSON.stringify(value, null, 2)
const fingerprintStatus = (status: string) => ['match', 'mismatch', 'inconclusive', 'unsupported', 'invalid'].includes(status) ? t('modelDetection.fingerprintStatus.' + status) : t('modelDetection.status.inconclusive')
const scheduleText = (plan: ModelDetectionPlan) => plan.schedule_type === 'daily' ? t('modelDetection.dailySummary', { time: plan.daily_time, timezone: plan.timezone }) : t('modelDetection.intervalSummary', { minutes: plan.interval_minutes })
const isActive = (planId: number) => overview.value.active.some(run => run.plan_id === planId)

async function loadOverview() {
  if (loading.value) return
  loading.value = true
  try { const data = await modelDetectionAPI.overview(); if (!disposed) { overview.value = data; error.value = '' } }
  catch (cause) { if (!disposed) error.value = extractApiErrorMessage(cause, t('modelDetection.failed')) }
  finally { loading.value = false }
}
async function refresh() { await Promise.all([loadOverview(), loadHistory()]) }
async function ensureAccount(id: number) {
  if (!id || accounts.value.some(account => account.id === id)) return
  try { const account = await getById(id); if (!disposed && !accounts.value.some(item => item.id === id)) accounts.value.push({ id: account.id, name: account.name }) }
  catch (cause) { accountsError.value = extractApiErrorMessage(cause, t('modelDetection.failed')) }
}
async function searchAccounts() {
  const request = ++accountRequest
  accountsLoading.value = true; accountsError.value = ''
  try {
    const data = await listAccounts(1, 100, { search: accountSearch.value.trim(), lite: '1' })
    if (request !== accountRequest || disposed) return
    const keep = accounts.value.filter(account => account.id === selectedAccountId.value || account.id === form.value.account_id)
    accounts.value = [...new Map([...keep, ...data.items.map(account => ({ id: account.id, name: account.name }))].map(account => [account.id, account])).values()]
    await ensureAccount(selectedAccountId.value)
  } catch (cause) { if (request === accountRequest) accountsError.value = extractApiErrorMessage(cause, t('modelDetection.failed')) }
  finally { if (request === accountRequest) accountsLoading.value = false }
}
function selectAccount() { history.value = []; nextBeforeId.value = null; void router.replace({ query: { ...route.query, account_id: selectedAccountId.value || undefined } }); void loadHistory() }
async function loadHistory(append = false) {
  const id = selectedAccountId.value
  const request = ++historyRequest
  if (!id) { history.value = []; nextBeforeId.value = null; historyLoading.value = false; return }
  const beforeId = append ? nextBeforeId.value ?? undefined : undefined
  if (append && !beforeId) return
  historyLoading.value = true; historyError.value = ''
  try {
    const data = await modelDetectionAPI.history(id, beforeId)
    if (request !== historyRequest || disposed) return
    history.value = append ? [...history.value, ...data.items] : data.items
    nextBeforeId.value = data.next_before_id
  } catch (cause) { if (request === historyRequest) historyError.value = extractApiErrorMessage(cause, t('modelDetection.failed')) }
  finally { if (request === historyRequest) historyLoading.value = false }
}
async function openPlan(plan?: ModelDetectionPlan) {
  editingPlanId.value = plan?.id ?? null
  form.value = plan ? detectionPlanInput(plan) : newDetectionPlan(selectedAccountId.value)
  planModelIds.value = []; planFailures.value = []; planSuccessCount.value = 0
  formError.value = ''; showPlan.value = true
  if (plan) await ensureAccount(plan.account_id)
}
function closePlan() { if (!saving.value) showPlan.value = false }
async function savePlan() {
  if (saving.value) return
  const input = detectionPlanInput(form.value)
  try {
    new Intl.DateTimeFormat('en', { timeZone: input.timezone })
    if (!input.account_id || (editingPlanId.value ? !input.model_id : !planModelIds.value.length || planModelIds.value.length > 20) || !input.timezone || input.drop_threshold <= 0 || input.drop_threshold > 100 || (input.schedule_type === 'interval' && (!Number.isInteger(input.interval_minutes) || input.interval_minutes < 1 || input.interval_minutes > 10080)) || (input.schedule_type === 'daily' && !/^([01]\d|2[0-3]):[0-5]\d$/.test(input.daily_time))) throw new Error('invalid')
  } catch { formError.value = t('modelDetection.invalidForm'); return }
  saving.value = true; formError.value = ''
  try {
    if (editingPlanId.value) {
      await modelDetectionAPI.update(editingPlanId.value, input)
      showPlan.value = false; app.showSuccess(t('modelDetection.saved'))
    } else {
      const { model_id: _modelId, reference_model: _referenceModel, ...schedule } = input
      const result = await modelDetectionAPI.createBatch({ ...schedule, model_ids: planModelIds.value })
      planFailures.value = result.results.filter(item => !item.plan)
      planSuccessCount.value = result.success
      planModelIds.value = planFailures.value.map(item => item.model_id)
      if (result.success > 0) app.showSuccess(t('modelDetection.saved'))
      if (!planFailures.value.length) showPlan.value = false
    }
    await loadOverview()
  } catch (cause) { formError.value = extractApiErrorMessage(cause, t('modelDetection.actionFailed')) }
  finally { saving.value = false }
}
async function planAction(plan: ModelDetectionPlan, action: () => Promise<unknown>, success: string) {
  if (busyId.value) return
  busyId.value = plan.id
  try { await action(); app.showSuccess(t(success)); await refresh() }
  catch (cause) { error.value = extractApiErrorMessage(cause, t('modelDetection.actionFailed')) }
  finally { busyId.value = null }
}
function run(plan: ModelDetectionPlan) { void planAction(plan, () => modelDetectionAPI.run(plan.id), 'modelDetection.queued') }
function togglePlan(plan: ModelDetectionPlan) { void planAction(plan, () => modelDetectionAPI.update(plan.id, { ...detectionPlanInput(plan), enabled: !plan.enabled }), 'modelDetection.saved') }
function onQueued() { app.showSuccess(t('modelDetection.queued')); void refresh() }
function closeDetail() { showDetail.value = false; detailRequest++; detailLoading.value = false; detailPending = false }
async function openDetail(id: number) {
  showDetail.value = true; detail.value = null
  await loadDetail(id)
}
async function loadDetail(id: number, silent = false) {
  if (silent && detailPending) return
  const request = ++detailRequest
  detailPending = true
  if (!silent) detailLoading.value = true
  detailError.value = ''
  try { const data = await modelDetectionAPI.detail(id); if (request === detailRequest && !disposed && showDetail.value) detail.value = data }
  catch (cause) { if (request === detailRequest && showDetail.value) detailError.value = extractApiErrorMessage(cause, t('modelDetection.failed')) }
  finally { if (request === detailRequest) { detailLoading.value = false; detailPending = false } }
}
watch(() => route.query.account_id, () => {
  const id = queryAccountId()
  if (id === selectedAccountId.value) return
  selectedAccountId.value = id; history.value = []; nextBeforeId.value = null
  void ensureAccount(id); void loadHistory()
})
watch(() => [showPlan.value, form.value.account_id] as const, async ([show, id]) => {
  const request = ++modelRequest
  models.value = []; modelsFailed.value = false
  modelsLoading.value = false
  if (!editingPlanId.value && show) { planModelIds.value = []; planFailures.value = [] }
  if (!show || !id) return
  modelsLoading.value = true
  try { const data = await getAvailableModels(id); if (request === modelRequest) models.value = data ?? [] }
  catch { if (request === modelRequest) modelsFailed.value = true }
  finally { if (request === modelRequest) modelsLoading.value = false }
})
onMounted(async () => {
  await Promise.all([refresh(), searchAccounts(), modelDetectionAPI.catalog().then(data => { catalog.value = data }).catch(cause => { error.value = extractApiErrorMessage(cause, t('modelDetection.failed')) })])
  if (!disposed) timer = setInterval(() => { if (!document.hidden) { void loadOverview(); if (selectedAccountId.value && !historyLoading.value && history.value.length <= 20) void loadHistory(); if (showDetail.value && detail.value && ['queued', 'running'].includes(detail.value.status)) void loadDetail(detail.value.id, true) } }, 10_000)
})
onUnmounted(() => { disposed = true; clearInterval(timer); historyRequest++; accountRequest++; modelRequest++; detailRequest++ })
</script>

<style scoped>
.panel { @apply overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800; }
.field-label { @apply mb-1 block text-sm font-medium; }
.btn { @apply min-h-11 shrink-0; }
.action { @apply inline-flex min-h-9 items-center justify-center gap-1.5 rounded-lg px-2 text-sm font-medium text-primary-600 transition-colors enabled:hover:bg-primary-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 disabled:cursor-not-allowed disabled:opacity-40 dark:text-primary-400 dark:enabled:hover:bg-primary-900/20; }
select.input { @apply min-w-0 cursor-pointer appearance-none truncate pr-10 disabled:cursor-not-allowed; background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 20 20' fill='none' stroke='%2394a3b8' stroke-width='1.5'%3E%3Cpath d='m6 8 4 4 4-4' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E"); background-repeat: no-repeat; background-position: right 0.75rem center; background-size: 1.25rem; }
summary { @apply min-h-10 rounded-lg px-2 py-2 transition-colors hover:bg-gray-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:hover:bg-dark-700; }
.evidence { @apply max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-50 p-3 font-mono text-xs leading-6 dark:bg-dark-900; overflow-wrap: anywhere; }
</style>
