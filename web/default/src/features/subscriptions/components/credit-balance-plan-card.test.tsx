import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
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

function installTestDom() {
  const window = new Window({ url: 'http://localhost' })
  const bindings = {
    window,
    document: window.document,
    navigator: window.navigator,
    Element: window.Element,
    HTMLElement: window.HTMLElement,
    HTMLInputElement: window.HTMLInputElement,
    HTMLTextAreaElement: window.HTMLTextAreaElement,
    Node: window.Node,
    Event: window.Event,
    MouseEvent: window.MouseEvent,
    PointerEvent: window.PointerEvent,
    MutationObserver: window.MutationObserver,
    getComputedStyle: window.getComputedStyle.bind(window),
    requestAnimationFrame: window.requestAnimationFrame.bind(window),
    cancelAnimationFrame: window.cancelAnimationFrame.bind(window),
  }
  const previous = new Map<string, PropertyDescriptor | undefined>()

  for (const [key, value] of Object.entries(bindings)) {
    previous.set(key, Object.getOwnPropertyDescriptor(globalThis, key))
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value,
    })
  }

  return () => {
    window.close()
    for (const [key, descriptor] of previous) {
      if (descriptor) {
        Object.defineProperty(globalThis, key, descriptor)
      } else {
        Reflect.deleteProperty(globalThis, key)
      }
    }
  }
}

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

  test('submits the editor and rehydrates controls from the persisted response', async (context) => {
    const restoreDom = installTestDom()
    // React DOM probes `document` while loading, so install Happy DOM before
    // importing this test-only client renderer.
    const { cleanup, fireEvent, render, waitFor } =
      await import('@testing-library/react/pure')
    const originalAdapter = api.defaults.adapter
    context.after(async () => {
      cleanup()
      await new Promise<void>((resolve) => setTimeout(resolve, 0))
      api.defaults.adapter = originalAdapter
      restoreDom()
    })

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
