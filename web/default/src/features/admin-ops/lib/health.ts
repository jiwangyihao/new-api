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
  AdminOpsConcurrencyUserStatus,
  AdminOpsHealthStatus,
} from '../types'

export type AdminOpsHealthTone = 'success' | 'warning' | 'destructive' | 'muted'

type ConcurrencyPressureInput = {
  active: number
  limit: number
  queued: number
  queue_capacity: number
}

export function getAdminOpsHealthTone(status: string): AdminOpsHealthTone {
  if (status === 'healthy') return 'success'
  if (status === 'degraded') return 'warning'
  if (status === 'critical') return 'destructive'
  return 'muted'
}

export function getAdminOpsHealthLabelKey(
  status: AdminOpsHealthStatus
): string {
  return `adminOps.health.${status}`
}

const adminOpsHealthReasonLabelKeys: Record<string, string> = {
  database_unhealthy: 'adminOps.health.reason.database_unhealthy',
  redis_unhealthy: 'adminOps.health.reason.redis_unhealthy',
  system_cpu_high: 'adminOps.health.reason.system_cpu_high',
  system_memory_high: 'adminOps.health.reason.system_memory_high',
  system_disk_high: 'adminOps.health.reason.system_disk_high',
  concurrency_queue_full_rejections:
    'adminOps.health.reason.concurrency_queue_full_rejections',
  concurrency_redis_errors: 'adminOps.health.reason.concurrency_redis_errors',
  concurrency_unavailable_rejections:
    'adminOps.health.reason.concurrency_unavailable_rejections',
  concurrency_queue_not_empty:
    'adminOps.health.reason.concurrency_queue_not_empty',
  concurrency_saturated_users:
    'adminOps.health.reason.concurrency_saturated_users',
  concurrency_queue_pressure_high:
    'adminOps.health.reason.concurrency_queue_pressure_high',
  channel_auto_disabled: 'adminOps.health.reason.channel_auto_disabled',
  traffic_error_rate_high: 'adminOps.health.reason.traffic_error_rate_high',
}

export function getAdminOpsHealthReasonLabelKey(code: string): string {
  return adminOpsHealthReasonLabelKeys[code] ?? 'adminOps.health.reason.unknown'
}
export function getAdminOpsConcurrencyUserStatus(
  value: ConcurrencyPressureInput
): AdminOpsConcurrencyUserStatus {
  if (value.queue_capacity > 0 && value.queued >= value.queue_capacity) {
    return 'queue_full_risk'
  }
  if (value.queued > 0) return 'queued'
  if (value.limit > 0 && value.active >= value.limit) return 'saturated'
  return 'normal'
}
