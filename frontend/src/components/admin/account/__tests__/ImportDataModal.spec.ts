import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'

const { importData, copyToClipboard } = vi.hoisted(() => ({
  importData: vi.fn(),
  copyToClipboard: vi.fn()
}))

const showError = vi.fn()
const showSuccess = vi.fn()
const showWarning = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importData
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const mountModal = () =>
  mount(ImportDataModal, {
    props: { show: true },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true
      }
    }
  })

const makeJsonFile = (name: string, content: string) => {
  const file = new File([content], name, { type: 'application/json' })
  Object.defineProperty(file, 'text', {
    value: () => Promise.resolve(content)
  })
  return file
}

describe('ImportDataModal', () => {
  beforeEach(() => {
    importData.mockReset()
    copyToClipboard.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()
    Object.defineProperty(URL, 'createObjectURL', {
      value: vi.fn(() => 'blob:import-json'),
      configurable: true,
      writable: true
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      value: vi.fn(),
      configurable: true,
      writable: true
    })
  })

  it('支持直接输入 CPA JSON 并导入为 sub2api 数据', async () => {
    importData.mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0
    })

    const wrapper = mountModal()
    const textarea = wrapper.get('[data-testid="account-import-input"]')
    const source = JSON.stringify([
      {
        type: 'codex',
        email: 'alpha@example.com',
        access_token: 'alpha-access',
        refresh_token: 'alpha-refresh',
        id_token: 'alpha-id',
        expired: '2099-01-01T00:00:00Z'
      },
      {
        type: 'claude',
        email: 'beta@example.com',
        access_token: 'beta-access',
        refresh_token: 'beta-refresh',
        expired: '2099-01-01T00:00:00Z'
      }
    ])

    await textarea.setValue(source)
    await flushPromises()

    const output = wrapper.get('[data-testid="account-import-output"]')
    expect((output.element as HTMLTextAreaElement).value).toContain('"type": "sub2api-data"')
    expect((output.element as HTMLTextAreaElement).value).toContain('alpha@example.com')

    await wrapper.get('[data-testid="account-import-copy"]').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(
      expect.stringContaining('"type": "sub2api-data"'),
      'admin.accounts.dataImportCopied'
    )

    await wrapper.get('[data-testid="account-import-submit"]').trigger('click')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        type: 'sub2api-data',
        version: 1,
        proxies: [],
        accounts: expect.arrayContaining([
          expect.objectContaining({
            name: 'alpha@example.com',
            platform: 'openai',
            type: 'oauth'
          }),
          expect.objectContaining({
            name: 'beta@example.com',
            platform: 'anthropic',
            type: 'oauth'
          })
        ])
      }),
      skip_default_group_bind: true
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.dataImportSuccess')
  })

  it('拖拽多文件时会自动识别 CPA 和 sub2api 结构并合并导入', async () => {
    importData.mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0
    })

    const wrapper = mountModal()
    const dropzone = wrapper.get('[data-testid="account-import-dropzone"]')

    const sub2apiFile = makeJsonFile(
      'sub2api.json',
      JSON.stringify({
        type: 'sub2api-data',
        version: 1,
        exported_at: '2026-08-19T00:00:00Z',
        proxies: [],
        accounts: [
          {
            name: 'sub-account@example.com',
            platform: 'openai',
            type: 'oauth',
            credentials: {
              access_token: 'sub-access'
            },
            concurrency: 10,
            priority: 1,
            rate_multiplier: 1,
            auto_pause_on_expired: true
          }
        ]
      })
    )
    const cpaFile = makeJsonFile(
      'cpa.json',
      JSON.stringify({
        type: 'xai',
        email: 'grok@example.com',
        access_token: 'grok-access',
        refresh_token: 'grok-refresh',
        id_token: 'grok-id',
        expired: '2099-01-01T00:00:00Z'
      })
    )

    await dropzone.trigger('drop', {
      dataTransfer: {
        files: [sub2apiFile, cpaFile]
      }
    })
    await flushPromises()

    const output = wrapper.get('[data-testid="account-import-output"]')
    expect((output.element as HTMLTextAreaElement).value).toContain('sub-account@example.com')
    expect((output.element as HTMLTextAreaElement).value).toContain('grok@example.com')

    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const createObjectURLSpy = vi.mocked(URL.createObjectURL)

    await wrapper.get('[data-testid="account-import-download"]').trigger('click')
    await flushPromises()
    expect(createObjectURLSpy).toHaveBeenCalled()
    expect(clickSpy).toHaveBeenCalled()

    await wrapper.get('[data-testid="account-import-submit"]').trigger('click')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        type: 'sub2api-data',
        version: 1,
        proxies: [],
        accounts: expect.arrayContaining([
          expect.objectContaining({
            name: 'sub-account@example.com',
            platform: 'openai'
          }),
          expect.objectContaining({
            name: 'grok@example.com',
            platform: 'grok'
          })
        ])
      }),
      skip_default_group_bind: true
    })

    clickSpy.mockRestore()
    createObjectURLSpy.mockRestore()
  })
})
