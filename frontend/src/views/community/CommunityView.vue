<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-5 text-gray-900 dark:text-gray-100">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t(hideVIP ? 'community.contactDescription' : 'community.description') }}</p>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>
      <div v-if="loading" class="p-10 text-center text-gray-500">{{ t('common.loading') }}</div>
      <div v-else-if="state" :class="['grid items-start gap-5', hideVIP ? 'max-w-xl' : 'lg:grid-cols-[minmax(260px,1fr)_minmax(0,2fr)]']">
        <section class="space-y-4 rounded-xl border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-800">
          <h2 class="text-lg font-semibold">{{ t('community.contactTitle') }}</h2>
          <p class="text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('community.contactDescription') }}</p>
          <a v-if="contactURL" :href="contactURL" target="_blank" rel="noopener noreferrer" data-test="contact-link" class="btn btn-primary w-full">{{ t('community.contact') }}</a>
          <p v-else class="rounded-lg bg-gray-50 p-3 text-sm text-gray-500 dark:bg-dark-900/50">{{ t('community.contactUnavailable') }}</p>
          <router-link to="/tickets" class="btn btn-secondary w-full">{{ t('community.tickets') }}</router-link>
        </section>

        <section v-if="!hideVIP" data-test="community-group" class="min-w-0 overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 p-6 dark:border-dark-700">
            <h2 class="text-lg font-semibold">{{ state.group_name || t('community.groupTitle') }}</h2>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="refreshing || issuing" data-test="refresh" @click="refresh()">{{ t(refreshing ? 'community.refreshing' : 'community.refresh') }}</button>
          </div>
          <div v-if="!state.enabled" class="p-6 text-sm leading-6 text-gray-500">{{ t('community.groupUnavailable') }}</div>
          <div v-else class="space-y-5 p-6">
            <p v-if="state.require_paid_recharge" class="text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('community.vipEligibility') }}</p>
            <div v-if="state.membership?.telegram_user_id" class="rounded-lg border border-primary-100 bg-primary-50/60 p-4 dark:border-primary-900/40 dark:bg-primary-900/10" aria-live="polite">
              <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
                <h3 class="text-sm font-semibold">{{ t(joined || state.membership.joined_at ? 'community.boundIdentity' : 'community.requestIdentity') }}</h3>
                <span :class="['badge', joined ? 'badge-success' : 'badge-gray']">{{ t('community.' + state.membership.status) }}</span>
              </div>
              <dl class="grid gap-3 text-sm sm:grid-cols-2">
                <div><dt class="text-xs text-gray-500">{{ t('community.telegramName') }}</dt><dd data-test="telegram-name" class="mt-1 break-words">{{ state.membership.telegram_name || '—' }}</dd></div>
                <div><dt class="text-xs text-gray-500">{{ t('community.telegramUsername') }}</dt><dd class="mt-1 break-all">{{ state.membership.telegram_username ? '@' + state.membership.telegram_username.replace(/^@/, '') : t('community.noUsername') }}</dd></div>
                <div class="sm:col-span-2"><dt class="text-xs text-gray-500">{{ t('community.telegramId') }}</dt><dd data-test="telegram-id" class="mt-1 font-mono">{{ state.membership.telegram_user_id }}</dd></div>
              </dl>
            </div>
            <p v-if="joined && state.membership?.joined_at" class="text-sm text-gray-500">{{ t('community.joinedAt', { time: date(state.membership.joined_at) }) }}</p>
            <template v-if="!joined">
              <div v-if="inviteURL && inviteActive" class="space-y-3">
                <span class="badge badge-warning">{{ t('community.pending') }}</span>
                <div><a :href="inviteURL" target="_blank" rel="noopener noreferrer" data-test="invite-link" class="btn btn-primary">{{ t('community.join') }}</a></div>
                <p class="text-xs text-gray-500">{{ t('community.expires', { time: date(state.invite!.expires_at) }) }}</p>
              </div>
              <p v-else class="text-sm leading-6 text-gray-500">{{ t(state.invite ? 'community.inviteExpired' : 'community.inviteDescription') }}</p>
              <p class="rounded-lg bg-amber-50 p-4 text-sm leading-6 text-amber-900 dark:bg-amber-900/20 dark:text-amber-200">{{ t('community.inviteHint') }}</p>
              <button type="button" :class="['btn', inviteURL && inviteActive ? 'btn-secondary' : 'btn-primary']" :disabled="issuing || refreshing" :data-test="state.membership || state.invite ? 'reissue' : 'get-invite'" @click="getInvitation">{{ t(issuing ? 'community.issuing' : state.membership || state.invite ? 'community.reissue' : 'community.getInvite') }}</button>
            </template>
            <p class="text-xs text-gray-500">{{ t('community.statusHint') }}</p>
          </div>
        </section>
      </div>
      <button v-else type="button" class="btn btn-secondary" :disabled="refreshing" @click="refresh()">{{ t('community.refresh') }}</button>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { communityAPI, type CommunityState } from '@/api/community'
