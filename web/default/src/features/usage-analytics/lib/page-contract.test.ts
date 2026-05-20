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
import { test } from 'node:test'

import {
  buildDefaultUsageAnalyticsFilters,
  buildUsageAnalyticsQueryKeys,
  buildUsageAnalyticsRankingDrilldown,
} from './page-contract'

test('builds default usage analytics filters for reset', () => {
  const filters = buildDefaultUsageAnalyticsFilters(1779321600)

  assert.equal(filters.start_timestamp, 1779321600 - 7 * 24 * 60 * 60)
  assert.equal(filters.end_timestamp, 1779321600)
  assert.equal(filters.group_by, 'token')
  assert.equal(filters.metric, 'total_tokens')
  assert.equal(filters.limit, 10)
})

test('builds stable query keys for three analytics endpoints', () => {
  const filters = buildDefaultUsageAnalyticsFilters(1779321600)
  const keys = buildUsageAnalyticsQueryKeys(filters)

  assert.deepEqual(keys.summary, ['usage-analytics', 'summary', filters])
  assert.deepEqual(keys.timeseries, ['usage-analytics', 'timeseries', filters])
  assert.deepEqual(keys.breakdown, ['usage-analytics', 'breakdown', filters])
})

test('builds ranking drilldown target for usage logs common route', () => {
  const filters = buildDefaultUsageAnalyticsFilters(1779321600)
  const target = buildUsageAnalyticsRankingDrilldown(filters, {
    token_id: 5,
    status: 'success',
  })

  assert.ok(target)
  assert.equal(target.to, '/usage-logs/$section')
  assert.deepEqual(target.params, { section: 'common' })
  assert.equal(target.search.tokenId, 5)
  assert.equal(target.search.status, 'success')
  assert.equal(Object.prototype.hasOwnProperty.call(target.search, 'type'), false)
})

test('returns disabled drilldown target for Other rows', () => {
  const filters = buildDefaultUsageAnalyticsFilters(1779321600)
  const target = buildUsageAnalyticsRankingDrilldown(filters, null)

  assert.equal(target, null)
})
