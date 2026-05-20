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
import { useThemeRadiusPx } from '@/lib/theme-radius'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { formatTokens } from '../lib/format'
import type { FreeUserRanking } from '../types'

type FreeUserBarDatum = FreeUserRanking & { rawTokens: number; series_label: string }

const chartColors = [
  '#3b82f6',
  '#8b5cf6',
  '#06b6d4',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#ec4899',
  '#6366f1',
  '#84cc16',
  '#f97316',
]

type FreeUsersBarChartProps = {
  rows: FreeUserRanking[]
}

function buildBarData(rows: FreeUserRanking[]): FreeUserBarDatum[] {
  return rows.map((row) => ({
    ...row,
    rawTokens: row.total_tokens,
    series_label: `#${row.rank} · ${row.display_name}`,
  }))
}

export function FreeUsersBarChart(props: FreeUsersBarChartProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const { customization } = useThemeCustomization()
  const barRadius = useThemeRadiusPx(
    '--radius-sm',
    `${customization.preset}:${customization.radius}`
  )

  const data = useMemo(() => buildBarData(props.rows), [props.rows])
  const spec = useMemo(() => {
    if (data.length === 0) return null
    return {
      type: 'bar' as const,
      direction: 'horizontal' as const,
      data: [{ id: 'free-users-bar', values: data }],
      xField: 'total_tokens',
      yField: 'series_label',
      seriesField: 'series_label',
      bar: {
        state: { hover: { stroke: '#000', lineWidth: 1 } },
        style: barRadius == null ? {} : { cornerRadius: barRadius },
      },
      label: {
        visible: true,
        position: 'outside',
        formatMethod: (value: number) => formatTokens(value),
        style: { fontSize: 11 },
      },
      axes: [
        {
          orient: 'bottom',
          label: {
            formatMethod: (val: number | string) => formatTokens(Number(val)),
            style: { fill: 'currentColor', fontSize: 10 },
          },
          visible: false,
        },
        {
          orient: 'left',
          type: 'band',
        },
      ],
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) =>
                String(datum?.series_label ?? ''),
              value: (datum: Record<string, unknown>) =>
                formatTokens(Number(datum?.rawTokens) || 0),
            },
          ],
        },
      },
      color: {
        specified: Object.fromEntries(
          data.map((item, index) => [
            item.series_label,
            chartColors[index % chartColors.length],
          ])
        ),
      },
      background: { fill: 'transparent' },
      animation: true,
      animationAppear: { duration: 500 },
    }
  }, [barRadius, data])

  if (data.length === 0) {
    return (
      <div className='text-muted-foreground/80 flex h-[380px] items-center justify-center p-1.5 text-xs sm:h-[520px] sm:p-2'>
        {t('No free-plan ranking data available')}
      </div>
    )
  }

  return (
    <div className='h-[380px] p-1.5 sm:h-[520px] sm:p-2'>
      {themeReady && spec ? (
        <VChart
          key={`free-users-bar-${resolvedTheme}`}
          spec={{
            ...spec,
            theme: resolvedTheme === 'dark' ? 'dark' : 'light',
            background: 'transparent',
          }}
          option={VCHART_OPTION}
        />
      ) : (
        <div className='text-muted-foreground/80 flex h-full items-center justify-center text-xs'>
          {t('No free-plan ranking data available')}
        </div>
      )}
    </div>
  )
}
