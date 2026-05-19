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
import type { UserSubscriptionRecord } from '../../subscriptions/types'
import { getSubscriptionDisplayLabel } from './subscription-display'

function makeRecord(
  id: number,
  planId: number,
  overrides: Partial<UserSubscriptionRecord> = {}
): UserSubscriptionRecord {
  return {
    subscription: {
      id,
      user_id: 1,
      plan_id: planId,
      status: 'active',
      start_time: 0,
      end_time: 0,
      amount_total: 0,
      amount_used: 0,
    },
    ...overrides,
  }
}

describe('subscription display labels', () => {
  test('uses plain summary plan title for trial subscriptions hidden from public plan map', () => {
    const record = makeRecord(1, 100, {
      subscription: {
        ...makeRecord(1, 100).subscription,
        grant_reason: 'trial_code',
      },
      plan: { title: '试用装可乐' },
    } as Partial<UserSubscriptionRecord>)

    assert.equal(
      getSubscriptionDisplayLabel(record, new Map(), 'Subscription'),
      '试用装可乐'
    )
  })

  test('uses public plan map title when summary plan title is missing', () => {
    const record = makeRecord(2, 200)
    const planTitleMap = new Map([[200, 'Basic']])

    assert.equal(getSubscriptionDisplayLabel(record, planTitleMap), 'Basic')
  })

  test('falls back to generic subscription label when no plan title is available', () => {
    const record = makeRecord(3, 300)

    assert.equal(getSubscriptionDisplayLabel(record, new Map()), 'Subscription')
  })

  test('ignores blank summary plan title and uses public plan map title', () => {
    const record = makeRecord(4, 400, {
      plan: { title: '   ' },
    } as Partial<UserSubscriptionRecord>)
    const planTitleMap = new Map([[400, 'Pro']])

    assert.equal(getSubscriptionDisplayLabel(record, planTitleMap), 'Pro')
  })
})
