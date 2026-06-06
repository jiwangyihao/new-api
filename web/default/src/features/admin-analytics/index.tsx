import { useEffect, useMemo, type JSX, type ReactNode } from 'react'
import { useQueries } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Label } from '@/components/ui/label'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { ADMIN_ANALYTICS_TABS } from './constants'
import { buildAdminAnalyticsDrilldown } from './lib/drilldown'
import {
  formatAdminMoneyAmount,
  formatAdminMoneyBreakdown,
  formatAdminPercent,
  formatAdminTokens,
} from './lib/format'
import {
  buildAdminAnalyticsRequestDescriptors,
  fetchAdminAnalyticsDescriptor,
} from './lib/page-contract'
import { switchAdminAnalyticsTab } from './lib/filters'
import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsDrilldownTarget,
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
  InvitationPaidSubscriptionsResponse,
  PaidSubscriptionValueResponse,
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
type DrilldownHandler = (
  target: AdminAnalyticsDrilldownTarget | null | undefined
) => void

type UsagePanelResponses = {
  summary?: PanelApiResponse<AdminUsageConsumptionSummaryResponse>
  timeseries?: PanelApiResponse<AdminUsageTimeseriesResponse>
  breakdown?: PanelApiResponse<AdminUsageBreakdownResponse>
}

type PaidSubscriptionValuePanelResponses = {
  summary?: PanelApiResponse<PaidSubscriptionValueResponse>
  users?: PanelApiResponse<PaidSubscriptionValueResponse>
  subscriptions?: PanelApiResponse<PaidSubscriptionValueResponse>
  plans?: PanelApiResponse<PaidSubscriptionValueResponse>
  sources?: PanelApiResponse<PaidSubscriptionValueResponse>
}

type InvitationPaidSubscriptionsPanelResponses = {
  summary?: PanelApiResponse<InvitationPaidSubscriptionsResponse>
  inviters?: PanelApiResponse<InvitationPaidSubscriptionsResponse>
  invitees?: PanelApiResponse<InvitationPaidSubscriptionsResponse>
  subscriptions?: PanelApiResponse<InvitationPaidSubscriptionsResponse>
}

