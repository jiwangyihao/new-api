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
import { afterEach, describe, test } from 'node:test'
import { api } from '@/lib/api'
import {
  confirmSubscriptionConversion,
  getSubscriptionConversionQuotes,
  SubscriptionConversionRequestError,
} from './api'

const originalGet = api.get
const originalPost = api.post

afterEach(() => {
  api.get = originalGet
  api.post = originalPost
})

describe('subscription conversion API errors', () => {
  test('preserves a stable business code without exposing server text as the error contract', async () => {
    let requestConfig: unknown
    api.post = (async (_url, _request, config) => {
      requestConfig = config
      return {
        data: {
          success: false,
          code: 'subscription_conversion_quote_stale',
          message: 'unsafe free-form server detail',
        },
      }
    }) as typeof api.post

    await assert.rejects(
      () =>
        confirmSubscriptionConversion({
          subscription_id: '7001',
          quote_id: 'quote-stable',
          idempotency_key: 'conversion-key',
        }),
      (error: unknown) => {
        assert.ok(error instanceof SubscriptionConversionRequestError)
        assert.equal(error.code, 'subscription_conversion_quote_stale')
        assert.notEqual(error.message, 'unsafe free-form server detail')
        return true
      }
    )
    assert.deepEqual(requestConfig, { skipBusinessError: true })
  })

  test('suppresses free-form quote errors while retaining their stable code', async () => {
    let requestConfig: unknown
    api.get = (async (_url, config) => {
      requestConfig = config
      return {
        data: {
          success: false,
          code: 'subscription_conversion_ineligible',
          message: 'unsafe free-form quote detail',
        },
      }
    }) as typeof api.get

    await assert.rejects(
      () => getSubscriptionConversionQuotes(),
      (error: unknown) => {
        assert.ok(error instanceof SubscriptionConversionRequestError)
        assert.equal(error.code, 'subscription_conversion_ineligible')
        assert.notEqual(error.message, 'unsafe free-form quote detail')
        return true
      }
    )
    assert.deepEqual(requestConfig, {
      disableDuplicate: true,
      skipBusinessError: true,
    })
  })
})
