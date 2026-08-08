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
import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowRightLeft, ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { TitledCard } from '@/components/ui/titled-card'
import {
  formatCompactCredit as formatCredit,
  formatDurationSeconds,
} from '@/features/subscriptions/lib'
import { getSubscriptionConversionErrorMessage } from '../errors'
import {
  useConfirmSubscriptionConversion,
  useSubscriptionConversionQuotes,
} from '../hooks/use-subscription-conversion-quotes'
import { deriveLiveConversionQuote } from '../live-quote'
import type {
  ConversionQuoteCategory,
  LiveSubscriptionConversionQuote,
  SubscriptionConversionConfirmRequest,
  SubscriptionConversionConfirmResult,
  SubscriptionConversionHistory,
  SubscriptionConversionQuote,
  SubscriptionConversionQuoteReason,
  SubscriptionConversionQuoteList,
} from '../types'

const MAX_DATE_SECONDS = 8_640_000_000_000n
const EMPTY_QUOTES: SubscriptionConversionQuote[] = []

const categorySections: Array<{
  category: ConversionQuoteCategory
  title: string
  description: string
}> = [
  {
    category: 'convertible',
    title: 'Convertible subscriptions',
    description:
      'Active subscriptions currently eligible for conversion preview.',
  },
  {
    category: 'expired_grace',
    title: 'Expired grace-period subscriptions',
    description:
      'Expired subscriptions still inside the rolling 336-hour grace period.',
  },
  {
    category: 'excluded',
    title: 'Excluded subscriptions',
    description:
      'Subscriptions shown with the exact reason conversion is unavailable.',
  },
]

function formatTimestamp(seconds: bigint): string {
  if (seconds < -MAX_DATE_SECONDS || seconds > MAX_DATE_SECONDS) {
    return `${seconds} UTC seconds`
  }
  const date = new Date(Number(seconds * 1000n))
  if (Number.isNaN(date.getTime())) {
    return `${seconds} UTC seconds`
  }
  return date.toLocaleString()
}

function reasonText(
  reason: SubscriptionConversionQuoteReason,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const remainingSeconds = reason.data?.remaining_seconds
  const duration = formatDurationSeconds(
    typeof remainingSeconds === 'string' ||
      typeof remainingSeconds === 'number' ||
      typeof remainingSeconds === 'bigint'
      ? remainingSeconds
      : '0',
    t
  )
  const source = reason.data?.source || ''
  switch (reason.code) {
    case 'global_conversion_disabled':
      return t('Timed subscription conversion is currently disabled')
    case 'entitlement_not_timed':
    case 'plan_not_timed':
      return t('This entitlement is not a timed subscription')
    case 'plan_not_found':
      return t('The source subscription plan is unavailable')
    case 'duration_not_one_month':
      return t('Only plans with an exact one-month duration can be converted')
    case 'reset_not_monthly':
      return t('Only plans with monthly Credit reset can be converted')
    case 'monthly_credit_not_positive':
      return t('The plan must grant a positive monthly Credit amount')
    case 'trial_plan':
    case 'trial_source':
      return t('Trial subscriptions cannot be converted')
    case 'monthly_invite_plan':
    case 'monthly_invite_source':
      return t('Monthly invitation subscriptions cannot be converted')
    case 'source_not_eligible':
      return t('The grant source {{source}} cannot be converted', { source })
    case 'plan_conversion_disabled':
      return t('Conversion is disabled for this plan')
    case 'status_not_eligible':
      return t('The subscription status is not eligible for conversion')
    case 'subscription_not_started':
      return t('The source subscription has not started yet')
    case 'outside_grace_period':
      return t('The 336-hour conversion grace period has ended')
    case 'grant_time_missing':
      return t('The latest grant time is unavailable')
    case 'cooldown_active':
      return t('Conversion cooldown is active ({{duration}} remaining)', {
        duration,
      })
    case 'gross_credit_not_positive':
      return t('The calculated gross Credit is not positive')
    case 'calculation_failed':
      return t('The quote could not be calculated safely')
    default:
      return t('Conversion is unavailable: {{reason}}', {
        reason: reason.code,
      })
  }
}