function isPaidSubscriptionAnalyticsTab(tab: AdminAnalyticsTab): boolean {
  return (
    tab === 'paid-subscription-value' ||
    tab === 'invitation-paid-subscriptions'
  )
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
      queryFn: () => fetchAdminAnalyticsDescriptor(descriptor, props.search),
      enabled: descriptor.enabled,
    })),
  })
  const isLoading = queries.some((query) => query.isLoading)
  const isFetching = queries.some(
    (query) => query.isFetching && !query.isLoading
  )
  const hasNetworkError = queries.some((query) => query.isError)
  const hasResponseError = queries.some((query) => panelFailed(query.data))
  const summarySnapshot = hasPanelData(queries[0]?.data)
    ? queries[0].data.data.range.snapshot_at
    : undefined

  useEffect(() => {
    if (!isPaidSubscriptionAnalyticsTab(props.search.tab)) return
    if (props.search.snapshot_at !== undefined) return
    if (summarySnapshot === undefined) return
    props.onSearchChange({ ...props.search, snapshot_at: summarySnapshot })
  }, [props.search, props.onSearchChange, summarySnapshot])

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
            onChange={(tab) =>
              props.onSearchChange(switchAdminAnalyticsTab(props.search, tab))
            }
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
            filters={props.search}
            onDrilldown={props.onDrilldown}
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
      <CardContent className='space-y-3'>
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
                if (next === undefined) return
                props.onApply({
                  ...props.value,
                  start_timestamp: next,
                  time_range_explicit: isPaidSubscriptionAnalyticsTab(
                    props.value.tab
                  )
                    ? true
                    : props.value.time_range_explicit,
                })
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
                if (next === undefined) return
                props.onApply({
                  ...props.value,
                  end_timestamp: next,
                  time_range_explicit: isPaidSubscriptionAnalyticsTab(
                    props.value.tab
                  )
                    ? true
                    : props.value.time_range_explicit,
                })
              }}
            />
          </div>
          {isPaidSubscriptionAnalyticsTab(props.value.tab) ? (
            <>
              <div className='grid gap-2'>
                <Label htmlFor='admin-analytics-time-range-explicit'>
                  {t('adminAnalytics.filters.timeRangeExplicit')}
                </Label>
                <label className='flex h-8 items-center gap-2 text-sm'>
                  <input
                    id='admin-analytics-time-range-explicit'
                    type='checkbox'
                    checked={props.value.time_range_explicit}
                    onChange={(event) =>
                      props.onApply({
                        ...props.value,
                        time_range_explicit: event.target.checked,
                      })
                    }
                  />
                  {t('adminAnalytics.filters.enableTimeRange')}
                </label>
              </div>
              <div className='grid gap-2'>
                <Label htmlFor='admin-analytics-excluded-mode'>
                  {t('adminAnalytics.filters.excludedMode')}
                </Label>
                <NativeSelect
                  id='admin-analytics-excluded-mode'
                  value={props.value.excluded_mode}
                  onChange={(event) =>
                    props.onApply({
                      ...props.value,
                      excluded_mode: event.target
                        .value as AdminAnalyticsCanonicalFilters['excluded_mode'],
                    })
                  }
                >
                  <NativeSelectOption value='included_only'>
                    {t('adminAnalytics.filters.excludedMode.includedOnly')}
                  </NativeSelectOption>
                  <NativeSelectOption value='include_excluded'>
                    {t('adminAnalytics.filters.excludedMode.includeExcluded')}
                  </NativeSelectOption>
                  <NativeSelectOption value='excluded_only'>
                    {t('adminAnalytics.filters.excludedMode.excludedOnly')}
                  </NativeSelectOption>
                </NativeSelect>
              </div>
              <div className='grid gap-2'>
                <Label htmlFor='admin-analytics-currency'>
                  {t('adminAnalytics.filters.currency')}
                </Label>
                <Input
                  id='admin-analytics-currency'
                  value={props.value.currency ?? ''}
                  placeholder='CNY'
                  onChange={(event) =>
                    props.onApply({
                      ...props.value,
                      currency:
                        event.target.value.trim().toUpperCase() || undefined,
                    })
                  }
                />
              </div>
              <div className='grid gap-2'>
                <Label htmlFor='admin-analytics-active-only'>
                  {t('adminAnalytics.filters.activeOnly')}
                </Label>
                <label className='flex h-8 items-center gap-2 text-sm'>
                  <input
                    id='admin-analytics-active-only'
                    type='checkbox'
                    checked={props.value.active_only}
                    onChange={(event) =>
                      props.onApply({
                        ...props.value,
                        active_only: event.target.checked,
                      })
                    }
                  />
                  {t('adminAnalytics.filters.activeOnlyEnabled')}
                </label>
              </div>
              <div className='grid gap-2'>
                <Label htmlFor='admin-analytics-snapshot-at'>
                  {t('adminAnalytics.filters.snapshotAt')}
                </Label>
                <Input
                  id='admin-analytics-snapshot-at'
                  type='datetime-local'
                  value={
                    props.value.snapshot_at === undefined
                      ? ''
                      : unixSecondsToDateTimeInput(props.value.snapshot_at)
                  }
                  onChange={(event) => {
                    const next = dateTimeInputToUnixSeconds(event.target.value)
                    props.onApply({
                      ...props.value,
                      snapshot_at: next,
                    })
                  }}
                />
              </div>
            </>
          ) : (
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
          )}
        </div>
        <ActiveFilterSummary value={props.value} onApply={props.onApply} />
      </CardContent>
    </Card>
  )
}

