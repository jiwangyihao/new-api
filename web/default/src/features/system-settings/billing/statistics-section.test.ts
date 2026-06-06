import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildSubscriptionAnalyticsExcludedUsersUpdate,
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
