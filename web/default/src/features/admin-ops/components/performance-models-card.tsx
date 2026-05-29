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
import type { AdminOpsSnapshot } from '../types'
import {
  formatAdminOpsCount,
  formatAdminOpsPercent,
  formatAdminOpsRate,
} from '../lib/format'
import { AdminOpsCardSkeleton, AdminOpsEmpty, AdminOpsPanel } from './shared'

export function PerformanceModelsCard(props: {
  snapshot?: AdminOpsSnapshot
  loading: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) return <AdminOpsCardSkeleton className='h-64 w-full' />

  const models = props.snapshot?.performance.models ?? []
  return (
    <AdminOpsPanel title={t('adminOps.performance.title')}>
      {models.length > 0 ? (
        <div className='overflow-x-auto'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('adminOps.performance.model')}</TableHead>
                <TableHead className='text-right'>
                  {t('adminOps.performance.avgLatency')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('adminOps.performance.avgTtft')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('adminOps.performance.successRate')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('adminOps.performance.avgTps')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('adminOps.performance.requestCount')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {models.map((model) => (
                <TableRow key={model.model_name}>
                  <TableCell className='max-w-[220px] truncate font-medium'>
                    {model.model_name}
                  </TableCell>
                  <TableCell className='text-right'>
                    {formatAdminOpsCount(model.avg_latency_ms)} {t('ms')}
                  </TableCell>
                  <TableCell className='text-right'>
                    {formatAdminOpsCount(model.avg_ttft_ms)} {t('ms')}
                  </TableCell>
                  <TableCell className='text-right'>
                    {formatAdminOpsPercent(model.success_rate / 100)}
                  </TableCell>
                  <TableCell className='text-right'>
                    {formatAdminOpsRate(model.avg_tps, t('Throughput short'))}
                  </TableCell>
                  <TableCell className='text-right'>
                    {formatAdminOpsCount(model.request_count)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : (
        <AdminOpsEmpty>{t('adminOps.empty.noData')}</AdminOpsEmpty>
      )}
    </AdminOpsPanel>
  )
}
