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
import { Badge } from '@/components/ui/badge'
import { formatAdminOpsCount, formatAdminOpsPercent } from '../lib/format'
import { getAdminOpsHealthReasonLabelKey } from '../lib/health'
import type {
  AdminOpsDependency,
  AdminOpsDependencyStatus,
  AdminOpsSnapshot,
} from '../types'
import { AdminOpsCardSkeleton, AdminOpsMetric, AdminOpsPanel } from './shared'

export function HealthSummaryCards(props: {
  snapshot?: AdminOpsSnapshot
  loading: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) return <AdminOpsCardSkeleton className='h-56 w-full' />

  const snapshot = props.snapshot
  const database = snapshot?.dependencies.database
  const redis = snapshot?.dependencies.redis
  const reasonDescription = (snapshot?.health.reasons ?? [])
    .slice(0, 2)
    .map((reason) => t(getAdminOpsHealthReasonLabelKey(reason)))
    .join(', ')
  return (
    <AdminOpsPanel title={t('adminOps.healthSummary.title')}>
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
        <DependencyMetric
          label={t('adminOps.healthSummary.database')}
          dependency={database}
        />
        <DependencyMetric
          label={t('adminOps.healthSummary.redis')}
          dependency={redis}
        />
        <AdminOpsMetric
          label={t('adminOps.healthSummary.cpu')}
          value={formatAdminOpsPercent((snapshot?.system.cpu_usage ?? 0) / 100)}
        />
        <AdminOpsMetric
          label={t('adminOps.healthSummary.memory')}
          value={formatAdminOpsPercent(
            (snapshot?.system.memory_usage ?? 0) / 100
          )}
        />
        <AdminOpsMetric
          label={t('adminOps.healthSummary.disk')}
          value={formatAdminOpsPercent(
            (snapshot?.system.disk_usage ?? 0) / 100
          )}
        />
        <AdminOpsMetric
          label={t('adminOps.healthSummary.activeConnections')}
          value={formatAdminOpsCount(snapshot?.runtime.active_connections ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.healthSummary.goroutines')}
          value={formatAdminOpsCount(snapshot?.runtime.goroutines ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.health.reasons')}
          value={formatAdminOpsCount(snapshot?.health.reasons.length ?? 0)}
          description={reasonDescription}
        />
      </div>
    </AdminOpsPanel>
  )
}

function DependencyMetric(props: {
  label: string
  dependency?: AdminOpsDependency
}) {
  const { t } = useTranslation()
  const status: AdminOpsDependencyStatus =
    props.dependency?.status ?? 'disabled'
  const enabledLabel = props.dependency?.enabled ? t('Enabled') : t('Disabled')
  return (
    <AdminOpsMetric
      label={props.label}
      value={
        <span className='flex flex-wrap items-center gap-2'>
          <Badge variant={status === 'critical' ? 'destructive' : 'secondary'}>
            {t(`adminOps.dependency.${status}`)}
          </Badge>
          <span>
            {formatAdminOpsCount(props.dependency?.latency_ms ?? 0)} {t('ms')}
          </span>
        </span>
      }
      description={props.dependency?.message || enabledLabel}
    />
  )
}
