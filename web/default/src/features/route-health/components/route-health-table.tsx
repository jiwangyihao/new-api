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
import { useMemo, useState } from 'react'
import {
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  useReactTable,
  type ColumnDef,
  type FilterFn,
} from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { DataTablePage } from '@/components/data-table'
import type { RouteHealthReport, RouteHealthRow } from '../types'
import { HealthStateBadge, useHealthStateLabels } from './health-state-badge'
import { ResourceDetails } from './resource-details'

const emptyRows: RouteHealthRow[] = []
const globalFilter: FilterFn<RouteHealthRow> = (
  row,
  _columnId,
  value: string
) => {
  const search = value.trim().toLowerCase()
  const item = row.original
  return [
    item.channel_id,
    item.channel_name,
    ...item.models,
    item.upstream_model,
    item.operation,
  ].some((part) => String(part).toLowerCase().includes(search))
}

export function RouteHealthTable(props: {
  report: RouteHealthReport | undefined
  isLoading: boolean
  isFetching: boolean
}) {
  const { t } = useTranslation()
  const labels = useHealthStateLabels()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const rows = props.report?.rows ?? emptyRows
  const selectedRow = rows.find((row) => row.id === selectedId)
  const columns = useMemo<ColumnDef<RouteHealthRow>[]>(
    () => [
      {
        accessorKey: 'channel_name',
        header: t('Channel'),
        meta: { mobileTitle: true },
        cell: ({ row }) => (
          <div className='break-words whitespace-normal'>
            <div className='font-medium'>{row.original.channel_name}</div>
            <div className='text-muted-foreground text-xs'>
              #{row.original.channel_id}
            </div>
          </div>
        ),
      },
      {
        id: 'models',
        accessorFn: (row) => row.upstream_model,
        header: t('Models'),
        cell: ({ row }) => (
          <div className='space-y-1 break-all whitespace-normal sm:max-w-64'>
            <div>
              {t('Client model aliases')}:{' '}
              {row.original.models.join(', ') || '-'}
            </div>
            <div className='text-muted-foreground text-xs'>
              {t('Upstream model')}: {row.original.upstream_model || '-'}
            </div>
          </div>
        ),
      },
      {
        accessorKey: 'operation',
        header: t('Operation'),
        cell: ({ row }) => (
          <span className='break-all whitespace-normal'>
            {row.original.operation || '-'}
          </span>
        ),
      },
      {
        accessorKey: 'state',
        header: t('Effective state'),
        filterFn: (row, id, values: string[]) =>
          !values?.length || values.includes(row.getValue(id)),
        cell: ({ row }) => (
          <div className='space-y-1'>
            <HealthStateBadge state={row.original.state} />
            {row.original.resources.some(
              (resource) =>
                resource.scope !== 'route' &&
                !['healthy', 'unobserved', 'protection_disabled'].includes(
                  resource.state
                )
            ) && (
              <p className='text-muted-foreground max-w-48 text-xs whitespace-normal'>
                {t('Shared channel or credential restrictions; see details.')}
              </p>
            )}
          </div>
        ),
      },
      {
        id: 'route_window',
        header: t('Route window'),
        cell: ({ row }) => {
          const resource = row.original.resources.find(
            (item) => item.scope === 'route'
          )
          return (
            <dl className='space-y-1 text-xs whitespace-normal tabular-nums'>
              <div>
                <dt className='inline'>{t('Successes')}: </dt>
                <dd className='inline'>{formatNumber(resource?.successes)}</dd>
              </div>
              <div>
                <dt className='inline'>{t('Failures')}: </dt>
                <dd className='inline'>{formatNumber(resource?.failures)}</dd>
              </div>
              <div>
                <dt className='inline'>{t('Consecutive failures')}: </dt>
                <dd className='inline'>
                  {formatNumber(resource?.consecutive_failures)}
                </dd>
              </div>
              <div>
                <dt className='inline'>{t('In flight')}: </dt>
                <dd className='inline'>{formatNumber(resource?.inflight)}</dd>
              </div>
            </dl>
          )
        },
      },
      {
        id: 'route_timing',
        header: t('Route timing'),
        cell: ({ row }) => {
          const resource = row.original.resources.find(
            (item) => item.scope === 'route'
          )
          return (
            <dl className='space-y-1 text-xs whitespace-normal tabular-nums'>
              <div>
                <dt>{t('Cooling until')}</dt>
                <dd>{formatTimestampToDate(resource?.opened_until)}</dd>
              </div>
              <div>
                <dt>{t('Next probe')}</dt>
                <dd>{formatTimestampToDate(resource?.next_probe)}</dd>
              </div>
            </dl>
          )
        },
      },
      {
        id: 'credentials',
        header: t('Ready / total credentials'),
        cell: ({ row }) => (
          <span className='tabular-nums'>
            {formatNumber(row.original.ready_credentials)} /{' '}
            {formatNumber(row.original.total_credentials)}
          </span>
        ),
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <Button
            variant='outline'
            size='sm'
            onClick={() => setSelectedId(row.original.id)}
          >
            {t('Details')}
          </Button>
        ),
      },
    ],
    [t]
  )
  const table = useReactTable({
    data: rows,
    columns,
    getRowId: (row) => row.id,
    globalFilterFn: globalFilter,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
    getPaginationRowModel: getPaginationRowModel(),
    initialState: { pagination: { pageIndex: 0, pageSize: 10 } },
  })
  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={props.isLoading}
        isFetching={props.isFetching}
        toolbarProps={{
          searchPlaceholder: t('Search channels, models, or operations...'),
          filters: [
            {
              columnId: 'state',
              title: t('Effective state'),
              options: Object.entries(labels).map(([value, label]) => ({
                value,
                label,
              })),
            },
          ],
          hideViewOptions: true,
        }}
        emptyTitle={t('No route health rows')}
        emptyDescription={t(
          'No configured channel and model routes match the current filters.'
        )}
        mobileProps={{ getRowKey: (row) => row.original.id }}
        afterTable={
          <p className='text-muted-foreground text-xs'>
            {t(
              'Table counters and timing are route-scope only. Channel and credential scopes are shown separately in details.'
            )}
          </p>
        }
      />
      <ResourceDetails
        row={selectedRow}
        generatedAt={props.report?.generated_at}
        onClose={() => setSelectedId(null)}
      />
    </>
  )
}
