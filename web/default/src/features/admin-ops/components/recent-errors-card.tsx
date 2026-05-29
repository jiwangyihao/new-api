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
import { useTranslation } from 'react-i18next'
import type { AdminOpsSnapshot } from '../types'
import { AdminOpsCardSkeleton, AdminOpsEmpty, AdminOpsPanel } from './shared'

export function RecentErrorsCard(props: {
  snapshot?: AdminOpsSnapshot
  loading: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) return <AdminOpsCardSkeleton className='h-64 w-full' />

  const errors = props.snapshot?.recent_errors ?? []
  return (
    <AdminOpsPanel title={t('adminOps.recentErrors.title')}>
      {errors.length > 0 ? (
        <div className='space-y-3'>
          {errors.map((error) => (
            <article key={error.id} className='rounded-lg border p-3'>
              <div className='flex flex-wrap items-center justify-between gap-2 text-xs'>
                <span className='text-muted-foreground'>
                  {t('adminOps.recentErrors.createdAt')}: {formatTimestamp(error.created_at)}
                </span>
                <span className='text-muted-foreground'>
                  {t('adminOps.recentErrors.requestId')}: {error.request_id || '—'}
                </span>
              </div>
              <div className='mt-2 grid gap-2 text-xs sm:grid-cols-2'>
                <span>
                  {t('User')}: {error.username || `#${error.user_id}`}
                </span>
                <span>
                  {t('adminOps.recentErrors.model')}: {error.model_name || '—'}
                </span>
                <span>
                  {t('adminOps.recentErrors.channel')}: {error.channel_id || '—'}
                </span>
              </div>
              <p className='text-muted-foreground mt-2 line-clamp-3 text-sm'>
                {error.content}
              </p>
            </article>
          ))}
        </div>
      ) : (
        <AdminOpsEmpty>{t('adminOps.recentErrors.empty')}</AdminOpsEmpty>
      )}
    </AdminOpsPanel>
  )
}

function formatTimestamp(seconds: number): string {
  const date = new Date(seconds * 1000)
  if (!Number.isFinite(date.getTime())) return '—'
  return date.toLocaleString()
}
