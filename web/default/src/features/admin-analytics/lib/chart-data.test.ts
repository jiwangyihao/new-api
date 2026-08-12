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
import test from 'node:test'
import type { AdminUsageGroup } from '../types'
import { orderQuotaBuckets, topNWithOther } from './chart-data'

test('quota buckets use fixed order', () => {
  const ordered = orderQuotaBuckets([
    {
      bucket: '90_100',
      subscription_count: 1,
      user_count: 1,
      token_limit: 1,
      token_used: 1,
      usage_rate: 1,
    },
    {
      bucket: '0_25',
      subscription_count: 1,
      user_count: 1,
      token_limit: 1,
      token_used: 0,
      usage_rate: 0,
    },
  ])
  assert.deepEqual(
    ordered.map((bucket) => bucket.bucket),
    ['0_25', '90_100']
  )
})

function group(
  key: string,
  requests: number,
  errors: number,
  tokens: number
): AdminUsageGroup {
  return {
    group_by: 'user',
    group_key: key,
    group_value: key,
    group_label: key,
    share: null,
    request_count: requests,
    success_count: requests - errors,
    error_count: errors,
    success_rate: 0,
    error_rate: 0,
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    metered_tokens: tokens,
    total_tokens: tokens,
    avg_latency_ms: 0,
    p95_latency_ms: 0,
    rpm: 0,
    tpm: 0,
    active_users: 0,
    active_api_keys: 0,
    first_used_at: 0,
    last_used_at: 0,
  }
}

test('topNWithOther creates non-drilldown other and recomputes error rate', () => {
  const result = topNWithOther(
    [group('a', 10, 0, 10), group('b', 10, 5, 20), group('c', 5, 5, 30)],
    1
  )
  assert.equal(result.groups.length, 1)
  assert.equal(result.other?.group_key, '__other__')
  assert.equal(result.other?.total_tokens, 50)
  assert.equal(result.other?.error_rate, 10 / 15)
  assert.equal(result.other?.drilldown, undefined)
})
