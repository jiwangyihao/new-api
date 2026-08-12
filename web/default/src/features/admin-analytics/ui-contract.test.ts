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
import test from 'node:test'

function readSource(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

const indexSource = readSource('./index.tsx')
const statisticsSource = readSource(
  '../system-settings/billing/statistics-section.tsx'
)

test('paid analytics exposes visible row controls instead of relying on hidden limit params', () => {
  assert.match(indexSource, /PaidAnalyticsRowControls/)
  assert.match(indexSource, /adminAnalytics\.pagination\.allRows/)
  assert.match(indexSource, /enableAdminAnalyticsAllRows/)
  assert.match(indexSource, /enableAdminAnalyticsPagedRows/)
})

test('paid analytics exposes visible sort controls', () => {
  assert.match(indexSource, /PaidAnalyticsSortControls/)
  assert.match(indexSource, /adminAnalytics\.sort\.field/)
  assert.match(indexSource, /recognized_remaining_value/)
  assert.match(indexSource, /sort_order/)
})

test('paid analytics links directly to subscription statistics exclusions', () => {
  assert.match(indexSource, /to='\/system-settings\/billing\/\$section'/)
  assert.match(indexSource, /params=\{\{ section: 'statistics' \}\}/)
  assert.match(indexSource, /adminAnalytics\.filters\.manageExcludedUsers/)
})

test('subscription statistics section has a user search selector for exclusions', () => {
  assert.match(statisticsSource, /searchUsers/)
  assert.match(statisticsSource, /ExcludedUserSearchSelect/)
  assert.match(
    statisticsSource,
    /systemSettings\.billing\.statistics\.searchUsers/
  )
})
