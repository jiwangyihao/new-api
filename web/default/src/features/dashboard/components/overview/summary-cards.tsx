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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowRight, Flame, ShieldCheck, TrendingDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { formatNumber } from '@/lib/format'
import { computeTimeRange } from '@/lib/time'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import { StaggerContainer, StaggerItem } from '@/components/page-transition'
import { getUserQuotaDates } from '@/features/dashboard/api'
import { useSummaryCardsConfig } from '@/features/dashboard/hooks/use-dashboard-config'
import {
  buildSubscriptionSummaryView,
  formatSubscriptionTokenAmount,
  type SubscriptionSummaryHealthLevel,
} from '@/features/dashboard/lib/subscription-summary'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { getSelfSubscriptionFull } from '@/features/subscriptions/api'
import { StatCard } from '../ui/stat-card'

const SUMMARY_SPARKLINE_BUCKETS = 12

type SummarySparklineKey = 'usage' | 'requests'

function getBucketIndex(
  timestamp: number,
  start: number,
  end: number,
  bucketCount: number
): number {
  if (end <= start) return 0
  const ratio = (timestamp - start) / (end - start)
  return Math.min(bucketCount - 1, Math.max(0, Math.floor(ratio * bucketCount)))
}

function buildSummarySparklines(
  data: QuotaDataItem[],
  start: number,
  end: number
): Record<SummarySparklineKey, number[]> {
  const usage = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)
  const requests = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)

  for (const item of data) {
    const timestamp = Number(item.created_at) || start
    const index = getBucketIndex(
      timestamp,
      start,
      end,
      SUMMARY_SPARKLINE_BUCKETS
    )
    usage[index] += Number(item.token_used) || 0
    requests[index] += Number(item.count) || 0
  }

  return {
    usage,
    requests,
  }
}

function getSummarySparkline(
  key: string,
  sparklineData: Record<SummarySparklineKey, number[]>
): number[] | undefined {
  if (key === 'usage') return sparklineData.usage
  if (key === 'requests') return sparklineData.requests
  return undefined
}

function getRunwayLabel(runwayDays: number | null): string {
  if (runwayDays === null) return ''
  if (runwayDays < 1) return 'Less than 1 day left'
  if (runwayDays > 999) return '999+ days'
  return `~${formatNumber(Math.floor(runwayDays))} days`
}

const HEALTH_CONFIG: Record<
  SubscriptionSummaryHealthLevel,
  { dotClass: string }
> = {
  healthy: {
    dotClass: 'bg-success',
  },
  caution: {
    dotClass: 'bg-warning',
  },
  critical: {
    dotClass: 'bg-destructive',
  },
}


