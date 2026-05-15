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
import { DataTableColumnHeader } from '@/components/data-table'
import { MaskedValueDisplay } from '@/components/masked-value-display'
import { StatusBadge } from '@/components/status-badge'
import type { TrialCode } from '../types'
import { TrialCodeRowActions } from './trial-code-row-actions'

function maskTrialCode(code: string): string {
  if (code.length <= 8) return code
  return `${code.slice(0, 4)}${'*'.repeat(Math.min(12, code.length - 8))}${code.slice(-4)}`
}

function isExpired(expiresAt: number): boolean {
  return expiresAt > 0 && expiresAt <= Math.floor(Date.now() / 1000)
}

export function useTrialCodesColumns(): ColumnDef<TrialCode>[] {
  const { t } = useTranslation()
  return [
    {
      accessorKey: 'id',
      meta: { label: 'ID', mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title='ID' />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground'>#{row.original.id}</span>
      ),
      size: 60,
    },
    {
      accessorKey: 'code',
      meta: { label: t('Trial code'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Trial code')} />
      ),
      cell: ({ row }) => {
        const code = row.original.code
        return (
          <MaskedValueDisplay
            label={t('Full Code')}
            fullValue={code}
            maskedValue={maskTrialCode(code)}
            copyTooltip={t('Copy code')}
            copyAriaLabel={t('Copy trial code')}
          />
        )
      },
      enableSorting: false,
      size: 180,
    },
    {
      accessorKey: 'plan_id',
      meta: { label: t('Trial plan ID') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Trial plan ID')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground'>{row.original.plan_id}</span>
      ),
      size: 100,
    },
    {
      accessorKey: 'enabled',
      meta: { label: t('Status'), mobileBadge: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        if (isExpired(row.original.expires_at)) {
          return (
            <StatusBadge
              label={t('Expired')}
              variant='warning'
              copyable={false}
            />
          )
        }
        return row.original.enabled ? (
          <StatusBadge label={t('Enable')} variant='success' copyable={false} />
        ) : (
          <StatusBadge label={t('Disable')} variant='neutral' copyable={false} />
        )
      },
      size: 100,
    },
    {
      accessorKey: 'max_redemptions',
      meta: { label: t('Max Redemptions') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Max Redemptions')} />
      ),
      cell: ({ row }) => {
        const maxRedemptions = row.original.max_redemptions
        return (
          <span className='text-muted-foreground'>
            {maxRedemptions > 0 ? maxRedemptions : t('Unlimited')}
          </span>
        )
      },
      size: 130,
    },
    {
      accessorKey: 'redeemed_count',
      meta: { label: t('Redeemed Count') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Redeemed Count')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground'>
          {row.original.redeemed_count}
        </span>
      ),
      size: 130,
    },
    {
      accessorKey: 'expires_at',
      meta: { label: t('Expires'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Expires')} />
      ),
      cell: ({ row }) => {
        const expiresAt = row.original.expires_at
        if (expiresAt <= 0) {
          return <StatusBadge label={t('Never')} variant='neutral' copyable={false} />
        }
        return (
          <span
            className={`min-w-[140px] font-mono text-sm ${isExpired(expiresAt) ? 'text-destructive' : ''}`}
          >
            {formatTimestampToDate(expiresAt)}
          </span>
        )
      },
      size: 150,
    },
    {
      id: 'actions',
      meta: { label: t('Actions') },
      cell: ({ row }) => <TrialCodeRowActions row={row} />,
      enableSorting: false,
      enableHiding: false,
      size: 50,
    },
  ]
}
