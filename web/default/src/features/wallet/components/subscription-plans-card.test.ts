import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  CODEX_PRO_MODE_TITLE_KEY,
  CODEX_PRO_MODE_OPTIONS,
  canUseCodexProModeControl,
  getCodexProModeFailureRollback,
  getCodexProUnavailableMessageKey,
  canResetSubscriptionQuotaFromRecord,
  getSubscriptionSourceLabel,
} from './subscription-plans-card'
import type {
  CodexProMode,
  CodexProUnavailableReason,
  UserSubscriptionRecord,
} from '@/features/subscriptions/types'

type TranslationFn = (key: string, options?: Record<string, unknown>) => string

const t: TranslationFn = (key) => key

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
      getSubscriptionSourceLabel(makeRecord({ grant_reason: 'invite_trial' }), t),
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
        { subscription: makeRecord({ grant_reason: 'admin', end_time: 2_000 }).subscription },
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

describe('wallet Codex Pro mode control contract', () => {
  test('offers the Codex Pro entry and three user modes with visible labels', () => {
    const modes = CODEX_PRO_MODE_OPTIONS.map((option) => option.value)

    assert.equal(CODEX_PRO_MODE_TITLE_KEY, 'Codex Pro')
    assert.deepEqual(modes, ['all', 'flexible', 'off'] satisfies CodexProMode[])
    assert.deepEqual(
      CODEX_PRO_MODE_OPTIONS.map((option) => option.labelKey),
      ['All', 'Flexible', 'Off']
    )
    assert.deepEqual(
      CODEX_PRO_MODE_OPTIONS.map((option) => option.descriptionKey),
      [
        'All eligible GPT-family Responses requests try Codex Pro without requiring the intent header.',
        'Only requests with X-NewAPI-Codex-Pro-Intent: codex-pro try Codex Pro in flexible mode.',
        'Codex Pro is disabled; eligible requests stay on the normal group.',
      ]
    )
  })

  test('keeps eligible off mode selectable so users can switch back', () => {
    assert.equal(
      canUseCodexProModeControl({
        codex_pro_mode: 'off',
        codex_pro_eligible: true,
        codex_pro_unavailable_reason: '',
      }),
      true
    )
  })

  test('maps ineligible reasons to action-oriented copy instead of raw enums', () => {
    const cases: Array<[CodexProUnavailableReason, string]> = [
      ['wallet_only', 'Your current billing preference will not create a subscription billing session.'],
      ['trial_subscription', 'Trial subscriptions do not support Codex Pro.'],
      ['reward_subscription', 'Invitation reward subscriptions do not support Codex Pro.'],
      ['no_paid_subscription', 'Please purchase an eligible paid subscription first.'],
    ]

    for (const [reason, expectedKey] of cases) {
      assert.equal(getCodexProUnavailableMessageKey(reason), expectedKey)
      assert.notEqual(getCodexProUnavailableMessageKey(reason), reason)
    }

    assert.equal(
      getCodexProUnavailableMessageKey('unexpected'),
      'Please purchase an eligible paid subscription first.'
    )
  })

  test('rolls optimistic mode changes back and exposes a translated failure message', () => {
    assert.deepEqual(
      getCodexProModeFailureRollback({
        previousMode: 'flexible',
        requestedMode: 'all',
      }),
      {
        mode: 'flexible',
        messageKey: 'Request failed',
      }
    )
  })
})
