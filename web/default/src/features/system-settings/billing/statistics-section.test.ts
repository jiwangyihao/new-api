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
import test from 'node:test'
import {
  buildSubscriptionAnalyticsExcludedUsersUpdate,
  formatSubscriptionAnalyticsExcludedAt,
  formatSubscriptionAnalyticsExcludedBy,
  normalizeSubscriptionAnalyticsExcludedUsers,
} from './statistics-section'

test('subscription analytics excluded users are serialized as JSON option value', () => {
  const excludedUsers = normalizeSubscriptionAnalyticsExcludedUsers({
    excludedUsers: [
      { user_id: 10, username: 'alice', reason: 'ops' },
      { user_id: 11, username: '', reason: '' },
    ],
  })

  assert.deepEqual(excludedUsers, [
    { user_id: 10, username: 'alice', reason: 'ops' },
    { user_id: 11 },
  ])

  const update = buildSubscriptionAnalyticsExcludedUsersUpdate(excludedUsers)
  assert.equal(update.key, 'subscription_analytics.excluded_users')
  assert.equal(
    update.value,
    '[{"user_id":10,"username":"alice","reason":"ops"},{"user_id":11}]'
  )
})

test('subscription analytics excluded users preserve audit metadata when saved', () => {
  const excludedUsers = normalizeSubscriptionAnalyticsExcludedUsers(
    {
      excludedUsers: [
        { user_id: 10, username: 'alice-updated', reason: 'updated' },
        { user_id: 12, username: 'carol', reason: '' },
      ],
    },
    [
      {
        user_id: 10,
        username: 'alice',
        reason: 'ops',
        excluded_at: 123,
        excluded_by: 7,
      },
      { user_id: 11, username: 'bob', excluded_at: 456, excluded_by: 8 },
    ]
  )

  assert.deepEqual(excludedUsers, [
    {
      user_id: 10,
      username: 'alice-updated',
      reason: 'updated',
      excluded_at: 123,
      excluded_by: 7,
    },
    { user_id: 12, username: 'carol' },
  ])
})

test('subscription analytics excluded users keep submitted audit metadata without existing config', () => {
  const excludedUsers = normalizeSubscriptionAnalyticsExcludedUsers({
    excludedUsers: [
      {
        user_id: 10,
        username: 'alice',
        reason: 'ops',
        excluded_at: 123,
        excluded_by: 7,
      },
    ],
  })

  assert.deepEqual(excludedUsers, [
    {
      user_id: 10,
      username: 'alice',
      reason: 'ops',
      excluded_at: 123,
      excluded_by: 7,
    },
  ])
})

test('subscription analytics excluded users do not move audit metadata to edited user id', () => {
  const excludedUsers = normalizeSubscriptionAnalyticsExcludedUsers(
    {
      excludedUsers: [
        {
          user_id: 12,
          username: 'carol',
          reason: 'new target',
          excluded_at: 123,
          excluded_by: 7,
        },
      ],
    },
    [
      {
        user_id: 10,
        username: 'alice',
        reason: 'ops',
        excluded_at: 123,
        excluded_by: 7,
      },
    ]
  )

  assert.deepEqual(excludedUsers, [
    { user_id: 12, username: 'carol', reason: 'new target' },
  ])
})

test('subscription analytics excluded audit metadata is displayable', () => {
  assert.equal(formatSubscriptionAnalyticsExcludedAt(undefined), '—')
  assert.equal(formatSubscriptionAnalyticsExcludedAt(0), '—')
  assert.match(
    formatSubscriptionAnalyticsExcludedAt(1_700_000_000),
    /^\d{4}-\d{2}-\d{2} /
  )
  assert.equal(formatSubscriptionAnalyticsExcludedBy(undefined), '—')
  assert.equal(formatSubscriptionAnalyticsExcludedBy(0), '—')
  assert.equal(formatSubscriptionAnalyticsExcludedBy(7), '7')
})
