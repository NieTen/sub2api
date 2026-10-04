<template>
  <CommunicationsLayout active="community">
    <div class="mx-auto max-w-6xl space-y-5 text-gray-900 dark:text-gray-100">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('community.settingsDescription') }}</p>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>
      <div v-if="loading" class="p-10 text-center text-gray-500 dark:text-dark-300">{{ t('common.loading') }}</div>
      <form v-else-if="loaded" class="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)]" :aria-busy="saving" @submit.prevent="save">
        <div class="min-w-0 space-y-5">
        <fieldset :disabled="saving" class="min-w-0 space-y-5 rounded-xl border border-gray-200 bg-white p-4 shadow-sm sm:p-6 dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center gap-3"><span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400"><Icon name="chat" aria-hidden="true" /></span><h2 class="font-semibold">{{ t('community.contactTitle') }}</h2></div>
          <label class="block">
            <span class="mb-1 block text-sm font-medium">{{ t('community.contactURL') }}</span>
            <input v-model="form.contact_url" name="contact-url" type="url" class="input" placeholder="https://example.com/support" maxlength="2048" />
            <span class="mt-2 block text-xs text-gray-500 dark:text-dark-300">{{ t('community.contactURLHint') }}</span>
          </label>
        </fieldset>
        <section class="space-y-3 rounded-xl border border-gray-200 bg-gray-50 p-4 text-sm leading-6 sm:p-6 dark:border-dark-700 dark:bg-dark-900/50">
          <h2 class="flex items-center gap-2 font-semibold"><Icon name="infoCircle" size="sm" class="text-primary-600 dark:text-primary-400" aria-hidden="true" />{{ t('community.prerequisites') }}</h2>
          <p class="text-gray-600 dark:text-dark-300">{{ t('community.privateGroup') }}</p>
          <p class="text-gray-600 dark:text-dark-300">{{ t('community.reuseBot') }}</p>
          <router-link to="/admin/communications" class="btn btn-secondary gap-2"><Icon name="cog" size="sm" aria-hidden="true" />{{ t('community.configureBot') }}</router-link>
          <p class="text-gray-600 dark:text-dark-300">{{ t('community.webhook') }}</p>
        </section>
        </div>
        <fieldset :disabled="saving" class="min-w-0 space-y-6 rounded-xl border border-gray-200 bg-white p-4 shadow-sm sm:p-6 dark:border-dark-700 dark:bg-dark-800">
          <label class="flex cursor-pointer items-center justify-between gap-4 border-b border-gray-100 pb-5 dark:border-dark-700">
            <span class="flex min-w-0 items-center gap-3 font-semibold"><span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400"><Icon name="users" aria-hidden="true" /></span>{{ t('community.enabled') }}</span>
            <span class="relative inline-flex shrink-0"><input v-model="form.enabled" name="community-enabled" type="checkbox" role="switch" :aria-checked="form.enabled" class="peer sr-only" /><span class="community-switch" aria-hidden="true"></span></span>
          </label>
          <div class="grid gap-5 sm:grid-cols-2">
          <label class="block sm:col-span-2"><span class="mb-1.5 block text-sm font-medium">{{ t('community.groupName') }}</span><input v-model="form.group_name" name="group-name" class="input" maxlength="100" /></label>
          <label class="block"><span class="mb-1.5 block text-sm font-medium">{{ t('community.groupChatID') }}</span><input v-model="form.group_chat_id" name="group-chat-id" class="input font-mono" inputmode="numeric" :required="form.enabled" placeholder="-1001234567890" /></label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium">{{ t('community.botUsername') }}</span>
            <input v-model="form.bot_username" name="bot-username" class="input" :required="form.enabled" placeholder="support_bot" maxlength="33" />
            <span class="mt-2 block text-xs leading-5 text-gray-500 dark:text-dark-300">{{ t('community.botUsernameHint') }}</span>
          </label>
          </div>
          <div class="divide-y divide-gray-200 rounded-xl border border-gray-200 bg-gray-50/70 dark:divide-dark-600 dark:border-dark-600 dark:bg-dark-900/40">
          <label class="flex cursor-pointer items-start justify-between gap-4 p-4">
            <span class="min-w-0"><span class="block text-sm font-medium">{{ t('community.vipOnly') }}</span><span class="mt-1 block text-xs leading-5 text-gray-500 dark:text-dark-300">{{ t('community.vipEligibility') }}</span></span>
            <span class="relative mt-0.5 inline-flex shrink-0"><input v-model="form.require_paid_recharge" name="require-paid-recharge" type="checkbox" role="switch" :aria-checked="form.require_paid_recharge" class="peer sr-only" /><span class="community-switch" aria-hidden="true"></span></span>
          </label>
          <label class="flex items-start justify-between gap-4 p-4" :class="form.require_paid_recharge ? 'cursor-pointer' : 'cursor-not-allowed opacity-60'">
            <span class="min-w-0"><span class="block text-sm font-medium">{{ t('community.loginPrompt') }}</span><span class="mt-1 block text-xs leading-5 text-gray-500 dark:text-dark-300">{{ t('community.loginPromptHint') }}</span></span>
            <span class="relative mt-0.5 inline-flex shrink-0"><input v-model="form.login_prompt_enabled" name="login-prompt-enabled" type="checkbox" role="switch" :aria-checked="form.login_prompt_enabled" class="peer sr-only" :disabled="!form.require_paid_recharge" /><span class="community-switch" aria-hidden="true"></span></span>
          </label>
          </div>
        </fieldset>
        <div class="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-gray-200 bg-white p-4 lg:col-span-2 dark:border-dark-700 dark:bg-dark-800"><p class="max-w-3xl text-xs leading-5 text-gray-500 dark:text-dark-300">{{ t('community.validateHint') }}</p><button type="submit" class="btn btn-primary min-w-28 gap-2" :disabled="saving"><Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="saving ? 'animate-spin' : ''" aria-hidden="true" />{{ t(saving ? 'community.saving' : 'common.save') }}</button></div>
      </form>
      <button v-else type="button" class="btn btn-secondary" @click="load">{{ t('community.refresh') }}</button>
    </div>
  </CommunicationsLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import CommunicationsLayout from '@/components/support/CommunicationsLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { communityAPI, type CommunitySettings } from '@/api/community'
