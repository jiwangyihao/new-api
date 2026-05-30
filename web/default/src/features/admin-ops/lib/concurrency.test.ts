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
import { buildAdminOpsConcurrencyPlanOptions } from './concurrency'

describe('admin ops concurrency helpers', () => {
  test('keeps admin plan options available when current rows are filtered', () => {
    const options = buildAdminOpsConcurrencyPlanOptions(
      [
        {
          user_id: 1,
          username: 'ops-user',
          active: 1,
          limit: 2,
          queued: 0,
          queue_capacity: 4,
          plan_id: 10,
          plan_title: 'Pro',
          plan_code: 'pro',
          amount_total: 0,
          amount_used: 0,
          token_limit: 100,
          token_used: 25,
          usage: 0.25,
          usage_used: 25,
          usage_total: 100,
          oldest_queued_seconds: 0,
          utilization: 0.5,
          queue_utilization: 0,
          status: 'normal',
        },
      ],
      [
        { id: 11, label: 'Basic' },
        { id: 10, label: 'Pro Plan' },
      ]
    )

    assert.deepEqual(options, [
      { id: 11, label: 'Basic' },
      { id: 10, label: 'Pro Plan' },
    ])
  })
})
