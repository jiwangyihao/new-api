import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import {
  buildKyrenOptionUpdates,
  getKyrenWebhookUrl,
  type PaymentFormValues,
} from './payment-settings-section'

function makePaymentValues(
  overrides: Partial<PaymentFormValues> = {}
): PaymentFormValues {
  return {
    PayAddress: '',
    EpayId: '',
    EpayKey: '',
    Price: 7.3,
    MinTopUp: 1,
    CustomCallbackAddress: '',
    PayMethods: '[]',
    AmountOptions: '[]',
    AmountDiscount: '{}',
    StripeApiSecret: '',
    StripeWebhookSecret: '',
    StripePriceId: '',
    StripeUnitPrice: 8,
    StripeMinTopUp: 1,
    StripePromotionCodesEnabled: false,
    CreemApiKey: '',
    CreemWebhookSecret: '',
    CreemTestMode: false,
    CreemProducts: '[]',
    KyrenApiKey: '',
    KyrenWebhookSecret: '',
    KyrenBaseURL: 'https://api.kyren.top',
    KyrenTopUpProducts: '[]',
    ServerAddress: '',
    ...overrides,
  }
}

describe('PaymentSettingsSection Kyren settings helpers', () => {
  test('renders full Kyren webhook URL when ServerAddress is configured', () => {
    assert.equal(
      getKyrenWebhookUrl('https://new-api.example.com/'),
      'https://new-api.example.com/api/kyren/webhook'
    )
    assert.equal(getKyrenWebhookUrl(''), null)
  })

  test('omits empty Kyren secrets, trims BaseURL, and never emits KyrenTopUpProducts through generic options', () => {
    const initial = makePaymentValues({
      KyrenApiKey: 'existing-api-key',
      KyrenWebhookSecret: 'existing-webhook-secret',
      KyrenBaseURL: 'https://api.kyren.top',
      KyrenTopUpProducts: '[{"id":"old"}]',
      ServerAddress: 'https://server.example.com',
    })
    const values = makePaymentValues({
      KyrenApiKey: '',
      KyrenWebhookSecret: '',
      KyrenBaseURL: 'https://staging-api.kyren.top///',
      KyrenTopUpProducts: '[{"id":"new"}]',
      ServerAddress: 'https://changed.example.com',
    })

    const updates = buildKyrenOptionUpdates(values, initial)

    assert.deepEqual(updates, [
      { key: 'KyrenBaseURL', value: 'https://staging-api.kyren.top' },
    ])
    assert.equal(
      updates.some((update) => update.key === 'KyrenTopUpProducts'),
      false
    )
    assert.equal(updates.some((update) => update.key === 'ServerAddress'), false)
  })
})

describe('PaymentSettingsSection Kyren UI and API contract', () => {
  test('contains the Kyren top-up products editor and webhook empty-state copy', () => {
    const source = readFileSync(
      new URL('./payment-settings-section.tsx', import.meta.url),
      'utf8'
    )

    assert.match(source, /KyrenTopUpProductsVisualEditor/)
    assert.match(source, /Server address is not configured/)
  })

  test('saves Kyren top-up products through the dedicated API only', () => {
    const sectionSource = readFileSync(
      new URL('./payment-settings-section.tsx', import.meta.url),
      'utf8'
    )
    const editorSource = readFileSync(
      new URL('./kyren-topup-products-visual-editor.tsx', import.meta.url),
      'utf8'
    )

    assert.doesNotMatch(sectionSource, /key:\s*['"]KyrenTopUpProducts['"]/) 
    assert.match(editorSource, /\/api\/payment\/kyren\/topup-products/) 
    assert.match(editorSource, /api\.put<.*KyrenTopUpProductsListResponse/) 
  })
})
