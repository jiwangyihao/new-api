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
import { formatAdminOpsCount } from '../lib/format'
import { AdminOpsCardSkeleton, AdminOpsMetric, AdminOpsPanel } from './shared'

export function ChannelHealthCard(props: {
  snapshot?: AdminOpsSnapshot
  loading: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) return <AdminOpsCardSkeleton />

  const channels = props.snapshot?.channels
  return (
    <AdminOpsPanel title={t('adminOps.channels.title')}>
      <div className='grid grid-cols-2 gap-3'>
        <AdminOpsMetric
          label={t('adminOps.channels.total')}
          value={formatAdminOpsCount(channels?.total ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.channels.enabled')}
          value={formatAdminOpsCount(channels?.enabled ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.channels.manualDisabled')}
          value={formatAdminOpsCount(channels?.manual_disabled ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.channels.autoDisabled')}
          value={formatAdminOpsCount(channels?.auto_disabled ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.channels.slow')}
          value={formatAdminOpsCount(channels?.slow_count ?? 0)}
        />
        <AdminOpsMetric
          label={t('adminOps.channels.staleTest')}
          value={formatAdminOpsCount(channels?.stale_test_count ?? 0)}
        />
      </div>
    </AdminOpsPanel>
  )
}
