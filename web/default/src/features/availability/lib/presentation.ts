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
import type { StatusVariant } from '@/components/status-badge'
import type { AvailabilityMetric, AvailabilityState } from '../types'

export const availabilityStates: Record<
  AvailabilityState,
  { labelKey: string; variant: StatusVariant }
> = {
  healthy: { labelKey: 'Healthy', variant: 'success' },
  degraded: { labelKey: 'Degraded', variant: 'warning' },
  unhealthy: { labelKey: 'Unhealthy', variant: 'danger' },
  unknown: { labelKey: 'No valid observations', variant: 'neutral' },
  incomplete: { labelKey: 'Incomplete coverage', variant: 'info' },
}
export function observedState(metric: AvailabilityMetric): AvailabilityState {
  if (metric.coverage === 'incomplete') return 'incomplete'
  if (metric.success_rate === null) return 'unknown'
  return metric.state
}
export function formatRate(value: number | null, locale: string): string {
  if (value === null) return '—'
  return new Intl.NumberFormat(locale, {
    maximumFractionDigits: 2,
    style: 'percent',
  }).format(value / 100)
}
export function formatObservedTime(
  value: number | null,
  locale: string
): string {
  if (value === null) return '—'
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    timeZoneName: 'short',
  }).format(new Date(value * 1000))
}
export function formatLatency(value: number | null, locale: string): string {
  if (value === null) return '—'
  return new Intl.NumberFormat(locale, {
    style: 'unit',
    unit: 'millisecond',
    maximumFractionDigits: 2,
  }).format(value)
}
