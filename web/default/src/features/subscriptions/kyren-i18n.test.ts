import assert from 'node:assert/strict'
import { test } from 'node:test'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zh from '@/i18n/locales/zh.json'

const requiredKeys = [
  '{{quota}} quota',
  'Amount is required',
  'Amount must be at least 0.01 CNY',
  'Duplicate Kyren top-up product ID',
  'Add Kyren top-up product',
  'CNY only',
  'Configuration for Kyren Pay integration',
  'Configure ServerAddress after deployment to expose the Kyren webhook URL.',
  'Configure a fixed CNY wallet top-up product for Kyren Pay.',
  'Configure fixed CNY Kyren top-up products. They are saved through the dedicated Kyren API.',
  'Create Kyren product',
  'Create new',
  'Currency matches',
  'Display name shown to users and Kyren.',
  'Edit Kyren top-up product',
  'Enter Kyren API key',
  'Failed to load Kyren top-up products',
  'Kyren API base URL. Trailing slashes are removed on save.',
  'Kyren API key (leave blank unless updating)',
  'Kyren Gateway',
  'Kyren Pay',
  'Kyren Product ID',
  'Kyren top-up product id is required',
  'Kyren Top-up',
  'Kyren checkout creation failed',
  'Kyren top-up products must be a JSON array',
  'Kyren top-up products only support CNY',
  'Kyren does not support free subscription plans',
  'Kyren does not support trial subscription plans',
  'Kyren payment is unavailable',
  'Kyren product ID',
  'Kyren product binding status',
  'Kyren product currency mismatch',
  'Kyren product is archived',
  'Kyren product is missing',
  'Kyren product is not bound',
  'Kyren product price mismatch',
  'Kyren product synced',
  'Kyren requires enabled and visible subscription plans',
  'Kyren settings were updated elsewhere. Please reload and try again.',
  'Kyren supports CNY subscription plans only',
  'Kyren top-up product currency mismatch',
  'Kyren top-up product is archived',
  'Kyren top-up product price mismatch',
  'Kyren top-up product status refreshed',
  'Kyren top-up product synced',
  'Kyren top-up products',
  'Kyren top-up products refreshed',
  'Kyren top-up uses CNY.',
  'Local top-up product ID is required',
  'Leave blank and use Sync to create it in Kyren.',
  'No Kyren product status loaded',
  'No Kyren top-up products configured. Click "Add Kyren top-up product" to get started.',
  'No Kyren top-up products match your search',
  'Not refreshed',
  'Open Kyren Checkout',
  'Pay with Kyren',
  'Please save the plan first',
  'Price matches',
  'Product name is required',
  'Product status',
  'Refresh Kyren status',
  'Save Kyren settings',
  'Quota must be at least 1',
  'Save the plan before syncing Kyren product status.',
  'Search Kyren top-up products...',
  'Server address is not configured',
  'Show this Kyren top-up product to users.',
  'Stable local ID used by wallet checkout.',
  'Sync Kyren product',
  'Sync to Kyren',
  'Unbound',
  'Webhook URL',
] as const

const locales = { en, zh, fr, ja, ru, vi } as const

for (const [name, locale] of Object.entries(locales)) {
  test(`${name} has Kyren translations`, () => {
    assert.ok(Object.hasOwn(locale, 'translation'))
    for (const key of requiredKeys) {
      assert.ok(Object.hasOwn(locale.translation, key), `${name}: ${key}`)
    }
  })
}