function ActiveFilterSummary(props: {
  value: AdminAnalyticsCanonicalFilters
  onApply: (next: AdminAnalyticsCanonicalFilters) => void
}): JSX.Element | null {
  const { t } = useTranslation()
  if (!isPaidSubscriptionAnalyticsTab(props.value.tab)) return null
  const chips: Array<{ key: string; label: string; clear: () => void }> = []
  if (props.value.user_ids.length > 0) {
    chips.push({
      key: 'user_ids',
      label: `${t('adminAnalytics.filters.userIds')}: ${props.value.user_ids.join(', ')}`,
      clear: () => props.onApply({ ...props.value, user_ids: [] }),
    })
  }
  if (props.value.plan_ids.length > 0) {
    chips.push({
      key: 'plan_ids',
      label: `${t('adminAnalytics.filters.planIds')}: ${props.value.plan_ids.join(', ')}`,
      clear: () => props.onApply({ ...props.value, plan_ids: [] }),
    })
  }
  if (props.value.inviter_id !== undefined) {
    chips.push({
      key: 'inviter_id',
      label: `${t('adminAnalytics.filters.inviterId')}: ${props.value.inviter_id}`,
      clear: () => props.onApply({ ...props.value, inviter_id: undefined }),
    })
  }
  if (props.value.invitee_id !== undefined) {
    chips.push({
      key: 'invitee_id',
      label: `${t('adminAnalytics.filters.inviteeId')}: ${props.value.invitee_id}`,
      clear: () => props.onApply({ ...props.value, invitee_id: undefined }),
    })
  }
  if (props.value.subscription_id !== undefined) {
    chips.push({
      key: 'subscription_id',
      label: `${t('adminAnalytics.filters.subscriptionId')}: ${props.value.subscription_id}`,
      clear: () => props.onApply({ ...props.value, subscription_id: undefined }),
    })
  }
  if (chips.length === 0) return null
  return (
    <div className='flex flex-wrap gap-2'>
      {chips.map((chip) => (
        <Button key={chip.key} type='button' variant='outline' size='xs' onClick={chip.clear}>
          {chip.label} · {t('adminAnalytics.filters.clear')}
        </Button>
      ))}
    </div>
  )
}

function unixSecondsToDateTimeInput(value: number): string {
  if (!Number.isFinite(value) || value < 0) return ''
  return new Date(value * 1000).toISOString().slice(0, 16)
}

function dateTimeInputToUnixSeconds(value: string): number | undefined {
  const trimmed = value.trim()
  if (trimmed === '') return undefined
  const timestamp = new Date(trimmed).getTime()
  if (Number.isNaN(timestamp)) return undefined
  return Math.floor(timestamp / 1000)
}

function normalizeAdminAnalyticsLimit(value: string): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return 20
  return Math.min(Math.max(Math.trunc(parsed), 1), 100)
}


function adminAnalyticsTabLabelKey(tab: AdminAnalyticsTab): string {
  return (
    ADMIN_ANALYTICS_TABS.find((item) => item.id === tab)?.labelKey ??
    'adminAnalytics.tabs.overview'
  )
}

function ActivePanel(props: {
  tab: AdminAnalyticsTab
  responses: Array<UnknownPanelResponse | undefined>
  loading: boolean
  error: boolean
  filters: AdminAnalyticsCanonicalFilters
  onDrilldown: (target: FrontendAdminAnalyticsDrilldownTarget) => void
}): JSX.Element {
  const handleDrilldown: DrilldownHandler = (target) => {
    const frontendTarget = buildAdminAnalyticsDrilldown(props.filters, target)
    if (frontendTarget) props.onDrilldown(frontendTarget)
  }

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
        <UsagePanel responses={usageResponses} onDrilldown={handleDrilldown} />
      </PanelCard>
    )
  }

  if (props.tab === 'paid-subscription-value') {
    const responses: PaidSubscriptionValuePanelResponses = {
      summary: props.responses[0] as
        | PanelApiResponse<PaidSubscriptionValueResponse>
        | undefined,
      users: props.responses[1] as
        | PanelApiResponse<PaidSubscriptionValueResponse>
        | undefined,
      subscriptions: props.responses[2] as
        | PanelApiResponse<PaidSubscriptionValueResponse>
        | undefined,
      plans: props.responses[3] as
        | PanelApiResponse<PaidSubscriptionValueResponse>
        | undefined,
      sources: props.responses[4] as
        | PanelApiResponse<PaidSubscriptionValueResponse>
        | undefined,
    }
    return (
      <PanelCard
        titleKey='adminAnalytics.tabs.paidSubscriptionValue'
        loading={props.loading}
        error={props.error}
      >
        <PaidSubscriptionValuePanel
          responses={responses}
          onDrilldown={handleDrilldown}
        />
      </PanelCard>
    )
  }

  if (props.tab === 'invitation-paid-subscriptions') {
    const responses: InvitationPaidSubscriptionsPanelResponses = {
      summary: props.responses[0] as
        | PanelApiResponse<InvitationPaidSubscriptionsResponse>
        | undefined,
      inviters: props.responses[1] as
        | PanelApiResponse<InvitationPaidSubscriptionsResponse>
        | undefined,
      invitees: props.responses[2] as
        | PanelApiResponse<InvitationPaidSubscriptionsResponse>
        | undefined,
      subscriptions: props.responses[3] as
        | PanelApiResponse<InvitationPaidSubscriptionsResponse>
        | undefined,
    }
    return (
      <PanelCard
        titleKey='adminAnalytics.tabs.invitationPaidSubscriptions'
        loading={props.loading}
        error={props.error}
      >
        <InvitationPaidSubscriptionsPanel
          responses={responses}
          onDrilldown={handleDrilldown}
        />
      </PanelCard>
    )
  }

  const response = props.responses[0]
  return (
    <PanelCard
      titleKey={adminAnalyticsTabLabelKey(props.tab)}
      loading={props.loading}
      error={props.error}
    >
      <SinglePanel
        tab={props.tab}
        response={response}
        onDrilldown={handleDrilldown}
      />
    </PanelCard>
  )
}

