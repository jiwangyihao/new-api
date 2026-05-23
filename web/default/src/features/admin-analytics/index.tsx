import { useMemo, type JSX } from 'react'
import { useQueries } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { adminAnalyticsApi } from './api'
import { ADMIN_ANALYTICS_TABS } from './constants'
import { formatAdminPercent, formatAdminTokens } from './lib/format'
import { buildAdminAnalyticsRequestDescriptors } from './lib/page-contract'
import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsInvitationRewardsResponse,
  AdminAnalyticsOverviewResponse,
  AdminAnalyticsPanelResponse,
  AdminAnalyticsPlanDistributionResponse,
  AdminAnalyticsQuotaDistributionResponse,
  AdminAnalyticsRisksResponse,
  AdminAnalyticsSubscriptionConversionResponse,
  AdminAnalyticsTab,
  AdminAnalyticsUserLifecycleResponse,
  AdminUsageBreakdownResponse,
  AdminUsageConsumptionSummaryResponse,
  AdminUsageTimeseriesResponse,
  ApiResponse,
  FrontendAdminAnalyticsDrilldownTarget,
} from './types'

export interface AdminAnalyticsPageProps {
  search: AdminAnalyticsCanonicalFilters
  onSearchChange: (next: AdminAnalyticsCanonicalFilters) => void
  onDrilldown: (target: FrontendAdminAnalyticsDrilldownTarget) => void
}

type PanelApiResponse<TData> = ApiResponse<AdminAnalyticsPanelResponse<TData>>
type UnknownPanelResponse = PanelApiResponse<unknown>

type UsagePanelResponses = {
  summary?: PanelApiResponse<AdminUsageConsumptionSummaryResponse>
  timeseries?: PanelApiResponse<AdminUsageTimeseriesResponse>
  breakdown?: PanelApiResponse<AdminUsageBreakdownResponse>
}

function hasPanelData<TData>(
  response: PanelApiResponse<TData> | undefined
): response is PanelApiResponse<TData> & {
  data: AdminAnalyticsPanelResponse<TData>
} {
  return response?.success === true && response.data !== undefined
}

function panelData<TData>(
  response: PanelApiResponse<TData> | undefined
): TData | undefined {
  return hasPanelData(response) ? response.data.data : undefined
}

function panelFailed<TData>(
  response: PanelApiResponse<TData> | undefined
): boolean {
  return (
    response !== undefined &&
    (response.success !== true || response.data === undefined)
  )
}

