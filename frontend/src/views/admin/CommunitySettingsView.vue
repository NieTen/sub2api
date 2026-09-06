<template>
  <CommunicationsLayout active="community">
    <div class="mx-auto max-w-3xl space-y-5 text-gray-900 dark:text-gray-100">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('community.settingsDescription') }}</p>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>
      <div v-if="loading" class="p-10 text-center text-gray-500">{{ t('common.loading') }}</div>
      <form v-else-if="loaded" class="space-y-5" @submit.prevent="save">
        <fieldset :disabled="saving" class="space-y-5 rounded-xl border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-800">
          <label class="block">
            <span class="mb-1 block text-sm font-medium">{{ t('community.contactURL') }}</span>
            <input v-model="form.contact_url" name="contact-url" type="url" class="input" placeholder="https://example.com/support" maxlength="2048" />
            <span class="mt-2 block text-xs text-gray-500">{{ t('community.contactURLHint') }}</span>
          </label>
        </fieldset>
        <fieldset :disabled="saving" class="space-y-5 rounded-xl border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-800">
          <label class="flex items-center gap-3 font-medium"><input v-model="form.enabled" name="community-enabled" type="checkbox" class="h-4 w-4 rounded" />{{ t('community.enabled') }}</label>
          <div class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <label class="flex items-center gap-3 text-sm font-medium"><input v-model="form.require_paid_recharge" name="require-paid-recharge" type="checkbox" class="h-4 w-4 rounded" />{{ t('community.vipOnly') }}</label>
            <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('community.vipEligibility') }}</p>
            <label class="flex items-center gap-3 text-sm font-medium"><input v-model="form.login_prompt_enabled" name="login-prompt-enabled" type="checkbox" class="h-4 w-4 rounded" :disabled="!form.require_paid_recharge" />{{ t('community.loginPrompt') }}</label>
            <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('community.loginPromptHint') }}</p>
          </div>
          <label class="block"><span class="mb-1 block text-sm font-medium">{{ t('community.groupName') }}</span><input v-model="form.group_name" name="group-name" class="input" maxlength="100" /></label>
          <label class="block"><span class="mb-1 block text-sm font-medium">{{ t('community.groupChatID') }}</span><input v-model="form.group_chat_id" name="group-chat-id" class="input" inputmode="numeric" :required="form.enabled" placeholder="-1001234567890" /></label>
          <label class="block">
            <span class="mb-1 block text-sm font-medium">{{ t('community.botUsername') }}</span>
            <input v-model="form.bot_username" name="bot-username" class="input" :required="form.enabled" placeholder="support_bot" maxlength="33" />
            <span class="mt-2 block text-xs text-gray-500">{{ t('community.botUsernameHint') }}</span>
          </label>
          <div class="space-y-3 rounded-lg bg-gray-50 p-4 text-sm leading-6 dark:bg-dark-900/50">
            <h2 class="font-semibold">{{ t('community.prerequisites') }}</h2>
            <p>{{ t('community.privateGroup') }}</p>
            <p>{{ t('community.reuseBot') }} <router-link to="/admin/communications" class="text-primary-600 underline dark:text-primary-400">{{ t('community.configureBot') }}</router-link></p>
            <p>{{ t('community.webhook') }}</p>
          </div>
          <p class="text-xs text-gray-500">{{ t('community.validateHint') }}</p>
        </fieldset>
        <div class="flex justify-end"><button type="submit" class="btn btn-primary" :disabled="saving">{{ t(saving ? 'community.saving' : 'common.save') }}</button></div>
      </form>
      <button v-else type="button" class="btn btn-secondary" @click="load">{{ t('community.refresh') }}</button>
    </div>
  </CommunicationsLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import CommunicationsLayout from '@/components/support/CommunicationsLayout.vue'
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
