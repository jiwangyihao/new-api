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
import {
  getSubscriptionConversionErrorMessage,
  SubscriptionConversionRequestError,
} from './errors'

const knownErrorMessages: Record<string, string> = {
  subscription_conversion_quote_stale:
    'The quote expired or authoritative facts changed. Review the refreshed quote and confirm again.',
  subscription_conversion_ineligible:
    'This subscription is no longer eligible for conversion. Review the refreshed quote before trying again.',
  subscription_conversion_idempotency_conflict:
    'This conversion conflicts with an earlier retry. Refresh the quote before trying again.',
  credit_fx_rate_missing:
    'The configured Credit exchange rate is invalid. Contact an administrator.',
  credit_fx_rate_empty:
    'The configured Credit exchange rate is invalid. Contact an administrator.',
  credit_fx_invalid_decimal:
    'The configured Credit exchange rate is invalid. Contact an administrator.',
  credit_fx_precision_exceeded:
    'The configured Credit exchange rate is invalid. Contact an administrator.',
  credit_fx_non_positive:
    'The configured Credit exchange rate is invalid. Contact an administrator.',
  credit_fx_direction_mismatch:
    'The configured Credit exchange rate is invalid. Contact an administrator.',
  credit_fx_unsupported_currency:
    'This currency pair is not supported for Credit conversion.',
  credit_fx_overflow: 'The conversion value is too large to calculate safely.',
}

describe('subscription conversion error messages', () => {
  test('maps every stable conversion and FX code to a localized key', () => {
    for (const [code, message] of Object.entries(knownErrorMessages)) {
      assert.equal(
        getSubscriptionConversionErrorMessage(
          new SubscriptionConversionRequestError(code),
          (key) => key
        ),
        message,
        code
      )
    }
  })

  test('uses the localized generic fallback for unknown and untyped errors', () => {
    assert.equal(
      getSubscriptionConversionErrorMessage(
        new SubscriptionConversionRequestError('future_server_code'),
        (key) => `localized:${key}`
      ),
      'localized:Unable to convert subscription'
    )
    assert.equal(
      getSubscriptionConversionErrorMessage(
        new Error('unsafe free-form server detail'),
        (key) => `localized:${key}`
      ),
      'localized:Unable to convert subscription'
    )
  })
})
