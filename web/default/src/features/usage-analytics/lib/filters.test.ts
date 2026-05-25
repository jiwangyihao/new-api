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
  buildApiKeyUsageAnalyticsSearch,
  buildUsageAnalyticsApiParams,
  buildUsageAnalyticsCanonicalFilters,
  buildUsageLogsDrilldownSearch,
  normalizeUsageAnalyticsSearch,
} from './filters'

test('normalizes empty search to recent seven days token analytics', () => {
  const now = 1_779_321_600
  const normalized = normalizeUsageAnalyticsSearch({}, now)

  assert.equal(normalized.group_by, 'token')
  assert.equal(normalized.metric, 'total_tokens')
  assert.equal(normalized.granularity, 'day')
  assert.equal(normalized.limit, 10)
  assert.equal(normalized.sort_by, 'total_tokens')
  assert.equal(normalized.sort_order, 'desc')
  assert.equal(normalized.end_timestamp, now)
  assert.equal(normalized.start_timestamp, now - 7 * 24 * 60 * 60)
})

test('builds stable canonical filters from normalized search', () => {
  const canonical = buildUsageAnalyticsCanonicalFilters(
    {
      token_ids: ['2', '1', '2'],
      model_names: 'gpt-4,claude',
      streams: 'true,false',
      statuses: 'success,error',
    },
    1_779_321_600
  )

  assert.deepEqual(canonical.token_ids, [1, 2])
  assert.deepEqual(canonical.model_names, ['claude', 'gpt-4'])
  assert.deepEqual(canonical.streams, ['false', 'true'])
  assert.deepEqual(canonical.statuses, ['error', 'success'])
})

test('falls back for invalid filters without runtime exceptions', () => {
  const canonical = buildUsageAnalyticsCanonicalFilters(
    {
      group_by: 'endpoint',
      metric: 'success_rate',
      granularity: 'minute',
      limit: -1,
      sort_by: 'bad',
      sort_order: 'sideways',
      token_ids: '2,-1,0,nope,1',
      streams: 'true,maybe,false',
      statuses: ['success', 'failed', 'error'],
    },
    1_779_321_600
  )

  assert.equal(canonical.group_by, 'token')
  assert.equal(canonical.metric, 'total_tokens')
  assert.equal(canonical.granularity, 'day')
  assert.equal(canonical.limit, 10)
  assert.equal(canonical.sort_by, 'total_tokens')
  assert.equal(canonical.sort_order, 'desc')
  assert.deepEqual(canonical.token_ids, [1, 2])
  assert.deepEqual(canonical.streams, ['false', 'true'])
  assert.deepEqual(canonical.statuses, ['error', 'success'])
})

test('clamps excessive limit to backend maximum', () => {
  const canonical = buildUsageAnalyticsCanonicalFilters(
    { metric: 'quota', limit: '500' },
    1_779_321_600
  )

  assert.equal(canonical.limit, 50)
  assert.equal(canonical.sort_by, 'quota')
})

test('serializes api params without deprecated dimension params', () => {
  const canonical = buildUsageAnalyticsCanonicalFilters(
    { model_names: ['gpt-4', 'claude'], groups: ['a,b', 'default'] },
    1_779_321_600
  )
  const params = buildUsageAnalyticsApiParams(canonical)

  assert.deepEqual(params.getAll('model_names'), ['claude', 'gpt-4'])
  assert.equal(params.has('groups'), false)
  assert.equal('groups' in canonical, false)
})

test('builds api key entry search without full key material', () => {
  const search = buildApiKeyUsageAnalyticsSearch({
    id: 42,
    name: 'prod',
    key: 'sk-secret',
  })

  assert.deepEqual(search, { group_by: 'token', token_ids: [42] })
  assert.equal(JSON.stringify(search).includes('sk-secret'), false)
})

test('builds usage logs drilldown search with status not numeric type', () => {
  const search = buildUsageLogsDrilldownSearch(
    { start_timestamp: 10, end_timestamp: 20 },
    {
      token_id: 5,
      model_name: 'gpt-4',
      status: 'error',
      is_stream: true,
    }
  )

  assert.deepEqual(search, {
    startTime: 10_000,
    endTime: 20_000,
    tokenId: 5,
    model: 'gpt-4',
    status: 'error',
    isStream: true,
  })
  assert.equal(Object.prototype.hasOwnProperty.call(search, 'type'), false)
  assert.equal(Object.prototype.hasOwnProperty.call(search, 'group'), false)
})

test('omits invalid token id from usage logs drilldown search', () => {
  const search = buildUsageLogsDrilldownSearch(
    { start_timestamp: 10, end_timestamp: 20 },
    { token_id: 0 }
  )

  assert.deepEqual(search, {
    startTime: 10_000,
    endTime: 20_000,
  })
})
