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
import { describe, test } from 'node:test'
import { useSystemConfigStore } from '@/stores/system-config-store'
import type { Redemption } from '../types'
import type { RedemptionBatchRow } from './redemption-batch'
import {
  formatRedemptionWalletValue,
  getRedemptionRowCopyText,
  getRedemptionRowCopyCount,
} from './redemption-batch'
import {
  transformFormDataToPayload,
  transformRedemptionToFormDefaults,
} from './redemption-form'

function setCurrencyConfig() {
  useSystemConfigStore.setState((state) => ({
    config: {
      ...state.config,
      currency: {
        ...state.config.currency,
        quotaDisplayType: 'CNY',
        quotaPerUnit: 500000,
        usdExchangeRate: 7.3,
      },
    },
  }))
}

describe('redemption wallet display', () => {
  test('formats stored wallet quota as one-to-one CNY balance without exchange-rate conversion', () => {
    setCurrencyConfig()

    assert.equal(formatRedemptionWalletValue(100 * 500000), '¥100.00')
  })
})

describe('redemption form payloads', () => {
  test('sends CNY wallet amount directly to backend instead of USD-converted quota', () => {
    setCurrencyConfig()

    const payload = transformFormDataToPayload({
      type: 'wallet',
      name: 'forty-cny',
      quota_cny: 40,
      plan_id: 0,
      count: 1,
    })

    assert.equal(payload.type, 'wallet')
    assert.equal(payload.quota, 40)
    assert.equal(payload.plan_id, 0)
  })

  test('sends selected subscription plan and zero quota to backend', () => {
    const payload = transformFormDataToPayload({
      type: 'subscription',
      name: 'plan-code',
      quota_cny: 40,
      plan_id: 7,
      count: 2,
    })

    assert.deepEqual(payload, {
      type: 'subscription',
      name: 'plan-code',
      quota: 0,
      plan_id: 7,
      expired_time: 0,
      count: 2,
    })
  })
})

describe('redemption form defaults', () => {
  test('converts stored wallet quota back to CNY amount one-to-one', () => {
    setCurrencyConfig()

    const defaults = transformRedemptionToFormDefaults({
      id: 1,
      user_id: 1,
      name: 'forty-cny',
      key: 'key',
      status: 1,
      type: 'wallet',
      quota: 40 * 500000,
      plan_id: 0,
      created_time: 0,
      redeemed_time: 0,
      expired_time: 0,
      used_user_id: 0,
      batch_id: 'batch-wallet',
    } satisfies Redemption)

    assert.equal(defaults.type, 'wallet')
    assert.equal(defaults.quota_cny, 40)
    assert.equal(defaults.plan_id, 0)
  })

  test('restores subscription type and plan id without converting quota', () => {
    const defaults = transformRedemptionToFormDefaults({
      id: 2,
      user_id: 1,
      name: 'plan-code',
      key: 'key',
      status: 1,
      type: 'subscription',
      quota: 0,
      plan_id: 7,
      plan: {
        id: 7,
        title: 'Pro',
        price_amount: 99,
        currency: 'CNY',
        duration_unit: 'month',
        duration_value: 1,
        quota_reset_period: 'monthly',
        enabled: true,
        sort_order: 0,
        max_purchase_per_user: 0,
        total_amount: 0,
      },
      created_time: 0,
      redeemed_time: 0,
      expired_time: 0,
      used_user_id: 0,
      batch_id: 'batch-subscription',
    } satisfies Redemption)

    assert.equal(defaults.type, 'subscription')
    assert.equal(defaults.quota_cny, 0)
    assert.equal(defaults.plan_id, 7)
  })
})

describe('redemption batch copy', () => {
  test('copies every code in a batch row as name and key lines', () => {
    const batch: RedemptionBatchRow = {
      id: 1,
      user_id: 1,
      name: 'summer-batch',
      key: 'first-key',
      status: 1,
      type: 'wallet',
      quota: 100,
      plan_id: 0,
      created_time: 0,
      redeemed_time: 0,
      expired_time: 0,
      used_user_id: 0,
      batch_id: 'batch-a',
      is_batch_row: true,
      children: [
        {
          id: 1,
          user_id: 1,
          name: 'summer-batch',
          key: 'first-key',
          status: 1,
          type: 'wallet',
          quota: 100,
          plan_id: 0,
          created_time: 0,
          redeemed_time: 0,
          expired_time: 0,
          used_user_id: 0,
          batch_id: 'batch-a',
          is_batch_row: false,
        },
        {
          id: 2,
          user_id: 1,
          name: 'summer-batch',
          key: 'second-key',
          status: 1,
          type: 'wallet',
          quota: 100,
          plan_id: 0,
          created_time: 0,
          redeemed_time: 0,
          expired_time: 0,
          used_user_id: 0,
          batch_id: 'batch-a',
          is_batch_row: false,
        },
      ],
    }

    assert.equal(getRedemptionRowCopyCount(batch), 2)
    assert.equal(
      getRedemptionRowCopyText(batch),
      'summer-batch\tfirst-key\nsummer-batch\tsecond-key'
    )
  })
})
