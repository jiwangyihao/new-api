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

  test('affiliate card documents invitation reward rules near referral link', () => {
    const source = readAffiliateRewardsSource()
    assert.match(source, /Invitation reward rules/)
    assert.match(source, /two longest valid paid referrals/)
    assert.match(source, /same tier/)
  })
})
