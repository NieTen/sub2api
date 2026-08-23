<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.dataImportTitle')"
    width="wide"
    close-on-click-outside
    @close="handleClose"
  >
    <form id="import-data-form" class="space-y-4" @submit.prevent="handleImport">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.dataImportHint') }}
      </div>
      <div
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
      >
        {{ t('admin.accounts.dataImportWarning') }}
      </div>

      <div class="grid gap-4 lg:grid-cols-2">
        <section class="space-y-3">
          <div>
            <label class="input-label">{{ t('admin.accounts.dataImportText') }}</label>
            <textarea
              v-model="sourceText"
              data-testid="account-import-input"
              class="input min-h-[240px] w-full resize-y font-mono text-xs leading-5"
              :placeholder="t('admin.accounts.dataImportPlaceholder')"
              spellcheck="false"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.accounts.dataImportTextHint') }}
            </p>
          </div>

          <div class="flex flex-wrap gap-2">
            <button type="button" class="btn btn-secondary" @click="openFilePicker">
              {{ t('common.chooseFile') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="!hasAnyInput"
              @click="clearInputs"
            >
              {{ t('admin.accounts.dataImportClear') }}
            </button>
          </div>

          <div
            data-testid="account-import-dropzone"
            class="flex items-center justify-between gap-3 rounded-lg border border-dashed px-4 py-3 transition-colors"
            :class="dragActive
              ? 'border-primary-400 bg-primary-50/70 dark:border-primary-500 dark:bg-primary-900/20'
              : 'border-gray-300 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'"
            @dragenter.prevent="handleDragEnter"
            @dragover.prevent
            @dragleave.prevent="handleDragLeave"
            @drop.prevent="handleDrop"
          >
            <div class="min-w-0">
              <div class="truncate text-sm text-gray-700 dark:text-dark-200" :title="selectedFilesTitle">
                {{ selectedFilesLabel || t('admin.accounts.dataImportSelectFile') }}
              </div>
              <div class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.accounts.dataImportDropHint') }}
              </div>
            </div>
            <button type="button" class="btn btn-secondary shrink-0" @click="openFilePicker">
              {{ t('common.chooseFile') }}
            </button>
          </div>

          <input
            ref="fileInput"
            type="file"
            class="hidden"
            accept="application/json,.json"
            multiple
            @change="handleFileChange"
          />

          <div
            v-if="files.length > 0"
            class="rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400"
            :title="selectedFilesTitle"
          >
            {{ t('admin.accounts.selectedCount', { count: files.length }) }}
            <span class="mx-1">·</span>
            <span class="break-all">{{ selectedFilesTitle }}</span>
          </div>

          <div
            v-if="parseError"
            class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300"
          >
            {{ parseError }}
          </div>
        </section>

        <section class="space-y-3">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.accounts.dataImportOutputTitle') }}
              </div>
              <div class="text-sm text-gray-600 dark:text-dark-300">
                {{ t('admin.accounts.dataImportOutputHint') }}
              </div>
            </div>
            <div class="flex flex-wrap justify-end gap-2">
              <button
                type="button"
                class="btn btn-secondary"
                data-testid="account-import-copy"
                :disabled="!outputText"
                @click="handleCopy"
              >
                {{ t('admin.accounts.dataImportCopy') }}
              </button>
              <button
                type="button"
                class="btn btn-primary"
                data-testid="account-import-download"
                :disabled="!outputText"
                @click="handleDownload"
              >
                {{ t('admin.accounts.dataImportDownload') }}
              </button>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <div class="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.dataImportInputRecords') }}</div>
              <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ previewInputCount }}</div>
            </div>
            <div class="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.dataImportAccounts') }}</div>
              <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ previewAccountCount }}</div>
            </div>
            <div class="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.dataImportProxies') }}</div>
              <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ previewProxyCount }}</div>
            </div>
            <div class="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.dataImportSkipped') }}</div>
              <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ previewSkippedCount }}</div>
            </div>
          </div>

          <div class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
            <div class="border-b border-gray-200 bg-gray-50 px-3 py-2 text-sm font-medium text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200">
              {{ t('admin.accounts.dataImportPreviewTitle') }}
            </div>
            <div class="max-h-64 overflow-auto">
              <table class="min-w-full table-fixed text-left text-sm">
                <thead class="sticky top-0 bg-white text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-dark-400">
                  <tr>
                    <th class="px-3 py-2 font-medium">{{ t('admin.accounts.columns.name') }}</th>
                    <th class="px-3 py-2 font-medium">{{ t('admin.accounts.dataImportSource') }}</th>
                    <th class="px-3 py-2 font-medium">{{ t('admin.accounts.columns.platform') }}</th>
                    <th class="px-3 py-2 font-medium">{{ t('admin.accounts.columns.type') }}</th>
                    <th class="px-3 py-2 font-medium">{{ t('admin.accounts.columns.status') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="(row, index) in previewRows" :key="`${row.source}-${row.name}-${index}`">
                    <td class="px-3 py-2 text-gray-900 dark:text-white">{{ row.name }}</td>
                    <td class="px-3 py-2">
                      <span
                        class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
                        :class="row.source === 'CPA'
                          ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
                          : 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'"
                      >
                        {{ row.source === 'CPA' ? t('admin.accounts.dataImportSourceCpa') : t('admin.accounts.dataImportSourceSub2Api') }}
                      </span>
                    </td>
                    <td class="px-3 py-2 text-gray-700 dark:text-dark-200">{{ row.platform }}</td>
                    <td class="px-3 py-2 text-gray-700 dark:text-dark-200">{{ row.type }}</td>
                    <td class="px-3 py-2">
                      <span class="inline-flex rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
                        {{ row.status }}
                      </span>
                    </td>
                  </tr>
                  <tr v-if="!previewRows.length">
                    <td colspan="5" class="px-3 py-6 text-center text-sm text-gray-500 dark:text-dark-400">
                      {{ t('admin.accounts.dataImportPreviewEmpty') }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div
            v-if="warnings.length"
            class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
          >
            <div class="font-medium">{{ t('admin.accounts.dataImportIssues') }}</div>
            <ul class="mt-1 space-y-1 text-xs">
              <li v-for="(item, index) in warnings" :key="`${item.source}-${index}`">
                {{ item.source }}: {{ item.reason }}
              </li>
            </ul>
          </div>

          <textarea
            ref="outputTextarea"
            data-testid="account-import-output"
            class="input min-h-[240px] w-full resize-y font-mono text-xs leading-5"
            readonly
            :value="outputText"
            :placeholder="t('admin.accounts.dataImportOutputPlaceholder')"
            spellcheck="false"
          />
        </section>
      </div>

      <div
        v-if="result"
        class="space-y-2 rounded-xl border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.dataImportResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.dataImportResultSummary', result) }}
        </div>

        <div v-if="errorItems.length" class="mt-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.dataImportErrors') }}
          </div>
          <div
            class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800"
          >
            <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
              {{ item.kind }} {{ item.name || item.proxy_key || '-' }} — {{ item.message }}
            </div>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          class="btn btn-primary"
          type="button"
          data-testid="account-import-submit"
          :disabled="importing"
          @click="handleImport"
        >
          {{ importing ? t('admin.accounts.dataImporting') : t('admin.accounts.dataImportButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import type { AdminDataImportResult } from '@/types'
import { buildImportPayload, parseInputText, type ImportParseIssue, type ImportPreviewRow, type ImportSourceEntry, type NormalizedImportResult } from './importDataParser'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const importing = ref(false)
const sourceText = ref('')
const files = ref<File[]>([])
const dragDepth = ref(0)
const parseError = ref('')
const previewState = ref<NormalizedImportResult | null>(null)
const result = ref<AdminDataImportResult | null>(null)
const hasCreatedData = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const outputTextarea = ref<HTMLTextAreaElement | null>(null)
let refreshVersion = 0

const dragActive = computed(() => dragDepth.value > 0)
const hasAnyInput = computed(() => sourceText.value.trim().length > 0 || files.value.length > 0)
const previewRows = computed<ImportPreviewRow[]>(() => previewState.value?.previewRows || [])
const warnings = computed<ImportParseIssue[]>(() => previewState.value?.issues || [])
const previewInputCount = computed(() => previewState.value?.inputCount || 0)
const previewAccountCount = computed(() => previewState.value?.accountCount || 0)
const previewProxyCount = computed(() => previewState.value?.proxyCount || 0)
const previewSkippedCount = computed(() => previewState.value?.skippedCount || 0)
const outputText = computed(() => previewState.value?.outputText || '')
const errorItems = computed(() => result.value?.errors || [])
const selectedFilesLabel = computed(() => {
  if (files.value.length === 0) return ''
  if (files.value.length === 1) return files.value[0]?.name || ''
  return t('admin.accounts.selectedCount', { count: files.value.length })
})
const selectedFilesTitle = computed(() => files.value.map((item) => item.name).join(', '))

watch(
  () => props.show,
  (open) => {
    if (open) {
      resetState()
    }
  }
)

watch(
  [sourceText, files],
  () => {
    void refreshPreview()
  },
  { deep: false }
)

const resetState = () => {
  refreshVersion += 1
  sourceText.value = ''
  files.value = []
  dragDepth.value = 0
  parseError.value = ''
  previewState.value = null
  result.value = null
  hasCreatedData.value = false
  if (fileInput.value) {
    fileInput.value.value = ''
  }
  if (outputTextarea.value) {
    outputTextarea.value.value = ''
  }
}

const openFilePicker = () => {
  if (importing.value) return
  fileInput.value?.click()
}

const isJsonFile = (sourceFile: File) => {
  const name = sourceFile.name.toLowerCase()
  return name.endsWith('.json') || sourceFile.type === 'application/json'
}

const setSelectedFiles = (sourceFiles: FileList | File[] | null | undefined) => {
  if (importing.value) return
  const incoming = Array.from(sourceFiles || [])
  const picked = incoming.filter(isJsonFile)
  if (!picked.length) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }
  if (picked.length < incoming.length) {
    appStore.showWarning(
      t('admin.accounts.dataImportIgnoredFiles', { count: incoming.length - picked.length })
    )
  }
  files.value = picked
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  setSelectedFiles(target.files)
  target.value = ''
}

const handleDragEnter = () => {
  if (importing.value) return
  dragDepth.value += 1
}

const handleDragLeave = () => {
  dragDepth.value = Math.max(0, dragDepth.value - 1)
}

const handleDrop = (event: DragEvent) => {
  dragDepth.value = 0
  if (importing.value) return
  setSelectedFiles(event.dataTransfer?.files)
}

const readFileAsText = async (sourceFile: File): Promise<string> => {
  if (typeof sourceFile.text === 'function') {
    return sourceFile.text()
  }

  if (typeof sourceFile.arrayBuffer === 'function') {
    const buffer = await sourceFile.arrayBuffer()
    return new TextDecoder().decode(buffer)
  }

  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error || new Error(t('admin.accounts.dataImportFailed')))
    reader.readAsText(sourceFile)
  })
}

const refreshPreview = async (): Promise<NormalizedImportResult | null> => {
  const version = ++refreshVersion

  if (!hasAnyInput.value) {
    if (version === refreshVersion) {
      parseError.value = ''
      previewState.value = null
    }
    return null
  }

  try {
    const entries: ImportSourceEntry[] = []

    const text = sourceText.value.trim()
    if (text) {
      const parsedText = parseInputText(text, t('admin.accounts.dataImportTextSource'))
      if (parsedText.errors.length > 0) {
        throw new Error(t('admin.accounts.dataImportParseFailed'))
      }
      entries.push(...parsedText.entries)
    }

    for (const sourceFile of files.value) {
      const fileText = await readFileAsText(sourceFile)
      const parsedFile = parseInputText(fileText, sourceFile.name)
      if (parsedFile.errors.length > 0) {
        throw new Error(t('admin.accounts.dataImportParseFailedFile', { name: sourceFile.name }))
      }
      entries.push(...parsedFile.entries)
    }

    if (version !== refreshVersion) return previewState.value

    if (!entries.length) {
      previewState.value = null
      parseError.value = ''
      return null
    }

    const normalized = buildImportPayload(entries)
    if (normalized.accountCount === 0 && normalized.proxyCount === 0) {
      throw new Error(
        files.value.length > 0
          ? t('admin.accounts.dataImportInvalidFile', {
              name: selectedFilesTitle.value || files.value[0]?.name || t('admin.accounts.dataImportTextSource')
            })
          : t('admin.accounts.dataImportParseFailed')
      )
    }
    previewState.value = normalized
    parseError.value = ''
    return normalized
  } catch (error: any) {
    if (version === refreshVersion) {
      previewState.value = null
      parseError.value = error?.message || t('admin.accounts.dataImportFailed')
    }
    return null
  }
}

const ensurePreview = async (): Promise<NormalizedImportResult | null> => {
  return refreshPreview()
}

const clearInputs = () => {
  if (importing.value) return
  resetState()
}

const handleCopy = async () => {
  const state = await ensurePreview()
  if (!state) {
    appStore.showError(parseError.value || t('admin.accounts.dataImportFailed'))
    return
  }
  await copyToClipboard(state.outputText, t('admin.accounts.dataImportCopied'))
}

const handleDownload = async () => {
  const state = await ensurePreview()
  if (!state) {
    appStore.showError(parseError.value || t('admin.accounts.dataImportFailed'))
    return
  }

  const blob = new Blob([state.outputText], { type: 'application/json' })
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `sub2api-account-import-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.json`
  link.click()
  window.URL.revokeObjectURL(url)
}

const handleClose = () => {
  if (importing.value) return
  if (hasCreatedData.value) {
    hasCreatedData.value = false
    emit('imported')
  }
  emit('close')
}

const handleImport = async () => {
  if (importing.value) return
  if (!hasAnyInput.value) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }

  importing.value = true
  try {
    const state = await ensurePreview()
    if (!state) {
      appStore.showError(parseError.value || t('admin.accounts.dataImportFailed'))
      return
    }

    const res = await adminAPI.accounts.importData({
      data: state.payload,
      skip_default_group_bind: true
    })

    result.value = res

    const msgParams: Record<string, unknown> = {
      account_created: res.account_created,
      account_failed: res.account_failed,
      proxy_created: res.proxy_created,
      proxy_reused: res.proxy_reused,
      proxy_failed: res.proxy_failed
    }

    if (res.account_failed > 0 || res.proxy_failed > 0) {
      if (res.account_created > 0 || res.proxy_created > 0) {
        hasCreatedData.value = true
      }
      appStore.showError(t('admin.accounts.dataImportCompletedWithErrors', msgParams))
    } else {
      appStore.showSuccess(t('admin.accounts.dataImportSuccess', msgParams))
      emit('imported')
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.dataImportFailed'))
  } finally {
    importing.value = false
  }
}
</script>
