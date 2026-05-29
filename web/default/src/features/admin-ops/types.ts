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
export type AdminOpsHealthStatus = 'healthy' | 'degraded' | 'critical'
export type AdminOpsDependencyStatus = 'healthy' | 'disabled' | 'critical'
export type AdminOpsConcurrencyMode = 'redis' | 'memory' | 'disabled'

export type AdminOpsHealth = {
  status: AdminOpsHealthStatus
  score: number
  reasons: string[]
}

export type AdminOpsRuntime = {
  version: string
  start_time: number
  uptime_seconds: number
  node_name: string
  active_connections: number
  goroutines: number
}

export type AdminOpsSystem = {
  cpu_usage: number
  memory_usage: number
  disk_usage: number
}

export type AdminOpsDependency = {
  enabled: boolean
  status: AdminOpsDependencyStatus
  latency_ms: number
  message: string
}

export type AdminOpsDependencies = {
  database: AdminOpsDependency
  redis: AdminOpsDependency
}

export type AdminOpsConcurrencySummary = {
  total_active: number
  total_queued: number
  active_users: number
  queued_users: number
  saturated_users: number
  queue_pressure: number
}

export type AdminOpsConcurrencyConfig = {
  ttl_seconds: number
  default_queue_capacity: number
  require_redis: boolean
  fail_open: boolean
}

export type AdminOpsConcurrencyCounters = {
  acquired_total: number
  queued_total: number
  queue_full_rejections_total: number
  unavailable_rejections_total: number
  redis_errors_total: number
}

export type AdminOpsConcurrencyUserStatus =
  | 'normal'
  | 'saturated'
  | 'queued'
  | 'queue_full_risk'

export type AdminOpsConcurrencyUser = {
  user_id: number
  username: string
  active: number
  limit: number
  queued: number
  queue_capacity: number
  oldest_queued_seconds: number
  utilization: number
  queue_utilization: number
  status: AdminOpsConcurrencyUserStatus | string
}

export type AdminOpsConcurrencyResponse = {
  mode: AdminOpsConcurrencyMode
  generated_at: number
  enabled: boolean
  summary: AdminOpsConcurrencySummary
  config: AdminOpsConcurrencyConfig
  counters: AdminOpsConcurrencyCounters
  users: AdminOpsConcurrencyUser[]
}

export type AdminOpsTraffic = {
  window_seconds: number
  requests: number
  errors: number
  rpm: number
  tpm: number
  error_rate: number
}

export type AdminOpsChannels = {
  total: number
  enabled: number
  manual_disabled: number
  auto_disabled: number
  slow_count: number
  stale_test_count: number
}

export type AdminOpsPerformanceModel = {
  model_name: string
  avg_latency_ms: number
  avg_ttft_ms: number
  success_rate: number
  avg_tps: number
  request_count: number
}

export type AdminOpsPerformance = {
  models: AdminOpsPerformanceModel[]
}

export type AdminOpsRecentError = {
  id: number
  created_at: number
  user_id: number
  username: string
  model_name: string
  channel_id: number
  content: string
  request_id: string
}

export type AdminOpsSnapshot = {
  generated_at: number
  health: AdminOpsHealth
  runtime: AdminOpsRuntime
  system: AdminOpsSystem
  dependencies: AdminOpsDependencies
  concurrency: AdminOpsConcurrencyResponse
  traffic: AdminOpsTraffic
  channels: AdminOpsChannels
  performance: AdminOpsPerformance
  recent_errors: AdminOpsRecentError[]
}

export type AdminOpsApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}
