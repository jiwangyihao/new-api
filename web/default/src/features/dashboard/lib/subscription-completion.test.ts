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
import { getSubscriptionCompletion } from './subscription-completion'
import type { UserSubscriptionRecord } from '@/features/subscriptions/types'

const NOW = 1_700_000_000

function subscription(
  overrides: Partial<UserSubscriptionRecord['subscription']>
): UserSubscriptionRecord {
  return {
    subscription: {
      id: 1,
      user_id: 1,
      plan_id: 1,
      status: 'active',
      start_time: NOW - 100,
      end_time: NOW + 100,
      amount_total: 0,
      amount_used: 0,
      ...overrides,
    },
  }
}

describe('dashboard subscription completion', () => {
  test('returns paid for an active non-trial subscription', () => {
    assert.equal(
      getSubscriptionCompletion([subscription({ grant_reason: 'order' })], NOW),
      'paid'
    )
  })

  test('returns trial for active trial-code or invite-trial subscriptions only', () => {
    assert.equal(
      getSubscriptionCompletion(
        [subscription({ grant_reason: ' trial_code ' })],
        NOW
      ),
      'trial'
    )
    assert.equal(
      getSubscriptionCompletion(
        [subscription({ grant_reason: 'invite_trial' })],
        NOW
      ),
      'trial'
    )
  })

  test('prefers paid when paid and trial subscriptions are both active', () => {
    assert.equal(
      getSubscriptionCompletion(
        [
          subscription({ id: 2, grant_reason: 'trial_code' }),
          subscription({ id: 3, grant_reason: 'order' }),
        ],
        NOW
      ),
      'paid'
    )
  })

  test('does not count non-purchase entitlements as paid plans', () => {
    assert.equal(
      getSubscriptionCompletion(
        [
          subscription({ id: 6, grant_reason: 'monthly_invite_entitlement' }),
          subscription({ id: 7, grant_reason: 'admin' }),
          subscription({ id: 8, grant_reason: '' }),
        ],
        NOW
      ),
      'none'
    )
  })

  test('treats legacy order source as paid when grant reason is absent', () => {
    assert.equal(
      getSubscriptionCompletion(
        [subscription({ id: 9, grant_reason: '', source: 'order' })],
        NOW
      ),
      'paid'
    )
  })

  test('ignores expired and inactive subscriptions', () => {
    assert.equal(
      getSubscriptionCompletion(
        [
          subscription({ id: 4, grant_reason: 'order', end_time: NOW - 1 }),
          subscription({ id: 5, grant_reason: 'order', status: 'cancelled' }),
        ],
        NOW
      ),
      'none'
    )
  })
})
