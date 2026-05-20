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

type FreeUserBarDatum = FreeUserRanking & { series_label: string }

type FreeUsersBarChartProps = {
  rows: FreeUserRanking[]
}

function buildBarData(rows: FreeUserRanking[]): FreeUserBarDatum[] {
  return rows.map((row) => ({
    ...row,
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
      bar: {
        style: barRadius == null ? {} : { cornerRadius: barRadius },
      },
      axes: [
        {
          orient: 'bottom',
          label: {
            formatMethod: (val: number | string) => formatTokens(Number(val)),
            style: { fill: 'currentColor', fontSize: 10 },
          },
          grid: { visible: true, style: { lineDash: [3, 3] } },
        },
        {
          orient: 'left',
          label: {
            style: { fill: 'currentColor', fontSize: 10 },
            autoHide: true,
            autoLimit: true,
          },
          tick: { visible: false },
        },
      ],
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) =>
                String(datum?.series_label ?? ''),
              value: (datum: Record<string, unknown>) =>
                formatTokens(Number(datum?.total_tokens) || 0),
            },
          ],
        },
      },
      animationAppear: { duration: 500 },
    }
  }, [barRadius, data])

  if (data.length === 0) {
    return (
      <div className='text-muted-foreground/80 flex h-full items-center justify-center text-xs'>
        {t('No free-plan ranking data available')}
      </div>
    )
  }

  return (
    <div className='h-64 sm:h-72'>
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
