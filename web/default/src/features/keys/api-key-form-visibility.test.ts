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
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import { formatApiKeyTokenCount } from './constants'
import {
  API_KEY_FORM_DEFAULT_VALUES,
  apiKeyFormSchema,
  transformApiKeyToFormDefaults,
  transformFormDataToPayload,
} from './lib/api-key-form'

function readKeysSource(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

const drawerSource = readKeysSource('./components/api-keys-mutate-drawer.tsx')
const formSource = readKeysSource('./lib/api-key-form.ts')
const columnsSource = readKeysSource('./components/api-keys-columns.tsx')
const tableSource = readKeysSource('./components/api-keys-table.tsx')
const apiSource = readKeysSource('./api.ts')
const rowActionsSource = readKeysSource(
  './components/data-table-row-actions.tsx'
)
const constantsSource = readKeysSource('./constants.ts')

const requiredI18nKeys = [
  'API Key Token Limit',
  'No token limit for this API key',
  'Token limit',
  'Enter token limit',
  'Limits only this API key. Requests still consume subscription tokens.',
  'Exhausted',
  'Token Limit Reached',
  'Reset token usage',
  'API key token usage reset',
  'This API key uses the new token limit model. Historical quota limits were not migrated.',
  'Name is required',
  'Token limit must be greater than 0',
  'API Key Codex Pro Mode',
  'Inherit user setting',
  'Use the user-level Codex Pro setting for this API key.',
  'Override Codex Pro behavior for this API key without bypassing subscription eligibility.',
]

type LocaleFile = { translation?: Record<string, unknown> }

describe('api key user-facing configuration form', () => {
  test('does not expose group selection in create or edit drawer', () => {
    assert.doesNotMatch(drawerSource, /ApiKeyGroupCombobox/)
    assert.doesNotMatch(
      drawerSource,
      /name=['"]group['"]|name=\{['"]group['"]\}/
    )
    assert.doesNotMatch(drawerSource, /t\(['"]Group['"]\)/)
    assert.doesNotMatch(drawerSource, /Select a group/)
    assert.doesNotMatch(drawerSource, /Cross-group retry/)
  })

  test('does not expose group details in API key table columns', () => {
    assert.doesNotMatch(
      columnsSource,
      /accessorKey:\s*['"]group['"]|id:\s*['"]group['"]|label:\s*t\(['"]Group['"]\)/
    )
    assert.doesNotMatch(columnsSource, /GroupBadge/)
    assert.doesNotMatch(columnsSource, /Cross-group/)
    assert.doesNotMatch(columnsSource, /getUserGroups/)
  })

  test('does not fetch user groups only to render API key form fields', () => {
    assert.doesNotMatch(drawerSource, /getUserGroups/)
    assert.doesNotMatch(drawerSource, /user-groups/)
  })

  test('does not depend on user-edited group fields in API key payloads', () => {
    assert.doesNotMatch(formSource, /data\.group/)
    assert.doesNotMatch(formSource, /data\.cross_group_retry/)
    assert.doesNotMatch(formSource, /group:\s*/)
    assert.doesNotMatch(formSource, /cross_group_retry:\s*/)
    assert.doesNotMatch(formSource, /apiKey\.group/)
    assert.doesNotMatch(formSource, /apiKey\.cross_group_retry/)
  })

  test('does not expose legacy quota currency fields for API key limits', () => {
    assert.doesNotMatch(formSource, /remain_quota_dollars/)
    assert.doesNotMatch(formSource, /parseQuotaFromDollars|quotaUnitsToDollars/)
    assert.doesNotMatch(
      drawerSource,
      /getCurrencyDisplay|getCurrencyLabel|Quota \(\{\{currency\}\}\)|WalletCards/
    )
    assert.doesNotMatch(columnsSource, /formatQuota\(/)
    assert.doesNotMatch(tableSource, /formatQuota\(/)
  })

  test('keeps legacy exhausted distinct from derived token limit reached status', () => {
    assert.match(constantsSource, /label:\s*'Exhausted'/)
    assert.match(constantsSource, /label:\s*'Token Limit Reached'/)
    assert.match(columnsSource, /getApiKeyStatusConfig\(row\.original\)/)
    assert.match(tableSource, /getApiKeyStatusConfig\(apiKey\)/)
  })

  test('defaults to no token limit and validates enabled token limits', () => {
    assert.equal(API_KEY_FORM_DEFAULT_VALUES.token_limit_enabled, false)
    assert.equal(
      apiKeyFormSchema.safeParse({
        ...API_KEY_FORM_DEFAULT_VALUES,
        name: '',
      }).success,
      false
    )
    const emptyName = apiKeyFormSchema.safeParse({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: '',
    })
    assert.equal(emptyName.success, false)
    if (!emptyName.success) {
      assert.equal(emptyName.error.issues[0]?.message, 'Name is required')
    }
    const zeroLimit = apiKeyFormSchema.safeParse({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: 'limited-key',
      token_limit_enabled: true,
      token_limit: 0,
    })
    assert.equal(zeroLimit.success, false)
    if (!zeroLimit.success) {
      assert.equal(
        zeroLimit.error.issues[0]?.message,
        'Token limit must be greater than 0'
      )
    }
    const decimalLimit = apiKeyFormSchema.safeParse({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: 'limited-key',
      token_limit_enabled: true,
      token_limit: 1.5,
    })
    assert.equal(decimalLimit.success, false)
    if (!decimalLimit.success) {
      assert.equal(
        decimalLimit.error.issues[0]?.message,
        'Token limit must be greater than 0'
      )
    }
    const nullLimit = apiKeyFormSchema.safeParse({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: 'limited-key',
      token_limit_enabled: true,
      token_limit: null,
    })
    assert.equal(nullLimit.success, false)
    if (!nullLimit.success) {
      assert.equal(
        nullLimit.error.issues[0]?.message,
        'Token limit must be greater than 0'
      )
    }
    const nanLimit = apiKeyFormSchema.safeParse({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: 'limited-key',
      token_limit_enabled: true,
      token_limit: Number.NaN,
    })
    assert.equal(nanLimit.success, false)
    if (!nanLimit.success) {
      assert.equal(
        nanLimit.error.issues[0]?.message,
        'Token limit must be greater than 0'
      )
    }
    assert.equal(
      apiKeyFormSchema.safeParse({
        ...API_KEY_FORM_DEFAULT_VALUES,
        name: 'limited-key',
        token_limit_enabled: true,
        token_limit: 0,
      }).success,
      false
    )
    assert.equal(
      apiKeyFormSchema.safeParse({
        ...API_KEY_FORM_DEFAULT_VALUES,
        name: 'limited-key',
        token_limit_enabled: true,
        token_limit: undefined,
      }).success,
      false
    )
    assert.equal(
      apiKeyFormSchema.safeParse({
        ...API_KEY_FORM_DEFAULT_VALUES,
        name: 'limited-key',
        token_limit_enabled: true,
        token_limit: 1.5,
      }).success,
      false
    )
    assert.equal(
      apiKeyFormSchema.safeParse({
        ...API_KEY_FORM_DEFAULT_VALUES,
        name: 'limited-key',
        token_limit_enabled: true,
        token_limit: 1000,
      }).success,
      true
    )
  })

  test('defaults Codex Pro mode to inherit, submits it, and preserves API values', () => {
    assert.equal(API_KEY_FORM_DEFAULT_VALUES.codex_pro_mode, 'inherit')

    const inheritPayload = transformFormDataToPayload({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: 'inherits-user-mode',
    })
    assert.equal(inheritPayload.codex_pro_mode, 'inherit')

    const allPayload = transformFormDataToPayload({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: 'forces-all-mode',
      codex_pro_mode: 'all',
    })
    assert.equal(allPayload.codex_pro_mode, 'all')

    const defaults = transformApiKeyToFormDefaults({
      id: 1,
      name: 'flex-key',
      key: 'sk-***',
      status: 1,
      remain_quota: 0,
      used_quota: 0,
      unlimited_quota: true,
      expired_time: -1,
      created_time: 1,
      accessed_time: 1,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
      token_limit_enabled: false,
      token_limit: 0,
      token_used: 0,
      token_remaining: 0,
      token_unlimited: true,
      codex_pro_mode: 'flexible',
    })
    assert.equal(defaults.codex_pro_mode, 'flexible')
  })

  test('exposes API key Codex Pro mode select and non-inherit table badge', () => {
    assert.match(
      drawerSource,
      /name=['"]codex_pro_mode['"]|name=\{['"]codex_pro_mode['"]\}/
    )
    assert.match(drawerSource, /API_KEY_CODEX_PRO_MODE_OPTIONS/)
    assert.match(formSource, /Inherit user setting/)
    assert.match(columnsSource, /Codex Pro/)
    assert.match(columnsSource, /codex_pro_mode/)
    assert.match(columnsSource, /inherit/)
  })

  test('does not migrate historical quota fields into token limit defaults', () => {
    const defaults = transformApiKeyToFormDefaults({
      id: 1,
      name: 'legacy',
      key: 'sk-***',
      status: 1,
      remain_quota: 999999,
      used_quota: 888888,
      unlimited_quota: false,
      expired_time: -1,
      created_time: 1,
      accessed_time: 1,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
      token_limit_enabled: false,
      token_limit: 0,
      token_used: 0,
      token_remaining: 0,
      token_unlimited: true,
      codex_pro_mode: 'inherit',
    })
    assert.equal(defaults.token_limit_enabled, false)
    assert.equal(defaults.token_limit, undefined)
  })

  test('submits token limit fields without legacy quota limit fields', () => {
    const limitedPayload = transformFormDataToPayload({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: 'limited',
      token_limit_enabled: true,
      token_limit: 1000,
    })
    assert.equal(limitedPayload.token_limit_enabled, true)
    assert.equal(limitedPayload.token_limit, 1000)
    assert.equal('remain_quota' in limitedPayload, false)
    assert.equal('unlimited_quota' in limitedPayload, false)
    assert.equal('remain_quota_dollars' in limitedPayload, false)

    const unlimitedPayload = transformFormDataToPayload({
      ...API_KEY_FORM_DEFAULT_VALUES,
      name: 'unlimited',
      token_limit_enabled: false,
      token_limit: undefined,
    })
    assert.equal(unlimitedPayload.token_limit_enabled, false)
    assert.equal(unlimitedPayload.token_limit, 0)
    assert.equal('remain_quota' in unlimitedPayload, false)
    assert.equal('unlimited_quota' in unlimitedPayload, false)
    assert.equal('remain_quota_dollars' in unlimitedPayload, false)
  })

  test('formats small API key token counts with caller-provided unit', () => {
    assert.equal(formatApiKeyTokenCount(0, 'tokens'), '0 tokens')
    assert.equal(formatApiKeyTokenCount(42, 'tokens'), '42 tokens')
    assert.equal(formatApiKeyTokenCount(42, '令牌'), '42 令牌')
    assert.doesNotMatch(constantsSource, /0 tokens|\$\{formatted\} tokens/)
  })

  test('exposes reset token usage action and API client', () => {
    assert.match(apiSource, /resetApiKeyTokenUsage/)
    assert.match(apiSource, /\/api\/token\/\$\{id\}\/reset-token-usage/)
    assert.match(rowActionsSource, /Reset token usage/)
    assert.match(rowActionsSource, /resetApiKeyTokenUsage\(apiKey\.id\)/)
    assert.match(rowActionsSource, /triggerRefresh\(\)/)
    assert.match(rowActionsSource, /toast\.success/)
    assert.match(rowActionsSource, /toast\.error/)
    assert.match(
      rowActionsSource,
      /isResettingTokenUsage|resettingTokenUsageId/
    )
  })

  for (const locale of ['en', 'zh', 'fr', 'ja', 'ru', 'vi']) {
    test(`api key token limit i18n keys exist in ${locale}`, () => {
      const source = readKeysSource(`../../i18n/locales/${locale}.json`)
      const localeFile = JSON.parse(source) as LocaleFile
      const translations = localeFile.translation ?? {}
      assert.notEqual(Object.keys(translations).length, 0)
      for (const key of requiredI18nKeys) {
        const value = translations[key]
        assert.equal(typeof value, 'string', `${locale} missing ${key}`)
        const text = value as string
        assert.notEqual(text.trim(), '', `${locale} has empty ${key}`)
      }
    })
  }
})
