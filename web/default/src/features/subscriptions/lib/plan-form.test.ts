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
  test('defaults GPT abuse warning limit to automatic mode', () => {
    assert.equal(PLAN_FORM_DEFAULTS.gpt_abuse_warning_limit, 0)
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
  })

  test('includes token, concurrency, and visibility fields in submit payload', () => {
    const values: PlanFormValues = {
      ...PLAN_FORM_DEFAULTS,
      title: 'Basic',
      price_amount: 40,
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
      price_amount: 40,
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
