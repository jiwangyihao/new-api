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
import type { AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  waitFor,
} from '@testing-library/react/pure'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, beforeEach, describe, test } from 'node:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import {
  creditBalancePlanFormToRequest,
  creditBalancePlanToFormValues,
  submitCreditBalancePlanForm,
} from '../lib/credit-balance-plan-form'
import type { SubscriptionPlan } from '../types'
import { CreditBalancePlanCard } from './credit-balance-plan-card'
import {
  SubscriptionPurchaseDialog,
  pendingExternalOrderStorageKey,
} from './dialogs/subscription-purchase-dialog'

const originalAPIAdapter = api.defaults.adapter
const originalWindowOpen = window.open
const originalSessionStorageGetItem = window.sessionStorage.getItem
const originalSessionStorageRemoveItem = window.sessionStorage.removeItem
const testUserId = 7001

function setTestAuthUser(id = testUserId): void {
  useAuthStore.getState().auth.setUser({
    id,
    username: `user-${id}`,
    role: 1,
  })
}

beforeEach(() => {
  setTestAuthUser()
})

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAPIAdapter
  window.open = originalWindowOpen
  Object.defineProperty(window.sessionStorage, 'getItem', {
    configurable: true,
    writable: true,
    value: originalSessionStorageGetItem,
  })
  Object.defineProperty(window.sessionStorage, 'removeItem', {
    configurable: true,
    writable: true,
    value: originalSessionStorageRemoveItem,
  })
  window.sessionStorage.clear()
  useAuthStore.getState().auth.reset()
})

function makeCreditBalancePlan(
  overrides: Partial<SubscriptionPlan> = {}
): SubscriptionPlan {
  return {
    id: 9001,
    title: 'Credit balance',
    price_amount: 0,
    currency: 'CNY',
    valuation_currency: 'CNY',
    duration_unit: 'month',
    duration_value: 1,
    quota_reset_period: 'never',
    enabled: true,
    sort_order: 0,
    max_purchase_per_user: 0,
    total_amount: 0,
    entitlement_type: 'credit_balance',
    concurrency_limit: 7,
    queue_capacity: 13,
    business_code: 'credit_balance_global',
    credit_balance_configured: true,
    credit_balance_purchase_enabled: true,
    credit_balance_redemption_enabled: false,
    credit_balance_conversion_enabled: true,
    ...overrides,
  }
}

function makeExternalPurchasePlan(id: number): SubscriptionPlan {
  return {
    id,
    title: 'External Credit option',
    price_amount: 40,
    currency: 'CNY',
    duration_unit: 'month',
    duration_value: 1,
    quota_reset_period: 'monthly',
    enabled: true,
    sort_order: 1,
    max_purchase_per_user: 0,
    total_amount: 0,
    monthly_token_limit: 1000,
    concurrency_limit: 1,
    public_visible: true,
    unlimited_purchase_enabled: true,
    stripe_price_id: 'price_external_credit',
  }
}

