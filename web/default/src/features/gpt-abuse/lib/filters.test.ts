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
  buildGPTAbuseApiParams,
  buildGPTAbuseCanonicalSearch,
  updateGPTAbuseSearchForFilterChange,
  updateGPTAbuseSearchForPagination,
  updateGPTAbuseSearchForSorting,
} from './filters'

describe('gpt abuse filters', () => {
  test('round trips search values into API params', () => {
    const search = buildGPTAbuseCanonicalSearch(
      {
        start_timestamp: '1800000000',
        end_timestamp: '1800086400',
        keyword: ' abuse@example.com ',
        user_id: '42',
        status: 'active_suspended',
        kind: 'cyber_policy',
        severity: 'high',
        source: 'sse_response_failed',
        limit: '50',
        offset: '40',
        sort_by: 'effective_warning_count',
        sort_order: 'asc',
      },
      1_800_001_234
    )

    assert.deepEqual(search, {
      start_timestamp: 1_800_000_000,
      end_timestamp: 1_800_086_400,
      keyword: 'abuse@example.com',
      user_id: 42,
      status: 'active_suspended',
      kind: 'cyber_policy',
      severity: 'high',
      source: 'sse_response_failed',
      limit: 50,
      offset: 40,
      sort_by: 'effective_warning_count',
      sort_order: 'asc',
    })

    assert.equal(
      buildGPTAbuseApiParams(search).toString(),
      'start_timestamp=1800000000&end_timestamp=1800086400&keyword=abuse%40example.com&user_id=42&status=active_suspended&kind=cyber_policy&severity=high&source=sse_response_failed&limit=50&offset=40&sort_by=effective_warning_count&sort_order=asc'
    )
  })

  test('resets offset when filters change', () => {
    const current = buildGPTAbuseCanonicalSearch(
      { offset: 40, kind: 'cyber_policy' },
      1_800_001_234
    )

    const next = updateGPTAbuseSearchForFilterChange(current, {
      status: 'warning_only',
      severity: 'medium',
      source: 'http_error',
      keyword: 'user',
    })

    assert.equal(next.offset, 0)
    assert.equal(next.kind, 'cyber_policy')
    assert.equal(next.status, 'warning_only')
    assert.equal(next.severity, 'medium')
    assert.equal(next.source, 'http_error')
    assert.equal(next.keyword, 'user')
  })

  test('preserves filters when pagination or sorting changes', () => {
    const current = buildGPTAbuseCanonicalSearch(
      {
        keyword: 'user',
        status: 'warning_only',
        kind: 'cyber_policy',
        severity: 'high',
        source: 'http_error',
        offset: 40,
      },
      1_800_001_234
    )

    const paged = updateGPTAbuseSearchForPagination(current, {
      limit: 50,
      offset: 100,
    })
    const sorted = updateGPTAbuseSearchForSorting(paged, {
      sort_by: 'user_id',
      sort_order: 'asc',
    })

    assert.equal(sorted.keyword, 'user')
    assert.equal(sorted.status, 'warning_only')
    assert.equal(sorted.kind, 'cyber_policy')
    assert.equal(sorted.severity, 'high')
    assert.equal(sorted.source, 'http_error')
    assert.equal(sorted.limit, 50)
    assert.equal(sorted.offset, 100)
    assert.equal(sorted.sort_by, 'user_id')
    assert.equal(sorted.sort_order, 'asc')
  })
})
