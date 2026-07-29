import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, describe, test } from 'node:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'
import { api } from '@/lib/api'
import {
  creditBalancePlanFormToRequest,
  creditBalancePlanToFormValues,
  submitCreditBalancePlanForm,
} from '../lib/credit-balance-plan-form'
import type { SubscriptionPlan } from '../types'
import { CreditBalancePlanCard } from './credit-balance-plan-card'

const { cleanup, fireEvent, render, waitFor } =
  await import('@testing-library/react/pure')
const { SubscriptionPurchaseDialog } =
  await import('./dialogs/subscription-purchase-dialog')
const originalAPIAdapter = api.defaults.adapter

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAPIAdapter
})

function makeCreditBalancePlan(
  overrides: Partial<SubscriptionPlan> = {}
): SubscriptionPlan {
  return {
    id: 9001,
    title: 'Credit balance',
    price_amount: 0,
    currency: 'CNY',
    duration_unit: 'month',
    duration_value: 1,
    quota_reset_period: 'never',
    enabled: true,
    sort_order: 0,
    max_purchase_per_user: 0,
    total_amount: 0,
    entitlement_type: 'credit_balance',
    model_limits: 'gpt-4o,claude-3-7-sonnet',
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

describe('Credit balance plan admin component', () => {
  test('maps the dedicated plan response into visible editable fields', () => {
    const values = creditBalancePlanToFormValues(makeCreditBalancePlan())

    assert.deepEqual(values, {
      model_limits: 'gpt-4o,claude-3-7-sonnet',
      concurrency_limit: 7,
      queue_capacity: 13,
      business_code: 'credit_balance_global',
      configured: true,
      purchase_enabled: true,
      redemption_enabled: false,
      conversion_enabled: true,
    })
  })

  test('normalizes submitted model scope and preserves independent entry switches', () => {
    const payload = creditBalancePlanFormToRequest({
      model_limits: ' gpt-4o, claude-3-7-sonnet, gpt-4o ',
      concurrency_limit: 9,
      queue_capacity: 21,
      business_code: ' credit_balance_global ',
      configured: true,
      purchase_enabled: false,
      redemption_enabled: true,
      conversion_enabled: false,
    })

    assert.deepEqual(payload, {
      model_limits: 'gpt-4o,claude-3-7-sonnet',
      concurrency_limit: 9,
      queue_capacity: 21,
      business_code: 'credit_balance_global',
      configured: true,
      purchase_enabled: false,
      redemption_enabled: true,
      conversion_enabled: false,
    })
  })

  test('turning configuration off also disables every new entry point', () => {
    const payload = creditBalancePlanFormToRequest({
      model_limits: 'gpt-4o',
      concurrency_limit: 1,
      queue_capacity: 2,
      business_code: 'credit_balance_global',
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
      model_limits: 'server-model',
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

    fireEvent.input(view.getByLabelText('Model scope'), {
      target: { value: ' client-model, client-model ' },
    })
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
        model_limits: 'client-model',
        concurrency_limit: 9,
        queue_capacity: 21,
        business_code: 'client_business_code',
        configured: true,
        purchase_enabled: false,
        redemption_enabled: true,
        conversion_enabled: false,
      })
      assert.equal(
        (view.getByLabelText('Model scope') as HTMLTextAreaElement).value,
        'server-model'
      )
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

  test('supports keyboard mode selection and only exposes timed external checkout', async () => {
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
      monthly_token_limit: 1000,
      concurrency_limit: 1,
      public_visible: true,
      unlimited_purchase_enabled: true,
      stripe_price_id: 'price_stripe',
      creem_product_id: 'product_creem',
      kyren_product_id: 'product_kyren',
    }
    const view = render(
      <I18nextProvider i18n={i18n}>
        <SubscriptionPurchaseDialog
          open
          onOpenChange={() => {}}
          plan={{ plan: purchasePlan }}
          accountBalance={10000}
          enableStripe
          enableCreem
          enableKyrenSubscription
          creditBalancePurchaseEnabled
          creditBalancePlan={{
            model_limits: 'gpt-4o',
            concurrency_limit: 2,
            queue_capacity: 4,
          }}
        />
      </I18nextProvider>
    )

    assert.ok(view.getByRole('dialog'))
    assert.equal(view.queryByRole('button', { name: 'Stripe' }), null)
    const timed = view.getByRole('radio', { name: /Timed subscription/ })
    const credit = view.getByRole('radio', { name: /Credit balance/ })

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
    assert.equal(view.queryByRole('button', { name: 'Stripe' }), null)
    assert.equal(view.queryByRole('button', { name: 'Creem' }), null)
    assert.equal(view.queryByRole('button', { name: 'Pay with Kyren' }), null)
    assert.ok(
      view.getByText((_text, element) =>
        Boolean(
          element?.getAttribute('data-slot') === 'alert-description' &&
          element.textContent?.includes('gpt-4o')
        )
      )
    )
  })

  test('restores Credit preference and submits an explicit balance purchase', async () => {
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
    let submitted: Record<string, unknown> | undefined
    let refreshCount = 0
    let closed = false
    api.defaults.adapter = async (config) => {
      assert.equal(config.method, 'post')
      assert.equal(config.url, '/api/subscription/balance/pay')
      submitted = JSON.parse(String(config.data))
      return {
        data: {
          success: true,
          message: 'success',
          data: {
            order: {
              id: 1,
              user_id: 2,
              plan_id: purchasePlan.id,
              money: purchasePlan.price_amount,
              trade_no: 'balance-order',
              payment_method: 'account_balance',
              payment_provider: 'balance',
              status: 'success',
              create_time: 1,
              complete_time: 2,
              provider_payload: '',
            },
            purchase_mode: 'credit_balance',
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
        config,
      }
    }
    const view = render(
      <I18nextProvider i18n={i18n}>
        <SubscriptionPurchaseDialog
          open
          onOpenChange={(open) => {
            closed = !open
          }}
          plan={{ plan: purchasePlan }}
          accountBalance={10000}
          creditBalancePurchaseEnabled
          lastPurchaseMode='credit_balance'
          onPurchaseSuccess={() => {
            refreshCount += 1
          }}
          creditBalancePlan={{
            model_limits: 'gpt-4o',
            concurrency_limit: 2,
            queue_capacity: 4,
          }}
        />
      </I18nextProvider>
    )

    const credit = view.getByRole('radio', { name: /Credit balance/ })
    await waitFor(() =>
      assert.equal(credit.getAttribute('aria-checked'), 'true')
    )
    fireEvent.click(
      view.getByRole('button', { name: /Pay with Account Balance/ })
    )

    await waitFor(() => assert.ok(submitted))
    assert.equal(submitted?.plan_id, purchasePlan.id)
    assert.equal(submitted?.purchase_mode, 'credit_balance')
    assert.equal(typeof submitted?.idempotency_key, 'string')
    assert.ok(String(submitted?.idempotency_key).length > 0)
    assert.equal(refreshCount, 1)
    assert.equal(closed, true)
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
    assert.match(markup, /credit-balance-model-limits/)
    assert.match(markup, /gpt-4o,claude-3-7-sonnet/)
    assert.match(markup, /credit-balance-purchase-enabled/)
    assert.match(markup, /credit-balance-redemption-enabled/)
    assert.match(markup, /credit-balance-conversion-enabled/)
    assert.match(markup, /type="submit"/)
  })
})
