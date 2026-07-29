import { createElement } from 'react'
import {
  QueryClient,
  QueryClientProvider,
  QueryObserver,
} from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  waitFor,
} from '@testing-library/react/pure'
import i18next from 'i18next'
import assert from 'node:assert/strict'
import { afterEach, describe, test } from 'node:test'
import { renderToString } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { api } from '@/lib/api'
import { subscriptionQueryKeys } from '@/features/subscriptions/query-keys'
import type {
  PlanChannelTokenEquivalent,
  PlanRecord,
  SelfSubscriptionData,
  SubscriptionChannelTokenEquivalent,
  UserSubscriptionRecord,
} from '@/features/subscriptions/types'
import {
  formatPlanChannelEquivalent,
  formatSubscriptionChannelEquivalent,
  getVisibleChannelEquivalents,
  shouldShowChannelEquivalents,
} from '../lib/subscription-display'
import {
  billingStrategyOptions,
  canResetSubscriptionQuotaFromRecord,
  getSubscriptionSourceLabel,
  renderPlanChannelEquivalentLabels,
  renderPlanChannelEquivalentNotes,
  renderSubscriptionChannelEquivalentLabels,
  SubscriptionBillingStrategyControl,
  SubscriptionPlansCard,
} from './subscription-plans-card'

type TranslationFn = (key: string, options?: Record<string, unknown>) => string
const t: TranslationFn = (key, values) => {
  const translated =
    key === 'about' ? 'about' : key === 'Custom' ? 'Translated Custom' : key
  return translated.replace(/{{(\w+)}}/g, (_match, name: string) =>
    String(values?.[name] ?? '')
  )
}

const testI18n = i18next.createInstance()
await testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

const originalAPIAdapter = api.defaults.adapter

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAPIAdapter
})

describe('wallet subscription query keys', () => {
  test('uses a wallet plans key distinct from home public plans', () => {
    assert.deepEqual(subscriptionQueryKeys.walletPlans, [
      'subscriptions',
      'plans',
    ])
    assert.deepEqual(subscriptionQueryKeys.selfSummary, [
      'subscriptions',
      'self',
      'summary',
    ])
    assert.notDeepEqual(
      subscriptionQueryKeys.walletPlans,
      subscriptionQueryKeys.homePublicPlans
    )
  })
})

function createWalletQueryClient(
  plans: PlanRecord[],
  selfSummary: SelfSubscriptionData
): QueryClient {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  })
  queryClient.setQueryData(subscriptionQueryKeys.walletPlans, {
    success: true,
    data: plans,
  })
  queryClient.setQueryData(subscriptionQueryKeys.selfSummary, {
    success: true,
    data: selfSummary,
  })

  return queryClient
}

function renderSubscriptionPlansCardWithQueryClient(
  queryClient: QueryClient
): string {
  return renderToString(
    createElement(
      I18nextProvider,
      { i18n: testI18n },
      createElement(
        QueryClientProvider,
        { client: queryClient },
        createElement(SubscriptionPlansCard, { topupInfo: null })
      )
    )
  )
}

function makeSelfSubscriptionData(
  overrides: Partial<SelfSubscriptionData> = {}
): SelfSubscriptionData {
  return {
    billing_preference: 'subscription_first',
    codex_pro_mode: 'all',
    codex_pro_eligible: false,
    codex_pro_unavailable_reason: '',
    subscriptions: [],
    all_subscriptions: [],
    active_subscription_id: 0,
    summary: {
      active_count: 0,
      token_limit: 0,
      token_used: 0,
      token_remaining: 0,
      token_unlimited: true,
      concurrency_limit: 0,
      gpt_abuse_warning_limit: 0,
      gpt_abuse_warning_count: 0,
      gpt_abuse_warning_remaining: 0,
      gpt_abuse_limit_enabled: false,
    },
    ...overrides,
  }
}

