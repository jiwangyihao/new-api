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
export type AvailabilityRange = '1h' | '24h' | '7d' | '30d'
export type AvailabilityState =
  | 'healthy'
  | 'degraded'
  | 'unhealthy'
  | 'unknown'
  | 'incomplete'
export interface AvailabilityMetric {
  state: AvailabilityState
  success_rate: number | null
  first_response_ms: number | null
  low_sample: boolean
  coverage: 'complete' | 'incomplete'
  last_observed_at: number | null
  has_failures: boolean
}
export interface AvailabilityBucket extends AvailabilityMetric {
  start: number
  end: number
}
export interface AvailabilityModel {
  name: string
  active: boolean
  current: AvailabilityMetric
  summary: AvailabilityMetric
  buckets: AvailabilityBucket[]
}
export interface AvailabilityGroup {
  id: number
  name: string
  description: string
  current: AvailabilityMetric
  summary: AvailabilityMetric
  buckets: AvailabilityBucket[]
  models: AvailabilityModel[]
}
export interface AvailabilityReport {
  range: AvailabilityRange
  window_start: number
  window_end: number
  current_start: number
  generated_at: number
  coverage_start: number
  refresh_seconds: number
  bucket_seconds: number
  groups: AvailabilityGroup[]
}