function formatConversionFormula(
  blocks: bigint | string,
  creditBasis: bigint | string,
  currentRemainingCredit: bigint | string,
  grossCredit: bigint | string
): string {
  return `${blocks} × ${formatCredit(creditBasis)} + ${formatCredit(currentRemainingCredit)} = ${formatCredit(grossCredit)}`
}

interface QuoteInstanceProps {
  quote: SubscriptionConversionQuote
  live: LiveSubscriptionConversionQuote | null
  onPreview: (sourceSubscriptionId: string) => void
  previewing: boolean
}

function QuoteInstance({
  quote,
  live,
  onPreview,
  previewing,
}: QuoteInstanceProps) {
  const { t } = useTranslation()
  if (!live) {
    return (
      <article className='border-destructive/40 rounded-lg border p-3'>
        <h4 className='font-medium'>
          {quote.plan_title || t('Unknown subscription plan')}
        </h4>
        <p className='text-destructive mt-2 text-sm' role='alert'>
          {t(
            'The quote contains an invalid integer and cannot be displayed safely.'
          )}
        </p>
      </article>
    )
  }

  const countdown =
    live.cooldownRemainingSeconds > 0n
      ? t('Conversion cooldown: {{duration}} remaining', {
          duration: formatDurationSeconds(live.cooldownRemainingSeconds, t),
        })
      : live.withinGrace
        ? t('Conversion grace period: {{duration}} remaining', {
            duration: formatDurationSeconds(live.graceRemainingSeconds, t),
          })
        : t('Time remaining: {{duration}}', {
            duration: formatDurationSeconds(live.remainingSeconds, t),
          })
  const reasons = liveQuoteReasons(quote, live)
  const excluded = live.category === 'excluded'

  return (
    <article
      className='bg-background rounded-lg border p-3'
      aria-labelledby={`conversion-quote-${quote.source_subscription_id}`}
    >
      <div className='flex flex-wrap items-start justify-between gap-2'>
        <div>
          <h4
            id={`conversion-quote-${quote.source_subscription_id}`}
            className='font-medium'
          >
            {quote.plan_title || t('Unknown subscription plan')}
          </h4>
          <p className='text-muted-foreground mt-0.5 text-xs'>
            {t('Subscription #{{id}}', {
              id: quote.source_subscription_id,
            })}
          </p>
        </div>
        <Badge variant={excluded ? 'outline' : 'secondary'}>
          {live.category === 'convertible'
            ? t('Convertible')
            : live.category === 'expired_grace'
              ? t('Expired grace period')
              : t('Excluded')}
        </Badge>
      </div>

      {excluded && reasons[0] && (
        <Alert variant='destructive' className='mt-3'>
          <AlertTitle>{t('Conversion is currently unavailable')}</AlertTitle>
          <AlertDescription>{reasonText(reasons[0], t)}</AlertDescription>
        </Alert>
      )}

      <dl className='mt-3 grid gap-2 text-xs sm:grid-cols-2'>
        <div>
          <dt className='text-muted-foreground'>{t('Ends')}</dt>
          <dd>{formatTimestamp(live.endTime)}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Remaining time')}</dt>
          <dd>{formatDurationSeconds(live.remainingSeconds, t)}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('Full future 31-day periods')}
          </dt>
          <dd>{live.full31DayBlocks.toString()}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('Unused Credit in current period')}
          </dt>
          <dd>{formatCredit(live.currentRemainingCredit)}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {excluded
              ? t('Potential Credit before debt if eligible')
              : t('Estimated Credit before debt')}
          </dt>
          <dd>{formatCredit(live.grossCredit)}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {excluded
              ? t('Potential available Credit if eligible')
              : t('Estimated available Credit')}
          </dt>
          <dd>{formatCredit(live.netAvailableCredit)}</dd>
        </div>
      </dl>

      <p
        className='text-muted-foreground mt-3 text-xs'
        role='timer'
        aria-live='off'
      >
        {countdown}
      </p>
      <div className='mt-2'>
        <div className='text-muted-foreground mb-1 text-xs'>
          {t(
            'Full future periods × monthly Credit + unused current-period Credit'
          )}
        </div>
        <code className='bg-muted block overflow-x-auto rounded px-2 py-1.5 text-xs'>
          {formatConversionFormula(
            live.full31DayBlocks,
            live.creditBasis,
            live.currentRemainingCredit,
            live.grossCredit
          )}
        </code>
      </div>

      {live.canConfirm && (
        <div className='mt-3 flex justify-end'>
          <Button
            type='button'
            size='sm'
            disabled={previewing}
            onClick={() => onPreview(quote.source_subscription_id)}
          >
            {t('Preview conversion')}
          </Button>
        </div>
      )}
    </article>
  )
}

