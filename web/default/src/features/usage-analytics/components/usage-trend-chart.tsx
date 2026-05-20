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
import { useEffect, useMemo, useState, type ComponentProps } from 'react'
import { VChart } from '@visactor/react-vchart'
import { LineChart } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useTheme } from '@/context/theme-provider'
import { VCHART_OPTION } from '@/lib/vchart'
import { ErrorState } from '@/components/error-state'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  buildTrendChartData,
  isAdditiveMetric,
  type UsageAnalyticsTrendChartDatum,
} from '../lib/chart-data'
import {
  formatLatencyMs,
  formatUsagePercent,
  formatUsageTokens,
} from '../lib/format'
import type {
  UsageAnalyticsMetric,
  UsageAnalyticsTimeseriesPoint,
} from '../types'

interface UsageTrendChartProps {
  points?: UsageAnalyticsTimeseriesPoint[]
  metric: UsageAnalyticsMetric
  loading?: boolean
  error?: boolean
}

interface TrendChartDatum extends UsageAnalyticsTrendChartDatum {
  series_id: string
  series_label: string
}

type VChartSpec = Record<string, unknown>
type VChartComponentSpec = ComponentProps<typeof VChart>['spec']

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

function formatMetricValue(metric: UsageAnalyticsMetric, value: number): string {
  if (metric === 'avg_latency_ms' || metric === 'p95_latency_ms') {
    return formatLatencyMs(value)
  }
  if (metric === 'error_rate') return formatUsagePercent(value)
  return formatUsageTokens(value)
}

function buildSeriesLabel(datum: UsageAnalyticsTrendChartDatum): string {
  if (datum.group_label) return datum.group_label
  if (datum.group_value) return datum.group_value
  return datum.group_key
}

function buildSpec(
  values: TrendChartDatum[],
  metric: UsageAnalyticsMetric
): VChartSpec {
  const additive = isAdditiveMetric(metric)
  return {
    type: additive ? 'area' : 'line',
    data: [{ id: 'usage-trend', values }],
    xField: 'time_label',
    yField: 'value',
    seriesField: 'series_id',
    stack: additive,
    point: { visible: !additive },
    legends: { visible: true, orient: 'bottom', selectMode: 'multiple' },
    axes: [
      {
        orient: 'bottom',
        label: {
          style: { fill: 'currentColor', fontSize: 10 },
          autoHide: true,
        },
        tick: { visible: false },
      },
      {
        orient: 'left',
        label: {
          formatMethod: (value: number | string) =>
            formatMetricValue(metric, Number(value) || 0),
          style: { fill: 'currentColor', fontSize: 10 },
        },
        grid: { visible: true, style: { lineDash: [3, 3] } },
      },
    ],
    tooltip: {
      dimension: {
        title: {
          value: (datum: Record<string, unknown>) =>
            String(datum.time_label ?? ''),
        },
        content: [
          {
            key: (datum: Record<string, unknown>) =>
              String(datum.series_label ?? datum.series_id ?? ''),
            value: (datum: Record<string, unknown>) =>
              Number(datum.value) || 0,
          },
        ],
        updateContent: (
          items: Array<{ key: string; value: string | number }>
        ) =>
          items
            .sort((first, second) => Number(second.value) - Number(first.value))
            .map((item) => ({
              key: item.key,
              value: formatMetricValue(metric, Number(item.value) || 0),
            })),
      },
      mark: {
        content: [
          {
            key: (datum: Record<string, unknown>) =>
              String(datum.series_label ?? datum.series_id ?? ''),
            value: (datum: Record<string, unknown>) =>
              formatMetricValue(metric, Number(datum.value) || 0),
          },
        ],
      },
    },
    area: { style: { fillOpacity: additive ? 0.18 : 0 } },
    line: { style: { lineWidth: 2, curveType: 'monotone' } },
    background: 'transparent',
  }
}

export function UsageTrendChart(props: UsageTrendChartProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [themeReady, setThemeReady] = useState(false)
  const chartValues = useMemo<TrendChartDatum[]>(() => {
    if (!props.points) return []
    return buildTrendChartData(props.points, props.metric).map((datum) => ({
      ...datum,
      series_id: datum.group_key,
      series_label: buildSeriesLabel(datum),
    }))
  }, [props.metric, props.points])
  const spec = useMemo(() => {
    if (chartValues.length === 0) return null
    return buildSpec(chartValues, props.metric)
  }, [chartValues, props.metric])

  useEffect(() => {
    let cancelled = false
    const updateTheme = async () => {
      setThemeReady(false)
      if (!themeManagerPromise) {
        themeManagerPromise = import('@visactor/vchart').then(
          (module) => module.ThemeManager
        )
      }
      const ThemeManager = await themeManagerPromise
      if (cancelled) return
      ThemeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }
    updateTheme()
    return () => {
      cancelled = true
    }
  }, [resolvedTheme])

  if (props.loading) {
    return <Skeleton className='h-[320px] w-full rounded-xl sm:h-[420px]' />
  }

  if (props.error) {
    return (
      <ErrorState
        className='min-h-[320px] sm:min-h-[420px]'
        title={t('Failed to load usage analytics')}
        description={t('Try adjusting the time range or filters')}
      />
    )
  }

  if (chartValues.length === 0 || spec === null) {
    return (
      <Empty className='min-h-[320px] sm:min-h-[420px]'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <LineChart className='size-6' />
          </EmptyMedia>
          <EmptyTitle>{t('No usage data')}</EmptyTitle>
          <EmptyDescription>
            {t('Try adjusting the time range or filters')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <div className='h-[320px] w-full sm:h-[420px]'>
      {themeReady ? (
        <VChart
          key={`usage-trend-${resolvedTheme}-${props.metric}-${chartValues.length}`}
          spec={
            {
              ...spec,
              theme: resolvedTheme === 'dark' ? 'dark' : 'light',
              background: 'transparent',
            } as VChartComponentSpec
          }
          option={VCHART_OPTION}
        />
      ) : (
        <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
          {t('Loading')}
        </div>
      )}
    </div>
  )
}
