/** @jsxImportSource react */
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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  waitFor,
  within,
  type RenderResult,
} from '@testing-library/react/pure'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, beforeEach, describe, test } from 'node:test'
import { I18nextProvider } from 'react-i18next'
import { deriveLiveConversionQuote } from '../live-quote'
import type {
  SubscriptionConversionConfirmRequest,
  SubscriptionConversionConfirmResult,
  SubscriptionConversionHistory,
  SubscriptionConversionQuote,
  SubscriptionConversionQuoteList,
} from '../types'
import { TimedSubscriptionConversionQuotesCard } from './timed-subscription-conversion-quotes-card'

const activeQueryClients = new Set<QueryClient>()
const originalSetInterval = globalThis.setInterval
const originalClearInterval = globalThis.clearInterval

type CapturedInterval = {
  id: number
  delay: number
  callback: () => void
}

let currentTimeMs = 1_000_000
let nextIntervalId = -1
let capturedIntervals: CapturedInterval[] = []

beforeEach(() => {
  currentTimeMs = 1_000_000
  nextIntervalId = -1
  capturedIntervals = []
  globalThis.setInterval = ((callback: TimerHandler, delay?: number) => {
    const numericDelay = Number(delay || 0)
    if (numericDelay !== 1000 && numericDelay !== 5000) {
      return originalSetInterval(callback, delay)
    }
    const id = nextIntervalId--
    capturedIntervals.push({
      id,
      delay: numericDelay,
      callback: callback as () => void,
    })
    return id
  }) as typeof setInterval
  globalThis.clearInterval = ((id?: number | NodeJS.Timeout) => {
    const numericId = Number(id)
    if (capturedIntervals.some((entry) => entry.id === numericId)) {
      capturedIntervals = capturedIntervals.filter(
        (entry) => entry.id !== numericId
      )
      return
    }
    originalClearInterval(id)
  }) as unknown as typeof clearInterval
})

afterEach(() => {
  cleanup()
  for (const queryClient of activeQueryClients) queryClient.clear()
  activeQueryClients.clear()
  globalThis.setInterval = originalSetInterval
  globalThis.clearInterval = originalClearInterval
})

function makeQuote(
  overrides: Partial<SubscriptionConversionQuote> = {}
): SubscriptionConversionQuote {
  return {
    source_subscription_id: '7001',
    plan_id: '7101',
    plan_title: 'Monthly Pro',
    entitlement_type: 'timed',
    grant_source: 'order',
    status: 'active',
    category: 'convertible',
    database_now: '1800000000',
    start_time: '1797000000',
    end_time: '1802678400',
    remaining_seconds: '2678400',
    full_31_day_blocks: '1',
    credit_basis: '100',
    credit_basis_source: 'grant_snapshot',
    current_remaining_credit: '25',
    gross_credit: '125',
    current_debt: '20',
    estimated_debt_offset: '20',
    net_available_credit: '105',
    last_granted_at: '1797000000',
    last_grant_time_source: 'live_grant',
    last_grant_source: 'order',
    cooldown_status: 'ready',
    cooldown_remaining_seconds: '0',
    grace_status: 'not_started',
    grace_remaining_seconds: '0',
    expired: false,
    within_grace: false,
    eligible: true,
    can_confirm: true,
    reason_codes: [],
    reasons: [],
    ...overrides,
  }
}

function makeQuoteList(
  quotes: SubscriptionConversionQuote[] = [makeQuote()],
  conversions: SubscriptionConversionHistory[] = []
): SubscriptionConversionQuoteList {
  return { database_now: '1800000000', quotes, conversions }
}

function makeConversion(
  overrides: Partial<SubscriptionConversionHistory> = {}
): SubscriptionConversionHistory {
  return {
    id: '1',
    source_subscription_id: '7001',
    source_plan_id: '7101',
    source_plan_title: 'Monthly Pro',
    target_subscription_id: '7201',
    target_plan_id: '7301',
    ledger_id: '7401',
    source_status: 'active',
    grant_source: 'order',
    database_now: '1800000010',
    source_start_time: '1797000000',
    source_end_time: '1802678400',
    remaining_seconds: '2678390',
    full_31_day_blocks: '0',
    credit_basis: '100',
    credit_basis_source: 'grant_snapshot',
    current_remaining_credit: '20',
    gross_credit: '20',
    debt_offset: '5',
    net_available_credit: '15',
    available_credit_after: '115',
    settlement_debt_after: '0',
    balance_before: '100',
    balance_after: '115',
    last_granted_at: '1797000000',
    last_grant_time_source: 'live_grant',
    last_grant_source: 'order',
    converted_at: '1800000010',
    source_price_micros: '40000000',
    source_currency: 'CNY',
    target_currency: 'USD',
    valuation_credit_basis: '100',
    gross_cost_micros: '9863013',
    net_cost_micros: '9863013',
    unit_value_numerator_micros: '4000000',
    unit_value_denominator: '73',
    rule_version: 1,
    state_version_after: '1',
    fx_numerator: '10',
    fx_denominator: '73',
    fx_captured_at: '1800000010',
    fx_direction: 'CNY_TO_USD',
    ...overrides,
  }
}

