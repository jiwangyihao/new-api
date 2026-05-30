import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import {
  getKyrenSubscriptionAvailability,
  processKyrenSubscriptionPayment,
} from './dialogs/subscription-purchase-dialog'
import type { SubscriptionPlan } from '../types'

function makePlan(overrides: Partial<SubscriptionPlan> = {}): SubscriptionPlan {
  return {
    id: 1001,
    title: 'Pro',
    price_amount: 40,
    currency: 'CNY',
    duration_unit: 'month',
    duration_value: 1,
    quota_reset_period: 'never',
    enabled: true,
    sort_order: 1,
    max_purchase_per_user: 0,
    total_amount: 0,
    kyren_product_id: 'prod_kyren',
    public_visible: true,
    is_trial: false,
    ...overrides,
  }
}

function readSource(path: string): string {
  return readFileSync(path, 'utf8')
}

describe('subscription Kyren payment helper', () => {
  test('opens Kyren checkout for a purchasable CNY plan', async () => {
    const payCalls: Array<{ plan_id: number }> = []
    const openedUrls: string[] = []
    const paySubscriptionKyren = async (data: { plan_id: number }) => {
      payCalls.push(data)
      return {
        success: true,
        data: { checkout_url: 'https://checkout.example/sub' },
      }
    }
    const openCheckout = (url: string) => {
      openedUrls.push(url)
    }

    await processKyrenSubscriptionPayment({
      planId: 1001,
      paySubscriptionKyren,
      openCheckout,
    })

    assert.deepEqual(payCalls[0], { plan_id: 1001 })
    assert.equal(openedUrls[0], 'https://checkout.example/sub')
  })

  test('rejects unsafe Kyren checkout URLs', async () => {
    const openedUrls: string[] = []

    await assert.rejects(
      () =>
        processKyrenSubscriptionPayment({
          planId: 1001,
          paySubscriptionKyren: async () => ({
            success: true,
            message: 'success',
            data: { checkout_url: 'javascript:alert(1)' },
          }),
          openCheckout: (url) => {
            openedUrls.push(url)
          },
        }),
      /Kyren checkout creation failed/
    )

    assert.deepEqual(openedUrls, [])
  })

  test('marks Kyren subscription unavailable for trial, non-CNY, free, hidden, disabled, missing product, global disabled, or purchase limit reached plans', () => {
    const topupInfo = { enable_kyren_subscription: true }

    assert.equal(
      getKyrenSubscriptionAvailability(
        makePlan({ kyren_product_id: '' }),
        topupInfo,
        {}
      ).available,
      false
    )
    assert.equal(
      getKyrenSubscriptionAvailability(
        makePlan({ currency: 'USD' }),
        topupInfo,
        {}
      ).available,
      false
    )
    assert.equal(
      getKyrenSubscriptionAvailability(
        makePlan({ price_amount: 0 }),
        topupInfo,
        {}
      ).available,
      false
    )
    assert.equal(
      getKyrenSubscriptionAvailability(
        makePlan({ is_trial: true }),
        topupInfo,
        {}
      ).available,
      false
    )
    assert.equal(
      getKyrenSubscriptionAvailability(
        makePlan({ enabled: false }),
        topupInfo,
        {}
      ).available,
      false
    )
    assert.equal(
      getKyrenSubscriptionAvailability(
        makePlan({ public_visible: false }),
        topupInfo,
        {}
      ).available,
      false
    )
    assert.equal(
      getKyrenSubscriptionAvailability(
        makePlan(),
        { enable_kyren_subscription: false },
        {}
      ).available,
      false
    )
    assert.equal(
      getKyrenSubscriptionAvailability(
        makePlan({ max_purchase_per_user: 1 }),
        topupInfo,
        { purchaseCount: 1 }
      ).available,
      false
    )
  })

  test('wires wallet Kyren availability into the purchase dialog', () => {
    const source = readSource(
      'src/features/wallet/components/subscription-plans-card.tsx'
    )

    assert.match(
      source,
      /enableKyrenSubscription=\{!!topupInfo\?\.enable_kyren_subscription\}/
    )
  })

  test('renders subscription Kyren product management controls in the plan drawer', () => {
    const source = readSource(
      'src/features/subscriptions/components/subscriptions-mutate-drawer.tsx'
    )

    assert.match(source, /name='kyren_product_id'/)
    assert.match(source, /getSubscriptionKyrenProduct/)
    assert.match(source, /syncSubscriptionKyrenProduct/)
    assert.match(source, /Create Kyren product/)
    assert.match(source, /loadKyrenStatus/)
    assert.match(source, /No Kyren product status loaded/)
    assert.match(source, /Kyren product is missing/)
    assert.match(source, /Kyren product is archived/)
    assert.match(source, /Kyren product price mismatch/)
    assert.match(source, /Kyren product currency mismatch/)
    assert.match(source, /Sync to Kyren/)
    assert.match(source, /Refresh Kyren status/)
  })

  test('writes synced Kyren product id back to the plan form', () => {
    const source = readSource(
      'src/features/subscriptions/components/subscriptions-mutate-drawer.tsx'
    )

    assert.match(source, /form\.setValue\(\s*['"]kyren_product_id['"]/)
    assert.match(source, /res\.data\?\.product_id/)
  })
})
