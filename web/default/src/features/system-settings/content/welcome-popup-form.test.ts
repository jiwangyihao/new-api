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
  buildWelcomePopupFormDefaults,
  collectWelcomePopupSettingUpdates,
  countUnicodeCharacters,
  welcomePopupFormSchema,
  type WelcomePopupOptionValues,
} from './welcome-popup-form'

const defaults: WelcomePopupOptionValues = {
  'console_setting.welcome_popup_content': '欢迎',
  'console_setting.welcome_popup_enabled': true,
  'console_setting.welcome_popup_frequency': 'once_per_version',
}

describe('welcome popup form', () => {
  test('builds defaults from flat option values', () => {
    assert.deepEqual(buildWelcomePopupFormDefaults(defaults), {
      content: '欢迎',
      frequency: 'once_per_version',
    })
  })

  test('collects only changed content and frequency updates', () => {
    const values = buildWelcomePopupFormDefaults(defaults)
    values.content = '新的欢迎公告'
    values.frequency = 'once_per_day'

    assert.deepEqual(collectWelcomePopupSettingUpdates(values, defaults), [
      { key: 'console_setting.welcome_popup_content', value: '新的欢迎公告' },
      { key: 'console_setting.welcome_popup_frequency', value: 'once_per_day' },
    ])
  })

  test('does not emit unchanged updates', () => {
    assert.deepEqual(
      collectWelcomePopupSettingUpdates(
        buildWelcomePopupFormDefaults(defaults),
        defaults
      ),
      []
    )
  })

  test('rejects invalid frequency', () => {
    assert.equal(
      welcomePopupFormSchema.safeParse({
        content: 'x',
        frequency: 'every_login',
      }).success,
      false
    )
  })

  test('counts Unicode code points for the content limit', () => {
    assert.equal(countUnicodeCharacters('😀'), 1)
    assert.equal(
      welcomePopupFormSchema.safeParse({
        content: '😀'.repeat(2000),
        frequency: 'once_per_version',
      }).success,
      true
    )
    assert.equal(
      welcomePopupFormSchema.safeParse({
        content: '😀'.repeat(2001),
        frequency: 'once_per_version',
      }).success,
      false
    )
  })
})
