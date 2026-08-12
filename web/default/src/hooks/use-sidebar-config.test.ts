/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import { ROLE } from '@/lib/roles'
import type { NavGroup } from '@/components/layout/types'
import {
  filterNavGroupsByRole,
  filterSidebarNavGroupsForConfig,
  getDefaultSidebarModulesForTest,
  resolvePendingCommissionWithdrawalsBadgeForTest,
  shouldFetchAdminTasksSummaryForTest,
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

  test('keeps admin ops visible in the default admin sidebar', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'Admin Ops',
            url: '/admin-ops',
          },
        ],
      },
    ]

    const filtered = filterSidebarNavGroupsForConfig(groups, defaults, null)

    assert.equal(filtered[0]?.items[0]?.url, '/admin-ops')
  })

  test('applies sidebar permissions as a hard limit over saved user config', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          { title: 'System Settings', url: '/system-settings' },
          { title: 'Trial Abuse', url: '/trial-abuse' },
        ],
      },
    ]
    const userConfig = {
      admin: {
        enabled: true,
        setting: true,
        trial_abuse: true,
      },
    }
    const permissions = {
      admin: {
        setting: false,
      },
    }

    const filtered = filterSidebarNavGroupsForConfig(
      groups,
      defaults,
      userConfig,
      permissions
    )

    assert.deepEqual(
      filtered[0]?.items.map((item) => ('url' in item ? item.url : '')),
      ['/trial-abuse']
    )
  })

  test('treats section-level sidebar permission false as a hard limit', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [{ title: 'System Settings', url: '/system-settings' }],
      },
    ]
    const userConfig = {
      admin: {
        enabled: true,
        setting: true,
      },
    }

    const filtered = filterSidebarNavGroupsForConfig(
      groups,
      defaults,
      userConfig,
      {
        admin: false,
      }
    )

    assert.equal(filtered.length, 0)
  })

  test('lets user config explicitly hide admin ops', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'Admin Ops',
            url: '/admin-ops',
          },
        ],
      },
    ]
    const userConfig = {
      admin: {
        enabled: true,
        ops: false,
      },
    }

    const filtered = filterSidebarNavGroupsForConfig(
      groups,
      defaults,
      userConfig
    )

    assert.equal(filtered.length, 0)
  })

  test('keeps trial abuse visible in the default admin sidebar', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'Trial Abuse',
            url: '/trial-abuse',
          },
        ],
      },
    ]

    const filtered = filterSidebarNavGroupsForConfig(groups, defaults, null)

    assert.equal(defaults.admin?.trial_abuse, true)
    assert.equal(filtered[0]?.items[0]?.url, '/trial-abuse')
  })

  test('hides trial abuse when admin sidebar config disables the module', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'Trial Abuse',
            url: '/trial-abuse',
          },
        ],
      },
    ]
    const adminConfig = {
      ...defaults,
      admin: {
        ...(defaults.admin ?? { enabled: true }),
        trial_abuse: false,
      },
    }

    const filtered = filterSidebarNavGroupsForConfig(groups, adminConfig, null)

    assert.equal(filtered.length, 0)
  })

  test('lets user config explicitly hide trial abuse', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'Trial Abuse',
            url: '/trial-abuse',
          },
        ],
      },
    ]
    const userConfig = {
      admin: {
        enabled: true,
        trial_abuse: false,
      },
    }

    const filtered = filterSidebarNavGroupsForConfig(
      groups,
      defaults,
      userConfig
    )

    assert.equal(filtered.length, 0)
  })

  test('maps invitation commission withdrawals to admin invitation_commission module', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const groups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'Manual cashback requests',
            url: '/invitation-commission/withdrawals',
          },
        ],
      },
    ]

    const filtered = filterSidebarNavGroupsForConfig(groups, defaults, null)

    assert.equal(defaults.admin?.invitation_commission, true)
    assert.equal(
      filtered[0]?.items[0]?.url,
      '/invitation-commission/withdrawals'
    )
    assert.equal(
      filterSidebarNavGroupsForConfig(
        groups,
        {
          ...defaults,
          admin: { enabled: true, invitation_commission: false },
        },
        null
      ).length,
      0
    )
  })

  test('admin task summary query is enabled only for visible admin commission entry', () => {
    const adminGroups: NavGroup[] = [
      {
        id: 'admin',
        title: 'Admin',
        items: [
          {
            title: 'Manual cashback requests',
            url: '/invitation-commission/withdrawals',
          },
        ],
      },
    ]

    assert.equal(
      shouldFetchAdminTasksSummaryForTest(undefined, adminGroups),
      false
    )
    assert.equal(
      shouldFetchAdminTasksSummaryForTest(ROLE.USER, adminGroups),
      false
    )
    assert.equal(
      shouldFetchAdminTasksSummaryForTest(
        ROLE.ADMIN,
        filterSidebarNavGroupsForConfig(
          adminGroups,
          {
            admin: { enabled: true, invitation_commission: false },
          },
          null
        )
      ),
      false
    )
    assert.equal(
      shouldFetchAdminTasksSummaryForTest(ROLE.ADMIN, adminGroups),
      true
    )
    assert.equal(
      shouldFetchAdminTasksSummaryForTest(ROLE.SUPER_ADMIN, adminGroups),
      true
    )
  })

  test('admin task summary failure hides badge without toast', async () => {
    let toastCalled = false
    const result = await resolvePendingCommissionWithdrawalsBadgeForTest(
      async () => {
        throw new Error('network')
      },
      () => {
        toastCalled = true
      }
    )

    assert.equal(result, undefined)
    assert.equal(toastCalled, false)
  })

  test('sidebar registers manual cashback route and quiet task summary API', () => {
    const sidebarData = readFileSync('src/hooks/use-sidebar-data.ts', 'utf8')
    const appSidebar = readFileSync(
      'src/components/layout/components/app-sidebar.tsx',
      'utf8'
    )
    const adminApi = readFileSync(
      'src/features/invitation-commission/api.ts',
      'utf8'
    )

    assert.match(sidebarData, /Manual cashback requests/)
    assert.match(appSidebar, /pendingCommissionWithdrawals > 0/)
    assert.match(appSidebar, /shouldFetchAdminTasksSummary/)
    assert.match(adminApi, /skipErrorHandler/)
    assert.match(adminApi, /skipBusinessError/)
  })
})
