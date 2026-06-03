import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import {
  buildKyrenOptionUpdates,
  buildPaymentOptionUpdates,
  getKyrenWebhookUrl,
  type PaymentFormValues,
} from './payment-settings-section'

type WaffoPaymentFields = {
  WaffoMinTopUp: number
  WaffoUnitPrice: number
  WaffoPancakeMinTopUp: number
  WaffoPancakeUnitPrice: number
}

type PaymentSettingsValues = PaymentFormValues & WaffoPaymentFields

function makePaymentValues(
  overrides: Partial<PaymentSettingsValues> = {}
): PaymentSettingsValues {
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
    WaffoMinTopUp: 1,
    WaffoUnitPrice: 8,
    WaffoPancakeMinTopUp: 1,
    WaffoPancakeUnitPrice: 8,
    ...overrides,
  }
}

describe('PaymentSettingsSection payment amount helpers', () => {
  test('keeps CNY yuan amount settings independent from quotaPerUnit', () => {
    const values = makePaymentValues({
      MinTopUp: 40,
      AmountOptions: '[10,40]',
      AmountDiscount: '{"40":0.95}',
      StripeMinTopUp: 20,
      StripeUnitPrice: 7.3,
      WaffoMinTopUp: 30,
      WaffoUnitPrice: 7.1,
      WaffoPancakeMinTopUp: 25,
      WaffoPancakeUnitPrice: 6.9,
    })
    const initial = makePaymentValues({
      MinTopUp: 1,
      AmountOptions: '[]',
      AmountDiscount: '{}',
      StripeMinTopUp: 1,
      StripeUnitPrice: 8,
      WaffoMinTopUp: 1,
      WaffoUnitPrice: 8,
      WaffoPancakeMinTopUp: 1,
      WaffoPancakeUnitPrice: 8,
    })

    const updates = buildPaymentOptionUpdates(values, initial)

    assert.deepEqual(
      updates.filter((item) =>
        [
          'MinTopUp',
          'payment_setting.amount_options',
          'payment_setting.amount_discount',
          'StripeMinTopUp',
          'StripeUnitPrice',
          'WaffoMinTopUp',
          'WaffoUnitPrice',
          'WaffoPancakeMinTopUp',
          'WaffoPancakeUnitPrice',
        ].includes(item.key)
      ),
      [
        { key: 'MinTopUp', value: 40 },
        { key: 'payment_setting.amount_options', value: '[10,40]' },
        { key: 'payment_setting.amount_discount', value: '{"40":0.95}' },
        { key: 'StripeUnitPrice', value: 7.3 },
        { key: 'StripeMinTopUp', value: 20 },
        { key: 'WaffoMinTopUp', value: 30 },
        { key: 'WaffoUnitPrice', value: 7.1 },
        { key: 'WaffoPancakeMinTopUp', value: 25 },
        { key: 'WaffoPancakeUnitPrice', value: 6.9 },
      ]
    )
  })
})

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

describe('PaymentSettingsSection amount copy', () => {
  test('describes Waffo and Pancake minimums as credited CNY balance', () => {
    const waffoSource = readFileSync(
      new URL('./waffo-settings-section.tsx', import.meta.url),
      'utf8'
    )
    const pancakeSource = readFileSync(
      new URL('./waffo-pancake-settings-section.tsx', import.meta.url),
      'utf8'
    )

    const creditedBalanceCopy =
      /Minimum credited balance \(CNY\)|minimumCreditedBalanceCny|minimum_credited_balance_cny/i
    assert.match(waffoSource, creditedBalanceCopy)
    assert.match(pancakeSource, creditedBalanceCopy)
    assert.doesNotMatch(
      waffoSource,
      /Minimum top-up \(USD\)|Minimum top-up quantity|最低充值美元数量/
    )
    assert.doesNotMatch(
      pancakeSource,
      /Minimum top-up \(USD\)|Minimum top-up quantity|最低充值美元数量/
    )
  })

  test('describes amount options and discounts as account balance CNY yuan, not quota', () => {
    const optionsSource = readFileSync(
      new URL('./amount-options-visual-editor.tsx', import.meta.url),
      'utf8'
    )
    const discountSource = readFileSync(
      new URL('./amount-discount-visual-editor.tsx', import.meta.url),
      'utf8'
    )
    const discountDialogSource = readFileSync(
      new URL('./amount-discount-dialog.tsx', import.meta.url),
      'utf8'
    )
    const discountCopy = [discountSource, discountDialogSource].join('\n')
    const accountBalanceCnyCopy =
      /(?:account|credited) balance.*(?:CNY|yuan)|(?:CNY|yuan).*(?:account|credited) balance/i
    const oldAmountCopy = [
      optionsSource,
      discountSource,
      discountDialogSource,
    ].join('\n')

    assert.match(optionsSource, accountBalanceCnyCopy)
    assert.match(discountCopy, accountBalanceCnyCopy)
    assert.doesNotMatch(
      oldAmountCopy,
      /Recharge Amount \(USD\)|Preset recharge amounts|recharge amount threshold|\$\{amount\}|\$\{discount\.amount\}|formatQuotaShort|\bquota\b/i
    )
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
