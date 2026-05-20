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
import { BarChart3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatTimestampToDate } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { TableEmpty } from '@/components/data-table/table-empty'
import { ErrorState } from '@/components/error-state'
import {
  buildRankingRows,
  mergeTopNWithOther,
  type UsageAnalyticsRankingRow,
} from '../lib/chart-data'
import {
  buildUsageAnalyticsRankingDrilldown,
  type UsageAnalyticsRankingDrilldownTarget,
} from '../lib/page-contract'
import {
  formatLatencyMs,
  formatUsagePercent,
  formatUsageTokens,
} from '../lib/format'
import type {
  UsageAnalyticsCanonicalFilters,
  UsageAnalyticsGroup,
  UsageAnalyticsMetric,
} from '../types'

interface UsageRankingTableProps {
  groups?: UsageAnalyticsGroup[]
  metric: UsageAnalyticsMetric
  filters: UsageAnalyticsCanonicalFilters
  loading?: boolean
  error?: boolean
  onDrilldown?: (target: UsageAnalyticsRankingDrilldownTarget) => void
}

const integerFormatter = new Intl.NumberFormat('en-US', {
  maximumFractionDigits: 0,
})

function formatInteger(value: number): string {
  if (!Number.isFinite(value)) return '0'
  return integerFormatter.format(Math.max(0, value))
}

function formatMetricValue(
  row: UsageAnalyticsRankingRow,
  metric: UsageAnalyticsMetric
): string {
  if (metric === 'request_count') return formatInteger(row.request_count)
  if (metric === 'total_tokens') return formatUsageTokens(row.total_tokens)
  if (metric === 'quota') return formatUsageTokens(row.quota)
  if (metric === 'error_rate') return formatUsagePercent(row.error_rate)
  if (metric === 'avg_latency_ms') return formatLatencyMs(row.avg_latency_ms)
  return formatLatencyMs(row.p95_latency_ms)
}

function formatShareOrRate(row: UsageAnalyticsRankingRow): string {
  if (row.share !== null) return formatUsagePercent(row.share)
  return `${formatUsagePercent(row.success_rate)} / ${formatUsagePercent(
    row.error_rate
  )}`
}

function formatLastUsed(timestamp: number): string {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '--'
  return formatTimestampToDate(timestamp, 'seconds')
}

function buildDimensionContent(
  row: UsageAnalyticsRankingRow,
  t: (key: string) => string
) {
  const isOther = row.drilldown === null
  return (
    <div className='min-w-48 space-y-1'>
      <div className='flex flex-wrap items-center gap-1.5'>
        <span className='font-medium'>{isOther ? t('Other') : row.display_label}</span>
        {row.token_deleted && (
          <Badge variant='outline' className='text-muted-foreground'>
            {t('Deleted API Key')}
          </Badge>
        )}
      </div>
      {row.masked_key !== null && (
        <div className='text-muted-foreground font-mono text-xs'>
          {row.masked_key}
        </div>
      )}
    </div>
  )
}

function RankingSkeletonRows() {
  return (
    <>
      {Array.from({ length: 5 }, (_, index) => (
        <TableRow key={index}>
          {Array.from({ length: 9 }, (__, cellIndex) => (
            <TableCell key={cellIndex}>
              <Skeleton className='h-5 w-full min-w-16' />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  )
}

export function UsageRankingTable(props: UsageRankingTableProps) {
  const { t } = useTranslation()
  const rows = useMemo(() => {
    if (!props.groups) return []
    const merged = mergeTopNWithOther(
      props.groups,
      props.metric,
      props.filters.limit
    )
    return buildRankingRows(merged)
  }, [props.filters.limit, props.groups, props.metric])

  if (props.error) {
    return (
      <ErrorState
        title={t('Failed to load usage analytics')}
        description={t('Try adjusting the time range or filters')}
      />
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Usage Ranking')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Rank')}</TableHead>
              <TableHead>{t('Dimension')}</TableHead>
              <TableHead>{t('Metric Value')}</TableHead>
              <TableHead>{t('Share / Rate')}</TableHead>
              <TableHead>{t('Request Count')}</TableHead>
              <TableHead>{t('Total Tokens')}</TableHead>
              <TableHead>{t('Latency')}</TableHead>
              <TableHead>{t('Last Used')}</TableHead>
              <TableHead>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.loading ? <RankingSkeletonRows /> : null}
            {!props.loading && rows.length === 0 ? (
              <TableEmpty
                colSpan={9}
                title={t('No matching usage data')}
                description={t('Try adjusting the time range or filters')}
                icon={<BarChart3 className='size-6' />}
              />
            ) : null}
            {!props.loading &&
              rows.map((row, index) => {
                const target = buildUsageAnalyticsRankingDrilldown(
                  props.filters,
                  row.drilldown
                )
                const disabled = target === null || props.onDrilldown === undefined
                return (
                  <TableRow key={row.id}>
                    <TableCell className='font-mono tabular-nums'>
                      {index + 1}
                    </TableCell>
                    <TableCell>{buildDimensionContent(row, t)}</TableCell>
                    <TableCell className='font-medium tabular-nums'>
                      {formatMetricValue(row, props.metric)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatShareOrRate(row)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatInteger(row.request_count)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatUsageTokens(row.total_tokens)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatLatencyMs(row.avg_latency_ms)} /{' '}
                      {formatLatencyMs(row.p95_latency_ms)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatLastUsed(row.last_used_at)}
                    </TableCell>
                    <TableCell>
                      <Button
                        type='button'
                        size='sm'
                        variant='outline'
                        disabled={disabled}
                        title={
                          target === null
                            ? t('This item cannot be drilled down')
                            : t('View logs for this item')
                        }
                        onClick={() => {
                          if (target === null || props.onDrilldown === undefined) {
                            return
                          }
                          props.onDrilldown(target)
                        }}
                      >
                        {t('View Logs')}
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
