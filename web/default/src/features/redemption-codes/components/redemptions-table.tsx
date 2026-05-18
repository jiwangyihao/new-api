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
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type ExpandedState,
  type SortingState,
  type VisibilityState,
  flexRender,
  getCoreRowModel,
  getExpandedRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { TableCell, TableRow } from '@/components/ui/table'
import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
} from '@/components/data-table'
import { getRedemptions, searchRedemptions } from '../api'
import {
  REDEMPTION_STATUS,
  getRedemptionStatusOptions,
  getRedemptionTypeOptions,
} from '../constants'
import {
  aggregateRedemptionsByBatch,
  isRedemptionBatchRow,
  isRedemptionExpired,
  type RedemptionBatchRow,
} from '../lib'
import { type GetRedemptionsParams } from '../types'
import { DataTableBulkActions } from './data-table-bulk-actions'
import { useRedemptionsColumns } from './redemptions-columns'
import { useRedemptions } from './redemptions-provider'

const route = getRouteApi('/_authenticated/redemption-codes/')

function isDisabledRedemptionRow(redemption: RedemptionBatchRow) {
  return (
    !isRedemptionBatchRow(redemption) &&
    (redemption.status !== REDEMPTION_STATUS.ENABLED ||
      isRedemptionExpired(redemption.expired_time, redemption.status))
  )
}

function getDisabledRowClassName(
  redemption: RedemptionBatchRow,
  isMobile: boolean
): string | undefined {
  if (!isDisabledRedemptionRow(redemption)) {
    return undefined
  }

  return isMobile ? DISABLED_ROW_MOBILE : DISABLED_ROW_DESKTOP
}

export function RedemptionsTable() {
  const { t } = useTranslation()
  const columns = useRedemptionsColumns()
  const { refreshTrigger } = useRedemptions()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [rowSelection, setRowSelection] = useState({})
  const [sorting, setSorting] = useState<SortingState>([])
  const [expanded, setExpanded] = useState<ExpandedState>({})
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'type', searchKey: 'type', type: 'array' },
    ],
  })

  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) || []
  const typeFilter =
    (columnFilters.find((filter) => filter.id === 'type')?.value as
      | string[]
      | undefined) || []

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'redemptions',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      statusFilter[0] || '',
      typeFilter[0] || '',
      refreshTrigger,
    ],
    queryFn: async () => {
      const selectedType =
        typeFilter[0] === 'wallet' || typeFilter[0] === 'subscription'
          ? typeFilter[0]
          : undefined
      const params: GetRedemptionsParams = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        status: statusFilter[0] ? Number(statusFilter[0]) : undefined,
        type: selectedType,
      }
      const keyword = globalFilter?.trim()
      const result = keyword
        ? await searchRedemptions({ ...params, keyword })
        : await getRedemptions(params)

      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const redemptions = useMemo(
    () => aggregateRedemptionsByBatch(data?.items || []),
    [data]
  )

  const table = useReactTable({
    data: redemptions,
    columns,
    state: {
      sorting,
      columnVisibility,
      rowSelection,
      columnFilters,
      globalFilter,
      pagination,
      expanded,
    },
    enableRowSelection: (row) => !isRedemptionBatchRow(row.original),
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onExpandedChange: setExpanded,
    getSubRows: (row) => row.children,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getExpandedRowModel: getExpandedRowModel(),
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  const redemptionStatusOptions = useMemo(
    () => getRedemptionStatusOptions(t),
    [t]
  )
  const redemptionTypeOptions = useMemo(() => getRedemptionTypeOptions(t), [t])

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Redemption Codes Found')}
      emptyDescription={t(
        'No redemption codes available. Create your first redemption code to get started.'
      )}
      skeletonKeyPrefix='redemptions-skeleton'
      toolbarProps={{
        searchPlaceholder: t('Filter by name, ID, code, or batch...'),
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: redemptionStatusOptions,
            singleSelect: true,
          },
          {
            columnId: 'type',
            title: t('Type'),
            options: redemptionTypeOptions,
            singleSelect: true,
          },
        ],
      }}
      getRowClassName={(row, { isMobile }) =>
        getDisabledRowClassName(row.original, isMobile)
      }
      renderRow={(row) => (
        <TableRow
          key={row.id}
          aria-expanded={row.getIsExpanded() || undefined}
          className={cn(
            row.getIsSelected() && 'bg-muted',
            row.depth > 0 && 'bg-muted/30',
            getDisabledRowClassName(row.original, false)
          )}
        >
          {row.getVisibleCells().map((cell) => (
            <TableCell key={cell.id}>
              {flexRender(cell.column.columnDef.cell, cell.getContext())}
            </TableCell>
          ))}
        </TableRow>
      )}
      bulkActions={<DataTableBulkActions table={table} />}
    />
  )
}