export function AdminAnalyticsPage(
  props: AdminAnalyticsPageProps
): JSX.Element {
  const { t } = useTranslation()
  const descriptors = useMemo(
    () => buildAdminAnalyticsRequestDescriptors(props.search),
    [props.search]
  )
  const queries = useQueries({
    queries: descriptors.map((descriptor) => ({
      queryKey: descriptor.queryKey,
      queryFn: () => fetchDescriptor(descriptor.id, props.search),
      enabled: descriptor.enabled,
    })),
  })
  const isLoading = queries.some((query) => query.isLoading)
  const isFetching = queries.some(
    (query) => query.isFetching && !query.isLoading
  )
  const hasNetworkError = queries.some((query) => query.isError)
  const hasResponseError = queries.some((query) => panelFailed(query.data))

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('adminAnalytics.title')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('adminAnalytics.description')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <AdminAnalyticsTabs
            value={props.search.tab}
            onChange={(tab) => props.onSearchChange({ ...props.search, tab })}
          />
          <AdminAnalyticsFilterBar
            value={props.search}
            onApply={props.onSearchChange}
          />
          {isFetching ? (
            <div className='text-muted-foreground text-xs'>
              {t('adminAnalytics.refreshing')}
            </div>
          ) : null}
          {hasNetworkError || hasResponseError ? (
            <ErrorState
              title={t('adminAnalytics.failedToLoad')}
              description={t('Try adjusting the time range or filters')}
            />
          ) : null}
          <ActivePanel
            tab={props.search.tab}
            responses={queries.map((query) => query.data)}
            loading={isLoading}
            error={hasNetworkError || hasResponseError}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function AdminAnalyticsTabs(props: {
  value: AdminAnalyticsTab
  onChange: (tab: AdminAnalyticsTab) => void
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap gap-2'>
      {ADMIN_ANALYTICS_TABS.map((tab) => (
        <Button
          key={tab.id}
          variant={props.value === tab.id ? 'default' : 'outline'}
          size='sm'
          onClick={() => props.onChange(tab.id)}
        >
          {t(tab.labelKey)}
        </Button>
      ))}
    </div>
  )
}

function AdminAnalyticsFilterBar(props: {
  value: AdminAnalyticsCanonicalFilters
  onApply: (next: AdminAnalyticsCanonicalFilters) => void
}): JSX.Element {
  const { t } = useTranslation()
  const startValue = unixSecondsToDateTimeInput(props.value.start_timestamp)
  const endValue = unixSecondsToDateTimeInput(props.value.end_timestamp)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Filters')}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
          <div className='grid gap-2'>
            <Label htmlFor='admin-analytics-start-time'>
              {t('Start Time')}
            </Label>
            <Input
              id='admin-analytics-start-time'
              type='datetime-local'
              value={startValue}
              onChange={(event) => {
                const next = dateTimeInputToUnixSeconds(event.target.value)
                if (next > 0) {
                  props.onApply({ ...props.value, start_timestamp: next })
                }
              }}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='admin-analytics-end-time'>{t('End Time')}</Label>
            <Input
              id='admin-analytics-end-time'
              type='datetime-local'
              value={endValue}
              onChange={(event) => {
                const next = dateTimeInputToUnixSeconds(event.target.value)
                if (next > 0) {
                  props.onApply({ ...props.value, end_timestamp: next })
                }
              }}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='admin-analytics-top-n'>{t('Top N')}</Label>
            <Input
              id='admin-analytics-top-n'
              type='number'
              min={1}
              max={100}
              value={props.value.top_n}
              onChange={(event) => {
                props.onApply({
                  ...props.value,
                  top_n: normalizeAdminAnalyticsLimit(event.target.value),
                })
              }}
            />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function unixSecondsToDateTimeInput(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return ''
  return new Date(value * 1000).toISOString().slice(0, 16)
}

function dateTimeInputToUnixSeconds(value: string): number {
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return 0
  return Math.floor(timestamp / 1000)
}

function normalizeAdminAnalyticsLimit(value: string): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return 20
  return Math.min(Math.max(Math.trunc(parsed), 1), 100)
}

async function fetchDescriptor(
  descriptorId: string,
  filters: AdminAnalyticsCanonicalFilters
): Promise<UnknownPanelResponse> {
  switch (descriptorId) {
    case 'overview':
      return adminAnalyticsApi.overview(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'plan-distribution':
      return adminAnalyticsApi.plans(filters) as Promise<UnknownPanelResponse>
    case 'quota-distribution':
      return adminAnalyticsApi.quota(filters) as Promise<UnknownPanelResponse>
    case 'user-lifecycle':
      return adminAnalyticsApi.users(filters) as Promise<UnknownPanelResponse>
    case 'subscription-conversion':
      return adminAnalyticsApi.conversion(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'invitation-rewards':
      return adminAnalyticsApi.invitations(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'usage-consumption/summary':
      return adminAnalyticsApi.usageSummary(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'usage-consumption/timeseries':
      return adminAnalyticsApi.usageTimeseries(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'usage-consumption/breakdown':
      return adminAnalyticsApi.usageBreakdown(
        filters
      ) as Promise<UnknownPanelResponse>
    case 'risks':
      return adminAnalyticsApi.risks(filters) as Promise<UnknownPanelResponse>
    default:
      return adminAnalyticsApi.overview(
        filters
      ) as Promise<UnknownPanelResponse>
  }
}

function ActivePanel(props: {
  tab: AdminAnalyticsTab
  responses: Array<UnknownPanelResponse | undefined>
  loading: boolean
  error: boolean
}): JSX.Element {
  if (props.tab === 'usage') {
    const usageResponses: UsagePanelResponses = {
      summary: props.responses[0] as
        | PanelApiResponse<AdminUsageConsumptionSummaryResponse>
        | undefined,
      timeseries: props.responses[1] as
        | PanelApiResponse<AdminUsageTimeseriesResponse>
        | undefined,
      breakdown: props.responses[2] as
        | PanelApiResponse<AdminUsageBreakdownResponse>
        | undefined,
    }
    return (
      <PanelCard
        titleKey='adminAnalytics.tabs.usage'
        loading={props.loading}
        error={props.error}
      >
        <UsagePanel responses={usageResponses} />
      </PanelCard>
    )
  }
  const response = props.responses[0]
  return (
    <PanelCard
      titleKey={`adminAnalytics.tabs.${props.tab}`}
      loading={props.loading}
      error={props.error}
    >
      <SinglePanel tab={props.tab} response={response} />
    </PanelCard>
  )
}

function PanelCard(props: {
  titleKey: string
  loading: boolean
  error: boolean
  children: React.ReactNode
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t(props.titleKey)}</CardTitle>
      </CardHeader>
      <CardContent>
        {props.loading ? (
          <div className='text-muted-foreground text-sm'>{t('Loading...')}</div>
        ) : props.error ? (
          <ErrorState
            title={t('adminAnalytics.failedToLoad')}
            description={t('Try adjusting the time range or filters')}
          />
        ) : (
          props.children
        )}
      </CardContent>
    </Card>
  )
}

function SinglePanel(props: {
  tab: Exclude<AdminAnalyticsTab, 'usage'>
  response: UnknownPanelResponse | undefined
}): JSX.Element {
  switch (props.tab) {
    case 'plans':
      return (
        <PlansPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsPlanDistributionResponse>
          )}
        />
      )
    case 'quota':
      return (
        <QuotaPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsQuotaDistributionResponse>
          )}
        />
      )
    case 'users':
      return (
        <UsersPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsUserLifecycleResponse>
          )}
        />
      )
    case 'conversion':
      return (
        <ConversionPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsSubscriptionConversionResponse>
          )}
        />
      )
    case 'invitations':
      return (
        <InvitationsPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsInvitationRewardsResponse>
          )}
        />
      )
    case 'risks':
      return (
        <RisksPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsRisksResponse>
          )}
        />
      )
    default:
      return (
        <OverviewPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsOverviewResponse>
          )}
        />
      )
  }
}

