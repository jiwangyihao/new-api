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
import {
  formatAdminOpsCount,
  formatAdminOpsDuration,
  formatAdminOpsPercent,
  formatAdminOpsRate,
} from '../lib/format'
import { AdminOpsCardSkeleton, AdminOpsMetric, AdminOpsPanel } from './shared'

export function RealtimeTrafficCard(props: {
  snapshot?: AdminOpsSnapshot
  loading: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) return <AdminOpsCardSkeleton />

  const traffic = props.snapshot?.traffic
  return (
    <AdminOpsPanel title={t('adminOps.traffic.title')}>
      <div className='grid grid-cols-2 gap-3'>
        <AdminOpsMetric
          label={t('adminOps.traffic.requests')}
          value={formatAdminOpsCount(traffic?.requests ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.traffic.errors')}
          value={formatAdminOpsCount(traffic?.errors ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.traffic.rpm')}
          value={formatAdminOpsRate(traffic?.rpm ?? 0, t('RPM'))}
        />
        <AdminOpsMetric
          label={t('adminOps.traffic.tpm')}
          value={formatAdminOpsRate(traffic?.tpm ?? 0, t('TPM'))}
        />
        <AdminOpsMetric
          label={t('adminOps.traffic.errorRate')}
          value={formatAdminOpsPercent(traffic?.error_rate ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.traffic.window')}
          value={formatAdminOpsDuration(traffic?.window_seconds ?? 0)}
        />
      </div>
    </AdminOpsPanel>
  )
}
