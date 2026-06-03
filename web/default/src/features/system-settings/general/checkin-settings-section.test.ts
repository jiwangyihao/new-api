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
