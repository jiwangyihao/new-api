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
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatLatencyMs, formatUsagePercent, formatUsageTokens } from '../lib/format'
import type { UsageAnalyticsSummaryResponse } from '../types'

interface UsageAnalyticsSummaryCardsProps {
  data?: UsageAnalyticsSummaryResponse
  loading?: boolean
  error?: boolean
}

interface SummaryMetricConfig {
  key: string
  labelKey: string
  value: string
}

const numberFormatter = new Intl.NumberFormat('en-US', {
  maximumFractionDigits: 0,
})

const rateFormatter = new Intl.NumberFormat('en-US', {
  maximumFractionDigits: 2,
})

function formatInteger(value: number): string {
  if (!Number.isFinite(value)) return '0'
  return numberFormatter.format(Math.max(0, value))
}

function formatRate(value: number): string {
  if (!Number.isFinite(value)) return '0'
  return rateFormatter.format(Math.max(0, value))
}

function buildSummaryMetrics(
  data?: UsageAnalyticsSummaryResponse
): SummaryMetricConfig[] {
  const total = data?.total
  return [
    {
      key: 'request_count',
      labelKey: 'Request Count',
      value: total ? formatInteger(total.request_count) : '--',
    },
    {
      key: 'total_tokens',
      labelKey: 'Total Tokens',
      value: total ? formatUsageTokens(total.total_tokens) : '--',
    },
    {
      key: 'quota',
      labelKey: 'Quota',
      value: total ? formatUsageTokens(total.quota) : '--',
    },
    {
      key: 'success_rate',
      labelKey: 'Success Rate',
      value: total ? formatUsagePercent(total.success_rate) : '--',
    },
    {
      key: 'error_rate',
      labelKey: 'Error Rate',
      value: total ? formatUsagePercent(total.error_rate) : '--',
    },
    {
      key: 'avg_latency_ms',
      labelKey: 'Average Latency',
      value: total ? formatLatencyMs(total.avg_latency_ms) : '--',
    },
    {
      key: 'p95_latency_ms',
      labelKey: 'P95 Latency',
      value: total ? formatLatencyMs(total.p95_latency_ms) : '--',
    },
    {
      key: 'active_key_count',
      labelKey: 'Active API Keys',
      value: total ? formatInteger(total.active_key_count) : '--',
    },
    {
      key: 'rpm',
      labelKey: 'RPM',
      value: total ? formatRate(total.rpm) : '--',
    },
    {
      key: 'tpm',
      labelKey: 'TPM',
      value: total ? formatRate(total.tpm) : '--',
    },
  ]
}

export function UsageAnalyticsSummaryCards(
  props: UsageAnalyticsSummaryCardsProps
) {
  const { t } = useTranslation()
  const metrics = buildSummaryMetrics(props.error ? undefined : props.data)

  return (
    <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-5'>
      {metrics.map((metric) => (
        <Card key={metric.key} size='sm'>
          <CardHeader className='pb-0'>
            <CardTitle className='text-muted-foreground text-xs font-medium'>
              {t(metric.labelKey)}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {props.loading ? (
              <Skeleton className='h-8 w-24' />
            ) : (
              <div className='text-2xl font-semibold tabular-nums'>
                {metric.value}
              </div>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
