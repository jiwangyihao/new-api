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
import { useMemo } from 'react'
import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import {
  formatConcurrencyLimit,
  formatPlanPrice,
  formatDuration,
  formatResetPeriod,
  formatTokenLimit,
} from '../lib'
import type { PlanRecord } from '../types'
import { DataTableRowActions } from './data-table-row-actions'

export function useSubscriptionsColumns(): ColumnDef<PlanRecord>[] {
  const { t } = useTranslation()

  return useMemo(
    (): ColumnDef<PlanRecord>[] => [
      {
        accessorFn: (row) => row.plan.id,
        id: 'id',
        meta: { label: 'ID', mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title='ID' />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>#{row.original.plan.id}</span>
        ),
        size: 60,
      },
      {
        accessorFn: (row) => row.plan.title,
        id: 'title',
        meta: { label: t('Plan'), mobileTitle: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Plan')} />
        ),
        cell: ({ row }) => {
          const plan = row.original.plan
          return (
            <div className='max-w-[200px]'>
              <div className='truncate font-medium'>{plan.title}</div>
              {plan.subtitle && (
                <div className='text-muted-foreground truncate text-xs'>
                  {plan.subtitle}
                </div>
              )}
            </div>
          )
        },
        size: 200,
      },
      {
        accessorFn: (row) => row.plan.price_amount,
        id: 'price',
        meta: { label: t('Price') },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Price')} />
        ),
        cell: ({ row }) => (
          <span className='font-semibold text-emerald-600'>
            {formatPlanPrice(
              row.original.plan.price_amount,
              row.original.plan.currency
            )}
          </span>
        ),
        size: 100,
      },
      {
        id: 'duration',
        meta: { label: t('Validity') },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Validity')} />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {formatDuration(row.original.plan, t)}
          </span>
        ),
        size: 100,
      },
      {
        id: 'reset',
        meta: { label: t('Quota Reset'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Quota Reset')} />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {formatResetPeriod(row.original.plan, t)}
          </span>
        ),
        size: 80,
      },
      {
        accessorFn: (row) => row.plan.sort_order,
        id: 'sort_order',
        meta: { label: t('Priority'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Priority')} />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {row.original.plan.sort_order}
          </span>
        ),
        size: 80,
      },
      {
        accessorFn: (row) => row.plan.enabled,
        id: 'enabled',
        meta: { label: t('Status'), mobileBadge: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Status')} />
        ),
        cell: ({ row }) =>
          row.original.plan.enabled ? (
            <StatusBadge
              label={t('Enable')}
              variant='success'
              copyable={false}
            />
          ) : (
            <StatusBadge
              label={t('Disable')}
              variant='neutral'
              copyable={false}
            />
          ),
        size: 80,
      },
      {
        id: 'payment',
        meta: { label: t('Payment Channel'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Payment Channel')} />
        ),
        cell: ({ row }) => {
          const plan = row.original.plan
          return (
            <div className='flex gap-1'>
              {plan.stripe_price_id && (
                <StatusBadge
                  label='Stripe'
                  variant='neutral'
                  copyable={false}
                />
              )}
              {plan.creem_product_id && (
                <StatusBadge label='Creem' variant='neutral' copyable={false} />
              )}
            </div>
          )
        },
        size: 140,
      },
      {
        id: 'monthly_token_limit',
        meta: { label: t('Monthly Token Limit'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('Monthly Token Limit')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {formatTokenLimit(row.original.plan.monthly_token_limit, t)}
          </span>
        ),
        size: 140,
      },
      {
        id: 'concurrency_limit',
        meta: { label: t('Concurrency Limit'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('Concurrency Limit')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {formatConcurrencyLimit(row.original.plan.concurrency_limit, t)}
          </span>
        ),
        size: 140,
      },
      {
        id: 'business_code',
        meta: { label: t('Business Code'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Business Code')} />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {row.original.plan.business_code || '-'}
          </span>
        ),
        size: 120,
      },
      {
        id: 'plan_flags',
        meta: { label: t('Plan Flags'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Plan Flags')} />
        ),
        cell: ({ row }) => {
          const plan = row.original.plan
          return (
            <div className='flex flex-wrap gap-1'>
              {plan.is_trial && (
                <StatusBadge
                  label={t('Trial Plan')}
                  variant='info'
                  copyable={false}
                />
              )}
              {plan.public_visible === false && (
                <StatusBadge
                  label={t('Hidden')}
                  variant='neutral'
                  copyable={false}
                />
              )}
              {plan.reward_eligible === false && (
                <StatusBadge
                  label={t('No Invitation Reward')}
                  variant='neutral'
                  copyable={false}
                />
              )}
              {plan.trial_duration_hours ? (
                <StatusBadge
                  label={t('{{count}} trial hours', {
                    count: plan.trial_duration_hours,
                  })}
                  variant='neutral'
                  copyable={false}
                />
              ) : null}
            </div>
          )
        },
        size: 180,
      },
      {
        id: 'actions',
        cell: ({ row }) => <DataTableRowActions row={row} />,
        size: 80,
      },
    ],
    [t]
  )
}