function liveQuoteReasons(
  quote: SubscriptionConversionQuote,
  live: LiveSubscriptionConversionQuote
): SubscriptionConversionQuoteReason[] {
  const reasons = quote.reasons.flatMap((reason) => {
    if (reason.code !== 'cooldown_active') return [reason]
    if (live.cooldownRemainingSeconds === 0n) return []
    return [
      {
        ...reason,
        data: {
          ...reason.data,
          remaining_seconds: live.cooldownRemainingSeconds.toString(),
        },
      },
    ]
  })
  if (
    live.expired &&
    !live.withinGrace &&
    !reasons.some((reason) => reason.code === 'outside_grace_period')
  ) {
    reasons.push({ code: 'outside_grace_period' })
  }
  if (
    live.grossCredit <= 0n &&
    !reasons.some((reason) => reason.code === 'gross_credit_not_positive')
  ) {
    reasons.push({ code: 'gross_credit_not_positive' })
  }
  const priority = [
    'global_conversion_disabled',
    'entitlement_not_timed',
    'plan_not_timed',
    'plan_not_found',
    'duration_not_one_month',
    'reset_not_monthly',
    'monthly_credit_not_positive',
    'trial_plan',
    'trial_source',
    'monthly_invite_plan',
    'monthly_invite_source',
    'source_not_eligible',
    'plan_conversion_disabled',
    'status_not_eligible',
    'subscription_not_started',
    'outside_grace_period',
    'grant_time_missing',
    'cooldown_active',
    'gross_credit_not_positive',
    'calculation_failed',
  ]
  return reasons
    .sort(
      (left, right) =>
        priority.indexOf(left.code) - priority.indexOf(right.code)
    )
    .slice(0, 1)
}