function OverviewPanel(props: {
  data: AdminAnalyticsOverviewResponse | undefined
}): JSX.Element {
  if (!props.data) return <EmptyAnalyticsPanel />
  return (
    <MetricGrid>
      <Metric
        labelKey='adminAnalytics.metrics.totalUsers'
        value={props.data.summary.users.total_users}
      />
      <Metric
        labelKey='adminAnalytics.metrics.activeSubscriptions'
        value={props.data.summary.subscriptions.active_count}
      />
      <Metric
        labelKey='adminAnalytics.metrics.tokenUsed'
        value={props.data.summary.quota.token_used}
      />
      <Metric
        labelKey='adminAnalytics.metrics.errorRate'
        value={formatAdminPercent(props.data.summary.usage.error_rate)}
      />
      <Metric
        labelKey='adminAnalytics.metrics.rewardUsers'
        value={props.data.summary.invitations.reward_users}
      />
      <Metric
        labelKey='adminAnalytics.metrics.riskWarnings'
        value={props.data.summary.risks.warning_count}
      />
    </MetricGrid>
  )
}

function PlansPanel(props: {
  data: AdminAnalyticsPlanDistributionResponse | undefined
}): JSX.Element {
  if (!props.data || props.data.groups.items.length === 0) {
    return <EmptyAnalyticsPanel />
  }
  return (
    <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
      {props.data.groups.items.map((group) => (
        <div
          key={`${group.plan_id}:${group.source}`}
          className='rounded-md border p-3'
        >
          <div className='font-medium'>{group.plan_title}</div>
          <div className='text-muted-foreground text-sm'>
            {group.subscription_count} subscriptions
          </div>
          <div className='mt-2 text-sm'>
            {formatAdminTokens(group.token_used)} /{' '}
            {formatAdminTokens(group.token_limit)}
          </div>
          <div className='text-muted-foreground text-xs'>
            {formatAdminPercent(group.usage_rate)} used
          </div>
        </div>
      ))}
    </div>
  )
}

