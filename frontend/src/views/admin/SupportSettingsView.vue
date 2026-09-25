<template>
  <CommunicationsLayout active="bot">
    <div class="space-y-5 text-gray-900 dark:text-gray-100">
      <p class="max-w-3xl text-sm text-gray-500 dark:text-dark-400">{{ t('support.settingsDescription') }}</p>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-600 dark:bg-red-900/20">{{ error }}</p>
      <div v-if="loading" class="p-8 text-center text-gray-500">{{ t('common.loading') }}</div>
      <form v-else-if="loaded" class="grid items-start gap-5 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]" @submit.prevent="save">
        <fieldset :disabled="saving" class="min-w-0 space-y-5 rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
          <h2 class="text-base font-semibold">{{ t('communications.emailSection') }}</h2>
          <label class="flex items-center gap-3 font-medium"><input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded" />{{ t('support.enabled') }}</label>
          <label class="block"><span class="mb-1 block text-sm font-medium">{{ t('support.adminEmails') }}</span><textarea v-model="emailText" rows="3" class="input" :placeholder="'support@example.com'"></textarea><span class="mt-2 block text-xs text-gray-500">{{ t('support.adminEmailsHint') }}</span></label>
          <p class="text-xs text-gray-500">{{ t('support.emailHelp') }} <router-link to="/admin/settings?tab=email" class="text-primary-600 underline">{{ t('nav.settings') }}</router-link></p>
        </fieldset>
        <fieldset :disabled="saving" class="min-w-0 space-y-5 rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
          <div><h2 class="text-base font-semibold">{{ t('communications.botSection') }}</h2><p class="mt-2 text-sm leading-6 text-gray-500">{{ t('support.telegramHelp') }}</p><p class="mt-2 text-xs leading-5 text-gray-500">{{ t('communications.botCredentialsHint') }}</p></div>
          <label class="block"><span class="mb-1 block text-sm font-medium">{{ t('support.botToken') }}</span><input v-model="form.telegram_bot_token" type="password" autocomplete="new-password" class="input" :disabled="form.clear_telegram_bot_token" :placeholder="t(form.telegram_bot_token_configured ? 'support.secretPlaceholder' : 'support.notConfigured')" /></label>
          <label v-if="form.telegram_bot_token_configured" class="flex items-center gap-2 text-sm"><input v-model="form.clear_telegram_bot_token" type="checkbox" />{{ t('support.clearToken') }}</label>
          <label class="block">
            <span class="mb-1 block text-sm font-medium">{{ t('support.chatId') }}</span>
            <input v-model="form.telegram_chat_id" type="text" inputmode="numeric" class="input" placeholder="-1001234567890" aria-describedby="support-chat-id-hint" />
            <span id="support-chat-id-hint" class="mt-2 block text-xs text-gray-500">{{ t('support.chatIdHint') }}</span>
          </label>
          <label class="block"><span class="mb-1 block text-sm font-medium">{{ t('support.allowedUsers') }}</span><textarea v-model="allowedUserText" rows="2" class="input" placeholder="123456789" aria-describedby="support-allowed-users-hint"></textarea><span id="support-allowed-users-hint" class="mt-2 block text-xs text-gray-500">{{ t('support.allowedUsersHint') }}</span></label>
          <label class="block">
            <span class="mb-1 block text-sm font-medium">{{ t('support.webhookSecret') }}</span>
            <input v-model="form.telegram_webhook_secret" type="password" autocomplete="new-password" class="input" aria-describedby="support-webhook-secret-hint" :disabled="form.clear_telegram_webhook_secret" :placeholder="t(form.telegram_webhook_secret_configured ? 'support.secretPlaceholder' : 'support.notConfigured')" />
            <span id="support-webhook-secret-hint" class="mt-2 block text-xs text-gray-500">{{ t('support.webhookSecretHint') }}</span>
          </label>
          <label v-if="form.telegram_webhook_secret_configured" class="flex items-center gap-2 text-sm"><input v-model="form.clear_telegram_webhook_secret" type="checkbox" />{{ t('support.clearSecret') }}</label>
          <div v-if="webhookUrl" class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900">
            <p class="text-sm font-medium">{{ t('support.webhookUrl') }}</p>
            <input :value="webhookUrl" readonly class="input mt-2 font-mono text-xs" :aria-label="t('support.webhookUrl')" @focus="($event.target as HTMLInputElement).select()" />
            <p role="status" class="mt-2 text-xs" :class="webhookRegistered ? 'text-green-700 dark:text-green-400' : 'text-gray-500'">{{ t(webhookRegistered ? 'support.webhookRegistered' : 'support.webhookNotRegistered') }}</p>
            <p class="mt-2 text-xs text-gray-500">{{ t('support.webhookHelp') }}</p>
          </div>
        </fieldset>
        <div class="flex justify-end xl:col-span-2"><button class="btn btn-primary" :disabled="saving">{{ t(saving ? 'common.saving' : 'common.save') }}</button></div>
      </form>
      <button v-else class="btn btn-secondary" @click="load">{{ t('support.refresh') }}</button>
    </div>
  </CommunicationsLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import CommunicationsLayout from '@/components/support/CommunicationsLayout.vue'
