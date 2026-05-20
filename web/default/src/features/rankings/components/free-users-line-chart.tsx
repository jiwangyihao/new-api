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
import { useMemo } from 'react'
import { VChart } from '@visactor/react-vchart'
import { useTranslation } from 'react-i18next'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'
import { formatTokens } from '../lib/format'
import type { FreeUserHistorySeries } from '../types'

export type FreeUserTrendMode = 'hourly' | 'cumulative'

type FreeUsersLineChartProps = {
  history: FreeUserHistorySeries
  mode: FreeUserTrendMode
}

export function FreeUsersLineChart(props: FreeUsersLineChartProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const points = useMemo(
    () => props.history.points.filter((point) => point.rank <= 10),
    [props.history.points]
  )
  const yField = props.mode === 'hourly' ? 'tokens' : 'cumulative_tokens'
  const spec = useMemo(() => {
    if (points.length === 0) return null
    return {
      type: 'line' as const,
      data: [{ id: 'free-users-line', values: points }],
      xField: 'hour_label',
      yField,
      seriesField: 'series_label',
      point: { visible: false },
      legends: { visible: true, orient: 'bottom' },
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
            formatMethod: (val: number | string) => formatTokens(Number(val)),
            style: { fill: 'currentColor', fontSize: 10 },
          },
          grid: { visible: true, style: { lineDash: [3, 3] } },
        },
      ],
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) =>
                String(datum?.series_label ?? ''),
              value: (datum: Record<string, unknown>) =>
                formatTokens(Number(datum?.[yField]) || 0),
            },
          ],
        },
        dimension: {
          title: {
            value: (datum: Record<string, unknown>) =>
              String(datum?.hour_label ?? ''),
          },
          content: [
            {
              key: (datum: Record<string, unknown>) =>
                String(datum?.series_label ?? ''),
              value: (datum: Record<string, unknown>) =>
                Number(datum?.[yField]) || 0,
            },
          ],
          updateContent: (
            array: Array<{ key: string; value: string | number }>
          ) =>
            array
              .sort((a, b) => Number(b.value) - Number(a.value))
              .map((item) => ({
                key: item.key,
                value: formatTokens(Number(item.value) || 0),
              })),
        },
      },
      animationAppear: { duration: 500 },
    }
  }, [points, yField])

  if (points.length === 0) {
    return (
      <div className='text-muted-foreground/80 flex h-full items-center justify-center text-xs'>
        {t('No free-plan trend data available')}
      </div>
    )
  }

  return (
    <div className='h-64 sm:h-72'>
      {themeReady && spec ? (
        <VChart
          key={`free-users-line-${resolvedTheme}-${props.mode}`}
          spec={{
            ...spec,
            theme: resolvedTheme === 'dark' ? 'dark' : 'light',
            background: 'transparent',
          }}
          option={VCHART_OPTION}
        />
      ) : (
        <div className='text-muted-foreground/80 flex h-full items-center justify-center text-xs'>
          {t('No free-plan trend data available')}
        </div>
      )}
    </div>
  )
}
