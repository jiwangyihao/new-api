import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  accountBalanceCentsToCnyAmount,
  accountBalanceCnyToCents,
  accountBalanceQuotaToCnyAmount,
  formatAccountBalanceForPlanPurchase,
  getAccountBalancePaymentState,
} from './subscription-balance'

describe('subscription balance purchase helpers', () => {
  test('converts stored CNY cents into account balance CNY amount', () => {
    assert.equal(accountBalanceCentsToCnyAmount(4000), 40)
    assert.equal(accountBalanceCentsToCnyAmount(3990), 39.9)
    assert.equal(accountBalanceCentsToCnyAmount(0), 0)
    assert.equal(accountBalanceCentsToCnyAmount(Number.NaN), 0)
  })

  test('keeps quota compatibility helper on CNY cents semantics', () => {
    assert.equal(accountBalanceQuotaToCnyAmount(3990), 39.9)
    assert.equal(accountBalanceQuotaToCnyAmount(Number.NaN), 0)
  })

  test('converts CNY amount input into stored account balance cents', () => {
    assert.equal(accountBalanceCnyToCents(40), 4000)
    assert.equal(accountBalanceCnyToCents(39.9), 3990)
    assert.equal(accountBalanceCnyToCents(0), 0)
    assert.equal(accountBalanceCnyToCents(Number.NaN), 0)
  })

  test('formats stored account balance cents for CNY plan purchases', () => {
    assert.equal(formatAccountBalanceForPlanPurchase(4000), '¥40.00')
    assert.match(formatAccountBalanceForPlanPurchase(3990), /39\.90/)
  })

  test('allows balance payment when stored CNY cents cover the plan price', () => {
    const state = getAccountBalancePaymentState({
      accountBalanceQuota: 3990,
      priceAmount: 39.9,
      currency: 'CNY',
    })

    assert.equal(state.supported, true)
    assert.equal(state.sufficient, true)
    assert.equal(state.disabled, false)
    assert.equal(state.disabledReason, null)
  })

  test('disables balance payment when stored CNY cents are one cent short', () => {
    const state = getAccountBalancePaymentState({
      accountBalanceQuota: 3989,
      priceAmount: 39.9,
      currency: 'CNY',
    })

    assert.equal(state.supported, true)
    assert.equal(state.sufficient, false)
    assert.equal(state.disabled, true)
    assert.equal(state.disabledReason, 'insufficient_balance')
  })

  test('disables balance payment for non-CNY plans', () => {
    const state = getAccountBalancePaymentState({
      accountBalanceQuota: 4000,
      priceAmount: 40,
      currency: 'USD',
    })

    assert.equal(state.supported, false)
    assert.equal(state.sufficient, false)
    assert.equal(state.disabled, true)
    assert.equal(state.disabledReason, 'unsupported_currency')
  })

  test('normalizes lowercase CNY plan currency', () => {
    const state = getAccountBalancePaymentState({
      accountBalanceQuota: 4000,
      priceAmount: 40,
      currency: 'cny',
    })

    assert.equal(state.supported, true)
    assert.equal(state.sufficient, true)
    assert.equal(state.disabled, false)
    assert.equal(state.disabledReason, null)
  })
})
