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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { getHomePublicPlansQuiet } from '@/features/subscriptions/api'
import {
  formatConcurrencyLimit,
  formatDuration,
  formatPlanPrice,
  formatCreditLimit,
} from '@/features/subscriptions/lib'
import type { PublicPlanRecord } from '@/features/subscriptions/types'
import {
  getHomePublicPlansQueryKey,
  hasMoreHomePlans,
  renderHomePlanChannelEquivalentLabels,
  selectHomePlanRecords,
} from '../../lib/plans-preview'

export function PlansPreview() {
  const { t } = useTranslation()
  const plansQuery = useQuery({
    queryKey: getHomePublicPlansQueryKey(),
    queryFn: getHomePublicPlansQuiet,
    staleTime: 60_000,
  })

  if (plansQuery.isLoading) {
    return (
      <section
        aria-label={t('Subscription Plans')}
        className='border-border/40 bg-muted/10 relative z-10 border-y'
      >
        <div className='mx-auto max-w-6xl px-6 py-10 md:py-12'>
          <div className='mb-8 flex flex-col gap-3 md:flex-row md:items-end md:justify-between'>
            <div className='space-y-3'>
              <Skeleton className='h-4 w-36' />
              <Skeleton className='h-8 w-80 max-w-full' />
            </div>
            <Skeleton className='h-10 w-32' />
          </div>
          <div className='grid gap-4 md:grid-cols-3'>
            <Skeleton className='h-56' />
            <Skeleton className='hidden h-56 md:block' />
            <Skeleton className='hidden h-56 md:block' />
          </div>
        </div>
      </section>
    )
  }

  const records: readonly PublicPlanRecord[] = plansQuery.data?.success
    ? (plansQuery.data.data ?? [])
    : []
  const plans = selectHomePlanRecords(records)
  if (plans.length === 0) return null

  const showAllPlansLink = hasMoreHomePlans(records)

  return (
    <section className='border-border/40 bg-muted/10 relative z-10 border-y'>
      <div className='mx-auto max-w-6xl px-6 py-10 md:py-12'>
        <div className='mb-8 flex flex-col gap-3 md:flex-row md:items-end md:justify-between'>
          <div>
            <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
              {t('Subscription Plans')}
            </p>
            <h2 className='text-2xl font-bold tracking-tight md:text-3xl'>
              {t('Pick a plan that fits your GPT usage.')}
            </h2>
          </div>
          {showAllPlansLink ? (
            <Button variant='outline' render={<Link to='/wallet' />}>
              {t('View all plans')}
            </Button>
          ) : null}
        </div>
        <div className='grid gap-4 md:grid-cols-3'>
          {plans.map((record) => (
            <PlanCard key={record.plan.id} record={record} />
          ))}
        </div>
      </div>
    </section>
  )
}

function PlanCard(props: { record: PublicPlanRecord }) {
  const { t } = useTranslation()
  const plan = props.record.plan

  const channelEquivalentLabels = renderHomePlanChannelEquivalentLabels(
    props.record,
    t
  )
  return (
    <Card className='h-full transition-shadow hover:shadow-md'>
      <CardContent className='flex h-full flex-col p-5'>
        <div className='mb-3 min-w-0'>
          <h3 className='truncate font-semibold'>
            {plan.title || t('Subscription Plans')}
          </h3>
          {plan.subtitle ? (
            <p className='text-muted-foreground mt-1 truncate text-sm'>
              {plan.subtitle}
            </p>
          ) : null}
        </div>

        <div className='text-primary mb-4 text-2xl font-bold'>
          {formatPlanPrice(plan.price_amount, plan.currency)}
        </div>

        <div className='flex-1 space-y-2'>
          <PlanMetric
            label={t('Validity Period')}
            value={formatDuration(plan, t)}
          />
          <PlanMetric
            label={t('Monthly Credits')}
            value={formatCreditLimit(plan.monthly_token_limit, t)}
          />
          {channelEquivalentLabels.length > 0 && (
            <div className='text-muted-foreground ml-5 space-y-1 text-xs'>
              <div>{t('Equivalent by channel')}:</div>
              {channelEquivalentLabels.map((label) => (
                <div key={label}>{label}</div>
              ))}
            </div>
          )}
          <PlanMetric
            label={t('Concurrency Limit')}
            value={formatConcurrencyLimit(plan.concurrency_limit, t)}
          />
        </div>

        <Button className='mt-5 w-full' render={<Link to='/wallet' />}>
          {t('Choose a plan')}
        </Button>
      </CardContent>
    </Card>
  )
}

function PlanMetric(props: { label: string; value: string }) {
  return (
    <div className='text-muted-foreground flex items-center gap-2 text-sm'>
      <Check className='text-primary size-3.5 shrink-0' aria-hidden='true' />
      <span>
        {props.label}: {props.value}
      </span>
    </div>
  )
}