function defaultConversionIdempotencyKey(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    return globalThis.crypto.randomUUID()
  }
  return `conversion-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function ConversionResultSummary({
  conversion,
}: {
  conversion: SubscriptionConversionHistory
}) {
  const { t } = useTranslation()
  const formula = formatConversionFormula(
    conversion.full_31_day_blocks,
    conversion.credit_basis,
    conversion.current_remaining_credit,
    conversion.gross_credit
  )
  return (
    <article
      className='bg-background rounded-lg border p-3'
      aria-label={t('Converted subscription #{{id}}', {
        id: conversion.source_subscription_id,
      })}
    >
      <h4 className='font-medium'>{conversion.source_plan_title}</h4>
      <p className='text-muted-foreground mt-0.5 text-xs'>
        {t('Subscription #{{id}}', {
          id: conversion.source_subscription_id,
        })}
      </p>
      <code className='bg-muted mt-2 block overflow-x-auto rounded px-2 py-1.5 text-xs'>
        {formula}
      </code>
      <dl className='mt-3 grid gap-2 text-xs sm:grid-cols-2'>
        <div>
          <dt className='text-muted-foreground'>{t('Gross Credit')}</dt>
          <dd>{formatCredit(BigInt(conversion.gross_credit))}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Debt offset')}</dt>
          <dd>{formatCredit(BigInt(conversion.debt_offset))}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Net available Credit')}</dt>
          <dd>{formatCredit(BigInt(conversion.net_available_credit))}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('Target Credit balance')}
          </dt>
          <dd>{formatCredit(BigInt(conversion.available_credit_after))}</dd>
        </div>
      </dl>
      {conversion.source_price_micros &&
        conversion.source_currency &&
        conversion.target_currency &&
        conversion.valuation_credit_basis &&
        conversion.gross_cost_micros !== undefined &&
        conversion.net_cost_micros !== undefined &&
        conversion.unit_value_numerator_micros &&
        conversion.unit_value_denominator &&
        conversion.rule_version !== undefined &&
        conversion.state_version_after &&
        conversion.fx_numerator &&
        conversion.fx_denominator &&
        conversion.fx_captured_at &&
        conversion.fx_direction && (
          <div className='mt-3 rounded-md border p-3'>
            <p className='font-medium'>
              {t('This is a rules-based valuation, not a new payment.')}
            </p>
            <dl className='mt-2 grid gap-2 text-xs sm:grid-cols-2'>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Source price micros')}
                </dt>
                <dd>{conversion.source_price_micros}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Source → target currency')}
                </dt>
                <dd>
                  {conversion.source_currency} → {conversion.target_currency}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Valuation Credit basis')}
                </dt>
                <dd>{conversion.valuation_credit_basis}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Gross cost micros')}
                </dt>
                <dd>{conversion.gross_cost_micros}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Net cost micros')}
                </dt>
                <dd>{conversion.net_cost_micros}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Unrounded unit value')}
                </dt>
                <dd>
                  {conversion.unit_value_numerator_micros} /{' '}
                  {conversion.unit_value_denominator}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Rule version')}</dt>
                <dd>
                  {t('Rule version')}: {conversion.rule_version}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('State version')}</dt>
                <dd>
                  {t('State version')}: {conversion.state_version_after}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Frozen FX')}</dt>
                <dd>
                  {conversion.fx_numerator} / {conversion.fx_denominator}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('FX direction')}</dt>
                <dd>{conversion.fx_direction}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('FX captured at')}</dt>
                <dd>{conversion.fx_captured_at}</dd>
              </div>
            </dl>
          </div>
        )}
    </article>
  )
}

function ConversionResultCard({
  title,
  conversion,
  role,
}: {
  title: string
  conversion: SubscriptionConversionHistory
  role?: 'status'
}) {
  const { t } = useTranslation()
  return (
    <Card role={role} aria-label={title}>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>
          {t('The server recalculated and committed the latest values.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <ConversionResultSummary conversion={conversion} />
      </CardContent>
    </Card>
  )
}

interface ConversionPreviewDialogProps {
  quote: SubscriptionConversionQuote | null
  elapsedSeconds: bigint
  open: boolean
  confirming: boolean
  confirmationError: string | null
  onConfirm: () => void
  onOpenChange: (open: boolean) => void
}

function ConversionPreviewDialog({
  quote,
  elapsedSeconds,
  open,
  confirming,
  confirmationError,
  onConfirm,
  onOpenChange,
}: ConversionPreviewDialogProps) {
  const { t } = useTranslation()
  const live = useMemo(() => {
    if (!quote) return null
    try {
      return deriveLiveConversionQuote(quote, elapsedSeconds)
    } catch {
      return null
    }
  }, [elapsedSeconds, quote])
  const reasons = quote && live ? liveQuoteReasons(quote, live) : []

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {t('Timed subscription conversion preview')}
          </DialogTitle>
          <DialogDescription>
            {t('Preview only — no conversion is submitted')}
          </DialogDescription>
        </DialogHeader>

        {quote && live && (
          <div className='space-y-4'>
            <div>
              <div className='font-medium'>{quote.plan_title}</div>
              <div className='text-muted-foreground text-xs'>
                {t('Subscription #{{id}}', {
                  id: quote.source_subscription_id,
                })}
              </div>
            </div>

            <dl className='grid gap-x-4 gap-y-2 text-sm sm:grid-cols-2'>
              <div>
                <dt className='text-muted-foreground'>{t('Database time')}</dt>
                <dd>{formatTimestamp(live.databaseNow)}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Expiration time')}
                </dt>
                <dd>{formatTimestamp(live.endTime)}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Remaining time')}</dt>
                <dd>{formatDurationSeconds(live.remainingSeconds, t)}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Full 31-day blocks')}
                </dt>
                <dd>{live.full31DayBlocks.toString()}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Credit basis')}</dt>
                <dd>{formatCredit(live.creditBasis)}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Credit basis source')}
                </dt>
                <dd>
                  {quote.credit_basis_source === 'grant_snapshot'
                    ? t('Latest grant snapshot')
                    : quote.credit_basis_source === 'current_plan_fallback'
                      ? t('Current plan fallback')
                      : t('Unavailable')}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Current remaining Credit')}
                </dt>
                <dd>{formatCredit(live.currentRemainingCredit)}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Gross Credit')}</dt>
                <dd>{formatCredit(live.grossCredit)}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Current settlement debt')}
                </dt>
                <dd>{formatCredit(live.currentDebt)}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('Estimated debt offset')}
                </dt>
                <dd>{formatCredit(live.estimatedDebtOffset)}</dd>
              </div>
              <div className='sm:col-span-2'>
                <dt className='text-muted-foreground'>
                  {t('Net available Credit')}
                </dt>
                <dd className='text-base font-semibold'>
                  {formatCredit(live.netAvailableCredit)}
                </dd>
              </div>
            </dl>

            <div>
              <div className='text-muted-foreground mb-1 text-xs'>
                {t('Conversion formula')}
              </div>
              <code className='bg-muted block overflow-x-auto rounded px-3 py-2 text-sm'>
                {formatConversionFormula(
                  live.full31DayBlocks,
                  live.creditBasis,
                  live.currentRemainingCredit,
                  live.grossCredit
                )}
              </code>
            </div>

            {quote.source_price_micros &&
              quote.source_currency &&
              quote.target_currency &&
              quote.valuation_credit_basis &&
              quote.gross_cost_micros !== undefined &&
              quote.net_cost_micros !== undefined &&
              quote.unit_value_numerator_micros &&
              quote.unit_value_denominator &&
              quote.rule_version !== undefined &&
              quote.fx_numerator &&
              quote.fx_denominator &&
              quote.fx_captured_at &&
              quote.fx_direction && (
                <Alert>
                  <AlertTitle>
                    {t('This is a rules-based valuation, not a new payment.')}
                  </AlertTitle>
                  <AlertDescription className='space-y-3'>
                    <dl className='grid gap-x-4 gap-y-2 text-xs sm:grid-cols-2'>
                      <div>
                        <dt className='text-muted-foreground'>
                          {t('Source price micros')}
                        </dt>
                        <dd>{quote.source_price_micros}</dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground'>
                          {t('Source → target currency')}
                        </dt>
                        <dd>
                          {quote.source_currency} → {quote.target_currency}
                        </dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground'>
                          {t('Gross cost micros')}
                        </dt>
                        <dd>{quote.gross_cost_micros}</dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground'>
                          {t('Net cost micros')}
                        </dt>
                        <dd>{quote.net_cost_micros}</dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground'>
                          {t('Unrounded unit value')}
                        </dt>
                        <dd>
                          {quote.unit_value_numerator_micros} /{' '}
                          {quote.unit_value_denominator}
                        </dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground'>
                          {t('Frozen FX')}
                        </dt>
                        <dd>
                          {quote.fx_numerator} / {quote.fx_denominator}
                        </dd>
                      </div>
                    </dl>
                    <dl className='grid gap-x-4 gap-y-2 text-xs sm:grid-cols-2'>
                      <div>
                        <dt className='text-muted-foreground'>
                          {t('Rule version')}
                        </dt>
                        <dd>{quote.rule_version}</dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground'>
                          {t('FX captured at')}
                        </dt>
                        <dd>{quote.fx_captured_at}</dd>
                      </div>
                    </dl>
                    <p>
                      {t(
                        'Confirmation freezes these valuation facts. Later plan-price, Credit-basis, or FX changes do not rewrite the conversion, ledger, or target Credit cost.'
                      )}
                    </p>
                  </AlertDescription>
                </Alert>
              )}

            {reasons.length > 0 && (
              <Alert variant='destructive'>
                <AlertTitle>
                  {t('Conversion is currently unavailable')}
                </AlertTitle>
                <AlertDescription>
                  <ul className='list-disc space-y-1 pl-4'>
                    {reasons.map((reason) => (
                      <li key={reason.code}>{reasonText(reason, t)}</li>
                    ))}
                  </ul>
                </AlertDescription>
              </Alert>
            )}

            <Alert>
              <AlertTitle>{t('Review the irreversible result')}</AlertTitle>
              <AlertDescription className='space-y-1'>
                <p>
                  {t(
                    'Converting is irreversible. The source timed subscription remains as a converted history record, and in-flight requests settle to the target Credit balance.'
                  )}
                </p>
                <p>
                  {t(
                    'The final submission will recalculate using the latest values.'
                  )}
                </p>
              </AlertDescription>
            </Alert>

            {confirmationError && (
              <Alert variant='destructive' role='alert'>
                <AlertTitle>{t('Unable to convert subscription')}</AlertTitle>
                <AlertDescription>{confirmationError}</AlertDescription>
              </Alert>
            )}
          </div>
        )}

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={confirming}
            onClick={() => onOpenChange(false)}
          >
            {t('Close preview')}
          </Button>
          {live?.canConfirm && (
            <Button type='button' disabled={confirming} onClick={onConfirm}>
              {confirming && <Spinner aria-hidden data-icon='inline-start' />}
              {confirming
                ? t('Converting subscription')
                : t('Submit conversion')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
interface TimedSubscriptionConversionQuotesCardProps {
  now?: () => number
  loadQuotes?: () => Promise<SubscriptionConversionQuoteList>
  confirmConversion?: (
    request: SubscriptionConversionConfirmRequest
  ) => Promise<SubscriptionConversionConfirmResult>
  createIdempotencyKey?: () => string
}
export function TimedSubscriptionConversionQuotesCard({
  now = Date.now,
  loadQuotes,
  confirmConversion,
  createIdempotencyKey = defaultConversionIdempotencyKey,
}: TimedSubscriptionConversionQuotesCardProps = {}) {
  const { t } = useTranslation()
  const quotesQuery = useSubscriptionConversionQuotes(loadQuotes)
  const confirmMutation = useConfirmSubscriptionConversion(confirmConversion)
  const [clockMs, setClockMs] = useState(() => now())
  const [receivedAtMs, setReceivedAtMs] = useState(() => now())
  const [previewSelection, setPreviewSelection] = useState<{
    quote: SubscriptionConversionQuote
    receivedAtMs: number
  } | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewingId, setPreviewingId] = useState<string | null>(null)
  const previewRequestId = useRef(0)
  const idempotencyAttempts = useRef(
    new Map<
      string,
      { quoteId: string; factsFingerprint: string; key: string }
    >()
  )
  const confirmationInFlight = useRef(false)
  const [confirmationError, setConfirmationError] = useState<string | null>(
    null
  )
  const [confirming, setConfirming] = useState(false)
  const [latestConversion, setLatestConversion] =
    useState<SubscriptionConversionHistory | null>(null)

  useEffect(() => {
    const interval = setInterval(() => setClockMs(now()), 1000)
    return () => clearInterval(interval)
  }, [now])

  useEffect(() => {
    const receivedAt = now()
    setReceivedAtMs(receivedAt)
    setClockMs(receivedAt)
  }, [now, quotesQuery.dataUpdatedAt])

  const elapsedSeconds = BigInt(
    Math.max(0, Math.floor((clockMs - receivedAtMs) / 1000))
  )
  const quotes = quotesQuery.data?.quotes ?? EMPTY_QUOTES
  const previewElapsedSeconds = BigInt(
    Math.max(
      0,
      Math.floor((clockMs - (previewSelection?.receivedAtMs ?? clockMs)) / 1000)
    )
  )
  const liveQuoteEntries = useMemo(
    () =>
      quotes.map((quote) => {
        try {
          return {
            quote,
            live: deriveLiveConversionQuote(quote, elapsedSeconds),
          }
        } catch {
          return { quote, live: null }
        }
      }),
    [elapsedSeconds, quotes]
  )

  const handlePreview = async (sourceSubscriptionId: string) => {
    const requestId = ++previewRequestId.current
    setPreviewingId(sourceSubscriptionId)
    try {
      const refreshed = await quotesQuery.refetch()
      const latest = refreshed.data?.quotes.find(
        (quote) => quote.source_subscription_id === sourceSubscriptionId
      )
      if (!latest || requestId !== previewRequestId.current) return
      const refreshedAt = now()
      setReceivedAtMs(refreshedAt)
      setClockMs(refreshedAt)
      setPreviewSelection({ quote: latest, receivedAtMs: refreshedAt })
      setPreviewOpen(true)
    } finally {
      if (requestId === previewRequestId.current) {
        setPreviewingId(null)
      }
    }
  }

  const handleConfirm = async () => {
    const selected = previewSelection?.quote
    if (!selected || confirmationInFlight.current) return
    confirmationInFlight.current = true
    setConfirming(true)
    setConfirmationError(null)
    try {
      const refreshed = await quotesQuery.refetch()
      const latest = refreshed.data?.quotes.find(
        (quote) =>
          quote.source_subscription_id === selected.source_subscription_id
      )
      if (!latest) {
        setConfirmationError(
          t('The source subscription is no longer convertible')
        )
        return
      }
      const refreshedAt = now()
      setReceivedAtMs(refreshedAt)
      setClockMs(refreshedAt)
      setPreviewSelection({ quote: latest, receivedAtMs: refreshedAt })
      if (
        latest.quote_id !== selected.quote_id ||
        latest.facts_fingerprint !== selected.facts_fingerprint
      ) {
        setConfirmationError(
          t(
            'The quote expired or authoritative facts changed. Review the refreshed quote and confirm again.'
          )
        )
        return
      }
      if (!latest.can_confirm) {
        setConfirmationError(t('The latest quote cannot be confirmed'))
        return
      }
      let attempt = idempotencyAttempts.current.get(
        latest.source_subscription_id
      )
      if (
        !attempt ||
        attempt.quoteId !== latest.quote_id ||
        attempt.factsFingerprint !== latest.facts_fingerprint
      ) {
        attempt = {
          quoteId: latest.quote_id,
          factsFingerprint: latest.facts_fingerprint,
          key: createIdempotencyKey(),
        }
        idempotencyAttempts.current.set(latest.source_subscription_id, attempt)
      }
      const result = await confirmMutation.mutateAsync({
        subscription_id: latest.source_subscription_id,
        quote_id: latest.quote_id,
        idempotency_key: attempt.key,
      })
      idempotencyAttempts.current.delete(latest.source_subscription_id)
      setLatestConversion(result.conversion)
      setPreviewOpen(false)
      await quotesQuery.refetch()
    } catch (error) {
      setConfirmationError(getSubscriptionConversionErrorMessage(error, t))
      const refreshed = await quotesQuery.refetch()
      const latest = refreshed.data?.quotes.find(
        (quote) =>
          quote.source_subscription_id === selected.source_subscription_id
      )
      if (latest) {
        const refreshedAt = now()
        setReceivedAtMs(refreshedAt)
        setClockMs(refreshedAt)
        setPreviewSelection({ quote: latest, receivedAtMs: refreshedAt })
      }
    } finally {
      confirmationInFlight.current = false
      setConfirming(false)
    }
  }

  if (quotesQuery.isLoading) {
    return (
      <Skeleton
        className='h-48 w-full'
        aria-label={t('Loading conversion quotes')}
      />
    )
  }

  if (quotesQuery.isError) {
    return (
      <Alert variant='destructive'>
        <AlertTitle>{t('Unable to load conversion quotes')}</AlertTitle>
        <AlertDescription>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => quotesQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  const conversions = quotesQuery.data?.conversions ?? []
  if (quotes.length === 0 && conversions.length === 0 && !latestConversion)
    return null

  return (
    <div className='flex flex-col gap-4'>
      {latestConversion && (
        <ConversionResultCard
          title={t('Latest conversion result')}
          conversion={latestConversion}
          role='status'
        />
      )}
      <TitledCard
        title={t('Timed subscription conversion quotes')}
        description={t(
          'Read-only estimates refresh from the server every five seconds.'
        )}
        icon={<ArrowRightLeft />}
        action={
          <p
            role='status'
            aria-label={t('Conversion quote refresh status')}
            aria-live='polite'
            className={cn(
              'text-muted-foreground text-xs',
              quotesQuery.isFetching && 'text-foreground'
            )}
          >
            {quotesQuery.isFetching
              ? t('Refreshing conversion quotes')
              : t('Conversion quotes are up to date')}
          </p>
        }
        contentClassName='flex flex-col gap-4'
      >
        {categorySections.map((section) => {
          const sectionQuotes = liveQuoteEntries.filter(
            (entry) => (entry.live?.category ?? 'excluded') === section.category
          )
          if (sectionQuotes.length === 0) return null
          return (
            <Collapsible
              key={section.category}
              defaultOpen={section.category !== 'excluded'}
              className='rounded-lg border'
            >
              <CollapsibleTrigger className='group flex w-full items-center justify-between gap-3 p-3 text-left'>
                <div>
                  <div className='flex items-center gap-2'>
                    <h3
                      id={`conversion-${section.category}`}
                      className='font-medium'
                    >
                      {t(section.title)}
                    </h3>
                    <Badge variant='outline'>{sectionQuotes.length}</Badge>
                  </div>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t(section.description)}
                  </p>
                </div>
                <ChevronDown className='text-muted-foreground transition-transform group-data-[panel-open]:rotate-180' />
              </CollapsibleTrigger>
              <CollapsibleContent className='border-t p-3'>
                <div className='grid gap-3'>
                  {sectionQuotes.map(({ quote, live }) => (
                    <QuoteInstance
                      key={quote.source_subscription_id}
                      quote={quote}
                      live={live}
                      onPreview={handlePreview}
                      previewing={previewingId === quote.source_subscription_id}
                    />
                  ))}
                </div>
              </CollapsibleContent>
            </Collapsible>
          )
        })}
        {conversions.length > 0 && (
          <section aria-labelledby='subscription-conversion-history'>
            <h3 id='subscription-conversion-history' className='font-medium'>
              {t('Conversion history')}
            </h3>
            <div className='mt-3 grid gap-3'>
              {conversions.map((conversion) => (
                <ConversionResultSummary
                  key={conversion.id}
                  conversion={conversion}
                />
              ))}
            </div>
          </section>
        )}
      </TitledCard>

      <ConversionPreviewDialog
        quote={previewSelection?.quote ?? null}
        elapsedSeconds={previewElapsedSeconds}
        open={previewOpen}
        confirming={confirming}
        confirmationError={confirmationError}
        onConfirm={handleConfirm}
        onOpenChange={(open) => {
          if (!confirming) {
            setPreviewOpen(open)
            if (!open) setConfirmationError(null)
          }
        }}
      />
    </div>
  )
}
