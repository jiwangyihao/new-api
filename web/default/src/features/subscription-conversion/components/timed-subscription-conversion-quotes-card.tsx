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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { useSubscriptionConversionQuotes } from '../hooks/use-subscription-conversion-quotes'
import { deriveLiveConversionQuote } from '../live-quote'
import type {
  ConversionQuoteCategory,
  LiveSubscriptionConversionQuote,
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

function formatCredit(value: bigint): string {
  return new Intl.NumberFormat().format(value)
}

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
  const seconds = reason.data?.remaining_seconds || '0'
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
    case 'outside_grace_period':
      return t('The 336-hour conversion grace period has ended')
    case 'grant_time_missing':
      return t('The latest grant time is unavailable')
    case 'cooldown_active':
      return t(
        'Conversion cooldown is active ({{seconds}} seconds remaining)',
        {
          seconds,
        }
      )
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
      ? t('{{seconds}} seconds of conversion cooldown remaining', {
          seconds: live.cooldownRemainingSeconds.toString(),
        })
      : live.withinGrace
        ? t('{{seconds}} seconds of conversion grace remaining', {
            seconds: live.graceRemainingSeconds.toString(),
          })
        : t('{{seconds}} seconds remaining', {
            seconds: live.remainingSeconds.toString(),
          })

  const reasons = liveQuoteReasons(quote, live)

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
        <Badge variant={live.category === 'excluded' ? 'outline' : 'secondary'}>
          {live.category === 'convertible'
            ? t('Convertible')
            : live.category === 'expired_grace'
              ? t('Expired grace period')
              : t('Excluded')}
        </Badge>
      </div>

      <div className='mt-3 grid gap-2 text-xs sm:grid-cols-2'>
        <div>
          <span className='text-muted-foreground'>{t('Ends')}</span>
          <div>{formatTimestamp(live.endTime)}</div>
        </div>
        <div>
          <span className='text-muted-foreground'>{t('31-day blocks')}</span>
          <div>{live.full31DayBlocks.toString()}</div>
        </div>
        <div>
          <span className='text-muted-foreground'>{t('Gross Credit')}</span>
          <div>{formatCredit(live.grossCredit)}</div>
        </div>
        <div>
          <span className='text-muted-foreground'>
            {t('Net available Credit')}
          </span>
          <div>{formatCredit(live.netAvailableCredit)}</div>
        </div>
      </div>

      <p
        className='text-muted-foreground mt-3 text-xs'
        role='timer'
        aria-live='off'
      >
        {countdown}
      </p>
      <code className='bg-muted mt-2 block overflow-x-auto rounded px-2 py-1.5 text-xs'>
        {live.formula}
      </code>

      {reasons.length > 0 && (
        <ul className='text-muted-foreground mt-3 list-disc space-y-1 pl-4 text-xs'>
          {reasons.map((reason) => (
            <li key={reason.code}>{reasonText(reason, t)}</li>
          ))}
        </ul>
      )}

      <div className='mt-3 flex justify-end'>
        <Button
          type='button'
          size='sm'
          variant={live.canConfirm ? 'default' : 'outline'}
          disabled={previewing}
          onClick={() => onPreview(quote.source_subscription_id)}
        >
          {t('Preview conversion')}
        </Button>
      </div>
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
  return reasons
}

interface ConversionPreviewDialogProps {
  quote: SubscriptionConversionQuote | null
  elapsedSeconds: bigint
  open: boolean
  onOpenChange: (open: boolean) => void
}

function ConversionPreviewDialog({
  quote,
  elapsedSeconds,
  open,
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
                <dt className='text-muted-foreground'>
                  {t('Remaining seconds')}
                </dt>
                <dd>{live.remainingSeconds.toString()}</dd>
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
                {live.formula}
              </code>
            </div>

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
                    'Converting is irreversible and removes the source timed subscription.'
                  )}
                </p>
                <p>
                  {t(
                    'The final submission will recalculate using the latest values.'
                  )}
                </p>
              </AlertDescription>
            </Alert>
          </div>
        )}

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Close preview')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
interface TimedSubscriptionConversionQuotesCardProps {
  now?: () => number
  loadQuotes?: () => Promise<SubscriptionConversionQuoteList>
}
export function TimedSubscriptionConversionQuotesCard({
  now = Date.now,
  loadQuotes,
}: TimedSubscriptionConversionQuotesCardProps = {}) {
  const { t } = useTranslation()
  const quotesQuery = useSubscriptionConversionQuotes(loadQuotes)
  const [clockMs, setClockMs] = useState(() => now())
  const [receivedAtMs, setReceivedAtMs] = useState(() => now())
  const [previewSelection, setPreviewSelection] = useState<{
    quote: SubscriptionConversionQuote
    receivedAtMs: number
  } | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewingId, setPreviewingId] = useState<string | null>(null)
  const previewRequestId = useRef(0)

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

  if (quotes.length === 0) return null

  return (
    <>
      <Card>
        <CardHeader className='border-b'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div>
              <CardTitle>{t('Timed subscription conversion quotes')}</CardTitle>
              <CardDescription>
                {t(
                  'Read-only estimates refresh from the server every five seconds.'
                )}
              </CardDescription>
            </div>
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
          </div>
        </CardHeader>
        <CardContent className='space-y-5'>
          {categorySections.map((section) => {
            const sectionQuotes = liveQuoteEntries.filter(
              (entry) =>
                (entry.live?.category ?? 'excluded') === section.category
            )
            if (sectionQuotes.length === 0) return null
            return (
              <section
                key={section.category}
                aria-labelledby={`conversion-${section.category}`}
              >
                <h3
                  id={`conversion-${section.category}`}
                  className='font-medium'
                >
                  {t(section.title)}
                </h3>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(section.description)}
                </p>
                <div className='mt-3 grid gap-3 lg:grid-cols-2'>
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
              </section>
            )
          })}
        </CardContent>
      </Card>

      <ConversionPreviewDialog
        quote={previewSelection?.quote ?? null}
        elapsedSeconds={previewElapsedSeconds}
        open={previewOpen}
        onOpenChange={setPreviewOpen}
      />
    </>
  )
}
