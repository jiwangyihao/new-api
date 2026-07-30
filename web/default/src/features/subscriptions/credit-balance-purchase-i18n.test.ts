import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

const locales = ['en', 'zh', 'fr', 'ja', 'ru', 'vi'] as const
const keys = [
  'Choose what this purchase adds',
  'Your saved choice is only a default. Confirm the mode for every purchase.',
  'Timed subscription',
  'Credit balance',
  'Credit balance service limits',
  'Added {{gross}} Credits; offset {{debt}} debt; {{available}} Credits available.',
  'Available Credit balance',
  'Settlement debt',
  'Credit purchase history',
  'Payment page opened',
  'Waiting for payment confirmation. You can close this dialog and resume here later.',
  'Unable to check payment status. Retry status check or payment.',
  'Retry status check',
  'Try payment again',
  'Payment failed or expired. You can try again.',
] as const

describe('Credit balance purchase translations', () => {
  for (const locale of locales) {
    test(`${locale} includes every purchase key`, () => {
      const payload = JSON.parse(
        readFileSync(
          new URL(`../../i18n/locales/${locale}.json`, import.meta.url),
          'utf8'
        )
      ) as { translation: Record<string, string> }

      for (const key of keys) {
        assert.equal(
          typeof payload.translation[key],
          'string',
          `${locale}: ${key}`
        )
        assert.ok(payload.translation[key].trim(), `${locale}: ${key}`)
      }
    })
  }
})
