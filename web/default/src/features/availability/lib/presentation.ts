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
import { dotColorMap, type StatusVariant } from '@/components/status-badge'
import type { AvailabilityMetric, AvailabilityState } from '../types'

type AvailabilityBand = AvailabilityState | 'excellent' | 'critical'

export const availabilityBands: Record<
  AvailabilityBand,
  {
    labelKey: string
    variant: StatusVariant
    dotClassName: string
    rangeLabel: string
    state: AvailabilityState
  }
> = {
  excellent: {
    labelKey: 'Excellent',
    variant: 'success',
    dotClassName: dotColorMap.success,
    rangeLabel: '≥99%',
    state: 'healthy',
  },
  healthy: {
    labelKey: 'Healthy',
    variant: 'success',
    dotClassName: 'bg-success/55',
    rangeLabel: '90%–<99%',
    state: 'healthy',
  },
  degraded: {
    labelKey: 'Degraded',
    variant: 'warning',
    dotClassName: dotColorMap.warning,
    rangeLabel: '80%–<90%',
    state: 'degraded',
  },
  unhealthy: {
    labelKey: 'Unhealthy',
    variant: 'danger',
    dotClassName: 'bg-destructive/55',
    rangeLabel: '50%–<80%',
    state: 'unhealthy',
  },
  critical: {
    labelKey: 'Critical',
    variant: 'danger',
    dotClassName: dotColorMap.danger,
    rangeLabel: '<50%',
    state: 'unhealthy',
  },
  unknown: {
    labelKey: 'No valid observations',
    variant: 'neutral',
    dotClassName: dotColorMap.neutral,
    rangeLabel: '',
    state: 'unknown',
  },
  incomplete: {
    labelKey: 'Incomplete coverage',
    variant: 'info',
    dotClassName: dotColorMap.info,
    rangeLabel: '',
    state: 'incomplete',
  },
}

export function observedBand(metric: AvailabilityMetric): AvailabilityBand {
  if (metric.coverage === 'incomplete') return 'incomplete'
  if (metric.success_rate === null) return 'unknown'
  if (metric.success_rate >= 99) return 'excellent'
  if (metric.success_rate >= 90) return 'healthy'
  if (metric.success_rate >= 80) return 'degraded'
  if (metric.success_rate >= 50) return 'unhealthy'
  return 'critical'
}

export function observedState(metric: AvailabilityMetric): AvailabilityState {
  return availabilityBands[observedBand(metric)].state
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