function PanelCard(props: {
  titleKey: string
  loading: boolean
  error: boolean
  children: ReactNode
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
  onDrilldown: DrilldownHandler
}): JSX.Element {
  switch (props.tab) {
    case 'plans':
      return (
        <PlansPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsPlanDistributionResponse>
          )}
          onDrilldown={props.onDrilldown}
        />
      )
    case 'quota':
      return (
        <QuotaPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsQuotaDistributionResponse>
          )}
          onDrilldown={props.onDrilldown}
        />
      )
    case 'users':
      return (
        <UsersPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsUserLifecycleResponse>
          )}
          onDrilldown={props.onDrilldown}
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
          onDrilldown={props.onDrilldown}
        />
      )
    case 'paid-subscription-value':
    case 'invitation-paid-subscriptions':
      return <EmptyAnalyticsPanel />
    case 'risks':
      return (
        <RisksPanel
          data={panelData(
            props.response as PanelApiResponse<AdminAnalyticsRisksResponse>
          )}
          onDrilldown={props.onDrilldown}
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
  onDrilldown: DrilldownHandler
}): JSX.Element {
  if (!props.data || props.data.groups.items.length === 0) {
    return <EmptyAnalyticsPanel />
  }
  return (
    <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
      {props.data.groups.items.map((group) => (
        <DrilldownCard
          key={`${group.plan_id}:${group.source}`}
          target={group.drilldown}
          onDrilldown={props.onDrilldown}
          className='rounded-md border p-3 text-left'
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
        </DrilldownCard>
      ))}
    </div>
  )
}

function QuotaPanel(props: {
  data: AdminAnalyticsQuotaDistributionResponse | undefined
  onDrilldown: DrilldownHandler
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
          drilldown: item.drilldown,
        }))}
        onDrilldown={props.onDrilldown}
      />
    </div>
  )
}

function UsersPanel(props: {
  data: AdminAnalyticsUserLifecycleResponse | undefined
  onDrilldown: DrilldownHandler
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
          drilldown: item.drilldown,
        }))}
        onDrilldown={props.onDrilldown}
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
  onDrilldown: DrilldownHandler
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
          drilldown: item.drilldown,
        }))}
        onDrilldown={props.onDrilldown}
      />
    </div>
  )
}

