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
import { useCallback, useMemo, useRef, useState } from 'react'
import type { TFunction } from 'i18next'
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
import { formatAdminMoneyAmount } from '@/features/admin-analytics/lib/format'
import {
  adjustUserCreditBalance,
  getAdminCreditBalanceLedger,
  getSubscriptionOrderRecoveryPreview,
  previewUserCreditBalanceAdjustment,
  recoverSubscriptionOrder,
} from '../api'
import type {
  CreditBalanceAdjustmentOperation,
  CreditBalanceAdjustmentPreviewResult,
  CreditBalanceLedgerFilters,
  PlanRecord,
  SubscriptionOrderRecoveryPreview,
} from '../types'
import {
  reconcileCreditAdjustmentRetry,
  type CreditAdjustmentFacts,
  type CreditAdjustmentRetryState,
} from './admin-credit-balance-retry'
import { CreditBalanceLedger } from './credit-balance-ledger'

interface AdminCreditBalancePanelProps {
  userId: number
  plans?: readonly PlanRecord[]
  onSuccess?: () => void
}

function newIdempotencyKey(userId: number): string {
  const random =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `admin-credit-${userId}-${random}`
}

function isEligibleAfterSalesPlan(record: PlanRecord): boolean {
  const plan = record.plan
  const priceMicros = plan.price_amount_micros?.trim()
  return Boolean(
    plan.enabled &&
    (plan.entitlement_type ?? 'timed') === 'timed' &&
    !plan.is_trial &&
    !plan.invite_trial &&
    priceMicros &&
    /^\d+$/.test(priceMicros) &&
    BigInt(priceMicros) > 0n &&
    Number.isSafeInteger(plan.monthly_token_limit) &&
    Number(plan.monthly_token_limit) > 0 &&
    plan.unlimited_purchase_enabled === true &&
    plan.currency?.trim()
  )
}

const maximumAdjustmentAmount = 1_000_000_000_000n

function isValidAdjustmentAmount(value: string): boolean {
  return /^[1-9]\d*$/.test(value) && BigInt(value) <= maximumAdjustmentAmount
}

export function creditBalanceAdjustmentErrorKey(
  code: string | undefined,
  operation: CreditBalanceAdjustmentOperation
): string {
  if (operation === 'decrease') {
    switch (code) {
      case 'credit_valuation_plan_required':
      case 'credit_valuation_plan_ineligible':
        return 'Credit decrease must not include a plan.'
      case 'credit_valuation_idempotency_mismatch':
        return 'This Credit decrease no longer matches its retry key. Change a fact and try again.'
      case 'credit_valuation_unsupported_currency':
        return 'The valuation currency is not supported for this Credit decrease.'
      case 'credit_valuation_invalid_fx':
        return 'The frozen FX snapshot is unavailable or invalid.'
      case 'credit_valuation_overflow':
        return 'The Credit amount is too large to value safely.'
      case 'credit_valuation_state_missing':
      case 'credit_valuation_state_mismatch':
        return 'The Credit valuation state changed. Refresh and try again.'
      case 'credit_valuation_migration_not_ready':
        return 'Credit operational valuation is not ready yet.'
      default:
        return 'The Credit decrease could not be completed safely.'
    }
  }

  switch (code) {
    case 'credit_valuation_plan_required':
      return 'Select an eligible after-sales grant plan.'
    case 'credit_valuation_plan_ineligible':
      return 'The selected plan is not eligible for an after-sales grant.'
    case 'credit_valuation_unsupported_currency':
      return 'The selected plan currency is not supported for operational valuation.'
    case 'credit_valuation_invalid_fx':
      return 'The frozen FX snapshot is unavailable or invalid.'
    case 'credit_valuation_overflow':
      return 'The Credit amount is too large to value safely.'
    case 'credit_valuation_state_missing':
    case 'credit_valuation_state_mismatch':
      return 'The Credit valuation state changed. Refresh and try again.'
    case 'credit_valuation_idempotency_mismatch':
      return 'This after-sales grant no longer matches its retry key. Change a fact and try again.'
    case 'credit_valuation_migration_not_ready':
      return 'Credit operational valuation is not ready yet.'
    default:
      return 'The after-sales grant could not be completed safely.'
  }
}
export function subscriptionOrderRecoveryErrorKey(
  code: string | undefined
): string {
  switch (code) {
    case 'subscription_order_recovery_invalid':
      return 'Enter a valid order, financial terminal, and reason.'
    case 'subscription_order_not_found':
      return 'The subscription order was not found for this user.'
    case 'subscription_order_status_invalid':
      return 'The subscription order is not eligible for financial recovery.'
    case 'subscription_order_snapshot_mismatch':
      return 'The frozen subscription order facts do not match.'
    case 'subscription_order_payment_provider_mismatch':
      return 'The payment provider does not match the subscription order.'
    case 'subscription_order_provider_identity_ambiguous':
      return 'The provider identity matches more than one subscription order.'
    case 'subscription_order_credit_recovery_not_applicable':
      return 'The subscription order has no Credit to recover.'
    case 'subscription_order_recovery_conflict':
    case 'credit_valuation_idempotency_mismatch':
      return 'This financial terminal no longer matches the committed recovery facts.'
    case 'credit_valuation_state_missing':
    case 'credit_valuation_state_mismatch':
      return 'The Credit valuation state changed. Refresh and try again.'
    case 'credit_valuation_migration_not_ready':
      return 'Credit operational valuation is not ready yet.'
    default:
      return 'The financial terminal could not be completed safely.'
  }
}