import { supportAPI, supportError, type SupportSettings } from '@/api/support'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const app = useAppStore()
const form = ref<SupportSettings>({ enabled: false, admin_emails: [], telegram_chat_id: '', telegram_allowed_user_ids: [] })
const emailText = ref('')
const allowedUserText = ref('')
const loading = ref(false)
const loaded = ref(false)
const saving = ref(false)
const error = ref('')
const defaultWebhookPath = '/api/v1/support/telegram/webhook'
const webhookUrl = computed(() => {
  // 新版后端返回路径，旧版后端返回完整 URL；两者都保留以兼容滚动升级。
  const path = form.value.telegram_webhook_path?.trim()
  if (path) return new URL(path, window.location.origin).toString()
  const legacyUrl = form.value.telegram_webhook_url?.trim()
  return legacyUrl || new URL(defaultWebhookPath, window.location.origin).toString()
})
const webhookRegistered = computed(() => Boolean(
  form.value.telegram_webhook_registered && form.value.telegram_webhook_url === webhookUrl.value &&
  !form.value.telegram_bot_token?.trim() && !form.value.telegram_webhook_secret?.trim() &&
  !form.value.clear_telegram_bot_token && !form.value.clear_telegram_webhook_secret
))
const split = (value: string) => value.split(/[\s,，;；]+/).map(item => item.trim()).filter(Boolean)

function apply(data: SupportSettings) {
  // 敏感值不在表单中回显；每次保存后立即清空输入。
  form.value = { ...data, telegram_bot_token: '', telegram_webhook_secret: '', clear_telegram_bot_token: false, clear_telegram_webhook_secret: false }
  emailText.value = (data.admin_emails || []).join('\n')
  allowedUserText.value = (data.telegram_allowed_user_ids || []).join(', ')
}

async function load() {
  loading.value = true
  error.value = ''
  try { apply(await supportAPI.settings()); loaded.value = true }
  catch (cause) { error.value = supportError(cause, t('support.loadFailed')) }
  finally { loading.value = false }
}

async function save() {
  if (saving.value) return
  const ids = split(allowedUserText.value)
  const emails = split(emailText.value)
  if (ids.some(id => !/^\d+$/.test(id) || !Number.isSafeInteger(Number(id)) || Number(id) <= 0)) {
    error.value = t('support.invalidIds'); return
  }
  if (emails.some(email => !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email))) {
    error.value = t('support.invalidEmails'); return
  }
  const webhookSecret = form.value.clear_telegram_webhook_secret ? '' : form.value.telegram_webhook_secret?.trim()
  // 只校验本次新输入的密钥；留空仍交由后端保留已配置的值。
  if (webhookSecret && !/^[A-Za-z0-9_-]{16,256}$/.test(webhookSecret)) {
    error.value = t('support.invalidWebhookSecret'); return
  }
  const botToken = form.value.clear_telegram_bot_token ? '' : form.value.telegram_bot_token?.trim()
  const hasBotToken = !form.value.clear_telegram_bot_token && Boolean(botToken || form.value.telegram_bot_token_configured)
  const hasWebhookSecret = !form.value.clear_telegram_webhook_secret && Boolean(webhookSecret || form.value.telegram_webhook_secret_configured)
  const registerWebhook = hasBotToken && hasWebhookSecret
  // 社群与工单共用回调；关闭工单通知仍需要接收入群和机器人消息。
  if (registerWebhook && !webhookUrl.value.startsWith('https://')) {
    error.value = t('support.invalidWebhookUrl'); return
  }
  saving.value = true
  error.value = ''
  try {
    apply(await supportAPI.saveSettings({
      ...form.value,
      admin_emails: [...new Set(emails)],
      telegram_allowed_user_ids: [...new Set(ids.map(Number))],
      telegram_chat_id: form.value.telegram_chat_id.trim(),
      telegram_bot_token: botToken,
      telegram_webhook_secret: webhookSecret,
      telegram_webhook_url: registerWebhook ? webhookUrl.value : '',
      telegram_webhook_registered: undefined
    }))
    app.showSuccess(t('support.saved'))
  } catch (cause) { error.value = supportError(cause, t('support.saveFailed')) }
  finally { saving.value = false }
}

onMounted(load)
</script>
