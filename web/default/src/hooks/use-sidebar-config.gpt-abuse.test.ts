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
