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

function readWalletSource(): string {
  return readFileSync('src/features/wallet/index.tsx', 'utf8')
}

function readSubscriptionPlansSource(): string {
  return readFileSync(
    'src/features/wallet/components/subscription-plans-card.tsx',
    'utf8'
  )
}

function readAffiliateRewardsSource(): string {
  return readFileSync(
    'src/features/wallet/components/affiliate-rewards-card.tsx',
    'utf8'
  )
}

function readBillingHistoryDialogSource(): string {
  return readFileSync(
    'src/features/wallet/components/dialogs/billing-history-dialog.tsx',
    'utf8'
  )
}

function readWalletStatsCardSource(): string {
  return readFileSync(
    'src/features/wallet/components/wallet-stats-card.tsx',
    'utf8'
  )
}

function readCreemProductsSectionSource(): string {
  return readFileSync(
    'src/features/wallet/components/creem-products-section.tsx',
    'utf8'
  )
}

function readCreemConfirmDialogSource(): string {
  return readFileSync(
    'src/features/wallet/components/dialogs/creem-confirm-dialog.tsx',
    'utf8'
  )
}

function readRechargeFormCardSource(): string {
  return readFileSync(
    'src/features/wallet/components/recharge-form-card.tsx',
    'utf8'
  )
}

function readTransferDialogSource(): string {
  return readFileSync(
    'src/features/wallet/components/dialogs/transfer-dialog.tsx',
    'utf8'
  )
}

function readUseAffiliateSource(): string {
  return readFileSync('src/features/wallet/hooks/use-affiliate.ts', 'utf8')
}

function readPaymentConfirmDialogSource(): string {
  return readFileSync(
    'src/features/wallet/components/dialogs/payment-confirm-dialog.tsx',
    'utf8'
  )
}

function readWalletFormatSource(): string {
  return readFileSync('src/features/wallet/lib/format.ts', 'utf8')
}

function readUsePaymentSource(): string {
  return readFileSync('src/features/wallet/hooks/use-payment.ts', 'utf8')
}

