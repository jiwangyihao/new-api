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
import { UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatTokens } from '../lib/format'
import type { FreeUserRanking } from '../types'

type FreeUsersListProps = {
  rows: FreeUserRanking[]
}

export function FreeUsersList(props: FreeUsersListProps) {
  const { t } = useTranslation()

  if (props.rows.length === 0) {
    return (
      <div className='text-muted-foreground/80 px-5 py-8 text-center text-sm'>
        {t('No free-plan ranking data available')}
      </div>
    )
  }

  return (
    <ol className='divide-y'>
      {props.rows.map((row) => (
        <li key={row.rank} className='flex items-center gap-3 px-5 py-3'>
          <span className='text-muted-foreground/80 w-8 shrink-0 text-right font-mono text-sm tabular-nums'>
            {t('Rank #{{rank}}', { rank: row.rank })}
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
            <div className='text-muted-foreground/80 text-xs'>{t('tokens')}</div>
          </div>
        </li>
      ))}
    </ol>
  )
}
