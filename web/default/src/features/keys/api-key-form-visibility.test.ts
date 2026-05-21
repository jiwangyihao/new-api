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

function readKeysSource(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

const drawerSource = readKeysSource('./components/api-keys-mutate-drawer.tsx')
const formSource = readKeysSource('./lib/api-key-form.ts')
const columnsSource = readKeysSource('./components/api-keys-columns.tsx')

describe('api key user-facing configuration form', () => {
  test('does not expose group selection in create or edit drawer', () => {
    assert.doesNotMatch(drawerSource, /ApiKeyGroupCombobox/)
    assert.doesNotMatch(drawerSource, /name=['"]group['"]|name=\{['"]group['"]\}/)
    assert.doesNotMatch(drawerSource, /t\(['"]Group['"]\)/)
    assert.doesNotMatch(drawerSource, /Select a group/)
    assert.doesNotMatch(drawerSource, /Cross-group retry/)
  })

  test('does not expose group details in API key table columns', () => {
    assert.doesNotMatch(columnsSource, /accessorKey:\s*['"]group['"]|id:\s*['"]group['"]|label:\s*t\(['"]Group['"]\)/)
    assert.doesNotMatch(columnsSource, /GroupBadge/)
    assert.doesNotMatch(columnsSource, /Cross-group/)
    assert.doesNotMatch(columnsSource, /getUserGroups/)
  })

  test('does not fetch user groups only to render API key form fields', () => {
    assert.doesNotMatch(drawerSource, /getUserGroups/)
    assert.doesNotMatch(drawerSource, /user-groups/)
  })

  test('does not depend on user-edited group fields in API key payloads', () => {
    assert.doesNotMatch(formSource, /data\.group/)
    assert.doesNotMatch(formSource, /data\.cross_group_retry/)
    assert.doesNotMatch(formSource, /group:\s*/)
    assert.doesNotMatch(formSource, /cross_group_retry:\s*/)
    assert.doesNotMatch(formSource, /apiKey\.group/)
    assert.doesNotMatch(formSource, /apiKey\.cross_group_retry/)
  })
})
