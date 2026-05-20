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
import { useMemo, type JSX } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ErrorState } from '@/components/error-state'
import {
  getUsageAnalyticsBreakdown,
  getUsageAnalyticsSummary,
  getUsageAnalyticsTimeseries,
} from './api'
import { UsageAnalyticsFilterBar } from './components/usage-analytics-filter-bar'
import { UsageAnalyticsSummaryCards } from './components/usage-analytics-summary-cards'
import { UsageBreakdownChart } from './components/usage-breakdown-chart'
import { UsageRankingTable } from './components/usage-ranking-table'
import { UsageTrendChart } from './components/usage-trend-chart'
import {
  buildUsageAnalyticsQueryKeys,
  type UsageAnalyticsRankingDrilldownTarget,
} from './lib/page-contract'
import type {
  ApiResponse,
  UsageAnalyticsBreakdownResponse,
  UsageAnalyticsCanonicalFilters,
  UsageAnalyticsSummaryResponse,
  UsageAnalyticsTimeseriesResponse,
} from './types'

export interface UsageAnalyticsPageProps {
  search: UsageAnalyticsCanonicalFilters
  onSearchChange: (next: Partial<UsageAnalyticsCanonicalFilters>) => void
  onDrilldown?: (target: UsageAnalyticsRankingDrilldownTarget) => void
}

function hasResponseData<TData>(
  response: ApiResponse<TData> | undefined
): response is ApiResponse<TData> & { data: TData } {
  return response?.success === true && response.data !== undefined
}

function responseFailed<TData>(response: ApiResponse<TData> | undefined): boolean {
  return response !== undefined && (response.success !== true || response.data === undefined)
}

function firstFailureMessage(
  summaryResponse: ApiResponse<UsageAnalyticsSummaryResponse> | undefined,
  timeseriesResponse: ApiResponse<UsageAnalyticsTimeseriesResponse> | undefined,
  breakdownResponse: ApiResponse<UsageAnalyticsBreakdownResponse> | undefined
): string | undefined {
  if (responseFailed(summaryResponse) && summaryResponse?.message) {
    return summaryResponse.message
  }
  if (responseFailed(timeseriesResponse) && timeseriesResponse?.message) {
    return timeseriesResponse.message
  }
  if (responseFailed(breakdownResponse) && breakdownResponse?.message) {
    return breakdownResponse.message
  }
  return undefined
}

export function UsageAnalyticsPage(
  props: UsageAnalyticsPageProps
): JSX.Element {
  const { t } = useTranslation()
  const queryKeys = useMemo(
    () => buildUsageAnalyticsQueryKeys(props.search),
    [props.search]
  )
  const summaryQuery = useQuery({
    queryKey: queryKeys.summary,
    queryFn: () => getUsageAnalyticsSummary(props.search),
  })
  const timeseriesQuery = useQuery({
    queryKey: queryKeys.timeseries,
    queryFn: () => getUsageAnalyticsTimeseries(props.search),
  })
  const breakdownQuery = useQuery({
    queryKey: queryKeys.breakdown,
    queryFn: () => getUsageAnalyticsBreakdown(props.search),
  })
  const summaryData = hasResponseData(summaryQuery.data)
    ? summaryQuery.data.data
    : undefined
  const timeseriesData = hasResponseData(timeseriesQuery.data)
    ? timeseriesQuery.data.data
    : undefined
  const breakdownData = hasResponseData(breakdownQuery.data)
    ? breakdownQuery.data.data
    : undefined
  const breakdownGroups = useMemo(() => {
    if (!breakdownData) return undefined
    if (breakdownData.other === null) return breakdownData.groups
    return [...breakdownData.groups, breakdownData.other]
  }, [breakdownData])
  const hasNetworkError =
    summaryQuery.isError || timeseriesQuery.isError || breakdownQuery.isError
  const hasResponseError =
    responseFailed(summaryQuery.data) ||
    responseFailed(timeseriesQuery.data) ||
    responseFailed(breakdownQuery.data)
  const failureMessage = firstFailureMessage(
    summaryQuery.data,
    timeseriesQuery.data,
    breakdownQuery.data
  )
  const errorDescription = failureMessage
    ? t('Server returned an analytics error: {{message}}', {
        message: failureMessage,
      })
    : t('Try adjusting the time range or filters')

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Usage Analytics')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Analyze your API usage across API keys, models, groups, streaming status, and request outcomes.'
            )}
          </p>

          <UsageAnalyticsFilterBar
            value={props.search}
            onApply={(next) => props.onSearchChange(next)}
          />

          {(hasNetworkError || hasResponseError) && (
            <ErrorState
              title={t('Failed to load usage analytics')}
              description={errorDescription}
            />
          )}

          <UsageAnalyticsSummaryCards
            data={summaryData}
            loading={summaryQuery.isLoading}
            error={summaryQuery.isError || responseFailed(summaryQuery.data)}
          />

          <div className='grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(320px,1fr)]'>
            <Card>
              <CardHeader>
                <CardTitle>{t('Usage Trend')}</CardTitle>
              </CardHeader>
              <CardContent>
                <UsageTrendChart
                  points={timeseriesData?.points}
                  metric={props.search.metric}
                  loading={timeseriesQuery.isLoading}
                  error={
                    timeseriesQuery.isError || responseFailed(timeseriesQuery.data)
                  }
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>{t('Usage Breakdown')}</CardTitle>
              </CardHeader>
              <CardContent>
                <UsageBreakdownChart
                  groups={breakdownGroups}
                  metric={props.search.metric}
                  loading={breakdownQuery.isLoading}
                  error={breakdownQuery.isError || responseFailed(breakdownQuery.data)}
                />
              </CardContent>
            </Card>
          </div>

          <UsageRankingTable
            groups={breakdownGroups}
            metric={props.search.metric}
            filters={props.search}
            loading={breakdownQuery.isLoading}
            error={breakdownQuery.isError || responseFailed(breakdownQuery.data)}
            onDrilldown={props.onDrilldown}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
