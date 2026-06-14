import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { test } from 'node:test'

import { DEFAULT_ENDPOINT } from '@/features/system-settings/models/constants'
import { buildModelDirectoryColumns } from './columns'
import { sanitizePricingSearchForRole } from './search'

function columnIdentityText(
  columns: Array<{ id?: string; accessorKey?: string }>
): string {
  return columns.map((column) => column.id || column.accessorKey || '').join(',')
}

test('public model directory columns do not include pricing fields', () => {
  const columns = buildModelDirectoryColumns({ isAdmin: false })

  assert.doesNotMatch(columnIdentityText(columns), /price|ratio|billing/i)
})

test('admin model directory columns keep pricing fields', () => {
  const columns = buildModelDirectoryColumns({ isAdmin: true })

  assert.match(columnIdentityText(columns), /price|ratio|billing/i)
})

test('public pricing route strips cost search params', () => {
  const search = sanitizePricingSearchForRole(
    { sort: 'price-low', quotaType: 1, tokenUnit: 'M', rechargePrice: 10 },
    false
  )

  assert.deepEqual(search, {})
})

test('admin pricing route preserves cost search params', () => {
  const search = sanitizePricingSearchForRole(
    { sort: 'price-low', quotaType: 1, tokenUnit: 'M', rechargePrice: 10 },
    true
  )

  assert.equal(search.sort, 'price-low')
  assert.equal(search.quotaType, 1)
  assert.equal(search.rechargePrice, 10)
})

test('default ratio sync endpoint uses ratio_config', () => {
  assert.equal(DEFAULT_ENDPOINT, '/api/ratio_config')
})

const TOKEN_DEPRICING_I18N_KEYS = [
  'Subscription tokens remaining',
  'Tokens used this cycle',
  'Tokens used in the last 24 hours',
  'Monitor subscription credits and request volume',
  'Low token balance',
  'Tokens depleted',
  'Subscription required',
  'Total Tokens Used',
  'Tokens used in selected range',
  'Token Usage Distribution',
  'Deducted Tokens',
  'Model Directory',
  'Browse available models and capabilities',
] as const

const LOCALES = ['en', 'zh', 'fr', 'ja', 'ru', 'vi'] as const

function readProjectFile(relativePath: string): string {
  return readFileSync(fileURLToPath(new URL(`../../${relativePath}`, import.meta.url)), 'utf8')
}

test('token depricing i18n keys are registered in static keys', () => {
  const staticKeysSource = readProjectFile('i18n/static-keys.ts')

  for (const key of TOKEN_DEPRICING_I18N_KEYS) {
    assert.match(staticKeysSource, new RegExp(`['\"]${key}['\"]`))
  }
})

test('token depricing i18n keys are translated for every supported locale', () => {
  const english = JSON.parse(readProjectFile('i18n/locales/en.json')).translation as Record<string, string>

  for (const locale of LOCALES) {
    const translations = JSON.parse(readProjectFile(`i18n/locales/${locale}.json`)).translation as Record<string, string>
    for (const key of TOKEN_DEPRICING_I18N_KEYS) {
      assert.equal(typeof translations[key], 'string', `${locale} missing ${key}`)
      assert.notEqual(translations[key].trim(), '', `${locale} empty ${key}`)
      if (locale !== 'en') {
        assert.notEqual(translations[key], english[key], `${locale} untranslated ${key}`)
      }
    }
  }
})
