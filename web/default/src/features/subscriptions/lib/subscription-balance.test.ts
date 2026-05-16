import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { useSystemConfigStore } from '@/stores/system-config-store'
import {
  getAccountBalancePaymentState,
  formatAccountBalanceForPlanPurchase,
} from './subscription-balance'

const defaultCurrency = {
  displayInCurrency: true,
  quotaDisplayType: 'USD' as const,
  quotaPerUnit: 500_000,
  usdExchangeRate: 1,
  customCurrencySymbol: '¤',
  customCurrencyExchangeRate: 1,
}

function setQuotaPerUnit(quotaPerUnit: number) {
  useSystemConfigStore.setState((state) => ({
    config: {
      ...state.config,
      currency: {
        ...defaultCurrency,
        quotaPerUnit,
      },
    },
  }))
}

describe('subscription balance purchase helpers', () => {
  test('converts stored quota units into CNY account balance display', () => {
    assert.equal(formatAccountBalanceForPlanPurchase(20_000_000), '¥40.00')
  })

  test('allows balance payment when CNY balance covers the plan price', () => {
    const state = getAccountBalancePaymentState({
      accountBalanceQuota: 20_000_000,
      priceAmount: 40,
      currency: 'CNY',
    })

    assert.equal(state.supported, true)
    assert.equal(state.sufficient, true)
    assert.equal(state.disabled, false)
  })

  test('uses configured quota-per-unit when converting account balance', () => {
    setQuotaPerUnit(1_000_000)

    assert.equal(formatAccountBalanceForPlanPurchase(40_000_000), '¥40.00')

    setQuotaPerUnit(500_000)
  })

  test('disables balance payment when account balance is insufficient', () => {
    const state = getAccountBalancePaymentState({
      accountBalanceQuota: 19_999_999,
      priceAmount: 40,
      currency: 'CNY',
    })

    assert.equal(state.supported, true)
    assert.equal(state.sufficient, false)
    assert.equal(state.disabled, true)
    assert.equal(state.disabledReason, 'insufficient_balance')
  })

  test('disables balance payment for non-CNY plans', () => {
    const state = getAccountBalancePaymentState({
      accountBalanceQuota: 20_000_000,
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
      accountBalanceQuota: 20_000_000,
      priceAmount: 40,
      currency: 'cny',
    })

    assert.equal(state.supported, true)
    assert.equal(state.sufficient, true)
    assert.equal(state.disabled, false)
    assert.equal(state.disabledReason, null)
  })
})
