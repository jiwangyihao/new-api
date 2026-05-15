import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  filterNavGroupsByRole,
  filterSidebarNavGroupsForConfig,
  getDefaultSidebarModulesForTest,
} from './use-sidebar-config'
import type { NavGroup } from '@/components/layout/types'

const navGroups: NavGroup[] = [
  {
    id: 'admin',
    title: 'Admin',
    items: [
      {
        title: 'Trial Codes',
        url: '/trial-codes',
      },
    ],
  },
]

describe('sidebar config defaults', () => {
  test('keeps new admin modules visible when saved user config omits them', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const userConfig = {
      admin: {
        enabled: true,
        channel: true,
        models: true,
        redemption: true,
        user: true,
        setting: true,
      },
    }

    const filtered = filterSidebarNavGroupsForConfig(
      navGroups,
      defaults,
      userConfig
    )

    assert.equal(filtered[0]?.items[0]?.url, '/trial-codes')
  })

  test('lets user config explicitly hide trial-code management', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const userConfig = {
      admin: {
        enabled: true,
        trial_code: false,
      },
    }

    const filtered = filterSidebarNavGroupsForConfig(
      navGroups,
      defaults,
      userConfig
    )

    assert.equal(filtered.length, 0)
  })

  test('hides admin navigation from non-admin command consumers', () => {
    const filtered = filterNavGroupsByRole(navGroups, 1)

    assert.equal(filtered.length, 0)
  })
})
