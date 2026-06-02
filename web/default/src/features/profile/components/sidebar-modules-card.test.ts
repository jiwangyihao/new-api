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
  buildDefaultSidebarModulesForSectionsForTest,
  clampSidebarModulesForPermissionsForTest,
} from './sidebar-modules-card'

const adminSections = [
  {
    key: 'admin',
    title: 'Admin',
    description: 'Admin modules',
    modules: [
      { key: 'trial_abuse', title: 'Trial Abuse', description: '' },
      { key: 'setting', title: 'System Settings', description: '' },
    ],
  },
]

describe('sidebar modules card defaults', () => {
  test('keeps modules unavailable to the current user hidden after reset', () => {
    const defaults = buildDefaultSidebarModulesForSectionsForTest(
      adminSections,
      { admin: { setting: false } }
    )

    assert.equal(defaults.admin?.trial_abuse, true)
    assert.equal(defaults.admin?.setting, false)
  })

  test('keeps saved modules unavailable to the current user disabled', () => {
    const defaults = buildDefaultSidebarModulesForSectionsForTest(
      adminSections,
      { admin: false },
      {
        admin: {
          enabled: true,
          trial_abuse: true,
          setting: true,
        },
      }
    )

    assert.equal(defaults.admin?.enabled, false)
    assert.equal(defaults.admin?.trial_abuse, false)
    assert.equal(defaults.admin?.setting, false)
  })

  test('clamps mutable sidebar config before saving', () => {
    const sanitized = clampSidebarModulesForPermissionsForTest(
      {
        admin: {
          enabled: true,
          trial_abuse: true,
          setting: true,
        },
      },
      { admin: { setting: false } }
    )

    assert.equal(sanitized.admin?.enabled, true)
    assert.equal(sanitized.admin?.trial_abuse, true)
    assert.equal(sanitized.admin?.setting, false)
  })

  test('clamps section enabled when permission disables the section', () => {
    const sanitized = clampSidebarModulesForPermissionsForTest(
      {
        admin: {
          enabled: true,
          trial_abuse: true,
          setting: true,
        },
      },
      { admin: { enabled: false } }
    )

    assert.equal(sanitized.admin?.enabled, false)
    assert.equal(sanitized.admin?.trial_abuse, false)
    assert.equal(sanitized.admin?.setting, false)
  })
})
