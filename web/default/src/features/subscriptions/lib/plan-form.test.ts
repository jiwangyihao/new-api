import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { SubscriptionPlan } from '../types'
import {
  PLAN_FORM_DEFAULTS,
  formValuesToPlanPayload,
  planToFormValues,
  type PlanFormValues,
} from './plan-form'

describe('subscription plan distributor form mapping', () => {
  test('defaults GPT abuse warning limit and Credit eligibility switches', () => {
    assert.equal(PLAN_FORM_DEFAULTS.gpt_abuse_warning_limit, 0)
    assert.equal(PLAN_FORM_DEFAULTS.unlimited_purchase_enabled, false)
    assert.equal(PLAN_FORM_DEFAULTS.timed_conversion_enabled, false)
  })

  test('maps backend distributor fields into form values', () => {
    const plan = {
      id: 1,
      title: 'Trial',
      price_amount: 0,
      currency: 'CNY',
      duration_unit: 'hour',
      duration_value: 24,
      quota_reset_period: 'never',
      enabled: true,
      sort_order: 1000,
      max_purchase_per_user: 0,
      total_amount: 0,
      monthly_token_limit: 0,
      concurrency_limit: 1,
      queue_capacity: 8,
      gpt_abuse_warning_limit: 6,
      is_trial: true,
      public_visible: false,
      trial_duration_hours: 24,
      reward_eligible: false,
      business_code: 'trial_24h',
      invite_trial: true,
    } satisfies SubscriptionPlan

    const values = planToFormValues(plan)

    assert.equal(values.monthly_token_limit, 0)
    assert.equal(values.concurrency_limit, 1)
    assert.equal(values.queue_capacity, 8)
    assert.equal(values.gpt_abuse_warning_limit, 6)
    assert.equal(values.is_trial, true)
    assert.equal(values.public_visible, false)
    assert.equal(values.trial_duration_hours, 24)
    assert.equal(values.reward_eligible, false)
    assert.equal(values.business_code, 'trial_24h')
    assert.equal(values.invite_trial, true)
    assert.equal(values.unlimited_purchase_enabled, false)
    assert.equal(values.timed_conversion_enabled, false)
  })

  test('includes token, concurrency, and visibility fields in submit payload', () => {
    const values: PlanFormValues = {
      ...PLAN_FORM_DEFAULTS,
      title: 'Basic',
      price_amount: '40',
      monthly_token_limit: 1_000_000_000,
      concurrency_limit: 1,
      queue_capacity: 10,
      gpt_abuse_warning_limit: 5,
      is_trial: false,
      public_visible: true,
      trial_duration_hours: 0,
      reward_eligible: true,
      business_code: ' basic_monthly ',
      invite_trial: true,
      unlimited_purchase_enabled: true,
      timed_conversion_enabled: false,
    }

    const payload = formValuesToPlanPayload(values)

    assert.equal(payload.plan.monthly_token_limit, 1_000_000_000)
    assert.equal(payload.plan.concurrency_limit, 1)
    assert.equal(payload.plan.queue_capacity, 10)
    assert.equal(payload.plan.gpt_abuse_warning_limit, 5)
    assert.equal(payload.plan.is_trial, false)
    assert.equal(payload.plan.public_visible, true)
    assert.equal(payload.plan.trial_duration_hours, 0)
    assert.equal(payload.plan.reward_eligible, true)
    assert.equal(payload.plan.business_code, 'basic_monthly')
    assert.equal(payload.plan.invite_trial, true)
    assert.equal(payload.plan.currency, 'CNY')
    assert.equal(payload.plan.unlimited_purchase_enabled, true)
    assert.equal(payload.plan.timed_conversion_enabled, false)
  })

  test('keeps explicit GPT abuse warning limit zero in submit payload', () => {
    const payload = formValuesToPlanPayload({
      ...PLAN_FORM_DEFAULTS,
      title: 'Automatic GPT abuse limit',
      gpt_abuse_warning_limit: 0,
    })

    assert.equal(payload.plan.gpt_abuse_warning_limit, 0)
    assert.equal(
      Object.prototype.hasOwnProperty.call(
        payload.plan,
        'gpt_abuse_warning_limit'
      ),
      true
    )
  })

  test('preserves kyren product id in plan form payload', () => {
    const payload = formValuesToPlanPayload({
      ...PLAN_FORM_DEFAULTS,
      title: 'Pro',
      price_amount: '40',
      kyren_product_id: 'prod_kyren',
    })

    assert.equal(payload.plan.kyren_product_id, 'prod_kyren')
  })

  test('omits blank business code instead of sending an empty unique value', () => {
    const payload = formValuesToPlanPayload({
      ...PLAN_FORM_DEFAULTS,
      title: 'Legacy',
      business_code: '   ',
    })

    assert.equal(payload.plan.business_code, undefined)
  })
})