import { supportError } from '@/api/support'
import { safeCommunityContactURL, safeCommunityTelegramURL } from '@/utils/communityLinks'
import { useAuthStore } from '@/stores/auth'

const { t, locale } = useI18n()
const auth = useAuthStore()
const state = ref<CommunityState | null>(null)
const loading = ref(true)
const refreshing = ref(false)
const issuing = ref(false)
const error = ref('')
const now = ref(Date.now())
let generation = 0
let pollUntil = Date.now() + 10 * 60 * 1000
let timer: ReturnType<typeof setInterval> | undefined

const date = (value: string) => new Date(value).toLocaleString(locale.value)
const contactURL = computed(() => safeCommunityContactURL(state.value?.contact_url))
const inviteURL = computed(() => safeCommunityTelegramURL(state.value?.invite?.url))
const inviteActive = computed(() => !!state.value?.invite && Date.parse(state.value.invite.expires_at) > now.value)
const joined = computed(() => state.value?.membership?.status === 'joined')
const hideVIP = computed(() => state.value?.require_paid_recharge === true && state.value.eligible !== true)

function apply(data: CommunityState) {
  state.value = data
  now.value = Date.now()
}

async function refresh(silent = false) {
  if (refreshing.value || issuing.value) return
  const current = ++generation
  refreshing.value = true
  if (!silent) pollUntil = Date.now() + 10 * 60 * 1000
  try {
    const data = await communityAPI.get()
    if (current !== generation) return
    apply(data)
    error.value = ''
  } catch (cause) {
    if (!silent && current === generation) error.value = supportError(cause, t('community.loadFailed'))
  } finally {
    if (current === generation) { refreshing.value = false; loading.value = false }
  }
}

async function getInvitation() {
  if (issuing.value || refreshing.value || !state.value?.enabled || hideVIP.value || joined.value) return
  const current = ++generation
  issuing.value = true
  error.value = ''
  try {
    // 只有用户点击才领取个人邀请；首次加入后的 Telegram 身份由服务端绑定。
    const data = await communityAPI.invite()
    if (current !== generation) return
    apply(data)
    pollUntil = Date.now() + 10 * 60 * 1000
  } catch (cause) {
    if (current === generation) error.value = supportError(cause, t('community.actionFailed'))
  } finally { if (current === generation) issuing.value = false }
}

onMounted(() => {
  timer = setInterval(() => {
    now.value = Date.now()
    if (document.hidden || now.value > pollUntil || !state.value?.enabled || hideVIP.value || joined.value) return
    if (inviteActive.value || state.value.membership?.status === 'pending') void refresh(true)
  }, 5000)
})
watch(() => auth.isAuthenticated ? auth.user?.id ?? null : null, (userId) => {
  // 切换账号立即清空群资料，同时使旧请求失效。
  generation++
  state.value = null
  error.value = ''
  loading.value = true
  refreshing.value = false
  issuing.value = false
  if (userId !== null) void refresh()
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { generation++; if (timer) clearInterval(timer) })
</script>
