import assert from 'node:assert/strict'
import test from 'node:test'
import type { AdminAnalyticsUsersDrilldownEnvelopeResponse } from '../types'
import { normalizeAdminAnalyticsUsersDrilldownResponse } from './admin-analytics-drilldown'

test('normalizes admin analytics panel envelope for users drilldown', () => {
  const response: AdminAnalyticsUsersDrilldownEnvelopeResponse = {
    success: true,
    message: '',
    data: {
      range: { start_timestamp: 1, end_timestamp: 2, snapshot_at: 2 },
      data: {
        users: {
          items: [],
          page: { limit: 20, offset: 0, total: 3, has_more: false },
          sort_order: 'asc',
        },
      },
    },
  }

  const normalized = normalizeAdminAnalyticsUsersDrilldownResponse(response)

  assert.equal(normalized.success, true)
  assert.equal(normalized.data?.users.page.total, 3)
})
