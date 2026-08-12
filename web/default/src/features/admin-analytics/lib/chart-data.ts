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
import { ADMIN_ANALYTICS_QUOTA_BUCKET_ORDER } from '../constants'
import type { AdminAnalyticsQuotaBucket, AdminUsageGroup } from '../types'

const bucketRank = new Map<string, number>(
  ADMIN_ANALYTICS_QUOTA_BUCKET_ORDER.map((bucket, index) => [bucket, index])
)

export function orderQuotaBuckets(
  buckets: readonly AdminAnalyticsQuotaBucket[]
): AdminAnalyticsQuotaBucket[] {
  return [...buckets].sort(
    (first, second) =>
      (bucketRank.get(first.bucket) ?? 999) -
      (bucketRank.get(second.bucket) ?? 999)
  )
}

export function topNWithOther(
  groups: readonly AdminUsageGroup[],
  limit: number
): { groups: AdminUsageGroup[]; other: AdminUsageGroup | null } {
  if (limit <= 0 || groups.length <= limit) {
    return { groups: [...groups], other: null }
  }
  const visible = groups.slice(0, limit)
  const rest = groups.slice(limit)
  const other = rest.reduce<AdminUsageGroup>(
    (acc, group) => ({
      ...acc,
      request_count: acc.request_count + group.request_count,
      success_count: acc.success_count + group.success_count,
      error_count: acc.error_count + group.error_count,
      quota: acc.quota + group.quota,
      prompt_tokens: acc.prompt_tokens + group.prompt_tokens,
      completion_tokens: acc.completion_tokens + group.completion_tokens,
      metered_tokens: acc.metered_tokens + group.metered_tokens,
      total_tokens: acc.total_tokens + group.total_tokens,
    }),
    {
      group_by: groups[0].group_by,
      group_key: '__other__',
      group_value: '__other__',
      group_label: 'Other',
      share: null,
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
      rpm: 0,
      tpm: 0,
      active_users: 0,
      active_api_keys: 0,
      first_used_at: 0,
      last_used_at: 0,
    }
  )
  if (other.request_count > 0) {
    other.success_rate = other.success_count / other.request_count
    other.error_rate = other.error_count / other.request_count
  }
  return { groups: visible, other }
}