describe('wallet Kyren payment flow', () => {
  test('submits Kyren payment with local top-up product id', async () => {
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
      openCheckout: (url) => {
        opened.push(url)
      },
    })

    assert.deepEqual(calls, [{ product_id: 'topup_cny_10' }])
    assert.deepEqual(opened, ['https://checkout.example/kyren'])
  })
})
describe('wallet page layout', () => {
  test('places subscription plans before add-funds redemption card', () => {
    const source = readWalletSource()
    const gridIndex = source.indexOf('xl:grid-cols-')
    const subscriptionIndex = source.indexOf('<SubscriptionPlansCard', gridIndex)
    const addFundsIndex = source.indexOf("id='wallet-add-funds'", gridIndex)

    assert.notEqual(gridIndex, -1, 'wallet page should render a responsive grid')
    assert.notEqual(
      subscriptionIndex,
      -1,
      'wallet page should render subscription plans in the main grid'
    )
    assert.notEqual(
      addFundsIndex,
      -1,
      'wallet page should render the add-funds/redemption card in the main grid'
    )
    assert.ok(
      subscriptionIndex < addFundsIndex,
      'desktop and mobile reading order should be subscription plans before redemption'
    )
  })

  test('wallet subscriptions expose active selection and quota reset actions', () => {
    const source = readSubscriptionPlansSource()
    assert.match(source, /setActiveSubscription/)
    assert.match(source, /resetSubscriptionQuota/)
    assert.match(source, /Set as active/)
    assert.match(source, /Reset quota/)
  })

  test('subscription usage display formats used zero as numeric tokens', () => {
    const source = readSubscriptionPlansSource()
    assert.match(source, /formatUsedTokenCount\(tokenUsed, t\)/)
    assert.doesNotMatch(source, /formatTokenLimit\(tokenUsed, t\)/)
  })

  test('affiliate card documents invitation reward rules near referral link', () => {
    const source = readAffiliateRewardsSource()
    assert.match(source, /Invitation reward rules/)
    assert.match(source, /two longest valid paid referrals/)
    assert.match(source, /same tier/)
  })

  test('affiliate card focuses referral copy on the link', () => {
    const source = readAffiliateRewardsSource()
    assert.match(source, /赔钱GPT超低价稳定GPT服务/)
    assert.doesNotMatch(source, /t\('Pending'\), formatQuota/)
    assert.doesNotMatch(source, /t\('Total Earned'\), formatQuota/)
    assert.doesNotMatch(source, /t\('Invites'\), String/)
    assert.doesNotMatch(source, /Transfer to Balance/)
    assert.match(source, /value=\{referralShareText\}/)
    assert.match(source, /CopyButton[\s\S]*value=\{referralShareText\}/)
  })

  test('wallet balance displays use account balance CNY cents formatting', () => {
    const source = readWalletStatsCardSource()

    assert.match(
      source,
      /formatAccountBalanceForPlanPurchase\(props\.user\?\.quota \?\? 0\)/
    )
  })

  test('Creem and Kyren products display credited CNY account balance', () => {
    const creemProductsSource = readCreemProductsSectionSource()
    const creemDialogSource = readCreemConfirmDialogSource()
    const rechargeSource = readRechargeFormCardSource()

    for (const source of [
      creemProductsSource,
      creemDialogSource,
      rechargeSource,
    ]) {
      assert.match(
        source,
        /formatAccountBalanceForPlanPurchase\(\s*product\.quota\s*\)/
      )
      assert.doesNotMatch(source, /formatNumber\(product\.quota\)/)
      assert.doesNotMatch(source, /\{\{quota\}\} quota/)
    }
  })

  test('Kyren top-up products keep direct checkout without wallet confirmation dialog', () => {
    const walletSource = readWalletSource()
    const rechargeSource = readRechargeFormCardSource()
    const kyrenHandlerStart = walletSource.indexOf(
      'const handleKyrenTopUpProductSelect'
    )
    const kyrenHandlerEnd = walletSource.indexOf(
      'const handleWaffoMethodSelect',
      kyrenHandlerStart
    )
    const usePaymentSource = readUsePaymentSource()
    const paymentHandlerStart = usePaymentSource.indexOf(
      'const processKyrenPayment'
    )
    const paymentHandlerEnd = usePaymentSource.indexOf(
      'const processWaffoPayment',
      paymentHandlerStart
    )
    const kyrenHandler = walletSource.slice(kyrenHandlerStart, kyrenHandlerEnd)
    const paymentHandler = usePaymentSource.slice(
      paymentHandlerStart,
      paymentHandlerEnd
    )

    assert.match(
      rechargeSource,
      /formatAccountBalanceForPlanPurchase\(\s*product\.quota\s*\)/
    )
    assert.match(paymentHandler, /processKyrenTopUpProductPayment/)
    assert.doesNotMatch(
      kyrenHandler,
      /setConfirmDialogOpen|setCreemDialogOpen/
    )
  })

  test('affiliate reward transfer accepts CNY amount and submits account balance cents', () => {
    const transferSource = readTransferDialogSource()
    const affiliateSource = readUseAffiliateSource()

    assert.doesNotMatch(transferSource, /QUOTA_PER_DOLLAR/)
    assert.match(transferSource, /MIN_TRANSFER_AMOUNT_CNY = 0\.01/)
    assert.match(transferSource, /min=\{MIN_TRANSFER_AMOUNT_CNY\}/)
    assert.match(transferSource, /step=\{MIN_TRANSFER_AMOUNT_CNY\}/)
    assert.match(transferSource, /onConfirm\(amount\)/)
    assert.match(
      transferSource,
      /formatAccountBalanceForPlanPurchase\(availableQuota\)/
    )
    assert.match(affiliateSource, /accountBalanceCnyToCents/)
    assert.match(
      affiliateSource,
      /const amountCents = accountBalanceCnyToCents\(amountCny\)/
    )
    assert.match(
      affiliateSource,
      /transferAffiliateQuota\(\{ quota: amountCents \}\)/
    )
  })

  test('payment confirmation separates credited balance from amount paid', () => {
    const source = readPaymentConfirmDialogSource()

    assert.match(source, /t\('Topup Amount'\)/)
    assert.match(source, /t\('You Pay'\)/)
    assert.match(source, /formatAccountBalanceForPlanPurchase/)
    assert.match(source, /accountBalanceCnyToCents\(topupAmount\)/)
    assert.doesNotMatch(source, /topupAmount \* usdExchangeRate/)
  })

  test('regular top-up presets display credited CNY amount without exchange-rate conversion', () => {
    const rechargeSource = readRechargeFormCardSource()
    const formatSource = readWalletFormatSource()

    assert.match(formatSource, /const displayValue = presetValue/)
    assert.doesNotMatch(formatSource, /presetValue \* usdExchangeRate/)
    assert.match(
      rechargeSource,
      /formatAccountBalanceForPlanPurchase\(\s*accountBalanceCnyToCents\(displayValue\)\s*\)/
    )
  })

  test('billing history uses credited balance DTO fields instead of amount units', () => {
    const source = readBillingHistoryDialogSource()

    assert.match(source, /credited_balance_display/)
    assert.match(source, /credited_balance_cents/)
    assert.match(source, /getCreditedBalanceDisplay\(record\)/)
    assert.match(source, /record\.credited_balance_display\?\.trim\(\)/)
    assert.match(source, /record\.credited_balance_cents/)
    assert.doesNotMatch(source, /formatCurrencyFromUSD\(record\.amount/)
    assert.match(source, /Number\.isFinite\(cents\)/)
    assert.doesNotMatch(source, /cents > 0/)
    assert.match(source, /corresponding account balance/)
    assert.doesNotMatch(source, /corresponding quota/)
  })
})