function QuotaPanel(props: {
  data: AdminAnalyticsQuotaDistributionResponse | undefined
}): JSX.Element {
  if (!props.data || props.data.buckets.length === 0)
    return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-4'>
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {props.data.buckets.map((bucket) => (
          <div key={bucket.bucket} className='rounded-md border p-3'>
            <div className='font-medium'>{bucket.bucket}</div>
            <div className='text-muted-foreground text-sm'>
              {bucket.subscription_count} subscriptions
            </div>
            <div className='text-muted-foreground text-xs'>
              {formatAdminPercent(bucket.usage_rate)} used
            </div>
          </div>
        ))}
      </div>
      <RankingList
        titleKey='adminAnalytics.rankings.highUsageUsers'
        items={props.data.high_usage_users.items.map((item) => ({
          key: String(item.subscription_id),
          label: item.username,
          value: formatAdminPercent(item.usage_rate),
        }))}
      />
    </div>
  )
}

function UsersPanel(props: {
  data: AdminAnalyticsUserLifecycleResponse | undefined
}): JSX.Element {
  if (!props.data) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-4'>
      <MetricGrid>
        <Metric
          labelKey='adminAnalytics.metrics.totalUsers'
          value={props.data.summary.total_users}
        />
        <Metric
          labelKey='adminAnalytics.metrics.newUsers'
          value={props.data.summary.new_users}
        />
        <Metric
          labelKey='adminAnalytics.metrics.paidUsers'
          value={props.data.summary.paid_users}
        />
        <Metric
          labelKey='adminAnalytics.metrics.rewardUsers'
          value={props.data.summary.reward_users}
        />
      </MetricGrid>
      <RankingList
        titleKey='adminAnalytics.rankings.users'
        items={props.data.users.items.map((item) => ({
          key: String(item.user_id),
          label: item.username,
          value: item.active_plan_title,
        }))}
      />
    </div>
  )
}

function ConversionPanel(props: {
  data: AdminAnalyticsSubscriptionConversionResponse | undefined
}): JSX.Element {
  if (!props.data) return <EmptyAnalyticsPanel />
  return (
    <MetricGrid>
      <Metric
        labelKey='adminAnalytics.metrics.trialUsers'
        value={props.data.summary.trial_users}
      />
      <Metric
        labelKey='adminAnalytics.metrics.paidUsers'
        value={props.data.summary.paid_users}
      />
      <Metric
        labelKey='adminAnalytics.metrics.trialToPaidRate'
        value={formatAdminPercent(props.data.summary.trial_to_paid_rate)}
      />
      <Metric
        labelKey='adminAnalytics.metrics.renewalUsers'
        value={props.data.summary.renewal_users}
      />
    </MetricGrid>
  )
}

