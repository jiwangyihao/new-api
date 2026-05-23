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
  buildBreakdownChartData,
  buildRankingRows,
  buildTrendChartData,
  isAdditiveMetric,
  mergeTopNWithOther,
} from './chart-data'
import type { UsageAnalyticsGroup, UsageAnalyticsTimeseriesPoint } from '../types'

function makeTimeseriesPoint(
  overrides: Partial<UsageAnalyticsTimeseriesPoint>
): UsageAnalyticsTimeseriesPoint {
  return {
    group_by: 'token',
    group_key: 'token:1',
    group_value: '1',
    group_label: 'API Key 1',
    drilldown: { token_id: 1 },
    request_count: 0,
    success_count: 0,
    error_count: 0,
    success_rate: 0,
    error_rate: 0,
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    metered_tokens: 0,
    total_tokens: 0,
    avg_latency_ms: 0,
    p95_latency_ms: 0,
    timestamp: 0,
    time_label: '',
    ...overrides,
  }
}

function makeGroup(overrides: Partial<UsageAnalyticsGroup>): UsageAnalyticsGroup {
  return {
    group_by: 'model',
    group_key: 'model:gpt-4o-mini',
    group_value: 'gpt-4o-mini',
    group_label: 'gpt-4o-mini',
    drilldown: { model_name: 'gpt-4o-mini' },
    request_count: 0,
    success_count: 0,
    error_count: 0,
    success_rate: 0,
    error_rate: 0,
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    metered_tokens: 0,
    total_tokens: 0,
    avg_latency_ms: 0,
    p95_latency_ms: 0,
    first_used_at: 0,
    last_used_at: 0,
    share: null,
    token: null,
    ...overrides,
  }
}

test('keeps series separated by group_key even when labels match', () => {
  const points = [
    makeTimeseriesPoint({
      timestamp: 10,
      time_label: '00:00',
      group_key: 'token:1',
      group_value: '1',
      group_label: 'Same',
      total_tokens: 10,
    }),
    makeTimeseriesPoint({
      timestamp: 10,
      time_label: '00:00',
      group_key: 'token:2',
      group_value: '2',
      group_label: 'Same',
      drilldown: { token_id: 2 },
      total_tokens: 20,
    }),
  ]

  const data = buildTrendChartData(points, 'total_tokens')

  assert.equal(new Set(data.map((item) => item.group_key)).size, 2)
  assert.deepEqual(
    data.map((item) => item.value),
    [10, 20]
  )
})

test('merges extra additive groups into non-drillable Other', () => {
  const groups = [
    makeGroup({
      group_key: 'a',
      group_value: 'a',
      group_label: 'A',
      total_tokens: 30,
      request_count: 3,
      quota: 3,
      drilldown: { model_name: 'a' },
    }),
    makeGroup({
      group_key: 'b',
      group_value: 'b',
      group_label: 'B',
      total_tokens: 20,
      request_count: 2,
      quota: 2,
      drilldown: { model_name: 'b' },
    }),
    makeGroup({
      group_key: 'c',
      group_value: 'c',
      group_label: 'C',
      total_tokens: 10,
      request_count: 1,
      quota: 1,
      drilldown: { model_name: 'c' },
    }),
  ]

  const merged = mergeTopNWithOther(groups, 'total_tokens', 2)
  const other = merged[2]

  assert.equal(merged.length, 3)
  assert.ok(other)
  assert.equal(other.group_key, 'other')
  assert.equal(other.total_tokens, 10)
  assert.equal(other.drilldown, null)
})

test('does not stack rate or latency metrics', () => {
  assert.equal(isAdditiveMetric('total_tokens'), true)
  assert.equal(isAdditiveMetric('error_rate'), false)
  assert.equal(isAdditiveMetric('avg_latency_ms'), false)
})

test('builds ranking rows with deleted token safe fields', () => {
  const rows = buildRankingRows([
    makeGroup({
      group_by: 'token',
      group_key: 'token:7',
      group_value: '7',
      group_label: '',
      token: {
        id: 7,
        name: 'deleted-history',
        masked_key: null,
        status: null,
        remain_quota: null,
        unlimited_quota: null,
        deleted: true,
      },
      drilldown: { token_id: 7 },
    }),
  ])

  assert.equal(rows[0]?.display_label, 'deleted-history')
  assert.equal(rows[0]?.masked_key, null)
})

test('builds empty state for non-additive breakdown metric', () => {
  const data = buildBreakdownChartData([], 'error_rate')
  assert.equal(data.kind, 'unsupported-share')
})
