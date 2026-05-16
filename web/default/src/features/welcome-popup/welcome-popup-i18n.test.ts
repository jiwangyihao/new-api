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

const LOCALES = ['en', 'zh', 'fr', 'ja', 'ru', 'vi'] as const
const WELCOME_POPUP_KEYS = [
  'Welcome Popup',
  'Configure login welcome announcement popup',
  'This popup appears after users enter the authenticated system area.',
  'This content is returned only to authenticated users.',
  'Welcome announcement content',
  'Markdown is supported. Raw HTML is not supported.',
  'Display frequency',
  'Show once per content update',
  'Show once per day',
  'Show every system session',
  'Save welcome popup',
  'Welcome popup content must be at most 2000 characters.',
  'Welcome announcement',
  'Close welcome popup',
  'I understand',
] as const
const DEFAULT_WELCOME_CONTENT = '欢迎使用赔钱GPT！'

function readSource(path: string): string {
  return readFileSync(path, 'utf8')
}

describe('welcome popup i18n smoke', () => {
  test('static keys include dynamic welcome popup labels', () => {
    const source = readSource('src/i18n/static-keys.ts')

    for (const key of WELCOME_POPUP_KEYS) {
      assert.match(source, new RegExp(JSON.stringify(key).slice(1, -1)))
    }
    assert.doesNotMatch(source, new RegExp(DEFAULT_WELCOME_CONTENT))
  })

  test('all locales include welcome popup UI labels', () => {
    for (const locale of LOCALES) {
      const localeJson = JSON.parse(
        readSource(`src/i18n/locales/${locale}.json`)
      ) as { translation: Record<string, string> }

      for (const key of WELCOME_POPUP_KEYS) {
        assert.equal(
          typeof localeJson.translation[key],
          'string',
          `${locale} missing ${key}`
        )
        assert.notEqual(localeJson.translation[key], '', `${locale} empty ${key}`)
      }
      assert.equal(localeJson.translation[DEFAULT_WELCOME_CONTENT], undefined)
    }
  })
})
