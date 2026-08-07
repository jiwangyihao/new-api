/** @jsxImportSource react */
import type { InternalAxiosRequestConfig } from 'axios'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, waitFor, within } from '@testing-library/react/pure'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, describe, test } from 'node:test'
import { I18nextProvider } from 'react-i18next'
import { api } from '@/lib/api'
import { AdminAnalyticsPage, ConversionPanel } from './index'
import { buildAdminAnalyticsCanonicalFilters } from './lib/filters'
import type {
  AdminAnalyticsDrilldownSubscriptionsResponse,
  AdminAnalyticsPanelResponse,
  AdminAnalyticsSubscriptionConversionResponse,
  ApiResponse,
} from './types'

const translations = {
  'adminAnalytics.conversion.summary': 'Conversion summary',
  'adminAnalytics.conversion.summaryFailed': 'Conversion summary unavailable',
  'adminAnalytics.conversion.historyFailed': 'Conversion history unavailable',
  'adminAnalytics.metrics.conversionCount': 'Conversions',
  'adminAnalytics.metrics.exactConversionCount': 'Exact conversions',
  'adminAnalytics.metrics.grossCredit': 'Gross Credit',
  'adminAnalytics.metrics.debtOffset': 'Debt offset',
  'adminAnalytics.metrics.netAvailableCredit': 'Net available Credit',
  'adminAnalytics.metrics.grossConversionValue': 'Gross conversion value',
  'adminAnalytics.metrics.netConversionValue': 'Net conversion value',
  'adminAnalytics.rankings.subscriptionConversionHistory':
    'Subscription conversion history',
  'adminAnalytics.lifecycle.converted': 'Converted',
  'adminAnalytics.fields.subscriptionId': 'Subscription ID',
  'adminAnalytics.fields.userId': 'User ID',
  'adminAnalytics.fields.planId': 'Plan ID',
  'adminAnalytics.fields.entitlementType': 'Entitlement type',
  'adminAnalytics.fields.lifecycleState': 'Lifecycle state',
  'adminAnalytics.fields.status': 'Status',
  'adminAnalytics.fields.sourceAttribution': 'Source attribution',
  'adminAnalytics.fields.availableCredit': 'Available Credit',
  'adminAnalytics.fields.settlementDebt': 'Settlement debt',
  'adminAnalytics.fields.graceRemainingSeconds': 'Grace remaining seconds',
  'adminAnalytics.fields.conversionId': 'Conversion ID',
  'adminAnalytics.fields.targetSubscriptionId': 'Target subscription ID',
  'adminAnalytics.fields.targetUserId': 'Target user ID',
  'adminAnalytics.fields.targetPlanId': 'Target plan ID',
  'adminAnalytics.fields.targetPlanTitle': 'Target plan title',
  'adminAnalytics.fields.startTime': 'Start time',
  'adminAnalytics.fields.endTime': 'End time',
  'Try adjusting the time range or filters':
    'Try adjusting the time range or filters',
}

type SummaryResponse = ApiResponse<
  AdminAnalyticsPanelResponse<AdminAnalyticsSubscriptionConversionResponse>
>
type HistoryResponse = ApiResponse<
  AdminAnalyticsPanelResponse<AdminAnalyticsDrilldownSubscriptionsResponse>
>

function panel<T>(data: T): ApiResponse<AdminAnalyticsPanelResponse<T>> {
  return {
    success: true,
    data: {
      range: { start_timestamp: 1, end_timestamp: 2, snapshot_at: 2 },
      data,
    },
  }
}

const summaryResponse: SummaryResponse = panel({
  summary: {
    conversion_count: 12,
    exact_conversion_count: 12,
    gross_credit: '9007199254740993',
    debt_offset: '5',
    net_available_credit: '9007199254740988',
    gross_value_by_currency: [
      { amount: 0, amount_micros: '32000000', currency: 'CNY' },
    ],
    net_value_by_currency: [
      { amount: 0, amount_micros: '31000000', currency: 'CNY' },
    ],
  },
})

