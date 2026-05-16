import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildSubscriptionBalancePayRequestBody } from './api'

describe('subscription balance payment API payload', () => {
  test('keeps plan id and idempotency key in request body', () => {
    assert.deepEqual(
      buildSubscriptionBalancePayRequestBody({
        plan_id: 42,
        idempotency_key: 'balance-pay-1',
      }),
      {
        plan_id: 42,
        idempotency_key: 'balance-pay-1',
      }
    )
  })
})