export function SummaryCards() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const { loading } = useStatus()

  const summaryTimeRange = useMemo(() => computeTimeRange(1), [])
  const requestCount = Number(user?.request_count ?? 0)

  const subscriptionSummaryQuery = useQuery({
    queryKey: ['subscriptions', 'self', 'summary'],
    queryFn: getSelfSubscriptionFull,
    staleTime: 60 * 1000,
  })

  const usageTrendQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'summary-sparklines',
      summaryTimeRange.start_timestamp,
      summaryTimeRange.end_timestamp,
    ],
    queryFn: async () =>
      getUserQuotaDates({
        start_timestamp: summaryTimeRange.start_timestamp,
        end_timestamp: summaryTimeRange.end_timestamp,
        default_time: 'hour',
      }),
    staleTime: 60 * 1000,
  })

  const recentUsage = useMemo(
    () =>
      (usageTrendQuery.data?.data ?? []).reduce(
        (total, item) => total + (Number(item.token_used) || 0),
        0
      ),
    [usageTrendQuery.data?.data]
  )

  const summaryView = useMemo(
    () =>
      buildSubscriptionSummaryView(
        subscriptionSummaryQuery.data?.data?.summary,
        recentUsage
      ),
    [recentUsage, subscriptionSummaryQuery.data?.data?.summary]
  )

  const healthCfg = HEALTH_CONFIG[summaryView.healthLevel]
  const runwayLabel = getRunwayLabel(summaryView.runwayDays)

  const sparklineData = useMemo(
    () =>
      buildSummarySparklines(
        usageTrendQuery.data?.data ?? [],
        summaryTimeRange.start_timestamp,
        summaryTimeRange.end_timestamp
      ),
    [
      summaryTimeRange.end_timestamp,
      summaryTimeRange.start_timestamp,
      usageTrendQuery.data?.data,
    ]
  )

  const items = useSummaryCardsConfig({
    remainingTokensDisplay: summaryView.remainingLabel,
    cycleTokensDisplay: summaryView.usedLabel,
    recentTokensDisplay: formatSubscriptionTokenAmount(recentUsage),
    requestCountDisplay: formatNumber(requestCount),
  }).map((config, index) => {
    const tones = ['rose', 'teal', 'rose', 'gray'] as const

    return {
      key: config.key,
      title: config.title,
      value: config.value,
      desc: config.description,
      icon: config.icon,
      tone: tones[index] ?? 'gray',
      sparkline: getSummarySparkline(config.key, sparklineData),
      sparklineVariant: 'line' as const,
    }
  })

  return (
    <div className='bg-card overflow-hidden rounded-2xl border shadow-xs'>
      <div className='grid xl:grid-cols-[minmax(0,1fr)_19rem]'>
        <div className='flex flex-col gap-3 p-4 sm:p-5'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div className='flex flex-col gap-1'>
              <h3 className='text-base font-semibold'>
                {t('Usage at a glance')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t('Monitor subscription tokens and request volume')}
              </p>
            </div>
          </div>
          <StaggerContainer className='grid gap-3 md:grid-cols-3'>
            {items.map((it) => (
              <StaggerItem
                key={it.key}
                className='bg-background/60 rounded-xl border p-3'
              >
                <StatCard
                  title={it.title}
                  value={it.value}
                  description={it.desc}
                  icon={it.icon}
                  tone={it.tone}
                  sparkline={it.sparkline}
                  sparklineVariant={it.sparklineVariant}
                  loading={loading || subscriptionSummaryQuery.isLoading}
                />
              </StaggerItem>
            ))}
          </StaggerContainer>
        </div>

        <div className='bg-warning/10 flex flex-col justify-between gap-4 border-t p-4 sm:p-5 xl:border-t-0 xl:border-l'>
          <div className='flex flex-col gap-3'>
            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Subscription tokens remaining')}
              </span>
              <span className='flex items-center gap-1.5'>
                <span
                  className={cn('size-1.5 rounded-full', healthCfg.dotClass)}
                  aria-hidden='true'
                />
                <span className='text-muted-foreground text-[11px] font-medium'>
                  {t(summaryView.statusLabelKey)}
                </span>
              </span>
            </div>

            <div className='font-mono text-2xl font-semibold tracking-tight'>
              {summaryView.remainingLabel}
            </div>

            <div className='grid grid-cols-2 gap-2'>
              <div className='bg-background/60 rounded-lg px-2.5 py-2'>
                <div className='text-muted-foreground flex items-center gap-1 text-[11px] leading-none font-medium'>
                  <Flame className='size-3 shrink-0' aria-hidden='true' />
                  <span className='truncate'>{t('Last 24h token usage')}</span>
                </div>
                <div className='text-foreground mt-1.5 truncate text-xs font-semibold tabular-nums'>
                  {formatSubscriptionTokenAmount(recentUsage)}
                </div>
              </div>
              <div className='bg-background/60 rounded-lg px-2.5 py-2'>
                <div className='text-muted-foreground flex items-center gap-1 text-[11px] leading-none font-medium'>
                  {summaryView.runwayDays !== null && summaryView.runwayDays < 3 ? (
                    <TrendingDown
                      className='size-3 shrink-0'
                      aria-hidden='true'
                    />
                  ) : (
                    <ShieldCheck
                      className='size-3 shrink-0'
                      aria-hidden='true'
                    />
                  )}
                  <span className='truncate'>{t('Runway')}</span>
                </div>
                <div
                  className={cn(
                    'mt-1.5 truncate text-xs font-semibold tabular-nums',
                    summaryView.healthLevel === 'critical' && 'text-destructive',
                    summaryView.healthLevel === 'caution' && 'text-warning'
                  )}
                >
                  {runwayLabel
                    ? t(runwayLabel)
                    : summaryView.healthLevel === 'critical'
                      ? t(summaryView.statusLabelKey)
                      : t('No recent token usage')}
                </div>
              </div>
            </div>
          </div>

          <Button
            className='justify-between'
            render={<Link to='/wallet' />}
          >
            <span>{t('Wallet')}</span>
            <ArrowRight data-icon='inline-end' />
          </Button>
        </div>
      </div>
    </div>
  )
}