import { supportError } from '@/api/support'
import { safeCommunityContactURL } from '@/utils/communityLinks'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const app = useAppStore()
const form = ref<CommunitySettings>({ enabled: false, require_paid_recharge: false, login_prompt_enabled: false, contact_url: '', group_chat_id: '', group_name: '', bot_username: '' })
const loading = ref(true)
const loaded = ref(false)
const saving = ref(false)
const error = ref('')
let disposed = false

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await communityAPI.settings()
    if (!disposed) { form.value = { ...data, require_paid_recharge: data.require_paid_recharge ?? false, login_prompt_enabled: data.login_prompt_enabled ?? false }; loaded.value = true }
  } catch (cause) { if (!disposed) error.value = supportError(cause, t('community.loadFailed')) }
  finally { if (!disposed) loading.value = false }
}

async function save() {
  if (saving.value) return
  error.value = ''
  const contactURL = form.value.contact_url.trim()
  const groupChatID = form.value.group_chat_id.trim()
  const botUsername = form.value.bot_username.trim().replace(/^@/, '')
  if (contactURL && !safeCommunityContactURL(contactURL)) { error.value = t('community.invalidContactURL'); return }
  if (form.value.enabled && !/^-[1-9]\d{0,18}$/.test(groupChatID)) { error.value = t('community.invalidChatID'); return }
  if (form.value.enabled && !/^[A-Za-z0-9_]{5,32}$/.test(botUsername)) { error.value = t('community.invalidBotUsername'); return }
  saving.value = true
  try {
    const data = await communityAPI.saveSettings({ enabled: form.value.enabled, require_paid_recharge: form.value.require_paid_recharge, login_prompt_enabled: form.value.login_prompt_enabled, contact_url: contactURL, group_chat_id: groupChatID, group_name: form.value.group_name.trim(), bot_username: botUsername })
    if (!disposed) { form.value = data; app.showSuccess(t('community.saved')) }
  } catch (cause) { if (!disposed) error.value = supportError(cause, t('community.saveFailed')) }
  finally { if (!disposed) saving.value = false }
}

onMounted(load)
onBeforeUnmount(() => { disposed = true })
</script>

<style scoped>
.community-switch {
  @apply inline-flex h-6 w-11 rounded-full bg-gray-300 p-0.5 transition-colors after:h-5 after:w-5 after:rounded-full after:bg-white after:shadow-sm after:transition-transform peer-checked:bg-primary-600 peer-checked:after:translate-x-5 peer-focus-visible:ring-2 peer-focus-visible:ring-primary-500 peer-focus-visible:ring-offset-2 peer-disabled:cursor-not-allowed dark:bg-dark-500 dark:peer-checked:bg-primary-500 dark:peer-focus-visible:ring-offset-dark-800;
}
</style>
