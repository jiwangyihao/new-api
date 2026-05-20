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
import { PieChart } from 'lucide-react'
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
import { buildBreakdownChartData } from '../lib/chart-data'
import { formatUsagePercent, formatUsageTokens } from '../lib/format'
import type { UsageAnalyticsGroup, UsageAnalyticsMetric } from '../types'

interface UsageBreakdownChartProps {
  groups?: UsageAnalyticsGroup[]
  metric: UsageAnalyticsMetric
  loading?: boolean
  error?: boolean
}

type VChartSpec = Record<string, unknown>
type VChartComponentSpec = ComponentProps<typeof VChart>['spec']

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

function buildSpec(values: UsageAnalyticsGroup[], metric: UsageAnalyticsMetric): VChartSpec {
  const chartData = buildBreakdownChartData(values, metric)
  if (chartData.kind !== 'share') return {}
  return {
    type: 'pie',
    data: [{ id: 'usage-breakdown', values: chartData.values }],
    categoryField: 'group_label',
    valueField: 'value',
    outerRadius: 0.82,
    innerRadius: 0.58,
    padAngle: 1,
    legends: { visible: true, orient: 'bottom' },
    label: {
      visible: true,
      formatMethod: (datum: Record<string, unknown>) =>
        formatUsagePercent(Number(datum.share) || 0),
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum: Record<string, unknown>) =>
              String(datum.group_label ?? ''),
            value: (datum: Record<string, unknown>) =>
              `${formatUsageTokens(Number(datum.value) || 0)} (${formatUsagePercent(
                Number(datum.share) || 0
              )})`,
          },
        ],
      },
    },
    background: 'transparent',
  }
}

export function UsageBreakdownChart(props: UsageBreakdownChartProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [themeReady, setThemeReady] = useState(false)
  const chartData = useMemo(
    () => buildBreakdownChartData(props.groups ?? [], props.metric),
    [props.groups, props.metric]
  )
  const spec = useMemo(() => {
    if (chartData.kind !== 'share') return null
    return buildSpec(props.groups ?? [], props.metric)
  }, [chartData.kind, props.groups, props.metric])

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

  if (chartData.kind === 'unsupported-share') {
    return (
      <Empty className='min-h-[320px] sm:min-h-[420px]'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <PieChart className='size-6' />
          </EmptyMedia>
          <EmptyTitle>{t('This metric does not support share chart')}</EmptyTitle>
          <EmptyDescription>
            {t('Select request count, total tokens, or quota to view share distribution')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  if (chartData.kind === 'empty' || spec === null) {
    return (
      <Empty className='min-h-[320px] sm:min-h-[420px]'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <PieChart className='size-6' />
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
          key={`usage-breakdown-${resolvedTheme}-${props.metric}-${chartData.values.length}`}
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
