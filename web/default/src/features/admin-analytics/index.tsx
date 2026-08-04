import { useEffect, useMemo, type JSX, type ReactNode } from 'react'
import { useQueries } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { ADMIN_ANALYTICS_MAX_LIMIT, ADMIN_ANALYTICS_TABS } from './constants'
import { buildAdminAnalyticsDrilldown } from './lib/drilldown'
import {
  enableAdminAnalyticsAllRows,
  enableAdminAnalyticsPagedRows,
  switchAdminAnalyticsTab,
} from './lib/filters'
import {
  formatAdminMoneyBreakdown,
  formatAdminPercent,
  formatAdminTokens,
} from './lib/format'
import {
  buildAdminAnalyticsRequestDescriptors,
  fetchAdminAnalyticsDescriptor,
} from './lib/page-contract'
import {
  adminAnalyticsCreditOverviewValues,
  adminAnalyticsLifecycleLabelKeys,
  adminAnalyticsCreditRankingValue,
  adminAnalyticsSubscriptionHistoryValues,
  invitationPaidInviteeCardValues,
  invitationPaidInviterCardValues,
  invitationPaidSubscriptionCardValues,
  paidSubscriptionValuePlanCardValues,
  paidSubscriptionValueSourceCardValues,
  paidSubscriptionValueSubscriptionCardValues,
  paidSubscriptionValueUserCardValues,
} from './lib/panel-fields'
import type {
  AdminAnalyticsCanonicalFilters,
  AdminAnalyticsDrilldownTarget,
  AdminAnalyticsDrilldownSubscriptionsResponse,
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

type ConversionPanelResponses = {
  summary?: PanelApiResponse<AdminAnalyticsSubscriptionConversionResponse>
  subscriptions?: PanelApiResponse<AdminAnalyticsDrilldownSubscriptionsResponse>
}

type InvitationPaidSubscriptionsPanelResponses = {
  summary?: PanelApiResponse<InvitationPaidSubscriptionsResponse>
  inviters?: PanelApiResponse<InvitationPaidSubscriptionsResponse>
  invitees?: PanelApiResponse<InvitationPaidSubscriptionsResponse>
  subscriptions?: PanelApiResponse<InvitationPaidSubscriptionsResponse>
}

function isPaidSubscriptionAnalyticsTab(tab: AdminAnalyticsTab): boolean {
  return (
    tab === 'paid-subscription-value' || tab === 'invitation-paid-subscriptions'
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
  const { search, onSearchChange } = props
  const descriptors = useMemo(
    () => buildAdminAnalyticsRequestDescriptors(search),
    [search]
  )
  const queries = useQueries({
    queries: descriptors.map((descriptor) => ({
      queryKey: [...descriptor.queryKey, search],
      queryFn: () => fetchAdminAnalyticsDescriptor(descriptor, search),
      enabled: descriptor.enabled,
    })),
  })
  const isLoading = queries.some((query) => query.isLoading)
  const isFetching = queries.some(
    (query) => query.isFetching && !query.isLoading
  )
  const hasNetworkError = queries.some((query) => query.isError)
  const hasResponseError = queries.some((query) => panelFailed(query.data))
  const queryLoadingStates = queries.map((query) => query.isLoading)
  const queryErrorStates = queries.map(
    (query) => query.isError || panelFailed(query.data)
  )
  const summarySnapshot = hasPanelData(queries[0]?.data)
    ? queries[0].data.data.range.snapshot_at
    : undefined

  useEffect(() => {
    if (!isPaidSubscriptionAnalyticsTab(search.tab)) return
    if (search.snapshot_at !== undefined) return
    if (summarySnapshot === undefined) return
    onSearchChange({ ...search, snapshot_at: summarySnapshot })
  }, [search, onSearchChange, summarySnapshot])

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
            value={search.tab}
            onChange={(tab) =>
              onSearchChange(switchAdminAnalyticsTab(search, tab))
            }
          />
          <AdminAnalyticsFilterBar
            value={search}
            onApply={onSearchChange}
          />
          {isFetching ? (
            <div
              className='text-muted-foreground text-xs'
              role='status'
              aria-live='polite'
            >
              {t('adminAnalytics.refreshing')}
            </div>
          ) : null}
          {search.tab !== 'conversion' &&
          (hasNetworkError || hasResponseError) ? (
            <ErrorState
              title={t('adminAnalytics.failedToLoad')}
              description={t('Try adjusting the time range or filters')}
            />
          ) : null}
          <ActivePanel
            tab={search.tab}
            responses={queries.map((query) => query.data)}
            loading={isLoading}
            error={hasNetworkError || hasResponseError}
            queryLoadingStates={queryLoadingStates}
            queryErrorStates={queryErrorStates}
            filters={search}
            onDrilldown={props.onDrilldown}
            onRefreshCurrentSnapshot={() =>
              onSearchChange({ ...search, snapshot_at: undefined })
            }
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
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  render={
                    <Link
                      to='/system-settings/billing/$section'
                      params={{ section: 'statistics' }}
                    />
                  }
                >
                  {t('adminAnalytics.filters.manageExcludedUsers')}
                </Button>
              </div>
              <div className='grid gap-2'>
                <Label htmlFor='admin-analytics-currency'>
                  {t('adminAnalytics.filters.currency')}
                </Label>
                <Input
                  id='admin-analytics-currency'
                  value={props.value.currency ?? ''}
                  placeholder='CNY'
                  onChange={(event) => {
                    const nextCurrency =
                      event.target.value.trim().toUpperCase() || undefined
                    props.onApply({
                      ...props.value,
                      currency: nextCurrency,
                      sort_by:
                        nextCurrency === undefined &&
                        props.value.sort_by !== undefined &&
                        paidAnalyticsMoneySortFields.has(props.value.sort_by)
                          ? undefined
                          : props.value.sort_by,
                    })
                  }}
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
              <PaidAnalyticsRowControls
                value={props.value}
                onApply={props.onApply}
              />
              <PaidAnalyticsSortControls
                value={props.value}
                onApply={props.onApply}
              />
            </>
          ) : (
            <div className='grid gap-2'>
              <Label htmlFor='admin-analytics-top-n'>{t('Top N')}</Label>
              <Input
                id='admin-analytics-top-n'
                type='number'
                min={0}
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

const paidAnalyticsMoneySortFields = new Set([
  'recognized_remaining_value',
  'plan_price',
  'recognized_invitation_paid_amount',
  'active_invitation_paid_amount',
  'active_invitation_remaining_value',
  'recognized_paid_amount',
  'active_remaining_value',
])

const paidValueSortOptions = [
  'recognized_remaining_value',
  'active_paid_plan_count',
  'earliest_end_time',
  'user_id',
  'end_time',
  'start_time',
  'plan_price',
  'subscription_id',
  'subscription_count',
  'user_count',
  'source',
  'grant_reason',
] as const

const invitationPaidSortOptions = [
  'recognized_invitation_paid_amount',
  'active_invitation_paid_amount',
  'active_invitation_remaining_value',
  'paid_invitee_count',
  'active_paid_invitee_count',
  'inviter_user_id',
  'recognized_paid_amount',
  'active_remaining_value',
  'paid_subscription_snapshot_count',
  'registered_at',
  'invitee_user_id',
  'recognized_remaining_value',
  'start_time',
  'end_time',
  'plan_price',
  'subscription_id',
] as const

function PaidAnalyticsSortControls(props: {
  value: AdminAnalyticsCanonicalFilters
  onApply: (next: AdminAnalyticsCanonicalFilters) => void
}): JSX.Element {
  const { t } = useTranslation()
  const options =
    props.value.tab === 'invitation-paid-subscriptions'
      ? invitationPaidSortOptions
      : paidValueSortOptions

  return (
    <div className='grid gap-2 lg:col-span-2'>
      <Label>{t('adminAnalytics.sort.field')}</Label>
      <div className='flex flex-wrap items-center gap-2'>
        <NativeSelect
          value={props.value.sort_by ?? ''}
          onChange={(event) => {
            const sortBy = event.target.value || undefined
            props.onApply({
              ...props.value,
              sort_by: sortBy,
              currency:
                sortBy !== undefined &&
                paidAnalyticsMoneySortFields.has(sortBy) &&
                props.value.currency === undefined
                  ? 'CNY'
                  : props.value.currency,
              offset: 0,
            })
          }}
        >
          <NativeSelectOption value=''>
            {t('adminAnalytics.sort.default')}
          </NativeSelectOption>
          {options.map((option) => (
            <NativeSelectOption key={option} value={option}>
              {t(`adminAnalytics.sort.${option}`)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <NativeSelect
          value={props.value.sort_order}
          onChange={(event) =>
            props.onApply({
              ...props.value,
              sort_order: event.target
                .value as AdminAnalyticsCanonicalFilters['sort_order'],
              offset: 0,
            })
          }
        >
          <NativeSelectOption value='desc'>
            {t('adminAnalytics.sort.desc')}
          </NativeSelectOption>
          <NativeSelectOption value='asc'>
            {t('adminAnalytics.sort.asc')}
          </NativeSelectOption>
        </NativeSelect>
      </div>
    </div>
  )
}

function PaidAnalyticsRowControls(props: {
  value: AdminAnalyticsCanonicalFilters
  onApply: (next: AdminAnalyticsCanonicalFilters) => void
}): JSX.Element {
  const { t } = useTranslation()
  const isAllRows = props.value.limit === 0
  const pageSize = isAllRows ? ADMIN_ANALYTICS_MAX_LIMIT : props.value.limit
  const currentPage = isAllRows
    ? 1
    : Math.floor(props.value.offset / Math.max(pageSize, 1)) + 1

  return (
    <div className='grid gap-2 lg:col-span-2'>
      <Label>{t('adminAnalytics.pagination.displayMode')}</Label>
      <div className='flex flex-wrap items-center gap-2'>
        <Button
          type='button'
          size='sm'
          variant={isAllRows ? 'default' : 'outline'}
          onClick={() =>
            props.onApply(enableAdminAnalyticsAllRows(props.value))
          }
        >
          {t('adminAnalytics.pagination.allRows')}
        </Button>
        <Button
          type='button'
          size='sm'
          variant={isAllRows ? 'outline' : 'default'}
          onClick={() =>
            props.onApply(enableAdminAnalyticsPagedRows(props.value))
          }
        >
          {t('adminAnalytics.pagination.pagedRows')}
        </Button>
        {!isAllRows ? (
          <>
            <NativeSelect
              aria-label={t('adminAnalytics.pagination.pageSize')}
              value={String(pageSize)}
              onChange={(event) =>
                props.onApply(
                  enableAdminAnalyticsPagedRows(
                    props.value,
                    Number(event.target.value)
                  )
                )
              }
            >
              <NativeSelectOption value='20'>20</NativeSelectOption>
              <NativeSelectOption value='50'>50</NativeSelectOption>
              <NativeSelectOption value={String(ADMIN_ANALYTICS_MAX_LIMIT)}>
                {ADMIN_ANALYTICS_MAX_LIMIT}
              </NativeSelectOption>
            </NativeSelect>
            <Button
              type='button'
              size='sm'
              variant='outline'
              disabled={props.value.offset <= 0}
              onClick={() =>
                props.onApply({
                  ...props.value,
                  offset: Math.max(props.value.offset - pageSize, 0),
                })
              }
            >
              {t('adminAnalytics.pagination.previousPage')}
            </Button>
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={() =>
                props.onApply({
                  ...props.value,
                  offset: props.value.offset + pageSize,
                })
              }
            >
              {t('adminAnalytics.pagination.nextPage')}
            </Button>
            <span className='text-muted-foreground text-xs'>
              {t('adminAnalytics.pagination.currentPage', {
                page: currentPage,
              })}
            </span>
          </>
        ) : null}
      </div>
      <div className='text-muted-foreground text-xs'>
        {isAllRows
          ? t('adminAnalytics.pagination.allRowsDescription')
          : t('adminAnalytics.pagination.pagedRowsDescription')}
      </div>
    </div>
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
      clear: () =>
        props.onApply({ ...props.value, subscription_id: undefined }),
    })
  }
  if (chips.length === 0) return null
  return (
    <div className='flex flex-wrap gap-2'>
      {chips.map((chip) => (
        <Button
          key={chip.key}
          type='button'
          variant='outline'
          size='xs'
          onClick={chip.clear}
        >
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
  const trimmed = value.trim().toLowerCase()
  if (trimmed === 'all' || trimmed === '0') return 0
  const parsed = Number(trimmed)
  if (!Number.isFinite(parsed)) return 0
  return Math.min(Math.max(Math.trunc(parsed), 0), 100)
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
  queryLoadingStates: boolean[]
  queryErrorStates: boolean[]
  filters: AdminAnalyticsCanonicalFilters
  onDrilldown: (target: FrontendAdminAnalyticsDrilldownTarget) => void
  onRefreshCurrentSnapshot: () => void
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

  if (props.tab === 'conversion') {
    const responses: ConversionPanelResponses = {
      summary: props.responses[0] as
        | PanelApiResponse<AdminAnalyticsSubscriptionConversionResponse>
        | undefined,
      subscriptions: props.responses[1] as
        | PanelApiResponse<AdminAnalyticsDrilldownSubscriptionsResponse>
        | undefined,
    }
    return (
      <PanelCard
        titleKey='adminAnalytics.tabs.conversion'
        loading={false}
        error={false}
      >
        <ConversionPanel
          responses={responses}
          summaryLoading={props.queryLoadingStates[0] ?? false}
          summaryError={props.queryErrorStates[0] ?? false}
          subscriptionsLoading={props.queryLoadingStates[1] ?? false}
          subscriptionsError={props.queryErrorStates[1] ?? false}
        />
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
          onRefreshCurrentSnapshot={props.onRefreshCurrentSnapshot}
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
          <div
            className='text-muted-foreground text-sm'
            role='status'
            aria-live='polite'
          >
            {t('Loading...')}
          </div>
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
      return <EmptyAnalyticsPanel />
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
  const creditValues = adminAnalyticsCreditOverviewValues(
    props.data.summary.quota,
    props.data.summary.subscriptions
  )
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
      {creditValues.map((value) => (
        <Metric
          key={value.labelKey}
          labelKey={value.labelKey}
          value={value.value}
        />
      ))}
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
  const { t } = useTranslation()
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
        items={props.data.high_usage_users.items.map((item) => {
          const value = adminAnalyticsCreditRankingValue(item)
          return {
            key: String(item.subscription_id),
            label: `${item.username} · ${t(value.labelKey)}`,
            value: value.value,
            drilldown: item.drilldown,
          }
        })}
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

export function ConversionPanel(props: {
  responses: ConversionPanelResponses
  summaryLoading: boolean
  summaryError: boolean
  subscriptionsLoading: boolean
  subscriptionsError: boolean
}): JSX.Element {
  const { t } = useTranslation()
  const summary = panelData(props.responses.summary)
  const subscriptions =
    panelData(props.responses.subscriptions)?.subscriptions?.items ?? []

  return (
    <div className='flex flex-col gap-4'>
      <section
        aria-labelledby='admin-analytics-conversion-summary'
        aria-busy={props.summaryLoading}
        aria-live='polite'
      >
        <div
          id='admin-analytics-conversion-summary'
          className='mb-2 text-sm font-medium'
        >
          {t('adminAnalytics.conversion.summary')}
        </div>
        {props.summaryLoading ? (
          <div className='text-muted-foreground text-sm' role='status'>
            {t('adminAnalytics.conversion.summaryLoading')}
          </div>
        ) : props.summaryError ? (
          <Alert variant='destructive'>
            <AlertTitle>
              {t('adminAnalytics.conversion.summaryFailed')}
            </AlertTitle>
            <AlertDescription>
              {t('Try adjusting the time range or filters')}
            </AlertDescription>
          </Alert>
        ) : summary ? (
          <MetricGrid>
            <Metric
              labelKey='adminAnalytics.metrics.trialUsers'
              value={summary.summary.trial_users}
            />
            <Metric
              labelKey='adminAnalytics.metrics.paidUsers'
              value={summary.summary.paid_users}
            />
            <Metric
              labelKey='adminAnalytics.metrics.trialToPaidRate'
              value={formatAdminPercent(summary.summary.trial_to_paid_rate)}
            />
            <Metric
              labelKey='adminAnalytics.metrics.renewalUsers'
              value={summary.summary.renewal_users}
            />
          </MetricGrid>
        ) : (
          <EmptyAnalyticsPanel />
        )}
      </section>

      <section
        aria-labelledby='admin-analytics-conversion-history'
        aria-busy={props.subscriptionsLoading}
        aria-live='polite'
      >
        <div
          id='admin-analytics-conversion-history'
          className='mb-2 text-sm font-medium'
        >
          {t('adminAnalytics.rankings.subscriptionConversionHistory')}
        </div>
        {props.subscriptionsLoading ? (
          <div className='text-muted-foreground text-sm' role='status'>
            {t('adminAnalytics.conversion.historyLoading')}
          </div>
        ) : props.subscriptionsError ? (
          <Alert variant='destructive'>
            <AlertTitle>
              {t('adminAnalytics.conversion.historyFailed')}
            </AlertTitle>
            <AlertDescription>
              {t('Try adjusting the time range or filters')}
            </AlertDescription>
          </Alert>
        ) : (
          <AnalyticsCardGrid
            titleKey='adminAnalytics.rankings.subscriptionConversionHistory'
            hideTitle
            items={subscriptions.map((item) => ({
              key: String(item.subscription_id),
              title: `${item.username || item.user_id} · ${item.plan_title}`,
              description: t(
                adminAnalyticsLifecycleLabelKeys[item.lifecycle_state]
              ),
              values: adminAnalyticsSubscriptionHistoryValues(item),
            }))}
          />
        )}
      </section>
    </div>
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
  onRefreshCurrentSnapshot: () => void
}): JSX.Element {
  const { t } = useTranslation()
  const summary = panelData(props.responses.summary)?.summary
  const users = panelData(props.responses.users)?.users?.items ?? []
  const subscriptions =
    panelData(props.responses.subscriptions)?.subscriptions?.items ?? []
  const plans = panelData(props.responses.plans)?.plans?.items ?? []
  const sources = panelData(props.responses.sources)?.sources?.items ?? []
  const hasCurrentOnly =
    subscriptions.some(
      (item) => item.snapshot_semantics === 'current_only'
    ) ||
    Object.values(props.responses).some(
      (response) =>
        hasPanelData(response) &&
        response.data.warnings?.some(
          (warning) => warning.reason === 'current_only'
        )
    )
  if (!summary) return <EmptyAnalyticsPanel />
  return (
    <div className='flex flex-col gap-4'>
      {hasCurrentOnly ? (
        <Alert>
          <AlertTitle>{t('adminAnalytics.warnings.currentOnlyTitle')}</AlertTitle>
          <AlertDescription className='flex flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between'>
            <span>
              {t('adminAnalytics.warnings.currentOnlyDescription')}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={props.onRefreshCurrentSnapshot}
            >
              {t('adminAnalytics.actions.refreshCurrentSnapshot')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      <MetricGrid>
        <Metric
          labelKey='adminAnalytics.metrics.remainingValue'
          value={formatAdminMoneyBreakdown(
            summary.recognized_remaining_value_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.exactRemainingValue'
          value={formatAdminMoneyBreakdown(
            summary.exact_remaining_value_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.estimatedRemainingValue'
          value={formatAdminMoneyBreakdown(
            summary.estimated_remaining_value_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.unknownCostCredit'
          value={summary.unknown_cost_credit ?? 0}
        />
        <Metric
          labelKey='adminAnalytics.metrics.tokenBasedValue'
          value={formatAdminMoneyBreakdown(
            summary.token_based_value_by_currency
          )}
        />
        <Metric
          labelKey='adminAnalytics.metrics.timeBasedValue'
          value={formatAdminMoneyBreakdown(
            summary.time_based_value_by_currency
          )}
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
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.paidSubscriptionUsers'
        items={users.map((item) => ({
          key: String(item.user_id),
          title: item.username || String(item.user_id),
          description: item.display_name
            ? `#${item.user_id} · ${item.display_name}`
            : `#${item.user_id}`,
          values: paidSubscriptionValueUserCardValues(item),
        }))}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.paidSubscriptionPlans'
        items={plans.map((item) => ({
          key: String(item.plan_id),
          title: item.plan_name || String(item.plan_id),
          description: item.plan_business_code,
          values: paidSubscriptionValuePlanCardValues(item),
        }))}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.paidSubscriptionSources'
        items={sources.map((item) => ({
          key: `${item.source}:${item.grant_reason}`,
          title: item.source || t('Unknown'),
          description: item.grant_reason || item.source_attribution,
          values: paidSubscriptionValueSourceCardValues(item),
        }))}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.paidSubscriptionRecords'
        items={subscriptions.map((item) => ({
          key: String(item.subscription_id),
          title: `${item.username || item.user_id} · ${item.plan_name}`,
          description: `${item.source} / ${item.grant_reason}`,
          drilldown: item.drilldown,
          values: paidSubscriptionValueSubscriptionCardValues(item),
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
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.invitationPaidInviters'
        items={inviters.map((item) => ({
          key: String(item.inviter_user_id),
          title: item.inviter_username || String(item.inviter_user_id),
          description: `#${item.inviter_user_id}`,
          values: invitationPaidInviterCardValues(item),
          drilldown: item.drilldown,
        }))}
        onDrilldown={props.onDrilldown}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.invitationPaidInvitees'
        items={invitees.map((item) => ({
          key: String(item.invitee_user_id),
          title: item.invitee_username || String(item.invitee_user_id),
          description: `#${item.invitee_user_id}`,
          values: invitationPaidInviteeCardValues(item),
        }))}
      />
      <AnalyticsCardGrid
        titleKey='adminAnalytics.rankings.invitationPaidRecords'
        items={subscriptions.map((item) => ({
          key: String(item.subscription_id),
          title: `${item.invitee_user_id} · ${item.plan_name}`,
          description: `${item.source} / ${item.grant_reason}`,
          drilldown: item.drilldown,
          values: invitationPaidSubscriptionCardValues(item),
        }))}
        onDrilldown={props.onDrilldown}
      />
    </div>
  )
}

function AnalyticsCardGrid(props: {
  titleKey: string
  hideTitle?: boolean
  items: Array<{
    key: string
    title: string
    description?: string
    values: Array<{ labelKey: string; value?: string; valueKey?: string }>
    drilldown?: AdminAnalyticsDrilldownTarget | null
  }>
  onDrilldown?: DrilldownHandler
}): JSX.Element {
  const { t } = useTranslation()
  if (props.items.length === 0) return <EmptyAnalyticsPanel />
  return (
    <div className='space-y-2'>
      {props.hideTitle ? null : (
        <div className='text-sm font-medium'>{t(props.titleKey)}</div>
      )}
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {props.items.map((item) => (
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
                  <dd className='text-right'>
                    {value.value ??
                      (value.valueKey ? t(value.valueKey) : '—')}
                  </dd>
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
      {props.items.map((item) => (
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
    case 'admin_usage_logs':
    case 'admin_subscriptions':
    case 'admin_invitations':
    case 'paid_subscription_value_subscription':
    case 'invitation_paid_inviter':
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