const historyResponse: HistoryResponse = panel({
  subscriptions: {
    page: { limit: 20, offset: 0, total: 1, has_more: false },
    sort_by: 'subscription_id',
    sort_order: 'desc',
    items: [
      {
        subscription_id: 41,
        user_id: 8,
        username: 'converted-user',
        plan_id: 10,
        plan_title: 'Timed Pro',
        source: 'order',
        status: 'converted',
        start_time: 1_700_000_000,
        end_time: 1_700_100_000,
        token_limit: 1_000,
        token_used: 200,
        remaining_tokens: 800,
        usage_rate: 0.2,
        entitlement_type: 'timed',
        lifecycle_state: 'converted',
        available_credit: 0,
        settlement_debt: 0,
        grace_remaining_seconds: 0,
        conversion_id: 51,
        target_subscription_id: 42,
        target_user_id: 8,
        target_plan_id: 11,
        target_plan_title: 'Credit Balance',
      },
    ],
  },
})

const failedResponse = { success: false, message: 'failed' } as const
const originalAPIAdapter = api.defaults.adapter

function axiosResponse(config: InternalAxiosRequestConfig, data: unknown) {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }
}

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAPIAdapter
})

async function renderConversionPanel(options: {
  summary: SummaryResponse
  history: HistoryResponse
}) {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: false,
    resources: { en: { translation: translations } },
    interpolation: { escapeValue: false },
  })
  return render(
    <I18nextProvider i18n={i18n}>
      <ConversionPanel
        responses={{
          summary: options.summary,
          subscriptions: options.history,
        }}
        summaryLoading={false}
        summaryError={!options.summary.success}
        subscriptionsLoading={false}
        subscriptionsError={!options.history.success}
      />
    </I18nextProvider>
  )
}

test('page preserves summary when the default conversion history request fails', async () => {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: false,
    resources: { en: { translation: translations } },
    interpolation: { escapeValue: false },
  })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
  const requestURLs: string[] = []
  api.defaults.adapter = async (config) => {
    const url = String(config.url)
    requestURLs.push(url)
    if (url.includes('/subscription-conversion?')) {
      return axiosResponse(config, summaryResponse)
    }
    if (url.includes('/drilldown/subscriptions?')) {
      return axiosResponse(config, failedResponse)
    }
    throw new Error(`unexpected request: ${config.method} ${url}`)
  }
  const search = buildAdminAnalyticsCanonicalFilters(
    { tab: 'conversion' },
    1_800_000_000
  )

  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <AdminAnalyticsPage
          search={search}
          onSearchChange={() => undefined}
          onDrilldown={() => undefined}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )

  await waitFor(() => assert.ok(view.getByText('Conversions')))
  await waitFor(() =>
    assert.ok(view.getByText('Conversion history unavailable'))
  )
  const historyURL = requestURLs.find((url) =>
    url.includes('/drilldown/subscriptions?')
  )
  assert.ok(historyURL)
  const historyParams = new URL(historyURL, 'http://localhost').searchParams
  assert.deepEqual(historyParams.getAll('subscription_statuses'), [
    'converted',
    'expired',
  ])
  assert.ok(
    requestURLs.some((url) => url.includes('/subscription-conversion?'))
  )
  queryClient.clear()
})

describe('admin analytics conversion query states', () => {
  test('keeps the successful summary visible when history fails', async () => {
    const view = await renderConversionPanel({
      summary: summaryResponse,
      history: failedResponse,
    })

    const summary = view.getByRole('region', { name: 'Conversion summary' })
    const history = view.getByRole('region', {
      name: 'Subscription conversion history',
    })
    assert.ok(within(summary).getByText('Conversions'))
    assert.equal(within(summary).getAllByText('12').length, 2)
    assert.ok(within(history).getByText('Conversion history unavailable'))
    assert.equal(view.queryByText('Conversion summary unavailable'), null)
  })

  test('keeps successful history visible when the summary fails', async () => {
    const view = await renderConversionPanel({
      summary: failedResponse,
      history: historyResponse,
    })

    const summary = view.getByRole('region', { name: 'Conversion summary' })
    const history = view.getByRole('region', {
      name: 'Subscription conversion history',
    })
    assert.ok(within(summary).getByText('Conversion summary unavailable'))
    assert.ok(within(history).getByText('converted-user · Timed Pro'))
    assert.ok(within(history).getByText('Converted'))
    assert.equal(view.queryByText('Conversion history unavailable'), null)
  })

  test('renders both successful query results together', async () => {
    const view = await renderConversionPanel({
      summary: summaryResponse,
      history: historyResponse,
    })

    assert.ok(view.getByText('Conversions'))
    assert.ok(view.getByText('¥32.00'))
    assert.ok(view.getByText('¥31.00'))
    assert.ok(view.getByText('9007199254740993'))
    assert.ok(view.getByText('converted-user · Timed Pro'))
    assert.equal(view.queryAllByRole('alert').length, 0)
  })
})
