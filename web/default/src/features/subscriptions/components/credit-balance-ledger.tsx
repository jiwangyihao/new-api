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
import { useCallback, useEffect, useState } from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type {
  ApiResponse,
  CreditBalanceLedgerEntry,
  CreditBalanceLedgerFilters,
} from '../types'

type LedgerLoader = (
  filters: CreditBalanceLedgerFilters
) => Promise<ApiResponse<CreditBalanceLedgerEntry[]>>

interface CreditBalanceLedgerProps {
  loadEntries: LedgerLoader
  initialEntries?: CreditBalanceLedgerEntry[]
  refreshKey?: number
}

const sourceOptions = [
  'subscription_order',
  'redemption',
  'subscription_conversion',
  'subscription_order_recovery',
  'admin_adjustment',
] as const

const typeOptions = [
  'purchase',
  'redemption',
  'subscription_conversion',
  'refund',
  'chargeback',
  'admin_increase',
  'admin_decrease',
] as const

const sourceLabelKeys: Record<string, string> = {
  subscription_order: 'Subscription order',
  redemption: 'Redemption',
  subscription_conversion: 'Subscription conversion',
  subscription_order_recovery: 'Order financial recovery',
  admin_adjustment: 'Admin adjustment',
}

const typeLabelKeys: Record<string, string> = {
  purchase: 'Purchase',
  redemption: 'Redemption',
  subscription_conversion: 'Subscription conversion',
  refund: 'Refund',
  chargeback: 'Chargeback',
  admin_increase: 'Admin Credit increase',
  admin_decrease: 'Admin Credit decrease',
}

export function creditBalanceLedgerSourceLabel(
  sourceType: string,
  t: TFunction
): string {
  return t(sourceLabelKeys[sourceType] || sourceType || 'Unknown')
}

export function creditBalanceLedgerTypeLabel(
  type: string,
  t: TFunction
): string {
  return t(typeLabelKeys[type] || type || 'Unknown')
}

export function formatCreditLedgerDelta(value: number): string {
  if (value > 0) return `+${value}`
  return String(value)
}

export function ledgerDateTimeToTimestamp(value: string): number | undefined {
  if (!value) return undefined
  const milliseconds = Date.parse(value)
  return Number.isFinite(milliseconds)
    ? Math.floor(milliseconds / 1000)
    : undefined
}

