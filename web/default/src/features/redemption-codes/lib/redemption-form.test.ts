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
import {
  transformFormDataToPayload,
  transformRedemptionToFormDefaults,
} from './redemption-form'
import type { Redemption } from '../types'

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
    } satisfies Redemption)

    assert.equal(defaults.type, 'subscription')
    assert.equal(defaults.quota_cny, 0)
    assert.equal(defaults.plan_id, 7)
  })
})
