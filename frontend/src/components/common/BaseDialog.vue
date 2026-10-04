<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="modal-overlay"
        :style="zIndexStyle"
        :aria-labelledby="dialogId"
        role="dialog"
        aria-modal="true"
        @click.self="handleClose"
      >
        <!-- Modal panel -->
        <div ref="dialogRef" :class="['modal-content', widthClasses, panelClass]" tabindex="-1" @click.stop>
          <!-- Header -->
          <div class="modal-header gap-3">
            <h3 :id="dialogId" class="modal-title min-w-0 break-words [overflow-wrap:anywhere]">
              {{ title }}
            </h3>
            <button
              v-if="showCloseButton"
              type="button"
              @click="emit('close')"
              class="-mr-2 inline-flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center rounded-xl p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-dark-200 dark:focus-visible:ring-offset-dark-900"
              :aria-label="t('common.close')"
            >
              <Icon name="x" size="md" />
            </button>
          </div>

          <!-- Body -->
          <div ref="modalBodyRef" :class="['modal-body', bodyClass]">
            <slot></slot>
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="modal-footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script lang="ts">
let dialogIdCounter = 0
const openDialogs = new Set<string>()
</script>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted, ref, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

// 生成唯一ID以避免多个对话框时ID冲突
const dialogId = `modal-title-${++dialogIdCounter}`
const { t } = useI18n()

// 焦点管理
const dialogRef = ref<HTMLElement | null>(null)
const modalBodyRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  zIndex?: number
  panelClass?: string
  bodyClass?: string
  trapFocus?: boolean
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  zIndex: 50
})

const emit = defineEmits<Emits>()

// Custom z-index style (overrides the default z-50 from CSS)
const zIndexStyle = computed(() => {
  return props.zIndex !== 50 ? { zIndex: props.zIndex } : undefined
})

const widthClasses = computed(() => {
  // Width guidance: narrow=confirm/short prompts, normal=standard forms,
  // wide=multi-section forms or rich content, extra-wide=analytics/tables,
  // full=full-screen or very dense layouts.
  const widths: Record<DialogWidth, string> = {
    narrow: 'max-w-md',
    normal: 'max-w-lg',
    wide: 'w-full sm:max-w-2xl md:max-w-3xl lg:max-w-4xl',
    'extra-wide': 'w-full sm:max-w-3xl md:max-w-4xl lg:max-w-5xl xl:max-w-6xl',
    full: 'w-full sm:max-w-4xl md:max-w-5xl lg:max-w-6xl xl:max-w-7xl'
  }
  return widths[props.width]
})

const handleClose = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  }
}

function topDialogElement(): HTMLElement | null {
  let top: HTMLElement | null = null
  let highestZIndex = -Infinity
  for (const id of openDialogs) {
    const overlay = document.getElementById(id)?.closest<HTMLElement>('[role="dialog"]')
    if (!overlay?.isConnected) continue
    const computedZIndex = Number.parseFloat(window.getComputedStyle(overlay).zIndex)
    const zIndex = Number.isFinite(computedZIndex) ? computedZIndex : 50
    // 同一层级按实际 DOM 顺序确定最上层，避免仅按打开时间误判 Teleport 顺序。
    if (!top || zIndex > highestZIndex || (zIndex === highestZIndex && (top.compareDocumentPosition(overlay) & Node.DOCUMENT_POSITION_FOLLOWING))) {
      top = overlay
      highestZIndex = zIndex
    }
  }
  return top
}

function isTopDialog(): boolean {
  return !!dialogRef.value && topDialogElement() === dialogRef.value.parentElement
}

function focusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(
    'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), summary, [tabindex]:not([tabindex="-1"])'
  )).filter(element => element.getClientRects().length > 0 && element.getAttribute('aria-hidden') !== 'true')
}

const handleEscape = (event: KeyboardEvent) => {
  if (!props.show || event.defaultPrevented || !isTopDialog()) return
  // 复杂配置弹窗可选择约束 Tab 焦点，避免键盘操作落到遮罩后的页面。
  if (props.trapFocus && event.key === 'Tab' && dialogRef.value) {
    const elements = focusableElements(dialogRef.value)
    const first = elements[0]
    const last = elements[elements.length - 1]
    const active = document.activeElement
    if (!first) {
      event.preventDefault()
      dialogRef.value.focus()
    } else if (event.shiftKey ? active === first || !dialogRef.value.contains(active) : active === last || !dialogRef.value.contains(active)) {
      event.preventDefault()
      ;(event.shiftKey ? last : first)?.focus()
    }
  }
  if (props.closeOnEscape && event.key === 'Escape') {
    event.preventDefault()
    emit('close')
  }
}

const updateScrollLock = (isOpen: boolean) => {
  if (isOpen) openDialogs.add(dialogId)
  else openDialogs.delete(dialogId)
  document.body.classList.toggle('modal-open', openDialogs.size > 0)
}

// Prevent body scroll when modal is open and manage focus
watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      // 保存当前焦点元素
      previousActiveElement = document.activeElement as HTMLElement
      // 使用CSS类而不是直接操作style,更易于管理多个对话框
      updateScrollLock(true)

      // 等待DOM更新后设置焦点到对话框
      await nextTick()
      if (modalBodyRef.value) {
        modalBodyRef.value.scrollTop = 0
      }
      if (props.show && dialogRef.value && isTopDialog()) {
        const focusTarget = focusableElements(dialogRef.value)[0] || dialogRef.value
        focusTarget.focus()
      }
    } else {
      updateScrollLock(false)
      // 关闭后台弹窗不能抢走前台焦点；关闭前台后优先恢复到仍打开的弹窗。
      const remainingTop = topDialogElement()
      if (previousActiveElement?.isConnected && (!remainingTop || remainingTop.contains(previousActiveElement))) {
        previousActiveElement.focus()
      } else if (previousActiveElement && remainingTop && !remainingTop.contains(document.activeElement)) {
        const focusTarget = focusableElements(remainingTop)[0] || remainingTop.querySelector<HTMLElement>('.modal-content')
        focusTarget?.focus()
      }
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  // 确保组件卸载时移除滚动锁定
  updateScrollLock(false)
})
</script>
