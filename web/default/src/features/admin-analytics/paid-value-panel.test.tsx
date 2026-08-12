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
/** @jsxImportSource react */
import type { InternalAxiosRequestConfig } from 'axios'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  fireEvent,
  render,
  waitFor,
} from '@testing-library/react/pure'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, test } from 'node:test'
import { I18nextProvider } from 'react-i18next'
import { api } from '@/lib/api'
import { AdminAnalyticsPage } from './index'
import { buildAdminAnalyticsCanonicalFilters } from './lib/filters'
import type {
  AdminAnalyticsPanelResponse,
  ApiResponse,
  PaidSubscriptionValueResponse,
} from './types'

const translations = {
  'adminAnalytics.tabs.paidSubscriptionValue':
    'Paid subscription operational remaining value',
  'adminAnalytics.metrics.remainingValue': 'Operational remaining value',
  'adminAnalytics.metrics.tokenBasedValue': 'Token-based value',
  'adminAnalytics.metrics.timeBasedValue': 'Time-based value',
  'adminAnalytics.metrics.exactRemainingValue': 'Exact remaining value',
  'adminAnalytics.metrics.estimatedRemainingValue': 'Estimated remaining value',
  'adminAnalytics.metrics.unknownCostCredit': 'Unknown-cost Credit',
  'adminAnalytics.metrics.excludedAuditValue': 'Excluded audit value',
  'adminAnalytics.metrics.activePaidSubscriptions':
    'Active priced entitlements',
  'adminAnalytics.metrics.activePaidUsers': 'Active paid users',
  'adminAnalytics.metrics.tokenValueUnavailable': 'Token value unavailable',
  'adminAnalytics.values.notApplicable': 'Not applicable',
  'adminAnalytics.warnings.currentOnlyTitle': 'Current Credit values shown',
  'adminAnalytics.warnings.currentOnlyDescription':
    'Credit changed while this analytics snapshot was loading.',
  'adminAnalytics.actions.refreshCurrentSnapshot': 'Refresh current snapshot',
  'adminAnalytics.rankings.paidSubscriptionRecords':
    'Priced entitlement records',
  'adminAnalytics.fields.subscriptionId': 'Subscription ID',
  'adminAnalytics.fields.userId': 'User ID',
  'adminAnalytics.fields.planId': 'Plan ID',
  'adminAnalytics.fields.planPrice': 'Plan price',
  'adminAnalytics.fields.recognizedRemainingValue':
    'Recognized remaining value',
  'adminAnalytics.fields.exactRemainingValue': 'Exact remaining value',
  'adminAnalytics.fields.estimatedRemainingValue': 'Estimated remaining value',
  'adminAnalytics.fields.startTime': 'Start time',
  'adminAnalytics.fields.endTime': 'End time',
  'adminAnalytics.fields.remainingSeconds': 'Remaining seconds',
  'adminAnalytics.fields.tokenLimit': 'Token limit',
  'adminAnalytics.fields.tokenUsed': 'Credit used',
  'adminAnalytics.fields.availableCredit': 'Available Credit',
  'adminAnalytics.fields.unknownCostCredit': 'Unknown-cost Credit',
  'adminAnalytics.fields.nextResetTime': 'Next reset time',
  'adminAnalytics.fields.valuationBasis': 'Valuation basis',
  'adminAnalytics.fields.valuationConfidence': 'Valuation confidence',
  'adminAnalytics.fields.valuationStateVersion': 'Valuation state version',
  'adminAnalytics.fields.valuationUpdatedAt': 'Valuation updated at',
  'adminAnalytics.fields.snapshotSemantics': 'Snapshot semantics',
  'adminAnalytics.fields.entitlementType': 'Entitlement type',
  'adminAnalytics.fields.sourceAttribution': 'Source attribution',
  'adminAnalytics.fields.possibleOrderId': 'Possible order ID',
  'adminAnalytics.fields.paymentProvider': 'Payment provider',
  'adminAnalytics.fields.paymentMethod': 'Payment method',
  'adminAnalytics.fields.orderRecordedAmount': 'Order recorded amount',
  'adminAnalytics.fields.excludedReason': 'Excluded reason',
  'adminAnalytics.valuationBasis.creditMovingWeightedAverage':
    'Moving weighted average',
  'adminAnalytics.valuationConfidence.exact': 'Exact',
  'adminAnalytics.snapshotSemantics.currentOnly': 'Current only',
  'adminAnalytics.sourceAttribution.movingWeightedPool':
    'Moving weighted pool',
  'adminAnalytics.rankings.paidSubscriptionUsers': 'Paid users',
  'adminAnalytics.rankings.paidSubscriptionPlans': 'Paid plans',
  'adminAnalytics.rankings.paidSubscriptionSources': 'Paid sources',
  'adminAnalytics.title': 'Operations Analytics',
  'adminAnalytics.description': 'Operational analytics',
  'adminAnalytics.refreshing': 'Refreshing analytics data...',
  'adminAnalytics.failedToLoad': 'Failed to load analytics',
  'Try adjusting the time range or filters':
    'Try adjusting the time range or filters',
  'Loading...': 'Loading...',
  Unknown: 'Unknown',
}

