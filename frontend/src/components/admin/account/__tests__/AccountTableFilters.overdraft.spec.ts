import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const SelectStub = {
  props: ['options'],
  template: `
    <div
      data-test="select-options"
      :data-values="options.map((option) => option.value).join(',')"
      :data-labels="options.map((option) => option.label).join(',')"
    ></div>
  `
}

describe('AccountTableFilters 透支状态筛选', () => {
  it('在状态筛选中提供透支中选项', () => {
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters: {
          platform: '',
          type: '',
          status: '',
          privacy_mode: '',
          group: ''
        },
        groups: []
      },
      global: {
        stubs: {
          Select: SelectStub,
          SearchInput: {
            template: '<div data-test="search-input"></div>'
          }
        }
      }
    })

    const statusSelect = wrapper
      .findAll('[data-test="select-options"]')
      .find((node) => node.attributes('data-values')?.includes('rate_limited'))

    expect(statusSelect?.attributes('data-values')).toContain('overdrafting')
    expect(statusSelect?.attributes('data-labels')).toContain('admin.accounts.status.overdrafting')
  })
})
