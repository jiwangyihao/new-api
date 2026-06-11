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
import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Checkbox } from '@/components/ui/checkbox'
import { Progress } from '@/components/ui/progress'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { getApiKeyStatusConfig, formatApiKeyTokenCount } from '../constants'
import { API_KEY_CODEX_PRO_MODE_OPTIONS } from '../lib'
import { type ApiKey, type ApiKeyCodexProMode } from '../types'
import {
  ApiKeyCell,
  ModelLimitsCell,
  IpRestrictionsCell,
} from './api-keys-cells'
import { DataTableRowActions } from './data-table-row-actions'

function getTokenProgressColor(percentage: number): string {
  if (percentage >= 90) return '[&_[data-slot=progress-indicator]]:bg-rose-500'
  if (percentage >= 70) return '[&_[data-slot=progress-indicator]]:bg-amber-500'
  return '[&_[data-slot=progress-indicator]]:bg-emerald-500'
}

function getApiKeyCodexProModeLabel(mode: ApiKeyCodexProMode): string {
  return (
    API_KEY_CODEX_PRO_MODE_OPTIONS.find((option) => option.value === mode)
      ?.labelKey ?? 'Inherit user setting'
  )
}

export function useApiKeysColumns(): ColumnDef<ApiKey>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label='Select all'
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label='Select row'
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
      meta: { label: t('Select') },
    },
    {
      accessorKey: 'name',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Name')} />
      ),
      cell: ({ row }) => (
        <div className='max-w-[200px] truncate font-medium'>
          {row.getValue('name')}
        </div>
      ),
      meta: { label: t('Name'), mobileTitle: true },
    },
    {
      accessorKey: 'status',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        const statusConfig = getApiKeyStatusConfig(row.original)
        if (!statusConfig) return null
        return (
          <StatusBadge
            label={t(statusConfig.label)}
            variant={statusConfig.variant}
            showDot={statusConfig.showDot}
            copyable={false}
          />
        )
      },
      filterFn: (row, id, value) => value.includes(String(row.getValue(id))),
      meta: { label: t('Status'), mobileBadge: true },
    },
    {
      id: 'key',
      accessorKey: 'key',
      header: t('API Key'),
      cell: ({ row }) => <ApiKeyCell apiKey={row.original} />,
      enableSorting: false,
      meta: { label: t('API Key') },
    },
    {
      id: 'token_limit',
      accessorKey: 'token_limit',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Token limit')} />
      ),
      cell: ({ row }) => {
        const apiKey = row.original
        if (!apiKey.token_limit_enabled || apiKey.token_unlimited) {
          return (
            <StatusBadge
              label={t('Unlimited')}
              variant='neutral'
              copyable={false}
            />
          )
        }

        const used = apiKey.token_used
        const limit = apiKey.token_limit
        const remaining = apiKey.token_remaining
        const percentage = limit > 0 ? Math.min((used / limit) * 100, 100) : 0

        return (
          <Tooltip>
            <TooltipTrigger render={<div className='w-[150px] space-y-1' />}>
              <div className='flex justify-between text-xs'>
                <span className='font-medium tabular-nums'>
                  {formatApiKeyTokenCount(used, t('tokens'))}
                </span>
                <span className='text-muted-foreground tabular-nums'>
                  {formatApiKeyTokenCount(limit, t('tokens'))}
                </span>
              </div>
              <Progress
                value={percentage}
                className={cn('h-1.5', getTokenProgressColor(percentage))}
              />
            </TooltipTrigger>
            <TooltipContent>
              <div className='space-y-1 text-xs'>
                <div>
                  {t('Used:')} {formatApiKeyTokenCount(used, t('tokens'))}
                </div>
                <div>
                  {t('Remaining:')}{' '}
                  {formatApiKeyTokenCount(remaining, t('tokens'))} (
                  {Math.max(100 - percentage, 0).toFixed(1)}%)
                </div>
                <div>
                  {t('Total:')} {formatApiKeyTokenCount(limit, t('tokens'))}
                </div>
              </div>
            </TooltipContent>
          </Tooltip>
        )
      },
      meta: { label: t('Token limit') },
    },
    {
      id: 'codex_pro_mode',
      accessorKey: 'codex_pro_mode',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Codex Pro')} />
      ),
      cell: ({ row }) => {
        const mode = row.original.codex_pro_mode
        if (!mode || mode === 'inherit') {
          return (
            <StatusBadge
              label={t('Inherit user setting')}
              variant='neutral'
              showDot={false}
              copyable={false}
            />
          )
        }

        return (
          <StatusBadge
            label={t(getApiKeyCodexProModeLabel(mode))}
            variant={mode === 'off' ? 'neutral' : 'purple'}
            copyable={false}
          />
        )
      },
      meta: { label: t('Codex Pro'), mobileHidden: true },
    },
    {
      id: 'model_limits',
      accessorKey: 'model_limits',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Models')} />
      ),
      cell: ({ row }) => <ModelLimitsCell apiKey={row.original} />,
      enableSorting: false,
      meta: { label: t('Models'), mobileHidden: true },
    },
    {
      id: 'allow_ips',
      accessorKey: 'allow_ips',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('IP Restriction')} />
      ),
      cell: ({ row }) => <IpRestrictionsCell apiKey={row.original} />,
      enableSorting: false,
      meta: { label: t('IP Restriction'), mobileHidden: true },
    },
    {
      accessorKey: 'created_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Created')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground font-mono text-xs tabular-nums'>
          {formatTimestampToDate(row.getValue('created_time'))}
        </span>
      ),
      meta: { label: t('Created'), mobileHidden: true },
    },
    {
      accessorKey: 'accessed_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Last Used')} />
      ),
      cell: ({ row }) => {
        const accessedTime = row.getValue('accessed_time') as number
        if (!accessedTime) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }
        return (
          <span className='text-muted-foreground font-mono text-xs tabular-nums'>
            {formatTimestampToDate(accessedTime)}
          </span>
        )
      },
      meta: { label: t('Last Used'), mobileHidden: true },
    },
    {
      accessorKey: 'expired_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Expires')} />
      ),
      cell: ({ row }) => {
        const expiredTime = row.getValue('expired_time') as number
        if (expiredTime === -1) {
          return (
            <StatusBadge
              label={t('Never')}
              variant='neutral'
              copyable={false}
            />
          )
        }
        const isExpired = expiredTime * 1000 < Date.now()
        return (
          <span
            className={cn(
              'font-mono text-xs tabular-nums',
              isExpired ? 'text-destructive' : 'text-muted-foreground'
            )}
          >
            {formatTimestampToDate(expiredTime)}
          </span>
        )
      },
      meta: { label: t('Expires'), mobileHidden: true },
    },
    {
      id: 'actions',
      cell: ({ row }) => <DataTableRowActions row={row} />,
      meta: { label: t('Actions') },
      size: 88,
    },
  ]
}
