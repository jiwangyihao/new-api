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
import type {
  UsageAnalyticsDrilldown,
  UsageAnalyticsGroup,
  UsageAnalyticsMetric,
  UsageAnalyticsTimeseriesPoint,
} from '../types'

export interface UsageAnalyticsTrendChartDatum {
  timestamp: number
  time_label: string
  group_key: string
  group_value: string
  group_label: string
  metric: UsageAnalyticsMetric
  value: number
  stacked: boolean
  drilldown: UsageAnalyticsDrilldown | null
}

export interface UsageAnalyticsRankingRow extends UsageAnalyticsGroup {
  id: string
  display_label: string
  masked_key: string | null
  token_deleted: boolean
}

export interface UsageAnalyticsBreakdownDatum {
  group_key: string
  group_value: string
  group_label: string
  metric: UsageAnalyticsMetric
  value: number
  share: number
  drilldown: UsageAnalyticsDrilldown | null
}

export type UsageAnalyticsBreakdownChartData =
  | {
      kind: 'unsupported-share'
      metric: UsageAnalyticsMetric
      values: []
    }
  | {
      kind: 'empty'
      metric: UsageAnalyticsMetric
      values: []
    }
  | {
      kind: 'share'
      metric: UsageAnalyticsMetric
      values: UsageAnalyticsBreakdownDatum[]
    }

export function isAdditiveMetric(metric: UsageAnalyticsMetric): boolean {
  return (
    metric === 'request_count' || metric === 'total_tokens' || metric === 'quota'
  )
}

function finiteMetricValue(value: number): number {
  return Number.isFinite(value) ? value : 0
}

function metricValue(
  item: UsageAnalyticsGroup | UsageAnalyticsTimeseriesPoint,
  metric: UsageAnalyticsMetric
): number {
  return finiteMetricValue(item[metric])
}

function addMetricFields(
  target: UsageAnalyticsGroup,
  source: UsageAnalyticsGroup
): void {
  target.request_count += finiteMetricValue(source.request_count)
  target.success_count += finiteMetricValue(source.success_count)
  target.error_count += finiteMetricValue(source.error_count)
  target.quota += finiteMetricValue(source.quota)
  target.prompt_tokens += finiteMetricValue(source.prompt_tokens)
  target.completion_tokens += finiteMetricValue(source.completion_tokens)
  target.metered_tokens += finiteMetricValue(source.metered_tokens)
  target.total_tokens += finiteMetricValue(source.total_tokens)
}

function recomputeDerivedOtherFields(other: UsageAnalyticsGroup): void {
  if (other.request_count > 0) {
    other.success_rate = other.success_count / other.request_count
    other.error_rate = other.error_count / other.request_count
    return
  }
  other.success_rate = 0
  other.error_rate = 0
}

function minNonZeroTimestamp(current: number, next: number): number {
  if (current <= 0) return next > 0 ? next : 0
  if (next <= 0) return current
  return Math.min(current, next)
}

function buildOtherGroup(groups: UsageAnalyticsGroup[]): UsageAnalyticsGroup {
  const firstGroup = groups[0]
  const other: UsageAnalyticsGroup = {
    group_by: firstGroup?.group_by ?? 'model',
    group_key: 'other',
    group_value: 'other',
    group_label: 'Other',
    drilldown: null,
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
  }

  let share = 0
  let hasShare = false
  for (const group of groups) {
    addMetricFields(other, group)
    if (group.share !== null) {
      share += finiteMetricValue(group.share)
      hasShare = true
    }
    other.first_used_at = minNonZeroTimestamp(
      other.first_used_at,
      finiteMetricValue(group.first_used_at)
    )
    other.last_used_at = Math.max(
      other.last_used_at,
      finiteMetricValue(group.last_used_at)
    )
  }

  recomputeDerivedOtherFields(other)
  other.share = hasShare ? share : null
  return other
}

function normalizedLimit(limit: number): number {
  if (!Number.isFinite(limit)) return 0
  return Math.max(0, Math.trunc(limit))
}

export function buildTrendChartData(
  points: UsageAnalyticsTimeseriesPoint[],
  metric: UsageAnalyticsMetric
): UsageAnalyticsTrendChartDatum[] {
  const stacked = isAdditiveMetric(metric)
  return points.map((point) => ({
    timestamp: point.timestamp,
    time_label: point.time_label,
    group_key: point.group_key,
    group_value: point.group_value,
    group_label: point.group_label,
    metric,
    value: metricValue(point, metric),
    stacked,
    drilldown: point.drilldown,
  }))
}

export function mergeTopNWithOther(
  groups: UsageAnalyticsGroup[],
  metric: UsageAnalyticsMetric,
  limit: number
): UsageAnalyticsGroup[] {
  const topLimit = normalizedLimit(limit)
  if (groups.length <= topLimit || !isAdditiveMetric(metric)) {
    return groups.slice(0, topLimit)
  }

  const topGroups = groups.slice(0, topLimit)
  const otherGroups = groups.slice(topLimit)
  return [...topGroups, buildOtherGroup(otherGroups)]
}

function fallbackGroupLabel(group: UsageAnalyticsGroup): string {
  if (group.group_label) return group.group_label
  if (group.group_value) return group.group_value
  if (group.group_key) return group.group_key
  return 'Unknown'
}

function rankingDisplayLabel(group: UsageAnalyticsGroup): string {
  const token = group.token
  if (!token) return fallbackGroupLabel(group)
  if (token.name) return token.name
  if (group.group_label) return group.group_label
  return token.deleted ? 'Deleted API Key' : fallbackGroupLabel(group)
}

export function buildRankingRows(
  groups: UsageAnalyticsGroup[]
): UsageAnalyticsRankingRow[] {
  return groups.map((group) => ({
    ...group,
    id: group.group_key,
    display_label: rankingDisplayLabel(group),
    masked_key: group.token?.masked_key ?? null,
    token_deleted: group.token?.deleted ?? false,
  }))
}

export function buildBreakdownChartData(
  groups: UsageAnalyticsGroup[],
  metric: UsageAnalyticsMetric
): UsageAnalyticsBreakdownChartData {
  if (!isAdditiveMetric(metric)) {
    return { kind: 'unsupported-share', metric, values: [] }
  }
  if (groups.length === 0) return { kind: 'empty', metric, values: [] }

  const total = groups.reduce((sum, group) => sum + metricValue(group, metric), 0)
  return {
    kind: 'share',
    metric,
    values: groups.map((group) => {
      const value = metricValue(group, metric)
      return {
        group_key: group.group_key,
        group_value: group.group_value,
        group_label: group.group_label,
        metric,
        value,
        share: group.share ?? (total > 0 ? value / total : 0),
        drilldown: group.drilldown,
      }
    }),
  }
}
