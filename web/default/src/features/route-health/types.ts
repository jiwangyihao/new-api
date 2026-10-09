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
export type RouteHealthState =
  | 'healthy'
  | 'unobserved'
  | 'cooling'
  | 'recovering'
  | 'saturated'
  | 'manually_disabled'
  | 'automatically_disabled'
  | 'no_credentials'
  | 'invalid_mapping'
  | 'protection_disabled'

export interface RouteHealthResource {
  scope: 'channel' | 'route' | 'credential'
  credential_index: number
  state: RouteHealthState
  successes: number
  failures: number
  consecutive_failures: number
  inflight: number
  opened_until: number
  next_probe: number
  probe: boolean
  recovery_successes: number
  degraded: boolean
}

export interface RouteHealthRow {
  id: string
  channel_id: number
  channel_name: string
  channel_status: number
  models: string[]
  upstream_model: string
  operation: string
  state: RouteHealthState
  ready_credentials: number
  total_credentials: number
  resources: RouteHealthResource[]
}

export interface RouteHealthReport {
  generated_at: number
  enabled: boolean
  storage: 'redis' | 'memory'
  degraded: boolean
  retry_times: number
  max_attempts: number
  sample_window_seconds: number
  failure_window_seconds: number
  rows: RouteHealthRow[]
}
