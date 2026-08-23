import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import StaticEmbedView from '../StaticEmbedView.vue'

describe('StaticEmbedView', () => {
  it('renders the configured iframe source', () => {
    const wrapper = mount(StaticEmbedView, {
      props: {
        src: '/tu.html',
        title: '图片页面',
      },
    })

    expect(wrapper.get('iframe').attributes('src')).toBe('/tu.html')
    expect(wrapper.get('iframe').attributes('title')).toBe('图片页面')
  })
})