describe('Credit balance plan admin component', () => {
  test('maps the dedicated plan response into visible editable fields', () => {
    const values = creditBalancePlanToFormValues(makeCreditBalancePlan())

    assert.deepEqual(values, {
      concurrency_limit: 7,
      queue_capacity: 13,
      business_code: 'credit_balance_global',
      valuation_currency: 'CNY',
      configured: true,
      purchase_enabled: true,
      redemption_enabled: false,
      conversion_enabled: true,
    })
  })

  test('omits legacy model scope and preserves independent entry switches', () => {
    const payload = creditBalancePlanFormToRequest({
      concurrency_limit: 9,
      queue_capacity: 21,
      business_code: ' credit_balance_global ',
      valuation_currency: 'USD',
      configured: true,
      purchase_enabled: false,
      redemption_enabled: true,
      conversion_enabled: false,
    })

    assert.deepEqual(payload, {
      concurrency_limit: 9,
      queue_capacity: 21,
      business_code: 'credit_balance_global',
      valuation_currency: 'USD',
      configured: true,
      purchase_enabled: false,
      redemption_enabled: true,
      conversion_enabled: false,
    })
  })

  test('turning configuration off also disables every new entry point', () => {
    const payload = creditBalancePlanFormToRequest({
      concurrency_limit: 1,
      queue_capacity: 2,
      business_code: 'credit_balance_global',
      valuation_currency: 'CNY',
      configured: false,
      purchase_enabled: true,
      redemption_enabled: true,
      conversion_enabled: true,
    })

    assert.equal(payload.purchase_enabled, false)
    assert.equal(payload.redemption_enabled, false)
    assert.equal(payload.conversion_enabled, false)
  })

  test('submits through the dedicated API and returns the persisted result', async () => {
    const expected = makeCreditBalancePlan({ concurrency_limit: 11 })
    let submitted: ReturnType<typeof creditBalancePlanFormToRequest> | undefined

    const result = await submitCreditBalancePlanForm(
      creditBalancePlanToFormValues(expected),
      async (payload) => {
        submitted = payload
        return { success: true, data: expected }
      }
    )

    assert.equal(submitted?.concurrency_limit, 11)
    assert.equal(result.success, true)
    assert.equal(result.data?.entitlement_type, 'credit_balance')
  })

  test('submits the editor and rehydrates controls from the persisted response', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })

    const persisted = makeCreditBalancePlan({
      concurrency_limit: 17,
      queue_capacity: 23,
      business_code: 'server_business_code',
      credit_balance_purchase_enabled: false,
      credit_balance_redemption_enabled: true,
      credit_balance_conversion_enabled: false,
    })
    let submitted: ReturnType<typeof creditBalancePlanFormToRequest> | undefined
    api.defaults.adapter = async (config) => {
      assert.equal(config.method, 'put')
      assert.equal(config.url, '/api/subscription/admin/credit-balance-plan')
      submitted = JSON.parse(String(config.data))
      return {
        data: { success: true, data: persisted },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }

    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Infinity },
        mutations: { retry: false },
      },
    })
    queryClient.setQueryData(['admin-credit-balance-plan'], {
      success: true,
      data: makeCreditBalancePlan(),
    })

    const view = render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <CreditBalancePlanCard />
        </QueryClientProvider>
      </I18nextProvider>
    )

    fireEvent.input(view.getByLabelText('Concurrency Limit'), {
      target: { value: '9' },
    })
    fireEvent.input(view.getByLabelText('Queue Capacity'), {
      target: { value: '21' },
    })
    fireEvent.input(view.getByLabelText('Business Code'), {
      target: { value: ' client_business_code ' },
    })
    fireEvent.click(
      view.getByRole('switch', { name: 'New Credit balance purchases' })
    )
    fireEvent.click(
      view.getByRole('switch', { name: 'New Credit balance redemptions' })
    )
    fireEvent.click(
      view.getByRole('switch', { name: 'New timed plan conversions' })
    )
    fireEvent.click(view.getByRole('button', { name: 'Save configuration' }))

    await waitFor(() => {
      assert.deepEqual(submitted, {
        concurrency_limit: 9,
        queue_capacity: 21,
        business_code: 'client_business_code',
        valuation_currency: 'CNY',
        configured: true,
        purchase_enabled: false,
        redemption_enabled: true,
        conversion_enabled: false,
      })
      assert.equal(view.queryByLabelText('Model scope'), null)
      assert.equal(
        (view.getByLabelText('Concurrency Limit') as HTMLInputElement).value,
        '17'
      )
      assert.equal(
        (view.getByLabelText('Queue Capacity') as HTMLInputElement).value,
        '23'
      )
      assert.equal(
        (view.getByLabelText('Business Code') as HTMLInputElement).value,
        'server_business_code'
      )
      assert.equal(
        view
          .getByRole('switch', { name: 'New Credit balance purchases' })
          .getAttribute('aria-checked'),
        'false'
      )
      assert.equal(
        view
          .getByRole('switch', { name: 'New Credit balance redemptions' })
          .getAttribute('aria-checked'),
        'true'
      )
      assert.equal(
        view
          .getByRole('switch', { name: 'New timed plan conversions' })
          .getAttribute('aria-checked'),
        'false'
      )
    })
  })

  test('supports keyboard mode selection and exposes external checkout for both modes', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const purchasePlan: SubscriptionPlan = {
      id: 9010,
      title: 'Monthly',
      price_amount: 40,
      currency: 'CNY',
      duration_unit: 'month',
      duration_value: 1,
      quota_reset_period: 'monthly',
      enabled: true,
      sort_order: 1,
      max_purchase_per_user: 0,
      total_amount: 0,
      monthly_token_limit: 1_500_000,
      concurrency_limit: 1,
      public_visible: true,
      unlimited_purchase_enabled: true,
      stripe_price_id: 'price_stripe',
      creem_product_id: 'product_creem',
      kyren_product_id: 'product_kyren',
    }
    const queryClient = new QueryClient()
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => {}}
            plan={{ plan: purchasePlan }}
            enableStripe
            enableCreem
            enableKyrenSubscription
            creditBalancePurchaseEnabled
            creditBalancePlan={{
              concurrency_limit: 2,
              queue_capacity: 4,
            }}
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    assert.ok(view.getByRole('dialog'))
    assert.ok(view.getByRole('button', { name: 'Stripe' }))
    const timed = view.getByRole('radio', { name: /Timed subscription/ })
    const credit = view.getByRole('radio', { name: /Credit balance/ })
    const dialogText = view.getByRole('dialog').textContent || ''
    assert.match(
      dialogText,
      /Adds 1\.5M permanent Credits using the global Credit balance service limits\./
    )
    assert.doesNotMatch(dialogText, /1500000/)

    fireEvent.click(timed)
    await waitFor(() =>
      assert.equal(timed.getAttribute('aria-checked'), 'true')
    )
    assert.ok(view.getByRole('button', { name: 'Stripe' }))
    assert.ok(view.getByRole('button', { name: 'Creem' }))
    assert.ok(view.getByRole('button', { name: 'Pay with Kyren' }))

    timed.focus()
    fireEvent.keyDown(timed, { key: 'ArrowDown', code: 'ArrowDown' })
    await waitFor(() =>
      assert.equal(credit.getAttribute('aria-checked'), 'true')
    )
    const creditSummary = view.getByText('Credits added').parentElement
    assert.match(creditSummary?.textContent || '', /1\.5M credits/)
    assert.ok(view.getByRole('button', { name: 'Stripe' }))
    assert.ok(view.getByRole('button', { name: 'Creem' }))
    assert.ok(view.getByRole('button', { name: 'Pay with Kyren' }))
    assert.ok(
      view.getByText((_text, element) =>
        Boolean(
          element?.getAttribute('data-slot') === 'alert-description' &&
          element.textContent?.includes('Concurrency Limit') &&
          element.textContent?.includes('Queue Capacity')
        )
      )
    )
  })

  test('keeps non-CNY Credit checkout on Stripe and Creem only', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const purchasePlan: SubscriptionPlan = {
      ...makeExternalPurchasePlan(9011),
      currency: 'USD',
      creem_product_id: 'product_creem_usd',
      kyren_product_id: 'product_kyren_usd',
    }
    const queryClient = new QueryClient()
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => {}}
            plan={{ plan: purchasePlan }}
            enableStripe
            enableCreem
            enableOnlineTopUp
            epayMethods={[{ type: 'alipay', name: 'Alipay' }]}
            enableKyrenSubscription
            creditBalancePurchaseEnabled
            lastPurchaseMode='credit_balance'
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const credit = view.getByRole('radio', { name: /Credit balance/ })
    await waitFor(() =>
      assert.equal(credit.getAttribute('aria-checked'), 'true')
    )
    assert.equal(
      view.getByRole('button', { name: 'Stripe' }).hasAttribute('disabled'),
      false
    )
    assert.equal(
      view.getByRole('button', { name: 'Creem' }).hasAttribute('disabled'),
      false
    )
    assert.equal(
      view
        .getByRole('button', { name: 'Pay with Kyren' })
        .hasAttribute('disabled'),
      true
    )
    assert.equal(
      view.queryByRole('button', { name: /Pay with Account Balance/ }),
      null
    )
    assert.equal(view.queryByRole('button', { name: 'Pay' }), null)
  })

  test('submits explicit Credit mode, polls the order, and refreshes after success', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const purchasePlan: SubscriptionPlan = {
      id: 9012,
      title: 'External Credit option',
      price_amount: 40,
      currency: 'CNY',
      duration_unit: 'month',
      duration_value: 1,
      quota_reset_period: 'monthly',
      enabled: true,
      sort_order: 1,
      max_purchase_per_user: 0,
      total_amount: 0,
      monthly_token_limit: 1000,
      concurrency_limit: 1,
      public_visible: true,
      unlimited_purchase_enabled: true,
      stripe_price_id: 'price_external_credit',
    }
    let submitted: Record<string, unknown> | undefined
    const statusGate = (
      Promise as PromiseConstructor & {
        withResolvers<T>(): {
          promise: Promise<T>
          resolve: (value: T | PromiseLike<T>) => void
          reject: (reason?: unknown) => void
        }
      }
    ).withResolvers<AxiosResponse<Record<string, unknown>>>()
    let openedUrl = ''
    let refreshCount = 0
    let closed = false
    window.open = ((url?: string | URL) => {
      openedUrl = String(url || '')
      return null
    }) as typeof window.open
    api.defaults.adapter = async (config) => {
      if (config.method === 'post') {
        assert.equal(config.url, '/api/subscription/stripe/pay')
        submitted = JSON.parse(String(config.data))
        return {
          data: {
            message: 'success',
            data: {
              pay_link: 'https://checkout.example/stripe-credit',
              order_id: 'stripe-credit-order',
            },
          },
          status: 200,
          statusText: 'OK',
          headers: {},
          config,
        }
      }
      assert.equal(config.method, 'get')
      assert.equal(config.url, '/api/subscription/orders/stripe-credit-order')
      return await statusGate.promise
    }
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={(open) => {
              closed = !open
            }}
            plan={{ plan: purchasePlan }}
            enableStripe
            creditBalancePurchaseEnabled
            lastPurchaseMode='credit_balance'
            onPurchaseSuccess={() => {
              refreshCount += 1
            }}
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const credit = view.getByRole('radio', { name: /Credit balance/ })
    await waitFor(() =>
      assert.equal(credit.getAttribute('aria-checked'), 'true')
    )
    fireEvent.click(view.getByRole('button', { name: 'Stripe' }))
    await waitFor(() =>
      assert.equal(openedUrl, 'https://checkout.example/stripe-credit')
    )
    assert.deepEqual(submitted, {
      plan_id: purchasePlan.id,
      purchase_mode: 'credit_balance',
    })
    assert.equal(openedUrl, 'https://checkout.example/stripe-credit')
    assert.ok(
      view.getByText(
        'Waiting for payment confirmation. You can close this dialog and resume here later.'
      )
    )
    assert.equal(view.getByRole('alert').getAttribute('aria-live'), 'polite')
    assert.deepEqual(
      JSON.parse(
        window.sessionStorage.getItem(
          pendingExternalOrderStorageKey(testUserId, purchasePlan.id)
        ) || '{}'
      ),
      {
        ownerUserId: testUserId,
        tradeNo: 'stripe-credit-order',
        provider: 'stripe',
        purchaseMode: 'credit_balance',
      }
    )

    statusGate.resolve({
      data: {
        success: true,
        data: {
          trade_no: 'stripe-credit-order',
          plan_id: purchasePlan.id,
          payment_provider: 'stripe',
          payment_method: 'stripe',
          purchase_mode: 'credit_balance',
          status: 'success',
          create_time: 1,
          complete_time: 2,
          credit_balance: {
            user_subscription_id: 3,
            plan_id: 4,
            gross_credit: 1000,
            debt_offset: 250,
            available_credit: 750,
            settlement_debt: 0,
            balance_before: -250,
            balance_after: 750,
            active: true,
            ledger_id: 5,
            status: 'available',
          },
        },
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config: {} as InternalAxiosRequestConfig,
    })
    await waitFor(() => assert.equal(refreshCount, 1))
    assert.equal(closed, true)
    assert.equal(
      window.sessionStorage.getItem(
        pendingExternalOrderStorageKey(testUserId, purchasePlan.id)
      ),
      null
    )
    queryClient.clear()
  })

  test('restores a pending checkout after a fresh dialog mount', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const purchasePlan = makeExternalPurchasePlan(9013)
    window.sessionStorage.setItem(
      pendingExternalOrderStorageKey(testUserId, purchasePlan.id),
      JSON.stringify({
        ownerUserId: testUserId,
        tradeNo: 'restored-stripe-order',
        provider: 'stripe',
        purchaseMode: 'credit_balance',
      })
    )
    let statusChecks = 0
    api.defaults.adapter = async (config) => {
      statusChecks += 1
      assert.equal(config.url, '/api/subscription/orders/restored-stripe-order')
      return {
        data: {
          success: true,
          data: {
            trade_no: 'restored-stripe-order',
            plan_id: purchasePlan.id,
            payment_provider: 'stripe',
            payment_method: 'stripe',
            purchase_mode: 'credit_balance',
            status: 'pending',
            create_time: 1,
            complete_time: 0,
          },
        },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => {}}
            plan={{ plan: purchasePlan }}
            enableStripe
            creditBalancePurchaseEnabled
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    await waitFor(() => assert.ok(statusChecks >= 1))
    const alert = view.getByRole('alert')
    assert.equal(alert.getAttribute('aria-live'), 'polite')
    assert.ok(
      view.getByText(
        'Waiting for payment confirmation. You can close this dialog and resume here later.'
      )
    )
    assert.ok(
      window.sessionStorage.getItem(
        pendingExternalOrderStorageKey(testUserId, purchasePlan.id)
      )
    )
    view.unmount()
    queryClient.clear()
  })

  test('stops polling the old order when account changes while open', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const purchasePlan = makeExternalPurchasePlan(9019)
    window.sessionStorage.setItem(
      pendingExternalOrderStorageKey(testUserId, purchasePlan.id),
      JSON.stringify({
        ownerUserId: testUserId,
        tradeNo: 'live-switch-order',
        provider: 'stripe',
        purchaseMode: 'credit_balance',
      })
    )
    let statusChecks = 0
    api.defaults.adapter = async (config) => {
      statusChecks += 1
      return {
        data: {
          success: true,
          data: {
            trade_no: 'live-switch-order',
            plan_id: purchasePlan.id,
            payment_provider: 'stripe',
            payment_method: 'stripe',
            purchase_mode: 'credit_balance',
            status: 'pending',
            create_time: 1,
            complete_time: 0,
          },
        },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => {}}
            plan={{ plan: purchasePlan }}
            enableStripe
            creditBalancePurchaseEnabled
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    await waitFor(() => assert.equal(statusChecks, 1))
    act(() => setTestAuthUser(testUserId + 1))
    await waitFor(() => assert.equal(view.queryByRole('alert'), null))
    await Promise.resolve()
    assert.equal(statusChecks, 1)
    view.unmount()
    queryClient.clear()
  })

  test('does not restore another user pending checkout after account switch', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const purchasePlan = makeExternalPurchasePlan(9018)
    window.sessionStorage.setItem(
      pendingExternalOrderStorageKey(testUserId, purchasePlan.id),
      JSON.stringify({
        ownerUserId: testUserId,
        tradeNo: 'previous-user-order',
        provider: 'stripe',
        purchaseMode: 'credit_balance',
      })
    )
    setTestAuthUser(testUserId + 1)
    let statusChecks = 0
    api.defaults.adapter = async () => {
      statusChecks += 1
      throw new Error('unexpected order lookup')
    }
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => {}}
            plan={{ plan: purchasePlan }}
            enableStripe
            creditBalancePurchaseEnabled
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    await new Promise((resolve) => setTimeout(resolve, 25))
    assert.equal(statusChecks, 0)
    assert.equal(view.queryByRole('alert'), null)
    assert.ok(
      window.sessionStorage.getItem(
        pendingExternalOrderStorageKey(testUserId, purchasePlan.id)
      )
    )
    queryClient.clear()
  })

  for (const terminalStatus of ['failed', 'expired'] as const) {
    test(`clears a restored ${terminalStatus} order and enables retry`, async () => {
      const i18n = createInstance()
      await i18n.init({
        lng: 'en',
        fallbackLng: false,
        resources: { en: { translation: {} } },
        interpolation: { escapeValue: false },
      })
      const purchasePlan = makeExternalPurchasePlan(
        terminalStatus === 'failed' ? 9014 : 9015
      )
      const storageKey = pendingExternalOrderStorageKey(
        testUserId,
        purchasePlan.id
      )
      window.sessionStorage.setItem(
        storageKey,
        JSON.stringify({
          ownerUserId: testUserId,
          tradeNo: `${terminalStatus}-stripe-order`,
          provider: 'stripe',
          purchaseMode: 'credit_balance',
        })
      )
      api.defaults.adapter = async (config) => ({
        data: {
          success: true,
          data: {
            trade_no: `${terminalStatus}-stripe-order`,
            plan_id: purchasePlan.id,
            payment_provider: 'stripe',
            payment_method: 'stripe',
            purchase_mode: 'credit_balance',
            status: terminalStatus,
            create_time: 1,
            complete_time: 2,
          },
        },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      })
      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      const view = render(
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <SubscriptionPurchaseDialog
              open
              onOpenChange={() => {}}
              plan={{ plan: purchasePlan }}
              enableStripe
              creditBalancePurchaseEnabled
            />
          </I18nextProvider>
        </QueryClientProvider>
      )

      await waitFor(() =>
        assert.equal(window.sessionStorage.getItem(storageKey), null)
      )
      assert.equal(
        (view.getByRole('button', { name: 'Stripe' }) as HTMLButtonElement)
          .disabled,
        false
      )
      queryClient.clear()
    })
  }

  test('offers manual status retry and safely abandons an unreachable order', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const purchasePlan = makeExternalPurchasePlan(9016)
    const storageKey = pendingExternalOrderStorageKey(
      testUserId,
      purchasePlan.id
    )
    window.sessionStorage.setItem(
      storageKey,
      JSON.stringify({
        ownerUserId: testUserId,
        tradeNo: 'unreachable-stripe-order',
        provider: 'stripe',
        purchaseMode: 'credit_balance',
      })
    )
    let statusChecks = 0
    api.defaults.adapter = async () => {
      statusChecks += 1
      throw new Error('status unavailable')
    }
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => {}}
            plan={{ plan: purchasePlan }}
            enableStripe
            creditBalancePurchaseEnabled
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const retry = await view.findByRole('button', {
      name: 'Retry status check',
    })
    fireEvent.click(retry)
    await waitFor(() => assert.ok(statusChecks >= 2))
    const tryAgain = await view.findByRole('button', {
      name: 'Try payment again',
    })
    fireEvent.click(tryAgain)
    await waitFor(() =>
      assert.equal(window.sessionStorage.getItem(storageKey), null)
    )
    assert.equal(
      (view.getByRole('button', { name: 'Stripe' }) as HTMLButtonElement)
        .disabled,
      false
    )
    queryClient.clear()
  })

  test('retries an unreachable Kyren order with its predecessor trade number', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const purchasePlan = {
      ...makeExternalPurchasePlan(9017),
      kyren_product_id: 'prod_retry_kyren',
    }
    const storageKey = pendingExternalOrderStorageKey(
      testUserId,
      purchasePlan.id
    )
    window.sessionStorage.setItem(
      storageKey,
      JSON.stringify({
        ownerUserId: testUserId,
        tradeNo: 'unreachable-kyren-order',
        provider: 'kyren',
        purchaseMode: 'credit_balance',
      })
    )
    let paymentRequest: Record<string, unknown> | null = null
    api.defaults.adapter = async (config) => {
      if (config.method === 'post' && config.url?.endsWith('/kyren/pay')) {
        paymentRequest = JSON.parse(String(config.data)) as Record<
          string,
          unknown
        >
        return {
          data: {
            success: true,
            data: {
              checkout_url: 'https://checkout.example/kyren-retry',
              order_id: 'successor-kyren-order',
            },
          },
          status: 200,
          statusText: 'OK',
          headers: {},
          config,
        }
      }
      throw new Error('status unavailable')
    }
    const openedUrls: string[] = []
    window.open = ((url?: string | URL) => {
      openedUrls.push(String(url || ''))
      return null
    }) as typeof window.open
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => {}}
            plan={{ plan: purchasePlan }}
            enableKyrenSubscription
            creditBalancePurchaseEnabled
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const tryAgain = await view.findByRole('button', {
      name: 'Try payment again',
    })
    fireEvent.click(tryAgain)
    await waitFor(() => assert.ok(paymentRequest))

    assert.deepEqual(paymentRequest, {
      plan_id: purchasePlan.id,
      purchase_mode: 'credit_balance',
      retry_trade_no: 'unreachable-kyren-order',
    })
    await waitFor(() =>
      assert.equal(
        JSON.parse(String(window.sessionStorage.getItem(storageKey))).tradeNo,
        'successor-kyren-order'
      )
    )
    assert.deepEqual(openedUrls, [
      'https://checkout.example/kyren-retry',
    ])
    queryClient.clear()
  })

  test('opens a reusable checkout from a pending order status response', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      resources: {
        en: {
          translation: {
            'Continue payment': 'Continue payment',
            'Waiting for payment confirmation. You can close this dialog and resume here later.':
              'Waiting for payment confirmation. You can close this dialog and resume here later.',
          },
        },
      },
      interpolation: { escapeValue: false },
    })
    const purchasePlan = makeExternalPurchasePlan(9018)
    window.sessionStorage.setItem(
      pendingExternalOrderStorageKey(testUserId, purchasePlan.id),
      JSON.stringify({
        ownerUserId: testUserId,
        tradeNo: 'reusable-kyren-order',
        provider: 'kyren',
        purchaseMode: 'timed',
      })
    )
    let openedUrl = ''
    window.open = ((url?: string | URL) => {
      openedUrl = String(url || '')
      return null
    }) as typeof window.open
    api.defaults.adapter = async (config) => ({
      data: {
        success: true,
        data: {
          trade_no: 'reusable-kyren-order',
          plan_id: purchasePlan.id,
          payment_provider: 'kyren',
          payment_method: 'kyren',
          purchase_mode: 'timed',
          status: 'pending',
          create_time: 1,
          complete_time: 0,
          checkout_url: 'https://checkout.example/reusable-kyren',
        },
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => undefined}
            plan={{ plan: purchasePlan }}
            enableKyrenSubscription
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    fireEvent.click(await view.findByRole('button', { name: 'Continue payment' }))

    assert.equal(openedUrl, 'https://checkout.example/reusable-kyren')
    view.unmount()
    queryClient.clear()
  })

  test('does not crash when pending-order storage access is denied', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    Object.defineProperty(window.sessionStorage, 'getItem', {
      configurable: true,
      writable: true,
      value: () => {
        throw new DOMException('storage denied', 'SecurityError')
      },
    })
    Object.defineProperty(window.sessionStorage, 'removeItem', {
      configurable: true,
      writable: true,
      value: () => {
        throw new DOMException('storage denied', 'SecurityError')
      },
    })
    const queryClient = new QueryClient()
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => {}}
            plan={{ plan: makeExternalPurchasePlan(9017) }}
            enableStripe
            creditBalancePurchaseEnabled
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    assert.ok(view.getByRole('dialog'))
    queryClient.clear()
  })

  test('restores Credit preference without exposing account balance payment', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })
    const purchasePlan: SubscriptionPlan = {
      id: 9011,
      title: 'Monthly Credit option',
      price_amount: 40,
      currency: 'CNY',
      duration_unit: 'month',
      duration_value: 1,
      quota_reset_period: 'monthly',
      enabled: true,
      sort_order: 1,
      max_purchase_per_user: 0,
      total_amount: 0,
      monthly_token_limit: 1000,
      concurrency_limit: 1,
      public_visible: true,
      unlimited_purchase_enabled: true,
    }
    const queryClient = new QueryClient()
    const view = render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <SubscriptionPurchaseDialog
            open
            onOpenChange={() => {}}
            plan={{ plan: purchasePlan }}
            creditBalancePurchaseEnabled
            lastPurchaseMode='credit_balance'
            creditBalancePlan={{
              concurrency_limit: 2,
              queue_capacity: 4,
            }}
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const credit = view.getByRole('radio', { name: /Credit balance/ })
    await waitFor(() =>
      assert.equal(credit.getAttribute('aria-checked'), 'true')
    )
    assert.equal(
      view.queryByRole('button', { name: /Pay with Account Balance/ }),
      null
    )
    assert.doesNotMatch(
      view.getByRole('dialog').textContent || '',
      /Account Balance/
    )
  })

  test('renders the persisted configuration as an accessible admin form', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
      interpolation: { escapeValue: false },
    })

    const queryClient = new QueryClient()
    queryClient.setQueryData(['admin-credit-balance-plan'], {
      success: true,
      data: makeCreditBalancePlan(),
    })
    const markup = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <CreditBalancePlanCard />
        </QueryClientProvider>
      </I18nextProvider>
    )

    assert.match(markup, /Credit balance plan/)
    assert.match(markup, /Renminbi account balance/)
    assert.match(markup, /Timed plans/)
    assert.match(markup, /<form/)
    assert.doesNotMatch(markup, /credit-balance-model-limits/)
    assert.doesNotMatch(markup, /Model scope/)
    assert.match(markup, /credit-balance-purchase-enabled/)
    assert.match(markup, /credit-balance-redemption-enabled/)
    assert.match(markup, /credit-balance-conversion-enabled/)
    assert.match(markup, /type="submit"/)
  })
})
