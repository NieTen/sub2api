<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950" :class="{ 'workspace-layout': isWorkspace }">
    <!-- 页面背景 -->
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <div :class="{ 'workspace-frame': isWorkspace, 'workspace-collapsed': isWorkspace && sidebarCollapsed }">
      <!-- 侧栏 -->
      <AppSidebar :workspace="isWorkspace" />

      <!-- 主内容区域 -->
      <div
        class="relative min-h-screen transition-all duration-300"
        :class="isWorkspace ? 'workspace-content' : [sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
      >
        <!-- 顶部工具栏 -->
        <AppHeader :compact="isWorkspace" />

        <main :class="isWorkspace ? 'workspace-main' : 'p-4 md:p-6 lg:p-8'">
          <slot />
        </main>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const props = withDefaults(defineProps<{ variant?: 'default' | 'workspace' }>(), { variant: 'default' })
const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')
const isWorkspace = computed(() => props.variant === 'workspace' && !isAdmin.value)

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>

<style scoped>
.workspace-layout {
  padding: 24px;
  background: #edf6ff;
  background-image: radial-gradient(ellipse at 50% 0, #fff 0, transparent 66%), linear-gradient(145deg, #eaf6ff, #deefff 55%, #f3f9ff);
}

.workspace-layout > .bg-mesh-gradient { display: none; }

.workspace-frame {
  position: relative;
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr);
  max-width: 1440px;
  min-height: calc(100vh - 48px);
  margin: 0 auto;
  border: 1px solid rgb(255 255 255 / 90%);
  border-radius: 24px;
  background: rgb(255 255 255 / 54%);
  box-shadow: 0 20px 70px rgb(57 123 210 / 12%);
}

.workspace-collapsed { grid-template-columns: 72px minmax(0, 1fr); }
.workspace-content { min-width: 0; min-height: 0; }
.workspace-main { min-width: 0; padding: 0 22px 24px; }

.dark .workspace-layout {
  background: #0b1424;
  background-image: radial-gradient(ellipse at 50% 0, #162b48, transparent 70%);
}

.dark .workspace-frame {
  border-color: #263b58;
  background: rgb(17 31 50 / 90%);
  box-shadow: 0 20px 70px rgb(0 0 0 / 20%);
}

@media (max-width: 1023px) {
  .workspace-layout { padding: 12px; }
  .workspace-frame { display: block; min-height: calc(100vh - 24px); border-radius: 20px; }
  .workspace-main { padding: 0 16px 20px; }
}

@media (max-width: 479px) {
  .workspace-layout { padding: 0; }
  .workspace-frame { min-height: 100vh; border: 0; border-radius: 0; }
  .workspace-main { padding: 0 12px 16px; }
}
</style>