export function CreditBalanceLedger({
  loadEntries,
  initialEntries = [],
  refreshKey = 0,
}: CreditBalanceLedgerProps) {
  const { t } = useTranslation()
  const [entries, setEntries] = useState(initialEntries)
  const [loading, setLoading] = useState(false)
  const [sourceType, setSourceType] = useState('all')
  const [operationType, setOperationType] = useState('all')
  const [startTime, setStartTime] = useState('')
  const [endTime, setEndTime] = useState('')

  const load = useCallback(
    async (filters: CreditBalanceLedgerFilters) => {
      setLoading(true)
      try {
        const response = await loadEntries(filters)
        if (!response.success) {
          toast.error(response.message || t('Loading failed'))
          return
        }
        setEntries(response.data || [])
      } catch {
        toast.error(t('Loading failed'))
      } finally {
        setLoading(false)
      }
    },
    [loadEntries, t]
  )

  useEffect(() => {
    void load({})
  }, [load, refreshKey])

  const applyFilters = () => {
    void load({
      source_type: sourceType === 'all' ? undefined : sourceType,
      type: operationType === 'all' ? undefined : operationType,
      start_time: ledgerDateTimeToTimestamp(startTime),
      end_time: ledgerDateTimeToTimestamp(endTime),
    })
  }

  const clearFilters = () => {
    setSourceType('all')
    setOperationType('all')
    setStartTime('')
    setEndTime('')
    void load({})
  }

  return (
    <section
      className='flex flex-col gap-3'
      aria-labelledby='credit-ledger-title'
    >
      <div>
        <h3 id='credit-ledger-title' className='font-medium'>
          {t('Credit balance history')}
        </h3>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Filter purchases, redemptions, conversions, refunds, chargebacks, and admin adjustments.'
          )}
        </p>
      </div>
      <FieldGroup className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
        <Field>
          <FieldLabel>{t('Source')}</FieldLabel>
          <Select
            value={sourceType}
            onValueChange={(value) => value !== null && setSourceType(value)}
          >
            <SelectTrigger aria-label={t('Ledger source filter')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value='all'>{t('All sources')}</SelectItem>
                {sourceOptions.map((option) => (
                  <SelectItem key={option} value={option}>
                    {creditBalanceLedgerSourceLabel(option, t)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel>{t('Operation type')}</FieldLabel>
          <Select
            value={operationType}
            onValueChange={(value) => value !== null && setOperationType(value)}
          >
            <SelectTrigger aria-label={t('Ledger operation filter')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value='all'>{t('All operations')}</SelectItem>
                {typeOptions.map((option) => (
                  <SelectItem key={option} value={option}>
                    {creditBalanceLedgerTypeLabel(option, t)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor='credit-ledger-start'>
            {t('Start time')}
          </FieldLabel>
          <Input
            id='credit-ledger-start'
            type='datetime-local'
            value={startTime}
            onChange={(event) => setStartTime(event.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='credit-ledger-end'>{t('End time')}</FieldLabel>
          <Input
            id='credit-ledger-end'
            type='datetime-local'
            value={endTime}
            onChange={(event) => setEndTime(event.target.value)}
          />
        </Field>
      </FieldGroup>
      <div className='flex flex-wrap gap-2'>
        <Button
          type='button'
          size='sm'
          onClick={applyFilters}
          disabled={loading}
        >
          {t('Apply filters')}
        </Button>
        <Button
          type='button'
          size='sm'
          variant='outline'
          onClick={clearFilters}
          disabled={loading}
        >
          {t('Clear filters')}
        </Button>
      </div>
      <p className='sr-only' role='status' aria-live='polite'>
        {loading
          ? t('Loading...')
          : t('{{count}} ledger entries loaded', { count: entries.length })}
      </p>
      {entries.length === 0 && !loading ? (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyTitle>{t('No Credit balance history')}</EmptyTitle>
            <EmptyDescription>
              {t('No ledger entries match the current filters.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className='max-h-80 overflow-auto rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Operation')}</TableHead>
                <TableHead>{t('Credit change')}</TableHead>
                <TableHead>{t('Balance before / after')}</TableHead>
                <TableHead>{t('Debt change')}</TableHead>
                <TableHead>{t('Source and time')}</TableHead>
                <TableHead>{t('Operator and reason')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map((entry) => (
                <TableRow key={entry.id}>
                  <TableCell>
                    {creditBalanceLedgerTypeLabel(entry.type, t)}
                  </TableCell>
                  <TableCell className='font-medium tabular-nums'>
                    {formatCreditLedgerDelta(entry.gross_credit)}
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    {entry.balance_before} → {entry.balance_after}
                    <div className='text-muted-foreground text-xs'>
                      {t('Available')}: {entry.available_credit_after}
                    </div>
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    {entry.debt_offset > 0
                      ? `${t('Debt offset')} ${entry.debt_offset}`
                      : `${t('Debt formed')} ${entry.debt_formed || 0}`}
                    <div className='text-muted-foreground text-xs'>
                      {t('Settlement debt')}: {entry.settlement_debt_after}
                    </div>
                  </TableCell>
                  <TableCell>
                    {creditBalanceLedgerSourceLabel(entry.source_type, t)} #
                    {entry.source_id}
                    <div className='text-muted-foreground text-xs'>
                      {new Date(entry.created_at * 1000).toLocaleString()}
                    </div>
                    {entry.payment_provider ? (
                      <div className='text-muted-foreground text-xs'>
                        {entry.payment_provider}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    {(entry.operator_user_id || 0) > 0
                      ? `${t('Operator')} #${entry.operator_user_id}`
                      : t('System')}
                    <div className='text-muted-foreground max-w-48 text-xs whitespace-normal'>
                      {entry.reason || '-'}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  )
}