interface QuotesCardTestView extends Pick<RenderResult, 'getByRole'> {
  getRequestCount: () => number
}

async function renderQuotesCard(
  response: SubscriptionConversionQuoteList = makeQuoteList(),
  options: {
    confirmConversion?: (
      request: SubscriptionConversionConfirmRequest
    ) => Promise<SubscriptionConversionConfirmResult>
  } = {}
) {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: false,
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
  activeQueryClients.add(queryClient)
  let requestCount = 0
  const confirmRequests: SubscriptionConversionConfirmRequest[] = []
  let currentResponse = response
  const loadQuotes = async () => {
    requestCount += 1
    return currentResponse
  }
  const confirmConversion = async (
    request: SubscriptionConversionConfirmRequest
  ): Promise<SubscriptionConversionConfirmResult> => {
    confirmRequests.push(request)
    return (
      options.confirmConversion?.(request) ?? {
        replayed: false,
        conversion: makeConversion(),
      }
    )
  }
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <TimedSubscriptionConversionQuotesCard
          now={() => currentTimeMs}
          loadQuotes={loadQuotes}
          confirmConversion={confirmConversion}
          createIdempotencyKey={() => 'conversion-client-key'}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await waitFor(() => assert.equal(requestCount, 1))
  await waitFor(() => view.getByText('Timed subscription conversion quotes'), {
    timeout: 3000,
  })
  return {
    ...view,
    getRequestCount: () => requestCount,
    getConfirmRequests: () => confirmRequests,
    setResponse: (next: SubscriptionConversionQuoteList) => {
      currentResponse = next
    },
  }
}

async function openConversionPreview(view: QuotesCardTestView) {
  const button = view.getByRole('button', { name: 'Preview conversion' })
  await waitFor(() => assert.equal(button.getAttribute('disabled'), null))
  fireEvent.click(button)
  await waitFor(() => assert.ok(view.getRequestCount() >= 2))
  return waitFor(() => view.getByRole('dialog'))
}

function runCapturedInterval(delay: number) {
  const interval = capturedIntervals.find((entry) => entry.delay === delay)
  assert.ok(interval, `missing ${delay}ms interval`)
  interval.callback()
}

