import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { NavGroup } from '@/components/layout/types'
import {
  filterNavGroupsByRole,
  filterSidebarNavGroupsForConfig,
  getDefaultSidebarModulesForTest,
} from './use-sidebar-config'

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

  test('keeps admin analytics visible by default and mapped to admin module', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'Operations Analytics',
            url: '/admin-analytics',
          },
        ],
      },
    ]

    const filtered = filterSidebarNavGroupsForConfig(groups, defaults, null)

    assert.equal(filtered[0]?.items[0]?.url, '/admin-analytics')
  })

  test('lets user config explicitly hide admin analytics', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'Operations Analytics',
            url: '/admin-analytics',
          },
        ],
      },
    ]
    const userConfig = {
      admin: {
        enabled: true,
        analytics: false,
      },
    }

    const filtered = filterSidebarNavGroupsForConfig(
      groups,
      defaults,
      userConfig
    )

    assert.equal(filtered.length, 0)
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

  test('drops obsolete application guides entries from legacy user config', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const userConfig = {
      personal: {
        enabled: true,
        topup: true,
        personal: true,
        app_guides: true,
      },
    }
    const groups: NavGroup[] = [
      {
        id: 'personal',
        title: 'Personal',
        items: [
          { title: 'Wallet', url: '/wallet' },
          { title: 'Application Guides', url: '/app-guides' },
          { title: 'Profile', url: '/profile' },
        ],
      },
    ]

    const filtered = filterSidebarNavGroupsForConfig(
      groups,
      defaults,
      userConfig
    )

    assert.equal(defaults.personal?.app_guides, undefined)
    assert.deepEqual(
      filtered[0]?.items.map((item) => ('url' in item ? item.url : '')),
      ['/wallet', '/profile']
    )
  })

  test('hides admin navigation from non-admin command consumers', () => {
    const filtered = filterNavGroupsByRole(navGroups, 1)

    assert.equal(filtered.length, 0)
  })
})
