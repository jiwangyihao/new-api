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
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import { processKyrenTopUpProductPayment } from './lib/payment'

function readSource(path: string): string {
  return readFileSync(path, 'utf8')
}

describe('wallet Kyren payment flow', () => {
  test('submits Kyren top-up products independently by local product id', async () => {
    const calls: Array<{ product_id: string }> = []
    const opened: string[] = []

    await processKyrenTopUpProductPayment({
      productId: 'topup_cny_10',
      requestKyrenPayment: async (request) => {
        calls.push(request)
        return {
          success: true,
          data: { checkout_url: 'https://checkout.example/kyren' },
        }
      },
      openCheckout: (url) => opened.push(url),
    })

    assert.deepEqual(calls, [{ product_id: 'topup_cny_10' }])
    assert.deepEqual(opened, ['https://checkout.example/kyren'])
  })
})

describe('subscriptions page layout', () => {
  test('mounts subscription and referral features without account-balance surfaces', () => {
    const page = readSource('src/features/wallet/index.tsx')
    const plans = readSource(
      'src/features/wallet/components/subscription-plans-card.tsx'
    )
    const affiliate = readSource(
      'src/features/wallet/components/affiliate-rewards-card.tsx'
    )

    assert.match(page, /<SubscriptionPlansCard/)
    assert.match(page, /<TimedSubscriptionConversionQuotesCard/)
    assert.match(page, /<AffiliateRewardsCard/)
    assert.doesNotMatch(
      page,
      /RechargeFormCard|WalletStatsCard|BillingHistoryDialog/
    )
    assert.doesNotMatch(
      plans,
      /accountBalance|Pay with Account Balance|onOpenBilling|Order History/
    )
    assert.match(affiliate, /Copy referral link/)
    assert.match(affiliate, /value=\{referralShareText\}/)
    assert.doesNotMatch(
      affiliate,
      /account|commission|transfer|withdrawal|cashback|formatAccountBalance/i
    )
  })

  test('keeps billing strategy outside My Subscriptions', () => {
    const page = readSource('src/features/wallet/index.tsx')
    const plans = readSource(
      'src/features/wallet/components/subscription-plans-card.tsx'
    )
    const mySubscriptionsIndex = plans.indexOf("{t('My Subscriptions')}")
    const strategyIndex = plans.indexOf(
      '<SubscriptionBillingStrategyControl data={selfSubscriptionData} />'
    )
    const plansIndex = plans.indexOf('{plans.length > 0 ?')

    assert.doesNotMatch(page, /SubscriptionBillingStrategyControl/)
    assert.ok(mySubscriptionsIndex >= 0)
    assert.ok(strategyIndex > mySubscriptionsIndex)
    assert.ok(plansIndex > strategyIndex)
  })

  test('keeps active selection and quota reset actions', () => {
    const plans = readSource(
      'src/features/wallet/components/subscription-plans-card.tsx'
    )

    assert.match(plans, /setActiveSubscription/)
    assert.match(plans, /resetSubscriptionQuota/)
    assert.match(plans, /Set as active/)
    assert.match(plans, /Reset credits/)
    assert.match(plans, /formatUsedCreditCount\(tokenUsed, t\)/)
    assert.doesNotMatch(plans, /formatCreditLimit\(tokenUsed, t\)/)
  })

  test('does not mount commission balance controls', () => {
    const page = readSource('src/features/wallet/index.tsx')
    const affiliate = readSource(
      'src/features/wallet/components/affiliate-rewards-card.tsx'
    )

    assert.doesNotMatch(
      page + affiliate,
      /onCommissionTransferSuccess|CommissionTransferDialog|CommissionWithdrawalDialog/
    )
  })
})
