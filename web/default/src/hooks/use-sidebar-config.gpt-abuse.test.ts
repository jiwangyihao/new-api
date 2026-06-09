import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { NavGroup } from '@/components/layout/types'
import {
  filterSidebarNavGroupsForConfig,
  getDefaultSidebarModulesForTest,
} from './use-sidebar-config'

describe('gpt abuse sidebar config', () => {
  test('maps /gpt-abuse to admin gpt_abuse module', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'GPT Abuse',
            url: '/gpt-abuse',
          },
        ],
      },
    ]

    const visible = filterSidebarNavGroupsForConfig(groups, defaults, null)
    const hidden = filterSidebarNavGroupsForConfig(
      groups,
      {
        ...defaults,
        admin: {
          ...(defaults.admin ?? { enabled: true }),
          gpt_abuse: false,
        },
      },
      null
    )

    assert.equal(defaults.admin?.gpt_abuse, true)
    assert.equal(visible[0]?.items[0]?.url, '/gpt-abuse')
    assert.equal(hidden.length, 0)
  })
})
