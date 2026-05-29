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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { AdminOpsConcurrencyResponse } from '../types'
import {
  formatAdminOpsCount,
  formatAdminOpsDuration,
  formatAdminOpsPercent,
} from '../lib/format'
import { getAdminOpsConcurrencyUserStatus } from '../lib/health'
import {
  AdminOpsCardSkeleton,
  AdminOpsEmpty,
  AdminOpsMetric,
  AdminOpsPanel,
} from './shared'

export function ConcurrencyQueueCard(props: {
  concurrency?: AdminOpsConcurrencyResponse
  loading: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) return <AdminOpsCardSkeleton className='h-80 w-full' />

  const data = props.concurrency
  const users = data?.users ?? []

  return (
    <AdminOpsPanel title={t('adminOps.concurrency.title')}>
      <div className='space-y-4'>
        <div className='grid grid-cols-2 gap-3 lg:grid-cols-4'>
          <AdminOpsMetric
            label={t('adminOps.concurrency.activeSlots')}
            value={formatAdminOpsCount(data?.summary.total_active ?? 0)}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.queuedRequests')}
            value={formatAdminOpsCount(data?.summary.total_queued ?? 0)}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.activeUsers')}
            value={formatAdminOpsCount(data?.summary.active_users ?? 0)}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.queuedUsers')}
            value={formatAdminOpsCount(data?.summary.queued_users ?? 0)}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.saturatedUsers')}
            value={formatAdminOpsCount(data?.summary.saturated_users ?? 0)}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.queuePressure')}
            value={formatAdminOpsPercent(data?.summary.queue_pressure ?? 0)}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.acquiredTotal')}
            value={formatAdminOpsCount(data?.counters.acquired_total ?? 0)}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.queuedTotal')}
            value={formatAdminOpsCount(data?.counters.queued_total ?? 0)}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.queueFullRejections')}
            value={formatAdminOpsCount(
              data?.counters.queue_full_rejections_total ?? 0
            )}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.unavailableRejections')}
            value={formatAdminOpsCount(
              data?.counters.unavailable_rejections_total ?? 0
            )}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.redisErrors')}
            value={formatAdminOpsCount(data?.counters.redis_errors_total ?? 0)}
          />
          <AdminOpsMetric
            label={t('adminOps.concurrency.defaultQueueCapacity')}
            value={formatAdminOpsCount(data?.config.default_queue_capacity ?? 0)}
          />
        </div>
        <div className='text-muted-foreground flex flex-wrap gap-3 text-xs'>
          <span>
            {t('adminOps.concurrency.mode')}:{' '}
            {t(`adminOps.mode.${data?.mode ?? 'disabled'}`)}
          </span>
          <span>
            {t('adminOps.concurrency.ttl')}:{' '}
            {formatAdminOpsDuration(data?.config.ttl_seconds ?? 0)}
          </span>
          <span>
            {t('adminOps.concurrency.requireRedis')}:{' '}
            {data?.config.require_redis ? t('Yes') : t('No')}
          </span>
          <span>
            {t('adminOps.concurrency.failOpen')}:{' '}
            {data?.config.fail_open ? t('Yes') : t('No')}
          </span>
        </div>
        {users.length > 0 ? (
          <div className='overflow-x-auto'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('User')}</TableHead>
                  <TableHead className='text-right'>
                    {t('adminOps.concurrency.active')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('adminOps.concurrency.queued')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('adminOps.concurrency.utilization')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('adminOps.concurrency.queueUtilization')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('adminOps.concurrency.oldestQueued')}
                  </TableHead>
                  <TableHead>{t('Status')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((user) => {
                  const status = getAdminOpsConcurrencyUserStatus(user)
                  const userLabel = user.username || `#${user.user_id}`
                  return (
                    <TableRow key={user.user_id}>
                      <TableCell>{userLabel}</TableCell>
                      <TableCell className='text-right'>
                        {formatAdminOpsCount(user.active)}/
                        {user.limit > 0 ? formatAdminOpsCount(user.limit) : t('Unlimited')}
                      </TableCell>
                      <TableCell className='text-right'>
                        {formatAdminOpsCount(user.queued)}/
                        {formatAdminOpsCount(user.queue_capacity)}
                      </TableCell>
                      <TableCell className='text-right'>
                        {formatAdminOpsPercent(user.utilization)}
                      </TableCell>
                      <TableCell className='text-right'>
                        {formatAdminOpsPercent(user.queue_utilization)}
                      </TableCell>
                      <TableCell className='text-right'>
                        {formatAdminOpsDuration(user.oldest_queued_seconds)}
                      </TableCell>
                      <TableCell>
                        {t(`adminOps.concurrency.status.${status}`)}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        ) : (
          <AdminOpsEmpty>{t('adminOps.empty.noData')}</AdminOpsEmpty>
        )}
      </div>
    </AdminOpsPanel>
  )
}
