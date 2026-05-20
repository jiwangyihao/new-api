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
import { useState } from 'react'
import { BarChart3, LineChart, Trophy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { formatTokens } from '../lib/format'
import type { FreeUserHistorySeries, FreeUserRanking } from '../types'
import { FreeUsersBarChart } from './free-users-bar-chart'
import { FreeUsersLineChart, type FreeUserTrendMode } from './free-users-line-chart'
import { FreeUsersList } from './free-users-list'

type FreeUsersSectionProps = {
  rows: FreeUserRanking[]
  totalTokens: number
  history: FreeUserHistorySeries
}

type FreeUsersView = 'bar' | 'trend'

export function FreeUsersSection(props: FreeUsersSectionProps) {
  const { t } = useTranslation()
  const [view, setView] = useState<FreeUsersView>('bar')
  const [trendMode, setTrendMode] = useState<FreeUserTrendMode>('hourly')

  return (
    <section className='bg-card overflow-hidden rounded-lg border'>
      <header className='flex items-start justify-between gap-4 px-5 py-4'>
        <div className='min-w-0 flex-1'>
          <h2 className='text-foreground inline-flex items-center gap-2 text-base font-semibold'>
            <Trophy className='size-4 text-amber-500' />
            {t('Free-plan token leaderboard')}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('A fun ranking of token usage during free plan access')}
          </p>
        </div>
        <div className='shrink-0 text-right'>
          <div className='text-foreground font-mono text-2xl font-semibold tabular-nums'>
            {formatTokens(props.totalTokens)}
          </div>
          <div className='text-muted-foreground/80 text-[10px] font-medium tracking-widest uppercase'>
            {t('tokens')}
          </div>
        </div>
      </header>

      <div className='border-t'>
        <div className='flex w-full flex-col gap-3 border-b px-3 py-2 sm:flex-row sm:items-center sm:justify-between sm:px-5 sm:py-3'>
          <div>
            <h3 className='text-foreground inline-flex items-center gap-2 text-sm font-semibold'>
              {view === 'bar' ? (
                <BarChart3 className='text-primary size-3.5' />
              ) : (
                <LineChart className='text-primary size-3.5' />
              )}
              {view === 'bar'
                ? t('Usage after free-plan activation')
                : t('24-hour trend')}
            </h3>
            <p className='text-muted-foreground/80 mt-0.5 text-xs'>
              {t(
                'Compare each ranked user within their first 24 hours of free-plan access'
              )}
            </p>
          </div>
          <div className='flex flex-wrap gap-2'>
            <button
              type='button'
              onClick={() => setView('bar')}
              className={cn(
                'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                view === 'bar'
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              )}
            >
              {t('Bar chart')}
            </button>
            <button
              type='button'
              onClick={() => setView('trend')}
              className={cn(
                'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                view === 'trend'
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              )}
            >
              {t('24-hour trend')}
            </button>
          </div>
        </div>
        <div className='p-1.5 sm:p-2'>
          {view === 'bar' ? (
            <FreeUsersBarChart rows={props.rows} />
          ) : (
            <div className='space-y-3'>
              <div className='flex w-fit flex-wrap gap-1.5 rounded-lg border p-0.5'>
                <button
                  type='button'
                  onClick={() => setTrendMode('hourly')}
                  className={cn(
                    'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                    trendMode === 'hourly'
                      ? 'bg-primary text-primary-foreground shadow-sm'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                  )}
                >
                  {t('Hourly usage')}
                </button>
                <button
                  type='button'
                  onClick={() => setTrendMode('cumulative')}
                  className={cn(
                    'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                    trendMode === 'cumulative'
                      ? 'bg-primary text-primary-foreground shadow-sm'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                  )}
                >
                  {t('Cumulative usage')}
                </button>
              </div>
              <FreeUsersLineChart history={props.history} mode={trendMode} />
            </div>
          )}
        </div>
      </div>

      <div className='border-t'>
        <FreeUsersList rows={props.rows} />
      </div>
    </section>
  )
}
