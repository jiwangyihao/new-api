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
  buildCheckinSettingsOptionUpdates,
  checkinSettingsToFormDefaults,
} from './checkin-settings-section'

describe('CheckinSettingsSection account balance conversion helpers', () => {
  test('restores check-in reward cents as CNY defaults', () => {
    const defaults = checkinSettingsToFormDefaults({
      enabled: true,
      minQuota: 2000,
      maxQuota: 3990,
    })

    assert.equal(defaults.minQuotaCny, '20.00')
    assert.equal(defaults.maxQuotaCny, '39.90')
  })

  test('saves check-in reward CNY inputs as cents', () => {
    const defaults = checkinSettingsToFormDefaults({
      enabled: true,
      minQuota: 2000,
      maxQuota: 3990,
    })

    const updates = buildCheckinSettingsOptionUpdates(
      {
        enabled: true,
        minQuotaCny: '0.20',
        maxQuotaCny: '1.50',
      },
      defaults
    )

    assert.deepEqual(
      updates.filter((update) =>
        ['checkin_setting.min_quota', 'checkin_setting.max_quota'].includes(
          update.key
        )
      ),
      [
        { key: 'checkin_setting.min_quota', value: '20' },
        { key: 'checkin_setting.max_quota', value: '150' },
      ]
    )
  })
})