function PaidSubscriptionValuePanel(props: {
  responses: PaidSubscriptionValuePanelResponses
  onDrilldown: DrilldownHandler
}): JSX.Element {
  const { t } = useTranslation()
  const summary = panelData(props.responses.summary)?.summary
  const users = panelData(props.responses.users)?.users?.items ?? []
  const subscriptions =
    panelData(props.responses.subscriptions)?.subscriptions?.items ?? []
  const plans = panelData(props.responses.plans)?.plans?.items ?? []
  const sources = panelData(props.responses.sources)?.sources?.items ?? []
  if (!summary) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-4'>
      <MetricGrid>
        <Metric
          labelKey='adminAnalytics.metrics.remainingValue'
          value={formatAdminMoneyBreakdown(
            summary.recognized_remaining_value_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.tokenBasedValue'
          value={formatAdminMoneyBreakdown(summary.token_based_value_by_currency)}
        />
        <Metric
          labelKey='adminAnalytics.metrics.timeBasedValue'
          value={formatAdminMoneyBreakdown(summary.time_based_value_by_currency)}
        />
        <Metric
          labelKey='adminAnalytics.metrics.excludedAuditValue'
          value={formatAdminMoneyBreakdown(
            summary.excluded_remaining_value_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.activePaidSubscriptions'
          value={summary.active_paid_subscription_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.activePaidUsers'
          value={summary.active_paid_user_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.tokenValueUnavailable'
          value={summary.token_value_unavailable_count}
        />
      </MetricGrid>
      <RankingList
        titleKey='adminAnalytics.rankings.paidSubscriptionUsers'
        items={users.map((item) => ({
          key: String(item.user_id),
          label: item.username || String(item.user_id),
          value: formatAdminMoneyBreakdown(
            item.recognized_remaining_value_by_currency
          ),
          drilldown: item.drilldown,
        }))}
        onDrilldown={props.onDrilldown}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.paidSubscriptionPlans'
        items={plans.map((item) => ({
          key: String(item.plan_id),
          title: item.plan_name || String(item.plan_id),
          description: item.plan_business_code,
          values: [
            {
              labelKey: 'adminAnalytics.metrics.remainingValue',
              value: formatAdminMoneyBreakdown(
                item.recognized_remaining_value_by_currency
              ),
            },
            {
              labelKey: 'adminAnalytics.metrics.activePaidUsers',
              value: formatAdminTokens(item.active_user_count),
            },
            {
              labelKey: 'adminAnalytics.metrics.activePaidSubscriptions',
              value: formatAdminTokens(item.active_subscription_count),
            },
          ],
        }))}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.paidSubscriptionSources'
        items={sources.map((item) => ({
          key: `${item.source}:${item.grant_reason}`,
          title: item.source || t('Unknown'),
          description: item.grant_reason || item.source_attribution,
          values: [
            {
              labelKey: 'adminAnalytics.metrics.remainingValue',
              value: formatAdminMoneyBreakdown(
                item.recognized_remaining_value_by_currency
              ),
            },
            {
              labelKey: 'adminAnalytics.metrics.excludedAuditValue',
              value: formatAdminMoneyBreakdown(
                item.excluded_remaining_value_by_currency
              ),
            },
            {
              labelKey: 'adminAnalytics.metrics.subscriptionCount',
              value: formatAdminTokens(item.subscription_count),
            },
          ],
        }))}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.paidSubscriptionRecords'
        items={subscriptions.map((item) => ({
          key: String(item.subscription_id),
          title: `${item.username || item.user_id} · ${item.plan_name}`,
          description: `${item.source} / ${item.grant_reason}`,
          drilldown: item.drilldown,
          values: [
            {
              labelKey: 'adminAnalytics.fields.planPrice',
              value: formatAdminMoneyAmount(item.plan_price),
            },
            {
              labelKey: 'adminAnalytics.fields.recognizedRemainingValue',
              value: formatAdminMoneyAmount(item.recognized_remaining_value),
            },
            {
              labelKey: 'adminAnalytics.fields.valuationBasis',
              value: item.valuation_basis,
            },
            {
              labelKey: 'adminAnalytics.fields.sourceAttribution',
              value: item.source_attribution,
            },
            {
              labelKey: 'adminAnalytics.fields.orderRecordedAmount',
              value: formatAdminMoneyAmount(item.order_recorded_amount),
            },
            {
              labelKey: 'adminAnalytics.fields.excludedReason',
              value: item.excluded ? item.excluded_reason || '—' : '—',
            },
          ],
        }))}
        onDrilldown={props.onDrilldown}
      />
    </div>
  )
}

function InvitationPaidSubscriptionsPanel(props: {
  responses: InvitationPaidSubscriptionsPanelResponses
  onDrilldown: DrilldownHandler
}): JSX.Element {
  const summary = panelData(props.responses.summary)?.summary
  const inviters = panelData(props.responses.inviters)?.inviters?.items ?? []
  const invitees = panelData(props.responses.invitees)?.invitees?.items ?? []
  const subscriptions =
    panelData(props.responses.subscriptions)?.subscriptions?.items ?? []
  if (!summary) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-4'>
      <MetricGrid>
        <Metric
          labelKey='adminAnalytics.metrics.invitationPaidAmount'
          value={formatAdminMoneyBreakdown(
            summary.recognized_invitation_paid_amount_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.activeInvitationPaidAmount'
          value={formatAdminMoneyBreakdown(
            summary.active_invitation_paid_amount_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.activeInvitationRemainingValue'
          value={formatAdminMoneyBreakdown(
            summary.active_invitation_remaining_value_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.excludedInvitationPaidAmount'
          value={formatAdminMoneyBreakdown(
            summary.excluded_invitation_paid_amount_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.excludedActiveRemainingValue'
          value={formatAdminMoneyBreakdown(
            summary.excluded_active_remaining_value_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.inviterCount'
          value={summary.inviter_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.inviteeCount'
          value={summary.invitee_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.paidInviteeCount'
          value={summary.paid_invitee_count}
        />
        <Metric
          labelKey='adminAnalytics.metrics.activePaidInviteeCount'
          value={summary.active_paid_invitee_count}
        />
      </MetricGrid>
      <RankingList
        titleKey='adminAnalytics.rankings.invitationPaidInviters'
        items={inviters.map((item) => ({
          key: String(item.inviter_user_id),
          label: item.inviter_username || String(item.inviter_user_id),
          value: formatAdminMoneyBreakdown(
            item.recognized_invitation_paid_amount_by_currency
          ),
          drilldown: item.drilldown,
        }))}
        onDrilldown={props.onDrilldown}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.invitationPaidInvitees'
        items={invitees.map((item) => ({
          key: String(item.invitee_user_id),
          title: item.invitee_username || String(item.invitee_user_id),
          description: `#${item.inviter_user_id}`,
          drilldown: item.drilldown,
          values: [
            {
              labelKey: 'adminAnalytics.metrics.invitationPaidAmount',
              value: formatAdminMoneyBreakdown(
                item.recognized_paid_amount_by_currency
              ),
            },
            {
              labelKey: 'adminAnalytics.metrics.activeInvitationRemainingValue',
              value: formatAdminMoneyBreakdown(
                item.active_remaining_value_by_currency
              ),
            },
            {
              labelKey: 'adminAnalytics.metrics.activePaidSubscriptions',
              value: formatAdminTokens(item.active_paid_subscription_count),
            },
            {
              labelKey: 'adminAnalytics.fields.excludedReason',
              value: item.excluded ? item.excluded_reason || '—' : '—',
            },
          ],
        }))}
        onDrilldown={props.onDrilldown}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.invitationPaidRecords'
        items={subscriptions.map((item) => ({
          key: String(item.subscription_id),
          title: `${item.invitee_user_id} · ${item.plan_name}`,
          description: `${item.source} / ${item.grant_reason}`,
          drilldown: item.drilldown,
          values: [
            {
              labelKey: 'adminAnalytics.fields.planPrice',
              value: formatAdminMoneyAmount(item.plan_price),
            },
            {
              labelKey: 'adminAnalytics.fields.confirmationUnits',
              value: String(item.recognized_paid_units),
            },
            {
              labelKey: 'adminAnalytics.fields.invitationPaidAmount',
              value: formatAdminMoneyAmount(item.recognized_paid_amount),
            },
            {
              labelKey: 'adminAnalytics.fields.unitInferenceBasis',
              value: item.unit_inference_basis,
            },
            {
              labelKey: 'adminAnalytics.fields.sourceAttribution',
              value: item.source_attribution,
            },
            {
              labelKey: 'adminAnalytics.fields.orderRecordedAmount',
              value: formatAdminMoneyAmount(item.order_recorded_amount),
            },
          ],
        }))}
        onDrilldown={props.onDrilldown}
      />
    </div>
  )
}

function AnalyticsCardGrid(props: {
  titleKey: string
  items: Array<{
    key: string
    title: string
    description?: string
    values: Array<{ labelKey: string; value: string }>
    drilldown?: AdminAnalyticsDrilldownTarget | null
  }>
  onDrilldown?: DrilldownHandler
}): JSX.Element {
  const { t } = useTranslation()
  if (props.items.length === 0) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-2'>
      <div className='text-sm font-medium'>{t(props.titleKey)}</div>
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {props.items.slice(0, 12).map((item) => (
          <DrilldownCard
            key={item.key}
            target={item.drilldown}
            onDrilldown={props.onDrilldown}
            className='rounded-md border p-3 text-left text-sm'
          >
            <div className='font-medium'>{item.title}</div>
            {item.description ? (
              <div className='text-muted-foreground text-xs'>
                {item.description}
              </div>
            ) : null}
            <dl className='mt-2 space-y-1'>
              {item.values.map((value) => (
                <div
                  key={value.labelKey}
                  className='grid grid-cols-[minmax(0,1fr)_auto] gap-3'
                >
                  <dt className='text-muted-foreground truncate'>
                    {t(value.labelKey)}
                  </dt>
                  <dd className='text-right'>{value.value || '—'}</dd>
                </div>
              ))}
            </dl>
          </DrilldownCard>
        ))}
      </div>
    </div>
  )
}

function UsagePanel(props: {
  responses: UsagePanelResponses
  onDrilldown: DrilldownHandler
}): JSX.Element {
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
            drilldown: group.drilldown,
          })
        )}
        onDrilldown={props.onDrilldown}
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
  onDrilldown: DrilldownHandler
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
        <DrilldownCard
          key={`${risk.category}:${risk.risk_key}`}
          target={risk.drilldown}
          onDrilldown={props.onDrilldown}
          className='rounded-md border p-3 text-left'
        >
          <div className='flex items-center justify-between gap-3'>
            <div className='font-medium'>{risk.title || risk.risk_key}</div>
            <div className='text-muted-foreground text-xs'>{risk.severity}</div>
          </div>
          <div className='text-muted-foreground mt-1 text-xs'>
            {risk.threshold}
          </div>
          <div className='mt-2 text-sm'>{risk.description}</div>
        </DrilldownCard>
      ))}
    </div>
  )
}

function MetricGrid(props: { children: ReactNode }): JSX.Element {
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
  items: Array<{
    key: string
    label: string
    value: string
    drilldown?: AdminAnalyticsDrilldownTarget | null
  }>
  onDrilldown?: DrilldownHandler
}): JSX.Element {
  const { t } = useTranslation()
  if (props.items.length === 0) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-2'>
      <div className='text-sm font-medium'>{t(props.titleKey)}</div>
      {props.items.slice(0, 10).map((item) => (
        <DrilldownCard
          key={item.key}
          target={item.drilldown}
          onDrilldown={props.onDrilldown}
          className='flex items-center justify-between gap-4 rounded-md border px-3 py-2 text-left text-sm'
        >
          <span className='truncate'>{item.label || item.key}</span>
          <span className='text-muted-foreground shrink-0'>{item.value}</span>
        </DrilldownCard>
      ))}
    </div>
  )
}

function isSupportedDrilldownTarget(
  target: AdminAnalyticsDrilldownTarget | null | undefined
): target is AdminAnalyticsDrilldownTarget {
  switch (target?.kind) {
    case 'admin_users':
    case 'admin_usage_logs':
    case 'admin_subscriptions':
    case 'admin_invitations':
    case 'paid_subscription_value_user':
    case 'paid_subscription_value_subscription':
    case 'invitation_paid_inviter':
    case 'invitation_paid_invitee':
      return true
    default:
      return false
  }
}

function DrilldownCard(props: {
  target: AdminAnalyticsDrilldownTarget | null | undefined
  onDrilldown: DrilldownHandler | undefined
  className: string
  children: ReactNode
}): JSX.Element {
  if (!isSupportedDrilldownTarget(props.target) || !props.onDrilldown) {
    return <div className={props.className}>{props.children}</div>
  }
  return (
    <button
      type='button'
      className={`${props.className} hover:bg-muted/50 focus-visible:ring-ring w-full cursor-pointer transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none`}
      onClick={() => props.onDrilldown?.(props.target)}
    >
      {props.children}
    </button>
  )
}

function EmptyAnalyticsPanel(): JSX.Element {
  const { t } = useTranslation()
  return <div className='text-muted-foreground text-sm'>{t('No data')}</div>
}