function formatMicrosCount(value: string): string {
  if (!/^-?\d+$/.test(value)) return value
  return new Intl.NumberFormat().format(BigInt(value))
}

interface CreditOutflowResultFacts {
  consumed_available_credit: number
  debt_formed: number
  removed_exact_cost_micros: string
  removed_estimated_cost_micros: string
  removed_unknown_credit: number
  valuation_currency: string
  rule_version: number
  state_version_after: number
  terminal_state: string
}

function formatCreditOutflowResult(
  t: TFunction,
  introduction: string,
  facts: CreditOutflowResultFacts
): string {
  return [
    introduction,
    `${t('Consumed available Credit')}: ${facts.consumed_available_credit}.`,
    `${t('Settlement debt formed')}: ${facts.debt_formed}.`,
    `${t('Exact value removed')}: ${formatMicrosCount(facts.removed_exact_cost_micros)} ${t('micros')}.`,
    `${t('Estimated value removed')}: ${formatMicrosCount(facts.removed_estimated_cost_micros)} ${t('micros')}.`,
    `${t('Unknown Credit removed')}: ${facts.removed_unknown_credit}.`,
    `${t('Valuation currency')}: ${facts.valuation_currency || '-'}.`,
    `${t('Rule and state version')}: ${facts.rule_version}/${facts.state_version_after}.`,
    `${t('Terminal state')}: ${t(facts.terminal_state || 'Unknown')}.`,
  ].join(' ')
}
export function AdminCreditBalancePanel({
  userId,
  plans = [],
  onSuccess,
}: AdminCreditBalancePanelProps) {
  const { t } = useTranslation()
  const [operation, setOperation] =
    useState<CreditBalanceAdjustmentOperation>('increase')
  const [selectedPlanId, setSelectedPlanId] = useState('')
  const eligiblePlans = useMemo(
    () => plans.filter(isEligibleAfterSalesPlan),
    [plans]
  )
  const [amount, setAmount] = useState('')
  const [adjustmentReason, setAdjustmentReason] = useState('')
  const [adjusting, setAdjusting] = useState(false)
  const [previewingAdjustment, setPreviewingAdjustment] = useState(false)
  const [adjustmentPreview, setAdjustmentPreview] =
    useState<CreditBalanceAdjustmentPreviewResult | null>(null)
  const adjustmentFactsVersion = useRef(0)
  const adjustmentRetry = useRef<CreditAdjustmentRetryState | null>(null)
  if (adjustmentRetry.current?.facts.userId !== userId) {
    adjustmentRetry.current = null
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

  const selectedPlan = useMemo(
    () =>
      eligiblePlans.find(({ plan }) => String(plan.id) === selectedPlanId)
        ?.plan ?? null,
    [eligiblePlans, selectedPlanId]
  )

  const getAdjustmentFacts = (
    overrides: Partial<CreditAdjustmentFacts> = {}
  ): CreditAdjustmentFacts => ({
    userId,
    operation,
    amount,
    planId: selectedPlanId,
    reason: adjustmentReason.trim(),
    ...overrides,
  })

  const invalidateAdjustmentAttempt = (
    facts: CreditAdjustmentFacts,
    event: 'retry' | 'success' = 'retry'
  ) => {
    adjustmentFactsVersion.current += 1
    adjustmentRetry.current = reconcileCreditAdjustmentRetry(
      adjustmentRetry.current,
      facts,
      event,
      () => newIdempotencyKey(userId)
    )
    setAdjustmentPreview(null)
    setPreviewingAdjustment(false)
  }

  const changeOperation = (value: string | null) => {
    if (value === null || value === operation) return
    const nextOperation = value as CreditBalanceAdjustmentOperation
    setOperation(nextOperation)
    setSelectedPlanId('')
    invalidateAdjustmentAttempt(
      getAdjustmentFacts({ operation: nextOperation, planId: '' })
    )
  }

  const changePlan = (value: string | null) => {
    if (value === null || value === selectedPlanId) return
    setSelectedPlanId(value)
    invalidateAdjustmentAttempt(getAdjustmentFacts({ planId: value }))
  }

  const changeAmount = (value: string) => {
    setAmount(value)
    invalidateAdjustmentAttempt(getAdjustmentFacts({ amount: value }))
  }

  const changeAdjustmentReason = (value: string) => {
    setAdjustmentReason(value)
    invalidateAdjustmentAttempt(
      getAdjustmentFacts({ reason: value.trim() })
    )
  }

  const previewAdjustment = async () => {
    if (
      operation !== 'increase' ||
      !selectedPlan ||
      !isValidAdjustmentAmount(amount)
    ) {
      toast.error(t('Select a plan and enter a positive whole Credit amount.'))
      return
    }
    const factsVersion = adjustmentFactsVersion.current
    setPreviewingAdjustment(true)
    setAdjustmentPreview(null)
    try {
      const response = await previewUserCreditBalanceAdjustment(userId, {
        operation,
        amount,
        plan_id: selectedPlan.id,
      })
      if (factsVersion !== adjustmentFactsVersion.current) return
      if (!response.success || !response.data) {
        toast.error(
          t(creditBalanceAdjustmentErrorKey(response.code, 'increase'))
        )
        return
      }
      setAdjustmentPreview(response.data)
    } catch {
      if (factsVersion === adjustmentFactsVersion.current) {
        toast.error(t('Request failed'))
      }
    } finally {
      if (factsVersion === adjustmentFactsVersion.current) {
        setPreviewingAdjustment(false)
      }
    }
  }

  const submitAdjustment = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (
      !isValidAdjustmentAmount(amount) ||
      !adjustmentReason.trim() ||
      (operation === 'increase' && !selectedPlan)
    ) {
      toast.error(
        t('Select a plan and enter a positive whole Credit amount and reason.')
      )
      return
    }
    const retryState = reconcileCreditAdjustmentRetry(
      adjustmentRetry.current,
      getAdjustmentFacts(),
      'retry',
      () => newIdempotencyKey(userId)
    )
    adjustmentRetry.current = retryState
    setAdjusting(true)
    try {
      const response = await adjustUserCreditBalance(userId, {
        operation,
        amount,
        ...(operation === 'increase' && selectedPlan
          ? { plan_id: selectedPlan.id }
          : {}),
        idempotency_key: retryState.idempotencyKey,
        reason: adjustmentReason.trim(),
      })
      if (!response.success || !response.data) {
        toast.error(
          t(creditBalanceAdjustmentErrorKey(response.code, operation))
        )
        return
      }
      const balance = response.data.credit_balance
      let message: string
      if (operation === 'decrease') {
        let decreaseMessage: string
        if (response.data.replayed) {
          decreaseMessage = t(
            'Credit decrease replayed without another withdrawal.'
          )
        } else {
          decreaseMessage = t('Credit decrease committed.')
        }
        message = formatCreditOutflowResult(t, decreaseMessage, response.data)
      } else if (response.data.replayed) {
        message = t(
          'The after-sales grant was safely replayed without adding Credit again.'
        )
      } else {
        message = t(
          'After-sales grant completed. Available: {{available}}, debt offset: {{debtOffset}}.',
          {
            available: balance.available_credit,
            debtOffset: response.data.debt_offset,
          }
        )
      }
      setStatusMessage(message)
      toast.success(message)
      setAmount('')
      setAdjustmentReason('')
      invalidateAdjustmentAttempt(
        getAdjustmentFacts({ amount: '', reason: '' }),
        'success'
      )
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
        toast.error(t(subscriptionOrderRecoveryErrorKey(response.code)))
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
        toast.error(t(subscriptionOrderRecoveryErrorKey(response.code)))
        return
      }
      const message = formatCreditOutflowResult(
        t,
        response.data.replayed
          ? t('Financial terminal replayed without another Credit withdrawal.')
          : t('Order marked {{status}} and {{credit}} Credit withdrawn.', {
              status: t(response.data.status),
              credit: response.data.gross_credit,
            }),
        response.data
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
              <Select value={operation} onValueChange={changeOperation}>
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
            {operation === 'increase' ? (
              <Field>
                <FieldLabel>{t('After-sales grant plan')}</FieldLabel>
                <Select value={selectedPlanId} onValueChange={changePlan}>
                  <SelectTrigger aria-label={t('After-sales grant plan')}>
                    <SelectValue placeholder={t('Select a plan')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {eligiblePlans.map(({ plan }) => (
                        <SelectItem key={plan.id} value={String(plan.id)}>
                          {plan.title || `#${plan.id}`}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                {eligiblePlans.length === 0 ? (
                  <FieldDescription>
                    {t('No eligible after-sales grant plans are available.')}
                  </FieldDescription>
                ) : null}
              </Field>
            ) : null}
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
                onChange={(event) => changeAmount(event.target.value)}
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
                onChange={(event) => changeAdjustmentReason(event.target.value)}
                required
              />
            </Field>
            {operation === 'increase' && selectedPlan ? (
              <Alert className='sm:col-span-2'>
                <AlertTitle>{t('Selected after-sales grant plan')}</AlertTitle>
                <AlertDescription className='grid grid-cols-1 gap-1 sm:grid-cols-2'>
                  <span>
                    {t('Plan price')}:{' '}
                    {formatAdminMoneyAmount({
                      amount: selectedPlan.price_amount,
                      amount_micros:
                        selectedPlan.price_amount_micros ?? undefined,
                      currency: selectedPlan.currency,
                    })}
                  </span>
                  <span>
                    {t('Plan Credit')}: {selectedPlan.monthly_token_limit}
                  </span>
                  <span>
                    {t('Source currency')}: {selectedPlan.currency}
                  </span>
                </AlertDescription>
              </Alert>
            ) : null}
            {adjustmentPreview ? (
              <Alert className='sm:col-span-2'>
                <AlertTitle>
                  {t('Authoritative operational value preview')}
                </AlertTitle>
                <AlertDescription className='grid grid-cols-1 gap-1 sm:grid-cols-2'>
                  <span>
                    {t('Gross Credit')}: {adjustmentPreview.gross_credit}
                  </span>
                  <span>
                    {t('Net Credit')}: {adjustmentPreview.net_credit}
                  </span>
                  <span>
                    {t('Gross operational value')}:{' '}
                    {formatAdminMoneyAmount({
                      amount: 0,
                      amount_micros: adjustmentPreview.gross_amount_micros,
                      currency: adjustmentPreview.valuation_currency,
                    })}{' '}
                    ({formatMicrosCount(adjustmentPreview.gross_amount_micros)}{' '}
                    {t('micros')})
                  </span>
                  <span>
                    {t('Net operational value')}:{' '}
                    {formatAdminMoneyAmount({
                      amount: 0,
                      amount_micros: adjustmentPreview.net_amount_micros,
                      currency: adjustmentPreview.valuation_currency,
                    })}{' '}
                    ({formatMicrosCount(adjustmentPreview.net_amount_micros)}{' '}
                    {t('micros')})
                  </span>
                  <span>
                    {t('Debt offset')}: {adjustmentPreview.debt_offset}
                  </span>
                  <span>
                    {t('Source currency')}: {adjustmentPreview.source_currency}
                  </span>
                  <span>
                    {t('Valuation currency')}:{' '}
                    {adjustmentPreview.valuation_currency}
                  </span>
                  <span>
                    {t('FX snapshot')}: {adjustmentPreview.fx_rate_numerator}/
                    {adjustmentPreview.fx_rate_denominator}{' '}
                    {adjustmentPreview.fx_direction || '-'} @{' '}
                    {adjustmentPreview.fx_captured_at || '-'}
                  </span>
                  <span>
                    {t('Valuation confidence')}: {adjustmentPreview.confidence}
                  </span>
                  <span>
                    {t('Rule and state version')}:{' '}
                    {adjustmentPreview.rule_version}/
                    {adjustmentPreview.state_version_after}
                  </span>
                </AlertDescription>
              </Alert>
            ) : null}
            <Field className='sm:col-span-2' orientation='horizontal'>
              {operation === 'increase' ? (
                <Button
                  type='button'
                  variant='outline'
                  onClick={previewAdjustment}
                  disabled={
                    previewingAdjustment ||
                    !selectedPlan ||
                    !isValidAdjustmentAmount(amount)
                  }
                >
                  {previewingAdjustment
                    ? t('Loading...')
                    : t('Preview operational value')}
                </Button>
              ) : null}
              <Button
                type='submit'
                disabled={
                  adjusting ||
                  !isValidAdjustmentAmount(amount) ||
                  !adjustmentReason.trim() ||
                  (operation === 'increase' && !selectedPlan)
                }
              >
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
