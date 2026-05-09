import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import GroupSelector from '../GroupSelector.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      if (key === 'common.selectedCount') return `selected:${params?.count ?? 0}`
      if (key === 'admin.groups.rateAndAccounts') return 'rate-and-accounts'
      return key
    }
  })
}))

describe('GroupSelector', () => {
  it('nvidia 账号只显示 openai 分组', () => {
    const wrapper = mount(GroupSelector, {
      props: {
        modelValue: [],
        platform: 'nvidia',
        searchable: false,
        groups: [
          { id: 1, name: 'openai-default', platform: 'openai', rate_multiplier: 1, subscription_type: 'standard', account_count: 2 },
          { id: 2, name: 'gemini-default', platform: 'gemini', rate_multiplier: 1, subscription_type: 'standard', account_count: 3 },
          { id: 3, name: 'anthropic-default', platform: 'anthropic', rate_multiplier: 1, subscription_type: 'standard', account_count: 4 }
        ]
      } as any,
      global: {
        stubs: {
          GroupBadge: {
            props: ['name'],
            template: '<span class="group-badge">{{ name }}</span>'
          },
          Icon: true
        }
      }
    })

    const labels = wrapper.findAll('.group-badge').map((node) => node.text())
    expect(labels).toEqual(['openai-default'])
    expect(wrapper.text()).not.toContain('gemini-default')
    expect(wrapper.text()).not.toContain('anthropic-default')
  })
})