type PaidValuePanelResponse = ApiResponse<
  AdminAnalyticsPanelResponse<PaidSubscriptionValueResponse>
>

function emptyList<T>() {
  return {
    page: { limit: 20, offset: 0, total: 0, has_more: false },
    sort_by: '',
    sort_order: 'desc' as const,
    items: [] as T[],
  }
}

const cny32 = { amount: 32, amount_micros: '32000000', currency: 'CNY' }
const cny0 = { amount: 0, amount_micros: '0', currency: 'CNY' }

const paidValueResponse: PaidValuePanelResponse = {
  success: true,
  data: {
    range: { start_timestamp: 0, end_timestamp: 123, snapshot_at: 123 },
    data: {
      summary: {
        recognized_remaining_value_by_currency: [cny32],
        token_based_value_by_currency: [cny32],
        time_based_value_by_currency: [],
        exact_remaining_value_by_currency: [cny32],
        estimated_remaining_value_by_currency: [cny0],
        excluded_remaining_value_by_currency: [],
        active_paid_subscription_count: 1,
        active_paid_user_count: 1,
        token_value_unavailable_count: 0,
        unknown_cost_credit: 0,
        unknown_timed_subscription_count: 0,
        credit_valuation_state_missing_count: 0,
      },
      users: emptyList(),
      plans: emptyList(),
      sources: emptyList(),
      subscriptions: {
        page: { limit: 20, offset: 0, total: 1, has_more: false },
        sort_by: 'subscription_id',
        sort_order: 'desc',
        items: [
          {
            subscription_id: 4,
            user_id: 1,
            username: 'alice',
            plan_id: 3,
            plan_name: 'Credit balance',
            source: 'credit_balance_pool',
            grant_reason: 'order',
            plan_price: cny0,
            start_time: 1,
            end_time: 0,
            remaining_seconds: 0,
            token_limit: 1_000,
            token_used: 200,
            available_credit: 800,
            unknown_cost_credit: 0,
            next_reset_time: 0,
            token_based_value: cny32,
            time_based_value: null,
            recognized_remaining_value: cny32,
            exact_remaining_value: cny32,
            estimated_remaining_value: cny0,
            valuation_basis: 'credit_moving_weighted_average',
            valuation_confidence: 'exact',
            valuation_state_version: 2,
            valuation_updated_at: 124,
            snapshot_semantics: 'current_only',
            entitlement_type: 'credit_balance',
            source_attribution: 'moving_weighted_pool',
            excluded: false,
            excluded_reason: '',
          },
        ],
      },
    },
  },
}

const originalAPIAdapter = api.defaults.adapter

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAPIAdapter
})

function axiosResponse(
  config: InternalAxiosRequestConfig,
  data: PaidValuePanelResponse
) {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }
}

test('Credit paid-value panel displays exact micros and refreshes current-only snapshots', async () => {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: false,
    resources: { en: { translation: translations } },
    interpolation: { escapeValue: false },
  })
  api.defaults.adapter = async (config) =>
    axiosResponse(config, paidValueResponse)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
  const changes: ReturnType<typeof buildAdminAnalyticsCanonicalFilters>[] = []
  const search = buildAdminAnalyticsCanonicalFilters(
    { tab: 'paid-subscription-value', snapshot_at: '123' },
    1_800_000_000
  )

  const rootRoute = createRootRoute({
    component: () => (
      <AdminAnalyticsPage
        search={search}
        onSearchChange={(next) => changes.push(next)}
        onDrilldown={() => undefined}
      />
    ),
  })
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })

  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nextProvider>
  )

  await waitFor(() => assert.ok(view.getAllByText('¥32.00').length >= 3))
  assert.ok(view.getAllByText('Exact remaining value').length >= 2)
  assert.ok(view.getByText('Not applicable'))
  assert.ok(view.getByText('Moving weighted average'))
  assert.ok(view.getByText('Exact'))
  assert.ok(view.getByText('Current Credit values shown'))
  const refresh = view.getByRole('button', { name: 'Refresh current snapshot' })
  fireEvent.click(refresh)
  assert.equal(changes.at(-1)?.snapshot_at, undefined)
  queryClient.clear()
})
