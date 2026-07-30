import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zh from '@/i18n/locales/zh.json'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  REDEMPTION_ERROR_MESSAGE_KEYS,
  RedemptionRequestError,
  getRedemptionErrorMessageKey,
  submitInitialRedemption,
} from './use-redemption'

const redemptionErrorTranslations: Record<string, Record<string, string>> = {
  en: en.translation,
  zh: zh.translation,
  fr: fr.translation,
  ja: ja.translation,
  ru: ru.translation,
  vi: vi.translation,
}

describe('redemption error localization', () => {
  test('maps every stable API code to all six supported locales', () => {
    for (const [code, key] of Object.entries(REDEMPTION_ERROR_MESSAGE_KEYS)) {
      assert.equal(getRedemptionErrorMessageKey(code), key)
      for (const [locale, translations] of Object.entries(
        redemptionErrorTranslations
      )) {
        const message = translations[key]
        assert.equal(typeof message, 'string', `${locale}: ${key}`)
        assert.notEqual(message.trim(), '', `${locale}: ${key}`)
        if (locale !== 'en') {
          assert.notEqual(message, key, `${locale}: ${key}`)
        }
      }
    }
    assert.equal(getRedemptionErrorMessageKey('unknown_error'), undefined)
  })
})

describe('initial redemption submission', () => {
  test('keeps the legacy wallet request mode-free', async () => {
    const requests: unknown[] = []
    let redeemed = false
    let modeRequired = false
    const errors: string[] = []

    await submitInitialRedemption({
      key: 'wallet-code',
      redeem: async (request) => {
        requests.push(request)
        return { type: 'wallet', quota: 100 }
      },
      onRedeemed: () => {
        redeemed = true
      },
      onModeRequired: () => {
        modeRequired = true
      },
      onError: (message) => errors.push(message),
      fallbackError: 'Redemption failed',
    })

    assert.deepEqual(requests, [{ key: 'wallet-code' }])
    assert.equal(redeemed, true)
    assert.equal(modeRequired, false)
    assert.deepEqual(errors, [])
  })

  test('reports an ordinary wallet redemption failure without opening mode selection', async () => {
    let modeRequired = false
    const errors: string[] = []

    await submitInitialRedemption({
      key: 'invalid-wallet-code',
      redeem: async () => {
        throw new RedemptionRequestError('Invalid redemption code')
      },
      onRedeemed: () => {
        assert.fail('failed redemption must not refresh success state')
      },
      onModeRequired: () => {
        modeRequired = true
      },
      onError: (message) => errors.push(message),
      fallbackError: 'Redemption failed',
    })

    assert.equal(modeRequired, false)
    assert.deepEqual(errors, ['Invalid redemption code'])
  })

  test('opens mode selection only for the structured mode-required error', async () => {
    let modeRequired = false
    const errors: string[] = []

    await submitInitialRedemption({
      key: 'subscription-code',
      redeem: async () => {
        throw new RedemptionRequestError(
          'Redemption mode is required',
          'redemption_mode_required'
        )
      },
      onRedeemed: () => {
        assert.fail('mode-required response is not a completed redemption')
      },
      onModeRequired: () => {
        modeRequired = true
      },
      onError: (message) => errors.push(message),
      fallbackError: 'Redemption failed',
    })

    assert.equal(modeRequired, true)
    assert.deepEqual(errors, [])
  })
})
