<template>
  <AppLayout>
    <div class="space-y-6">
      <section class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-wrap items-start justify-between gap-4 px-5 py-5 sm:px-6">
          <div class="min-w-0">
            <h1 class="text-xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('communications.title') }}</h1>
            <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('communications.description') }}</p>
          </div>
          <router-link to="/community" class="btn btn-secondary btn-sm shrink-0">
            <Icon name="externalLink" size="sm" aria-hidden="true" />
            {{ t('communications.userPage') }}
          </router-link>
        </div>
        <nav :aria-label="t('communications.navigation')" class="grid grid-cols-2 border-t border-gray-100 p-2 sm:flex sm:flex-wrap sm:gap-1 dark:border-dark-700">
          <router-link
            v-for="item in sections"
            :key="item.key"
            :to="item.path"
            :aria-current="active === item.key ? 'page' : undefined"
            class="flex min-h-11 items-center gap-2 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
            :class="active === item.key ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white'"
          >
            <Icon :name="item.icon" size="sm" aria-hidden="true" />
            {{ t(item.label) }}
          </router-link>
        </nav>
      </section>
      <slot />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ active: 'bot' | 'mail' | 'community' | 'members' | 'tickets' }>()
const { t } = useI18n()
const sections = [
  { key: 'bot', path: '/admin/communications', label: 'communications.bot', icon: 'cog' },
  { key: 'mail', path: '/admin/bulk-emails', label: 'communications.mail', icon: 'inbox' },
  { key: 'community', path: '/admin/community/settings', label: 'communications.community', icon: 'users' },
  { key: 'members', path: '/admin/community/members', label: 'communications.members', icon: 'link' },
  { key: 'tickets', path: '/admin/tickets', label: 'communications.tickets', icon: 'chat' }
] as const
</script>
