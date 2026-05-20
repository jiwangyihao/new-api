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
import { Trophy, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatTokens } from '../lib/format'
import type { FreeUserRanking } from '../types'

type FreeUsersSectionProps = {
  rows: FreeUserRanking[]
  totalTokens: number
}

export function FreeUsersSection(props: FreeUsersSectionProps) {
  const { t } = useTranslation()

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

      {props.rows.length === 0 ? (
        <div className='text-muted-foreground/80 border-t px-5 py-10 text-center text-sm'>
          {t('No free-plan ranking data available')}
        </div>
      ) : (
        <ol className='divide-y border-t'>
          {props.rows.map((row) => (
            <li key={row.rank} className='flex items-center gap-3 px-5 py-3'>
              <span className='text-muted-foreground/80 w-8 shrink-0 text-right font-mono text-sm tabular-nums'>
                {row.rank}.
              </span>
              <span className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-full'>
                <UserRound className='size-4' />
              </span>
              <div className='min-w-0 flex-1'>
                <div className='text-foreground truncate text-sm font-medium'>
                  {row.display_name}
                </div>
                <div className='text-muted-foreground/80 text-xs'>
                  {row.named ? t('Custom display name') : t('Anonymous entry')}
                </div>
              </div>
              <div className='shrink-0 text-right'>
                <div className='text-foreground font-mono text-sm font-semibold tabular-nums'>
                  {formatTokens(row.total_tokens)}
                </div>
                <div className='text-muted-foreground/80 text-xs'>
                  {t('tokens')}
                </div>
              </div>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
