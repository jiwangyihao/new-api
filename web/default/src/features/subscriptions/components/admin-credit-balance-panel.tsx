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
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  adjustUserCreditBalance,
  getAdminCreditBalanceLedger,
  getSubscriptionOrderRecoveryPreview,
  recoverSubscriptionOrder,
} from '../api'
import type {
  CreditBalanceAdjustmentOperation,
  CreditBalanceLedgerFilters,
  SubscriptionOrderRecoveryPreview,
} from '../types'
import { CreditBalanceLedger } from './credit-balance-ledger'

interface AdminCreditBalancePanelProps {
  userId: number
  onSuccess?: () => void
}

function newIdempotencyKey(userId: number): string {
  const random =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `admin-credit-${userId}-${random}`
}

export function AdminCreditBalancePanel({
  userId,
  onSuccess,
}: AdminCreditBalancePanelProps) {
  const { t } = useTranslation()
  const [operation, setOperation] =
    useState<CreditBalanceAdjustmentOperation>('increase')
  const [amount, setAmount] = useState('')
  const [adjustmentReason, setAdjustmentReason] = useState('')
  const [adjusting, setAdjusting] = useState(false)
  const adjustmentIdempotency = useRef({
    userId,
    key: newIdempotencyKey(userId),
  })
  if (adjustmentIdempotency.current.userId !== userId) {
    adjustmentIdempotency.current = {
      userId,
      key: newIdempotencyKey(userId),
    }
  }
  const [tradeNo, setTradeNo] = useState('')
  const [recoveryType, setRecoveryType] = useState<'refund' | 'chargeback'>(
    'refund'
  )
  const [recoveryReason, setRecoveryReason] = useState('')
  const [recovering, setRecovering] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [recoveryPreview, setRecoveryPreview] =
    useState<SubscriptionOrderRecoveryPreview | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)
  const [statusMessage, setStatusMessage] = useState('')

  const loadLedger = useCallback(
    (filters: CreditBalanceLedgerFilters) =>
      getAdminCreditBalanceLedger(userId, filters),
    [userId]
  )

  const submitAdjustment = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const numericAmount = Number(amount)
    if (
      !Number.isSafeInteger(numericAmount) ||
      numericAmount <= 0 ||
      numericAmount > 1_000_000_000_000 ||
      !adjustmentReason.trim()
    ) {
      toast.error(t('Enter a valid non-zero Credit amount and reason.'))
      return
    }
    setAdjusting(true)
    try {
      const response = await adjustUserCreditBalance(userId, {
        operation,
        amount: numericAmount,
        idempotency_key: adjustmentIdempotency.current.key,
        reason: adjustmentReason.trim(),
      })
      if (!response.success || !response.data) {
        toast.error(response.message || t('Request failed'))
        return
      }
      const balance = response.data.credit_balance
      const message = t(
        'Credit adjustment completed. Available: {{available}}, debt: {{debt}}.',
        {
          available: balance.available_credit,
          debt: balance.settlement_debt,
        }
      )
      setStatusMessage(message)
      toast.success(message)
      setAmount('')
      setAdjustmentReason('')
      adjustmentIdempotency.current.key = newIdempotencyKey(userId)
      setRefreshKey((value) => value + 1)
      onSuccess?.()
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setAdjusting(false)
    }
  }

  const previewRecoveryOrder = async () => {
    if (!tradeNo.trim()) {
      toast.error(t('Enter the order number.'))
      return
    }
    setPreviewing(true)
    setRecoveryPreview(null)
    try {
      const response = await getSubscriptionOrderRecoveryPreview(
        userId,
        tradeNo.trim()
      )
      if (!response.success || !response.data) {
        toast.error(response.message || t('Request failed'))
        return
      }
      setRecoveryPreview(response.data)
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setPreviewing(false)
    }
  }

  const submitRecovery = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (
      !tradeNo.trim() ||
      !recoveryReason.trim() ||
      !recoveryPreview ||
      recoveryPreview.trade_no !== tradeNo.trim() ||
      recoveryPreview.user_id !== userId
    ) {
      toast.error(t('Preview and verify this user order before confirming.'))
      return
    }
    setRecovering(true)
    try {
      const response = await recoverSubscriptionOrder(userId, tradeNo.trim(), {
        recovery_type: recoveryType,
        reason: recoveryReason.trim(),
      })
      if (!response.success || !response.data) {
        toast.error(response.message || t('Request failed'))
        return
      }
      const message = response.data.replayed
        ? t('Financial terminal replayed without another Credit withdrawal.')
        : t(
            'Order marked {{status}} and {{credit}} Credit withdrawn. Settlement debt: {{debt}}.',
            {
              status: t(response.data.status),
              credit: response.data.gross_credit,
              debt: response.data.settlement_debt,
            }
          )
      setStatusMessage(message)
      toast.success(message)
      setTradeNo('')
      setRecoveryReason('')
      setRecoveryPreview(null)
      setRefreshKey((value) => value + 1)
      onSuccess?.()
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setRecovering(false)
    }
  }

  return (
    <section
      className='flex flex-col gap-4'
      aria-labelledby='credit-finance-title'
    >
      <div>
        <h3 id='credit-finance-title' className='font-medium'>
          {t('Credit financial management')}
        </h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Use structured adjustments and verified order terminals. Direct cumulative-field edits are not supported.'
          )}
        </p>
      </div>

      <FieldSet className='rounded-md border p-3'>
        <FieldLegend variant='label'>{t('Adjust Credit balance')}</FieldLegend>
        <FieldDescription>
          {t('A decrease beyond available Credit creates settlement debt.')}
        </FieldDescription>
        <form onSubmit={submitAdjustment}>
          <FieldGroup className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
            <Field>
              <FieldLabel>{t('Operation')}</FieldLabel>
              <Select
                value={operation}
                onValueChange={(value) =>
                  value !== null &&
                  setOperation(value as CreditBalanceAdjustmentOperation)
                }
              >
                <SelectTrigger aria-label={t('Credit adjustment operation')}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value='increase'>
                      {t('Increase Credit')}
                    </SelectItem>
                    <SelectItem value='decrease'>
                      {t('Decrease Credit')}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel htmlFor='admin-credit-amount'>
                {t('Credit amount')}
              </FieldLabel>
              <Input
                id='admin-credit-amount'
                type='number'
                min={1}
                max={1_000_000_000_000}
                step={1}
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
                required
              />
            </Field>
            <Field className='sm:col-span-2'>
              <FieldLabel htmlFor='admin-credit-reason'>
                {t('Reason')}
              </FieldLabel>
              <Textarea
                id='admin-credit-reason'
                value={adjustmentReason}
                maxLength={255}
                onChange={(event) => setAdjustmentReason(event.target.value)}
                required
              />
            </Field>
            <Field className='sm:col-span-2'>
              <Button type='submit' disabled={adjusting}>
                {adjusting ? t('Saving...') : t('Submit Credit adjustment')}
              </Button>
            </Field>
          </FieldGroup>
        </form>
      </FieldSet>

      <FieldSet className='rounded-md border p-3'>
        <FieldLegend variant='label'>
          {t('Order financial terminal')}
        </FieldLegend>
        <FieldDescription>
          {t(
            'Use this authenticated fallback only after verifying a refund or chargeback that the payment provider cannot report reliably.'
          )}
        </FieldDescription>
        <form onSubmit={submitRecovery}>
          <FieldGroup className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
            <Field>
              <FieldLabel htmlFor='recovery-trade-no'>
                {t('Order number')}
              </FieldLabel>
              <InputGroup>
                <InputGroupInput
                  id='recovery-trade-no'
                  value={tradeNo}
                  onChange={(event) => {
                    setTradeNo(event.target.value)
                    setRecoveryPreview(null)
                  }}
                  required
                />
                <InputGroupAddon align='inline-end'>
                  <InputGroupButton
                    type='button'
                    variant='outline'
                    onClick={previewRecoveryOrder}
                    disabled={previewing || !tradeNo.trim()}
                  >
                    {previewing ? t('Loading...') : t('Preview order')}
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
            </Field>
            <Field>
              <FieldLabel>{t('Financial terminal')}</FieldLabel>
              <Select
                value={recoveryType}
                onValueChange={(value) =>
                  value !== null &&
                  setRecoveryType(value as 'refund' | 'chargeback')
                }
              >
                <SelectTrigger aria-label={t('Financial terminal')}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value='refund'>{t('Refund')}</SelectItem>
                    <SelectItem value='chargeback'>
                      {t('Chargeback')}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            {recoveryPreview ? (
              <Alert className='sm:col-span-2'>
                <AlertTitle>
                  {t('Verify order ownership and amount')}
                </AlertTitle>
                <AlertDescription className='grid grid-cols-1 gap-1 sm:grid-cols-2'>
                  <span>
                    {t('User')}: {recoveryPreview.username || '-'} (ID:{' '}
                    {recoveryPreview.user_id})
                  </span>
                  <span>
                    {t('Plan')}: {recoveryPreview.plan_title || '#'}
                    {recoveryPreview.plan_id}
                  </span>
                  <span>
                    {t('Paid amount')}: {recoveryPreview.amount_cents / 100}{' '}
                    {recoveryPreview.currency}
                  </span>
                  <span>
                    {t('Gross Credit withdrawal')}:{' '}
                    {recoveryPreview.gross_credit}
                  </span>
                  <span>
                    {t('Payment provider')}: {recoveryPreview.payment_provider}
                  </span>
                  <span>
                    {t('Status')}: {t(recoveryPreview.status)}
                  </span>
                </AlertDescription>
              </Alert>
            ) : null}
            <Field className='sm:col-span-2'>
              <FieldLabel htmlFor='recovery-reason'>{t('Reason')}</FieldLabel>
              <Textarea
                id='recovery-reason'
                value={recoveryReason}
                maxLength={255}
                onChange={(event) => setRecoveryReason(event.target.value)}
                required
              />
            </Field>
            <Field className='sm:col-span-2'>
              <Button
                type='submit'
                variant='destructive'
                disabled={recovering || !recoveryPreview}
              >
                {recovering ? t('Saving...') : t('Confirm financial terminal')}
              </Button>
            </Field>
          </FieldGroup>
        </form>
      </FieldSet>

      {statusMessage ? (
        <Alert>
          <AlertTitle>{t('Operation result')}</AlertTitle>
          <AlertDescription>{statusMessage}</AlertDescription>
        </Alert>
      ) : null}
      <p className='sr-only' role='status' aria-live='polite'>
        {statusMessage}
      </p>

      <CreditBalanceLedger loadEntries={loadLedger} refreshKey={refreshKey} />
    </section>
  )
}
