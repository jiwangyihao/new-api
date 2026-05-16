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
import { describe, test } from 'node:test'
import {
  filterSidebarNavGroupsForConfig,
  getDefaultSidebarModulesForTest,
} from './use-sidebar-config'
import type { NavGroup } from '@/components/layout/types'

const personalGroups: NavGroup[] = [
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

describe('removed application guides sidebar module', () => {
  test('does not add app_guides to default module config', () => {
    const defaults = getDefaultSidebarModulesForTest()

    assert.equal(defaults.personal?.app_guides, undefined)
  })

  test('does not keep the obsolete route visible through legacy config', () => {
    const defaults = getDefaultSidebarModulesForTest()
    const legacyUserConfig = {
      personal: {
        enabled: true,
        topup: true,
        personal: true,
        app_guides: true,
      },
    }

    const filtered = filterSidebarNavGroupsForConfig(
      personalGroups,
      defaults,
      legacyUserConfig
    )

    assert.deepEqual(
      filtered[0]?.items.map((item) => ('url' in item ? item.url : '')),
      ['/wallet', '/profile']
    )
  })
})