describe('timed subscription conversion quotes', () => {
  test('refreshes before confirmation and submits only source identity plus a client idempotency key', async () => {
    const view = await renderQuotesCard()
    const dialog = await openConversionPreview(view)

    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Submit conversion' })
    )

    await waitFor(() => assert.ok(view.getRequestCount() >= 3))
    await waitFor(() => assert.equal(view.getConfirmRequests().length, 1))
    assert.deepEqual(view.getConfirmRequests()[0], {
      subscription_id: '7001',
      idempotency_key: 'conversion-client-key',
    })
    const result = await waitFor(() =>
      view.getByRole('status', { name: 'Latest conversion result' })
    )
    assert.ok(within(result).getByText('Monthly Pro'))
    assert.ok(within(result).getByText('20'))
    assert.ok(within(result).getByText('5'))
    assert.ok(within(result).getByText('15'))
    assert.ok(within(result).getByText('115'))
  })

  test('keeps the submit disabled while pending and prevents duplicate confirmation', async () => {
    let resolveConfirmation:
      | ((value: SubscriptionConversionConfirmResult) => void)
      | undefined
    const pending = new Promise<SubscriptionConversionConfirmResult>(
      (resolve) => {
        resolveConfirmation = resolve
      }
    )
    const view = await renderQuotesCard(makeQuoteList(), {
      confirmConversion: () => pending,
    })
    const dialog = await openConversionPreview(view)
    const submit = within(dialog).getByRole('button', {
      name: 'Submit conversion',
    })
    fireEvent.click(submit)
    const pendingSubmit = await waitFor(() =>
      within(dialog).getByRole('button', { name: 'Converting subscription' })
    )
    assert.equal(pendingSubmit.getAttribute('disabled'), '')
    fireEvent.click(pendingSubmit)
    assert.equal(view.getConfirmRequests().length, 1)

    resolveConfirmation?.({ replayed: false, conversion: makeConversion() })
    await waitFor(() =>
      view.getByRole('status', { name: 'Latest conversion result' })
    )
  })

  test('refreshes the server quote and recovers after confirmation failure', async () => {
    const view = await renderQuotesCard(makeQuoteList(), {
      confirmConversion: async () => {
        throw new Error('Conversion conflict')
      },
    })
    const dialog = await openConversionPreview(view)
    view.setResponse(
      makeQuoteList([
        makeQuote({
          plan_title: 'Monthly Pro latest after conflict',
          current_remaining_credit: '10',
          gross_credit: '110',
          net_available_credit: '90',
        }),
      ])
    )
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Submit conversion' })
    )

    await waitFor(() => within(dialog).getByText('Conversion conflict'))
    await waitFor(() =>
      within(dialog).getByText('Monthly Pro latest after conflict')
    )
    assert.ok(view.getRequestCount() >= 4)
    assert.ok(within(dialog).getByRole('button', { name: 'Submit conversion' }))
  })

  test('renders converted subscriptions in history without a reverse action', async () => {
    const view = await renderQuotesCard(makeQuoteList([], [makeConversion()]))
    assert.ok(view.getByRole('heading', { name: 'Conversion history' }))
    const history = view.getByLabelText('Converted subscription #7001')
    assert.ok(within(history).getByText('0 × 100 + 20 = 20'))
    assert.equal(view.queryByRole('button', { name: /restore/i }), null)
  })

  test('shows frozen valuation and FX facts without rounding source integers', async () => {
    const view = await renderQuotesCard(makeQuoteList([], [makeConversion()]))
    const history = view.getByLabelText('Converted subscription #7001')

    assert.ok(within(history).getByText('40000000'))
    assert.ok(within(history).getByText('CNY → USD'))
    assert.equal(within(history).getAllByText('9863013').length, 2)
    assert.ok(within(history).getByText('4000000 / 73'))
    assert.ok(within(history).getByText('10 / 73'))
    assert.ok(within(history).getByText('CNY_TO_USD'))
    assert.ok(within(history).getByText('1800000010'))
    assert.ok(within(history).getByText('Rule version: 1'))
    assert.ok(within(history).getByText('State version: 1'))
    assert.ok(
      within(history).getByText(
        'This is a rules-based valuation, not a new payment.'
      )
    )
  })

  test('collapses excluded subscriptions and exposes only their primary reason', async () => {
    const view = await renderQuotesCard(
      makeQuoteList([
        makeQuote({
          start_time: '1800000001',
          category: 'excluded',
          eligible: false,
          can_confirm: false,
          reason_codes: ['subscription_not_started'],
          reasons: [{ code: 'subscription_not_started' }],
        }),
      ])
    )

    assert.ok(view.getByRole('heading', { name: 'Excluded subscriptions' }))
    assert.equal(
      view.queryByText('The source subscription has not started yet'),
      null
    )
    assert.equal(
      view.queryByRole('button', { name: 'Preview conversion' }),
      null
    )
    const trigger = view.getByRole('button', {
      name: /Excluded subscriptions/,
    })
    assert.equal(trigger.getAttribute('aria-expanded'), 'false')
    fireEvent.click(trigger)
    assert.ok(view.getByText('The source subscription has not started yet'))
    assert.ok(view.getByText('Potential available Credit if eligible'))
  })
  test('submits subscription identifiers above Number.MAX_SAFE_INTEGER without precision loss', async () => {
    const unsafeId = '9007199254740993'
    const view = await renderQuotesCard(
      makeQuoteList([makeQuote({ source_subscription_id: unsafeId })])
    )
    const dialog = await openConversionPreview(view)
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Submit conversion' })
    )
    await waitFor(() => assert.equal(view.getConfirmRequests().length, 1))
    assert.equal(view.getConfirmRequests()[0]?.subscription_id, unsafeId)
  })

  test('uses BigInt for every formula integer beyond Number.MAX_SAFE_INTEGER', () => {
    const unsafe = '9007199254740993'
    const live = deriveLiveConversionQuote(
      makeQuote({
        end_time: '1802678400',
        credit_basis: unsafe,
        current_remaining_credit: '2',
        gross_credit: '9007199254740995',
        current_debt: '1',
        estimated_debt_offset: '1',
        net_available_credit: '9007199254740994',
      }),
      0n
    )

    assert.equal(live.creditBasis, 9_007_199_254_740_993n)
    assert.equal(live.grossCredit, 9_007_199_254_740_995n)
    assert.equal(live.netAvailableCredit, 9_007_199_254_740_994n)
    assert.equal(live.formula, '1 × 9007199254740993 + 2 = 9007199254740995')
  })

  test('updates remaining time, block boundary, and formula after one local second', async () => {
    const view = await renderQuotesCard()
    assert.ok(view.getByText('1 × 100 + 25 = 125'))
    assert.ok(view.getByText('Time remaining: 1 months'))

    currentTimeMs += 1000
    await act(async () => runCapturedInterval(1000))

    assert.ok(view.getByText('0 × 100 + 25 = 25'))
    assert.ok(
      view.getByText('Time remaining: 30 days 23 hours 59 minutes 59 seconds')
    )
  })

  test('refreshes with React Query every five seconds, on focus, and before preview opens', async () => {
    const view = await renderQuotesCard()

    await act(async () => runCapturedInterval(5000))
    await waitFor(() => assert.equal(view.getRequestCount(), 2))

    await act(async () => window.dispatchEvent(new Event('focus')))
    await waitFor(() => assert.equal(view.getRequestCount(), 3))
    await waitFor(() =>
      assert.equal(
        view.getByRole('status', { name: 'Conversion quote refresh status' })
          .textContent,
        'Conversion quotes are up to date'
      )
    )

    view.setResponse(
      makeQuoteList([makeQuote({ plan_title: 'Monthly Pro latest' })])
    )
    fireEvent.click(view.getByRole('button', { name: 'Preview conversion' }))
    await waitFor(() => assert.equal(view.getRequestCount(), 4))
    const dialog = await waitFor(() => view.getByRole('dialog'))
    assert.ok(within(dialog).getByText('Monthly Pro latest'))
    assert.ok(
      within(dialog).getByText('Preview only — no conversion is submitted')
    )
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Close preview' })
    )
    await waitFor(() => assert.equal(view.queryByRole('dialog'), null))
  })

  test('pins the preview to the exact pre-open refresh response', async () => {
    const view = await renderQuotesCard()
    view.setResponse(
      makeQuoteList([makeQuote({ plan_title: 'Preview response snapshot' })])
    )
    fireEvent.click(view.getByRole('button', { name: 'Preview conversion' }))
    await waitFor(() => assert.equal(view.getRequestCount(), 2))
    const dialog = await waitFor(() => view.getByRole('dialog'))
    assert.ok(within(dialog).getByText('Preview response snapshot'))

    view.setResponse(
      makeQuoteList([makeQuote({ plan_title: 'Later cache response' })])
    )
    await act(async () => window.dispatchEvent(new Event('focus')))
    await waitFor(() => assert.equal(view.getRequestCount(), 3))

    assert.ok(within(dialog).getByText('Preview response snapshot'))
    assert.equal(within(dialog).queryByText('Later cache response'), null)
  })

  test('moves an instance across expiration and grace boundaries on the local clock', async () => {
    const view = await renderQuotesCard(
      makeQuoteList([
        makeQuote({
          database_now: '1800000000',
          end_time: '1800000001',
          remaining_seconds: '1',
          full_31_day_blocks: '0',
          gross_credit: '25',
          net_available_credit: '5',
        }),
      ])
    )
    assert.ok(view.getByRole('heading', { name: 'Convertible subscriptions' }))
    assert.ok(view.getByText('Convertible'))

    currentTimeMs += 1000
    await act(async () => runCapturedInterval(1000))
    assert.ok(
      view.getByRole('heading', {
        name: 'Expired grace-period subscriptions',
      })
    )
    assert.ok(view.getByText('Expired grace period'))

    currentTimeMs += (336 * 60 * 60 + 1) * 1000
    await act(async () => runCapturedInterval(1000))
    assert.ok(view.getByRole('heading', { name: 'Excluded subscriptions' }))
    const excludedTrigger = view.getByRole('button', {
      name: /Excluded subscriptions/,
    })
    assert.equal(excludedTrigger.getAttribute('aria-expanded'), 'false')
    fireEvent.click(excludedTrigger)
    assert.ok(view.getByText('Excluded'))
    assert.ok(view.getByText('The 336-hour conversion grace period has ended'))
  })

  test('keeps cooldown reasons synchronized with the local second clock', async () => {
    const view = await renderQuotesCard(
      makeQuoteList([
        makeQuote({
          last_granted_at: '1799913660',
          category: 'excluded',
          eligible: false,
          can_confirm: false,
          reason_codes: ['cooldown_active'],
          reasons: [
            { code: 'cooldown_active', data: { remaining_seconds: '60' } },
          ],
          cooldown_status: 'active',
          cooldown_remaining_seconds: '60',
        }),
      ])
    )
    const cooldownTrigger = view.getByRole('button', {
      name: /Excluded subscriptions/,
    })
    fireEvent.click(cooldownTrigger)
    assert.ok(
      view.getByText('Conversion cooldown is active (1 minutes remaining)')
    )

    currentTimeMs += 1000
    await act(async () => runCapturedInterval(1000))
    assert.ok(
      view.getByText('Conversion cooldown is active (59 seconds remaining)')
    )

    currentTimeMs += 59_000
    await act(async () => runCapturedInterval(1000))
    assert.equal(view.queryByText(/Conversion cooldown is active/), null)
    assert.ok(view.getByRole('heading', { name: 'Convertible subscriptions' }))
  })

  test('adds the exact exclusion reason when the live gross Credit reaches zero', async () => {
    const view = await renderQuotesCard(
      makeQuoteList([
        makeQuote({
          current_remaining_credit: '0',
          gross_credit: '100',
          estimated_debt_offset: '0',
          net_available_credit: '100',
        }),
      ])
    )
    assert.ok(view.getByRole('heading', { name: 'Convertible subscriptions' }))

    currentTimeMs += 1000
    await act(async () => runCapturedInterval(1000))

    assert.ok(view.getByRole('heading', { name: 'Excluded subscriptions' }))
    fireEvent.click(
      view.getByRole('button', { name: /Excluded subscriptions/ })
    )
    assert.ok(view.getByText('The calculated gross Credit is not positive'))
  })

  test('renders each instance in semantic sections with formula, reasons, and accessible dynamic status', async () => {
    const response = makeQuoteList([
      makeQuote(),
      makeQuote({
        source_subscription_id: '7002',
        category: 'expired_grace',
        end_time: '1799999940',
        remaining_seconds: '0',
        expired: true,
        within_grace: true,
        full_31_day_blocks: '0',
        gross_credit: '25',
        net_available_credit: '5',
        grace_status: 'active',
        grace_remaining_seconds: '120',
      }),
      makeQuote({
        source_subscription_id: '7003',
        category: 'excluded',
        eligible: false,
        can_confirm: false,
        last_granted_at: '1799913660',
        reason_codes: ['cooldown_active'],
        reasons: [
          { code: 'cooldown_active', data: { remaining_seconds: '60' } },
        ],
        cooldown_status: 'active',
        cooldown_remaining_seconds: '60',
      }),
    ])
    const view = await renderQuotesCard(response)

    assert.ok(view.getByRole('heading', { name: 'Convertible subscriptions' }))
    assert.ok(
      view.getByRole('heading', { name: 'Expired grace-period subscriptions' })
    )
    assert.ok(view.getByRole('heading', { name: 'Excluded subscriptions' }))
    assert.ok(view.getByText('Subscription #7001'))
    assert.ok(view.getByText('Subscription #7002'))
    assert.equal(view.queryByText('Subscription #7003'), null)
    assert.ok(
      view.getByRole('status', { name: 'Conversion quote refresh status' })
    )
    assert.equal(view.getAllByRole('timer').length, 2)
    fireEvent.click(
      view.getByRole('button', { name: /Excluded subscriptions/ })
    )
    assert.ok(view.getByText('Subscription #7003'))
    assert.ok(
      view.getByText('Conversion cooldown is active (1 minutes remaining)')
    )
    assert.equal(view.getAllByRole('timer').length, 3)

    fireEvent.click(
      view.getAllByRole('button', { name: 'Preview conversion' })[0]
    )
    await waitFor(() => assert.equal(view.getRequestCount(), 2))
    const dialog = await waitFor(() => view.getByRole('dialog'))
    assert.ok(within(dialog).getByText('1 × 100 + 25 = 125'))
    assert.ok(
      within(dialog).getByText(
        'Converting is irreversible. The source timed subscription remains as a converted history record, and in-flight requests settle to the target Credit balance.'
      )
    )
    assert.ok(
      within(dialog).getByText(
        'The final submission will recalculate using the latest values.'
      )
    )
    assert.ok(within(dialog).getByRole('button', { name: 'Submit conversion' }))
    assert.ok(within(dialog).getByRole('button', { name: 'Close preview' }))
  })
})