describe('subscription plan exact price round-trip', () => {
  test('builds authoritative micros from the original decimal input', () => {
    const values = {
      ...PLAN_FORM_DEFAULTS,
      title: 'Exact price',
      price_amount: '9007199254.740991',
    } satisfies PlanFormValues

    const payload = formValuesToPlanPayload(values)

    assert.equal(payload.plan.price_amount_micros, '9007199254740991')
    const serialized = JSON.stringify(payload)
    const parsed = JSON.parse(serialized) as {
      plan: { price_amount: string; price_amount_micros: string }
    }
    assert.equal(parsed.plan.price_amount, '9007199254.740991')
    assert.equal(parsed.plan.price_amount_micros, '9007199254740991')
  })

  test('refreshes the original decimal text from authoritative micros', () => {
    const plan = {
      ...formValuesToPlanPayload({
        ...PLAN_FORM_DEFAULTS,
        title: 'Exact price',
        price_amount: '40.123456',
      }).plan,
      id: 1,
      title: 'Exact price',
      price_amount: 40.123456,
      price_amount_micros: '40123456',
    } as SubscriptionPlan

    const values = planToFormValues(plan)

    assert.equal(values.price_amount, '40.123456')
  })
  test('does not promote a legacy display price during unrelated edits', () => {
    const plan = {
      ...formValuesToPlanPayload({
        ...PLAN_FORM_DEFAULTS,
        title: 'Legacy price',
        price_amount: '40.123456',
      }).plan,
      id: 2,
      title: 'Legacy price',
      price_amount: 40.123456,
      price_amount_micros: null,
    } as SubscriptionPlan

    const values = planToFormValues(plan)
    values.title = 'Renamed legacy price'
    const payload = formValuesToPlanPayload(values)

    assert.equal(values.price_amount, '40.123456')
    assert.equal('price_amount' in payload.plan, false)
    assert.equal('price_amount_micros' in payload.plan, false)
  })

  test('labels legacy display text as non-authoritative form state', () => {
    const values = planToFormValues({
      ...formValuesToPlanPayload({
        ...PLAN_FORM_DEFAULTS,
        title: 'Legacy display only',
        price_amount: '0',
      }).plan,
      id: 3,
      title: 'Legacy display only',
      price_amount: 0.1 + 0.2,
      price_amount_micros: null,
    } as SubscriptionPlan)

    assert.equal(values.price_amount, '0.30000000000000004')
    assert.equal(values.price_amount_source, 'legacy')
    assert.equal(values.price_amount_changed, false)
  })

  test('never promotes legacy Number edge cases without explicit input', () => {
    const cases = [
      { name: 'zero', price: 0 },
      { name: 'large integer boundary', price: Number.MAX_SAFE_INTEGER },
      { name: 'floating-sensitive decimal', price: 0.1 + 0.2 },
    ]

    for (const testCase of cases) {
      const values = planToFormValues({
        ...formValuesToPlanPayload({
          ...PLAN_FORM_DEFAULTS,
          title: testCase.name,
          price_amount: '0',
        }).plan,
        id: 4,
        title: testCase.name,
        price_amount: testCase.price,
        price_amount_micros: null,
      } as SubscriptionPlan)
      const payload = formValuesToPlanPayload(values)

      assert.equal(values.price_amount_source, 'legacy')
      assert.equal('price_amount' in payload.plan, false)
      assert.equal('price_amount_micros' in payload.plan, false)
    }
  })

  test('promotes only explicit decimal text edits to authoritative micros', () => {
    const values = planToFormValues({
      ...formValuesToPlanPayload({
        ...PLAN_FORM_DEFAULTS,
        title: 'Legacy edited explicitly',
        price_amount: '0',
      }).plan,
      id: 5,
      title: 'Legacy edited explicitly',
      price_amount: 0.1 + 0.2,
      price_amount_micros: null,
    } as SubscriptionPlan)
    values.price_amount = '0.300001'
    values.price_amount_changed = true

    const payload = formValuesToPlanPayload(values)

    assert.equal(payload.plan.price_amount, '0.300001')
    assert.equal(payload.plan.price_amount_micros, '300001')
  })

  test('converts zero and maximum supported decimal text without Number', () => {
    const cases = [
      { decimal: '0', micros: '0' },
      { decimal: '9223372036854.775807', micros: '9223372036854775807' },
    ]

    for (const testCase of cases) {
      const payload = formValuesToPlanPayload({
        ...PLAN_FORM_DEFAULTS,
        title: 'New exact boundary',
        price_amount: testCase.decimal,
      })

      assert.equal(payload.plan.price_amount, testCase.decimal)
      assert.equal(payload.plan.price_amount_micros, testCase.micros)
    }
  })
})
