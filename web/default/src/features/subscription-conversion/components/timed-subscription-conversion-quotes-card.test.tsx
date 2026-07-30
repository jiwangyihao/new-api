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
} from '@testing-library/react/pure'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, beforeEach, describe, test } from 'node:test'
import { I18nextProvider } from 'react-i18next'
import { CONVERSION_QUOTE_REFETCH_MS } from '../hooks/use-subscription-conversion-quotes'
import { deriveLiveConversionQuote } from '../live-quote'
import type {
  SubscriptionConversionQuote,
  SubscriptionConversionQuoteList,
} from '../types'
import { TimedSubscriptionConversionQuotesCard } from './timed-subscription-conversion-quotes-card'

const originalSetInterval = globalThis.setInterval
const originalClearInterval = globalThis.clearInterval

type CapturedInterval = {
  id: number
  delay: number
  callback: () => void
}

let currentTimeMs = 1_000_000
let nextIntervalId = 1
let capturedIntervals: CapturedInterval[] = []

beforeEach(() => {
  currentTimeMs = 1_000_000
  nextIntervalId = 1
  capturedIntervals = []
  globalThis.setInterval = ((callback: TimerHandler, delay?: number) => {
    const id = nextIntervalId++
    capturedIntervals.push({
      id,
      delay: Number(delay || 0),
      callback: callback as () => void,
    })
    return id
  }) as typeof setInterval
  globalThis.clearInterval = ((id?: number | NodeJS.Timeout) => {
    const numericId = Number(id)
    capturedIntervals = capturedIntervals.filter(
      (entry) => entry.id !== numericId
    )
  }) as unknown as typeof clearInterval
})

afterEach(() => {
  cleanup()
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
  quotes: SubscriptionConversionQuote[] = [makeQuote()]
): SubscriptionConversionQuoteList {
  return { database_now: '1800000000', quotes }
}

async function renderQuotesCard(
  response: SubscriptionConversionQuoteList = makeQuoteList()
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
  let requestCount = 0
  let currentResponse = response
  const loadQuotes = async () => {
    requestCount += 1
    return currentResponse
  }
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <TimedSubscriptionConversionQuotesCard
          now={() => currentTimeMs}
          loadQuotes={loadQuotes}
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
    setResponse: (next: SubscriptionConversionQuoteList) => {
      currentResponse = next
    },
  }
}

function runCapturedInterval(delay: number) {
  const interval = capturedIntervals.find((entry) => entry.delay === delay)
  assert.ok(interval, `missing ${delay}ms interval`)
  interval.callback()
}

describe('timed subscription conversion quotes', () => {
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
    assert.ok(view.getByText('2678400 seconds remaining'))

    currentTimeMs += 1000
    await act(async () => runCapturedInterval(1000))

    assert.ok(view.getByText('0 × 100 + 25 = 25'))
    assert.ok(view.getByText('2678399 seconds remaining'))
  })

  test('refreshes with React Query every five seconds, on focus, and before preview opens', async () => {
    const view = await renderQuotesCard()
    assert.equal(CONVERSION_QUOTE_REFETCH_MS, 5000)

    await act(async () => runCapturedInterval(CONVERSION_QUOTE_REFETCH_MS))
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
    assert.ok(
      view.getByText('Conversion cooldown is active (60 seconds remaining)')
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
    assert.ok(view.getByText('Subscription #7003'))
    assert.ok(
      view.getByText('Conversion cooldown is active (60 seconds remaining)')
    )
    assert.ok(
      view.getByRole('status', { name: 'Conversion quote refresh status' })
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
        'Converting is irreversible and removes the source timed subscription.'
      )
    )
    assert.ok(
      within(dialog).getByText(
        'The final submission will recalculate using the latest values.'
      )
    )
    assert.equal(
      within(dialog).queryByRole('button', { name: 'Submit conversion' }),
      null
    )
    assert.ok(within(dialog).getByRole('button', { name: 'Close preview' }))
  })
})