describe('subscription billing strategy control', () => {
  test('renders all account strategies, active subscription, and candidate order', () => {
    const active = makeRecord(
      { id: 42, end_time: 4_102_444_800, is_active_selected: true },
      { title: 'Active Pro' }
    )
    const fallback = makeRecord(
      { id: 43, end_time: 4_102_444_900 },
      { title: 'Fallback Basic' }
    )
    const data = makeSelfSubscriptionData({
      billing_strategy: 'active_fallback',
      active_subscription_id: 42,
      billing_candidate_subscription_ids: [42, 43],
      subscriptions: [active, fallback],
      all_subscriptions: [active, fallback],
    })

    const markup = renderToString(
      createElement(
        I18nextProvider,
        { i18n: testI18n },
        createElement(SubscriptionBillingStrategyControl, {
          data,
          onUpdated: async () => undefined,
        })
      )
    )

    assert.equal(billingStrategyOptions.length, 3)
    assert.match(markup, /Single active subscription/)
    assert.match(markup, /Active subscription fallback/)
    assert.match(markup, /Timed subscriptions first/)
    assert.match(markup, /Active Pro/)
    assert.match(markup, /Fallback Basic/)
    assert.match(markup, /aria-pressed="true"/)
  })

  test('submits one strategy and refreshes the self summary on success', async () => {
    const data = makeSelfSubscriptionData({
      billing_strategy: 'single_active',
      billing_candidate_subscription_ids: [],
    })
    let submitted: Record<string, unknown> | undefined
    let refreshCount = 0
    api.defaults.adapter = async (config) => {
      assert.equal(config.method, 'put')
      assert.equal(config.url, '/api/subscription/self/billing-strategy')
      submitted = JSON.parse(String(config.data))
      return {
        data: {
          success: true,
          data: { billing_strategy: 'timed_first' },
        },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }

    const view = render(
      createElement(
        I18nextProvider,
        { i18n: testI18n },
        createElement(SubscriptionBillingStrategyControl, {
          data,
          onUpdated: async () => {
            refreshCount += 1
          },
        })
      )
    )

    fireEvent.click(
      view.getByRole('button', { name: 'Timed subscriptions first' })
    )

    await waitFor(() => assert.ok(submitted))
    assert.deepEqual(submitted, { billing_strategy: 'timed_first' })
    assert.equal(refreshCount, 1)
  })

  test('keeps the server-selected strategy when the update fails', async () => {
    const data = makeSelfSubscriptionData({
      billing_strategy: 'active_fallback',
      billing_candidate_subscription_ids: [],
    })
    let requestCount = 0
    let refreshCount = 0
    api.defaults.adapter = async (config) => {
      requestCount += 1
      return {
        data: { success: false, message: 'Rejected strategy' },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }

    const view = render(
      createElement(
        I18nextProvider,
        { i18n: testI18n },
        createElement(SubscriptionBillingStrategyControl, {
          data,
          onUpdated: async () => {
            refreshCount += 1
          },
        })
      )
    )
    const selected = view.getByRole('button', {
      name: 'Active subscription fallback',
    })
    const rejected = view.getByRole('button', {
      name: 'Timed subscriptions first',
    })

    fireEvent.click(rejected)

    await waitFor(() => assert.equal(requestCount, 1))
    assert.equal(refreshCount, 0)
    assert.equal(selected.getAttribute('aria-pressed'), 'true')
    assert.equal(rejected.getAttribute('aria-pressed'), 'false')
  })

  test('supports arrow-key focus while preserving one selected strategy', async () => {
    const data = makeSelfSubscriptionData({
      billing_strategy: 'single_active',
      billing_candidate_subscription_ids: [],
    })
    const view = render(
      createElement(
        I18nextProvider,
        { i18n: testI18n },
        createElement(SubscriptionBillingStrategyControl, {
          data,
          onUpdated: async () => undefined,
        })
      )
    )
    const first = view.getByRole('button', {
      name: 'Single active subscription',
    })
    const second = view.getByRole('button', {
      name: 'Active subscription fallback',
    })

    first.focus()
    fireEvent.keyDown(first, { key: 'ArrowRight', code: 'ArrowRight' })
    await Promise.resolve()

    assert.equal(document.activeElement, second)
    assert.equal(first.getAttribute('aria-pressed'), 'true')
    assert.equal(second.getAttribute('aria-pressed'), 'false')
  })
})

describe('wallet subscription React Query rendering', () => {
  test('renders updated channel equivalents from the same wallet query client data', async () => {
    type WalletPlansQueryData = { success: boolean; data: PlanRecord[] }
    const selfSummary = makeSelfSubscriptionData()
    const queryClient = createWalletQueryClient([], selfSummary)
    const observer = new QueryObserver<WalletPlansQueryData>(queryClient, {
      queryKey: subscriptionQueryKeys.walletPlans,
      enabled: false,
    })
    const observedData: (WalletPlansQueryData | undefined)[] = [
      observer.getCurrentResult().data,
    ]
    const unsubscribe = observer.subscribe((result) => {
      observedData.push(result.data)
    })

    try {
      const initial = renderSubscriptionPlansCardWithQueryClient(queryClient)
      assert.doesNotMatch(initial, /Equivalent by channel/)
      assert.doesNotMatch(initial, /OpenAI: about 500K tokens/)

      const updatedPlans: PlanRecord[] = [
        {
          plan: {
            ...makeRecord().plan!,
            id: 91,
            title: 'Query Updated Plan',
            monthly_token_limit: 1_000_000,
            channel_token_equivalents: [
              {
                kind: 'single',
                channel_type: 1,
                channel_type_name: 'OpenAI',
                variant_count: 1,
                multiplier: 2,
                equivalent_token_limit: 500_000,
              },
            ],
          },
        },
      ]
      queryClient.setQueryData(subscriptionQueryKeys.walletPlans, {
        success: true,
        data: updatedPlans,
      })
      await Promise.resolve()

      const updated = renderSubscriptionPlansCardWithQueryClient(queryClient)

      assert.equal(
        observer.getCurrentResult().data,
        queryClient.getQueryData(subscriptionQueryKeys.walletPlans)
      )
      assert.ok(
        observedData.some(
          (data) => data?.data[0]?.plan.title === 'Query Updated Plan'
        )
      )
      assert.match(updated, /Query Updated Plan/)
      assert.match(updated, /Equivalent by channel/)
      assert.match(updated, /OpenAI: about 500K tokens/)
    } finally {
      unsubscribe()
      queryClient.clear()
    }
  })

  test('renders active remaining equivalents with estimation copy', () => {
    const activeRecord = makeRecord(
      {
        id: 42,
        end_time: 4_102_444_800,
        is_active_selected: true,
        token_limit: 1_000_000,
        token_used: 1_000_000,
      },
      { title: 'Active Plan' }
    )

    const baseSelfSummary = makeSelfSubscriptionData()
    const selfSummary = makeSelfSubscriptionData({
      subscriptions: [activeRecord],
      all_subscriptions: [activeRecord],
      active_subscription_id: 42,
      summary: {
        ...baseSelfSummary.summary,
        active_count: 1,
        active_subscription_id: 42,
        subscription_id: 42,
        token_limit: 1_000_000,
        token_used: 1_000_000,
        token_remaining: 0,
        token_unlimited: false,
        channel_token_equivalents: [
          {
            kind: 'single',
            channel_type: 1,
            channel_type_name: 'OpenAI',
            variant_count: 1,
            multiplier: 2,
            equivalent_token_limit: 500_000,
            equivalent_token_remaining: 0,
          },
        ],
      },
    })
    const queryClient = createWalletQueryClient([], selfSummary)

    try {
      const html = renderSubscriptionPlansCardWithQueryClient(queryClient)

      assert.match(html, /OpenAI: about 0 tokens/)
      assert.match(
        html,
        /Estimated by current channel multiplier\. Actual deduction depends on the channel used\./
      )
    } finally {
      queryClient.clear()
    }
  })
  test('renders exhausted Credit balance and ledger without unlimited copy', () => {
    const creditRecord = makeRecord(
      {
        id: 51,
        plan_id: 52,
        entitlement_type: 'credit_balance',
        end_time: 0,
        token_limit: 0,
        token_used: 0,
        is_active_selected: true,
      },
      {
        id: 52,
        title: 'Credit balance plan',
        entitlement_type: 'credit_balance',
      }
    )
    const base = makeSelfSubscriptionData()
    const selfSummary = makeSelfSubscriptionData({
      active_subscription_id: 51,
      subscriptions: [],
      all_subscriptions: [creditRecord],
      credit_balance: {
        user_subscription_id: 51,
        plan_id: 52,
        gross_credit: 0,
        debt_offset: 0,
        available_credit: 0,
        settlement_debt: 0,
        balance_before: 0,
        balance_after: 0,
        active: true,
        ledger_id: 61,
        status: 'exhausted',
      },
      credit_balance_ledger: [
        {
          id: 61,
          user_id: 1,
          user_subscription_id: 51,
          type: 'purchase',
          idempotency_key: 'credit-order-61',
          source_type: 'subscription_order',
          source_id: 62,
          gross_credit: 1000,
          debt_offset: 250,
          balance_before: -250,
          balance_after: 750,
          available_credit_after: 750,
          settlement_debt_after: 0,
          reason: 'purchase',
          created_at: 1,
        },
      ],
      summary: {
        ...base.summary,
        active_subscription_id: 51,
        token_unlimited: false,
      },
    })
    const queryClient = createWalletQueryClient([], selfSummary)

    try {
      const html = renderSubscriptionPlansCardWithQueryClient(queryClient)

      assert.match(html, /Available Credit balance/)
      assert.match(html, /Settlement debt/)
      assert.match(html, /exhausted/)
      assert.match(html, /Credit purchase history/)
      assert.match(html, /\+<!-- -->1000/)
      assert.match(html, /Debt offset<!-- --> <!-- -->250/)
      assert.match(html, /Credit balance<!-- -->:<!-- --> <!-- -->0 credits/)
      assert.doesNotMatch(html, /Unlimited credits/)
    } finally {
      queryClient.clear()
    }
  })
})

describe('wallet channel token equivalent display', () => {
  test('formats single, range, and unlimited plan equivalents', () => {
    const single: PlanChannelTokenEquivalent = {
      kind: 'single',
      channel_type: 1,
      channel_type_name: 'gpt',
      variant_count: 1,
      multiplier: 1,
      equivalent_token_limit: 1_000_000,
    }
    const range: PlanChannelTokenEquivalent = {
      kind: 'range',
      channel_type: 14,
      channel_type_name: 'gitlab',
      variant_count: 2,
      min_multiplier: 1.5,
      max_multiplier: 2,
      equivalent_token_limit_min: 500_000,
      equivalent_token_limit_max: 666_666,
    }
    const unlimited: PlanChannelTokenEquivalent = {
      kind: 'unlimited',
      channel_type: 1,
      channel_type_name: 'gpt',
      variant_count: 1,
      token_unlimited: true,
    }

    assert.equal(formatPlanChannelEquivalent(single, t), 'gpt: about 1M tokens')
    assert.equal(
      formatPlanChannelEquivalent(range, t),
      'gitlab: about 500K tokens - 666.67K tokens'
    )
    assert.equal(
      formatPlanChannelEquivalent(unlimited, t),
      'gpt: Unlimited tokens'
    )
  })

  test('formats fixed-request equivalents as exact request counts', () => {
    const fixed: PlanChannelTokenEquivalent = {
      kind: 'fixed_request',
      value_type: 'single',
      channel_type: 1,
      channel_type_name: 'OpenAI',
      variant_count: 1,
      fixed_request_credits: 80_000,
      equivalent_request_limit: 12,
    }
    const plan = makeRecord(
      {},
      {
        channel_credit_equivalents: [fixed],
      }
    ).plan!

    assert.equal(formatPlanChannelEquivalent(fixed, t), 'OpenAI: 12 requests')
    assert.deepEqual(renderPlanChannelEquivalentLabels(plan, t), [
      'OpenAI: 12 requests',
    ])
    assert.deepEqual(renderPlanChannelEquivalentNotes(plan, t), [
      'Fixed-request channel equivalents show exact full requests available.',
    ])
  })

  test('uses backend group name as authoritative label', () => {
    const groupLabeled: PlanChannelTokenEquivalent = {
      kind: 'single',
      channel_type: 8,
      channel_type_name: 'gpt',
      variant_count: 1,
      multiplier: 2,
      equivalent_token_limit: 500_000,
    }
    const anotherGroup: PlanChannelTokenEquivalent = {
      kind: 'single',
      channel_type: 9999,
      channel_type_name: 'gitlab',
      variant_count: 1,
      multiplier: 2,
      equivalent_token_limit: 500_000,
    }
    const groupRemaining: SubscriptionChannelTokenEquivalent = {
      kind: 'single',
      channel_type: 8,
      channel_type_name: 'gpt',
      variant_count: 1,
      multiplier: 2,
      equivalent_token_limit: 500_000,
      equivalent_token_remaining: 0,
    }

    // 后端 channel_type_name（分组名）权威，即使 channel_type 与某静态渠道类型 id 相同也不取静态名。
    assert.equal(
      formatPlanChannelEquivalent(groupLabeled, t),
      'gpt: about 500K tokens'
    )
    assert.equal(
      formatPlanChannelEquivalent(anotherGroup, t),
      'gitlab: about 500K tokens'
    )
    assert.equal(
      formatSubscriptionChannelEquivalent(groupRemaining, t),
      'gpt: about 0 tokens'
    )
  })

  test('formats current subscription remaining zero as finite zero tokens', () => {
    const remaining: SubscriptionChannelTokenEquivalent = {
      kind: 'single',
      channel_type: 1,
      channel_type_name: 'OpenAI',
      variant_count: 1,
      multiplier: 2,
      equivalent_token_limit: 500_000,
      equivalent_token_remaining: 0,
    }

    assert.equal(
      formatSubscriptionChannelEquivalent(remaining, t),
      'OpenAI: about 0 tokens'
    )
  })

  test('hides all single 1.0 equivalents and reports overflow count', () => {
    const allDefault: PlanChannelTokenEquivalent[] = [
      {
        kind: 'single',
        channel_type: 1,
        channel_type_name: 'OpenAI',
        variant_count: 1,
        multiplier: 1,
        equivalent_token_limit: 1_000,
      },
      {
        kind: 'single',
        channel_type: 14,
        channel_type_name: 'Claude',
        variant_count: 1,
        multiplier: 1,
        equivalent_token_limit: 1_000,
      },
    ]
    const mixed: PlanChannelTokenEquivalent[] = [
      ...allDefault,
      {
        kind: 'single',
        channel_type: 24,
        channel_type_name: 'Gemini',
        variant_count: 1,
        multiplier: 2,
        equivalent_token_limit: 500,
      },
      {
        kind: 'range',
        channel_type: 3,
        channel_type_name: 'Azure',
        variant_count: 2,
        min_multiplier: 1,
        max_multiplier: 2,
        equivalent_token_limit_min: 500,
        equivalent_token_limit_max: 1_000,
      },
    ]

    assert.equal(shouldShowChannelEquivalents(allDefault), false)
    assert.equal(shouldShowChannelEquivalents(mixed), true)
    assert.deepEqual(getVisibleChannelEquivalents(mixed, 3), {
      items: mixed.slice(0, 3),
      hiddenCount: 1,
    })
  })

  test('renders plan equivalent labels and suppresses all 1.0 plan labels', () => {
    const plan = makeRecord(
      {},
      {
        channel_token_equivalents: [
          {
            kind: 'single',
            channel_type: 1,
            channel_type_name: 'OpenAI',
            variant_count: 1,
            multiplier: 2,
            equivalent_token_limit: 500_000,
          },
          {
            kind: 'unlimited',
            channel_type: 24,
            channel_type_name: 'Gemini',
            variant_count: 1,
            token_unlimited: true,
          },
        ],
      }
    ).plan!
    const defaultOnly = makeRecord(
      {},
      {
        channel_token_equivalents: [
          {
            kind: 'single',
            channel_type: 1,
            channel_type_name: 'OpenAI',
            variant_count: 1,
            multiplier: 1,
            equivalent_token_limit: 1_000_000,
          },
        ],
      }
    ).plan!

    assert.deepEqual(renderPlanChannelEquivalentLabels(plan, t), [
      'OpenAI: about 500K tokens',
      'Gemini: Unlimited tokens',
    ])
    assert.deepEqual(renderPlanChannelEquivalentLabels(defaultOnly, t), [])
  })

  test('renders current active summary remaining equivalents with finite zero', () => {
    const summary = {
      channel_token_equivalents: [
        {
          kind: 'single',
          channel_type: 1,
          channel_type_name: 'OpenAI',
          variant_count: 1,
          multiplier: 2,
          equivalent_token_limit: 500_000,
          equivalent_token_remaining: 0,
        },
      ],
    }

    assert.deepEqual(
      renderSubscriptionChannelEquivalentLabels({ summary } as never, true, t),
      ['OpenAI: about 0 tokens']
    )
    assert.deepEqual(
      renderSubscriptionChannelEquivalentLabels({ summary } as never, false, t),
      []
    )
  })
})
function makeRecord(
  subscriptionOverrides: Partial<UserSubscriptionRecord['subscription']> = {},
  planOverrides: Partial<NonNullable<UserSubscriptionRecord['plan']>> = {}
): UserSubscriptionRecord {
  return {
    subscription: {
      id: 1,
      user_id: 1,
      plan_id: 1,
      status: 'active',
      source: '',
      start_time: 0,
      end_time: 2_000,
      amount_total: 0,
      amount_used: 0,
      ...subscriptionOverrides,
    },
    plan: {
      id: 1,
      title: 'Plan',
      price_amount: 80,
      currency: 'CNY',
      duration_unit: 'month',
      duration_value: 1,
      quota_reset_period: 'monthly',
      enabled: true,
      sort_order: 1,
      max_purchase_per_user: 0,
      total_amount: 1,
      is_trial: false,
      invite_trial: false,
      channel_token_equivalents: [],
      ...planOverrides,
    },
  }
}

describe('wallet subscription source labels', () => {
  test('uses backend source_label before raw grant reason', () => {
    assert.equal(
      getSubscriptionSourceLabel(
        makeRecord({ source_label: 'paid', grant_reason: 'trial_code' }),
        t
      ),
      'Paid plan'
    )
  })

  test('treats legacy redemption source as paid', () => {
    assert.equal(
      getSubscriptionSourceLabel(makeRecord({ grant_reason: 'redemption' }), t),
      'Paid plan'
    )
    assert.equal(
      getSubscriptionSourceLabel(makeRecord({ source: 'redemption' }), t),
      'Paid plan'
    )
  })

  test('treats legacy admin source as paid only for paid non-trial plans', () => {
    assert.equal(
      getSubscriptionSourceLabel(makeRecord({ grant_reason: 'admin' }), t),
      'Paid plan'
    )
    assert.equal(
      getSubscriptionSourceLabel(
        makeRecord(
          { grant_reason: 'admin' },
          { price_amount: 0, is_trial: true }
        ),
        t
      ),
      'Unknown'
    )
    assert.equal(
      getSubscriptionSourceLabel(
        makeRecord({ grant_reason: 'admin' }, { price_amount: 0 }),
        t
      ),
      'Unknown'
    )
    assert.equal(
      getSubscriptionSourceLabel(
        makeRecord(
          { grant_reason: 'admin' },
          { price_amount: 80, invite_trial: true }
        ),
        t
      ),
      'Unknown'
    )
    assert.equal(
      getSubscriptionSourceLabel(
        { subscription: makeRecord({ grant_reason: 'admin' }).subscription },
        t
      ),
      'Unknown'
    )
  })

  test('keeps invitation reward and trial labels distinct', () => {
    assert.equal(
      getSubscriptionSourceLabel(
        makeRecord({ grant_reason: 'monthly_invite_entitlement' }),
        t
      ),
      'Invitation reward'
    )
    assert.equal(
      getSubscriptionSourceLabel(makeRecord({ grant_reason: 'trial_code' }), t),
      'Trial'
    )
    assert.equal(
      getSubscriptionSourceLabel(
        makeRecord({ grant_reason: 'invite_trial' }),
        t
      ),
      'Trial'
    )
  })
})

describe('wallet subscription quota reset visibility', () => {
  test('shows reset for active redemption when backend allows reset', () => {
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({
          grant_reason: 'redemption',
          source_label: 'paid',
          can_reset_quota: true,
          end_time: 2_000,
        }),
        1_000
      ),
      true
    )
  })

  test('legacy redemption fallback can reset when backend flag is absent', () => {
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'redemption', end_time: 2_000 }),
        1_000
      ),
      true
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ source: 'redemption', end_time: 2_000 }),
        1_000
      ),
      true
    )
  })

  test('legacy admin fallback can reset only for paid non-trial plans', () => {
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'admin', end_time: 2_000 }),
        1_000
      ),
      true
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord(
          { grant_reason: 'admin', end_time: 2_000 },
          { price_amount: 0 }
        ),
        1_000
      ),
      false
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord(
          { grant_reason: 'admin', end_time: 2_000 },
          { price_amount: 80, is_trial: true }
        ),
        1_000
      ),
      false
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord(
          { grant_reason: 'admin', end_time: 2_000 },
          { price_amount: 80, invite_trial: true }
        ),
        1_000
      ),
      false
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        {
          subscription: makeRecord({ grant_reason: 'admin', end_time: 2_000 })
            .subscription,
        },
        1_000
      ),
      false
    )
  })

  test('does not show reset for trial or expired subscriptions', () => {
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'trial_code', end_time: 2_000 }),
        1_000
      ),
      false
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'invite_trial', end_time: 2_000 }),
        1_000
      ),
      false
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'redemption', end_time: 500 }),
        1_000
      ),
      false
    )
  })
})
