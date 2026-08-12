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
const invalidFXMessage =
  'The configured Credit exchange rate is invalid. Contact an administrator.'

const SUBSCRIPTION_CONVERSION_ERROR_MESSAGE_KEYS: Record<string, string> = {
  subscription_conversion_quote_stale:
    'The quote expired or authoritative facts changed. Review the refreshed quote and confirm again.',
  subscription_conversion_ineligible:
    'This subscription is no longer eligible for conversion. Review the refreshed quote before trying again.',
  subscription_conversion_idempotency_conflict:
    'This conversion conflicts with an earlier retry. Refresh the quote before trying again.',
  credit_fx_rate_missing: invalidFXMessage,
  credit_fx_rate_empty: invalidFXMessage,
  credit_fx_invalid_decimal: invalidFXMessage,
  credit_fx_precision_exceeded: invalidFXMessage,
  credit_fx_non_positive: invalidFXMessage,
  credit_fx_direction_mismatch: invalidFXMessage,
  credit_fx_unsupported_currency:
    'This currency pair is not supported for Credit conversion.',
  credit_fx_overflow: 'The conversion value is too large to calculate safely.',
}

export class SubscriptionConversionRequestError extends Error {
  readonly code?: string

  constructor(code?: string) {
    super('Subscription conversion request failed')
    this.name = 'SubscriptionConversionRequestError'
    this.code = code
  }
}

export function getSubscriptionConversionErrorMessage(
  error: unknown,
  t: (key: string) => string
): string {
  const key =
    error instanceof SubscriptionConversionRequestError && error.code
      ? SUBSCRIPTION_CONVERSION_ERROR_MESSAGE_KEYS[error.code]
      : undefined
  return t(key ?? 'Unable to convert subscription')
}
