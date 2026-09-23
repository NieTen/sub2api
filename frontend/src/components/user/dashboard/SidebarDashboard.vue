<template>
  <div ref="pageContainer" data-testid="sidebar-dashboard" />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import apiClient from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { createSidebarPageOverlay } from './sidebar-overlay.js'

const props = defineProps<{ mode: 'dashboard' | 'guide' }>()
const router = useRouter()
const authStore = useAuthStore()
const pageContainer = ref<HTMLElement | null>(null)
let page: ReturnType<typeof createSidebarPageOverlay> | undefined

onMounted(() => {
  if (!pageContainer.value) return

  // 复用原页面效果，挂载和清理由组件管理，请求沿用登录续期与错误处理。
  page = createSidebarPageOverlay({
    target: pageContainer.value,
    navigate: (path) => { void router.push(path) },
    request: async (path) => {
      if (path === '/auth/me') {
        await authStore.refreshUser()
        return authStore.user
      }
      const { data } = await apiClient.get(path)
      return data
    },
  })
  page.render(props.mode)
})

watch(() => props.mode, (mode) => page?.render(mode))
onBeforeUnmount(() => page?.destroy())
</script>