function InvitationsPanel(props: {
  data: AdminAnalyticsInvitationRewardsResponse | undefined
}): JSX.Element {
  if (!props.data) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-4'>
      <MetricGrid>
        <Metric
          labelKey='adminAnalytics.metrics.inviters'
          value={props.data.summary.inviters_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.directInvites'
          value={props.data.summary.direct_invite_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.qualifiedInvites'
          value={props.data.summary.qualified_invite_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.rewardSubscriptions'
          value={props.data.summary.reward_subscriptions}
        />
      </MetricGrid>
      <RankingList
        titleKey='adminAnalytics.rankings.inviters'
        items={props.data.inviters.items.map((item) => ({
          key: String(item.inviter_id),
          label: item.inviter_username,
          value: formatAdminTokens(item.direct_invite_count),
        }))}
      />
    </div>
  )
}

function UsagePanel(props: { responses: UsagePanelResponses }): JSX.Element {
  const summary = panelData(props.responses.summary)
  const timeseries = panelData(props.responses.timeseries)
  const breakdown = panelData(props.responses.breakdown)
  if (!summary) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-4'>
      <MetricGrid>
        <Metric
          labelKey='adminAnalytics.metrics.requests'
          value={summary.total.request_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.tokenUsed'
          value={summary.total.total_tokens}
        />
        <Metric
          labelKey='adminAnalytics.metrics.errors'
          value={summary.total.error_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.activeApiKeys'
          value={summary.total.active_api_keys}
        />
      </MetricGrid>
      <RankingList
        titleKey='adminAnalytics.rankings.usageGroups'
        items={(breakdown?.groups.items ?? summary.groups.items).map(
          (group) => ({
            key: group.group_key,
            label: group.group_label,
            value: formatAdminTokens(group.total_tokens),
          })
        )}
      />
      {timeseries ? (
        <div className='text-muted-foreground text-xs'>
          {timeseries.points.length} trend points
        </div>
      ) : null}
    </div>
  )
}

function RisksPanel(props: {
  data: AdminAnalyticsRisksResponse | undefined
}): JSX.Element {
  if (!props.data) return <EmptyAnalyticsPanel />
  const risks = [
    ...props.data.plan_risks.items,
    ...props.data.user_risks.items,
    ...props.data.invitation_risks.items,
    ...props.data.system_risks.items,
  ]
  if (risks.length === 0) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-2'>
      {risks.map((risk) => (
        <div
          key={`${risk.category}:${risk.risk_key}`}
          className='rounded-md border p-3'
        >
          <div className='flex items-center justify-between gap-3'>
            <div className='font-medium'>{risk.title || risk.risk_key}</div>
            <div className='text-muted-foreground text-xs'>{risk.severity}</div>
          </div>
          <div className='text-muted-foreground mt-1 text-xs'>
            {risk.threshold}
          </div>
          <div className='mt-2 text-sm'>{risk.description}</div>
        </div>
      ))}
    </div>
  )
}

function MetricGrid(props: { children: React.ReactNode }): JSX.Element {
  return (
    <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
      {props.children}
    </div>
  )
}

function Metric(props: {
  labelKey: string
  value: number | string
}): JSX.Element {
  const { t } = useTranslation()
  return (
    <div className='rounded-md border p-3'>
      <div className='text-muted-foreground text-xs'>{t(props.labelKey)}</div>
      <div className='text-lg font-semibold'>
        {typeof props.value === 'number'
          ? formatAdminTokens(props.value)
          : props.value}
      </div>
    </div>
  )
}

function RankingList(props: {
  titleKey: string
  items: Array<{ key: string; label: string; value: string }>
}): JSX.Element {
  const { t } = useTranslation()
  if (props.items.length === 0) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-2'>
      <div className='text-sm font-medium'>{t(props.titleKey)}</div>
      {props.items.slice(0, 10).map((item) => (
        <div
          key={item.key}
          className='flex items-center justify-between gap-4 rounded-md border px-3 py-2 text-sm'
        >
          <span className='truncate'>{item.label || item.key}</span>
          <span className='text-muted-foreground shrink-0'>{item.value}</span>
        </div>
      ))}
    </div>
  )
}

function EmptyAnalyticsPanel(): JSX.Element {
  const { t } = useTranslation()
  return <div className='text-muted-foreground text-sm'>{t('No data')}</div>
}
