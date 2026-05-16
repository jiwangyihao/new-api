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

const homeSource = readFileSync(
  new URL('./components/sections/how-it-works.tsx', import.meta.url),
  'utf8'
)
const dashboardSource = readFileSync(
  new URL(
    '../dashboard/components/overview/overview-dashboard.tsx',
    import.meta.url
  ),
  'utf8'
)

const expectedOrder = ['Create API', 'Try Playground', 'Choose a plan']

function stepIndexes(source: string): number[] {
  return expectedOrder.map((text) => source.indexOf(`t('${text}')`))
}

function assertOrderedQuickStart(source: string): void {
  const indexes = stepIndexes(source)
  assert.ok(indexes.every((index) => index >= 0), 'missing quick start step')
  assert.deepEqual(
    indexes,
    [...indexes].sort((a, b) => a - b),
    'quick start steps should be ordered create API -> playground -> plan'
  )
  assert.match(source, /OpenCode-ready API help/)
}

describe('Issue #3 quick start copy', () => {
  test('keeps the home quick start steps in the requested order', () => {
    assertOrderedQuickStart(homeSource)
  })

  test('keeps dashboard quick start aligned with the home page', () => {
    assertOrderedQuickStart(dashboardSource)
    assert.match(dashboardSource, /to: '\/keys'/)
    assert.match(dashboardSource, /to: '\/playground'/)
    assert.match(dashboardSource, /to: '\/pricing'/)
    assert.match(dashboardSource, /getSelfSubscriptionFull/)
    assert.doesNotMatch(
      dashboardSource,
      /completed: remainQuota > 0 \|\| usedQuota > 0/
    )
    assert.match(dashboardSource, /completed: hasActiveSubscription/)
    assert.match(
      dashboardSource,
      /queryKey: \['dashboard', 'overview', 'self-subscriptions', user\?\.id\]/
    )
    assert.match(dashboardSource, /enabled: Boolean\(user\?\.id\)/)
  })
})
