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

function readClassicSubscriptionCardSource() {
  return readFileSync('src/components/topup/SubscriptionPlansCard.jsx', 'utf8')
}

function readClassicUserSubscriptionsModalSource() {
  return readFileSync(
    'src/components/table/users/modals/UserSubscriptionsModal.jsx',
    'utf8'
  )
}

describe('classic subscription display labels', () => {
  test('wallet subscription list consumes summary plan titles for hidden trials', () => {
    const source = readClassicSubscriptionCardSource()

    assert.match(source, /record\?\.plan\?\.title/)
    assert.match(source, /record\?\.plan_title/)
    assert.match(source, /subscription\.grant_reason === 'trial_code'/)
    assert.match(source, /return title;/)
    assert.doesNotMatch(source, /planTitle\s*\?\s*`\$\{planTitle\} · \$\{t\('订阅'\)\}/)
  })

  test('admin user subscription dialog consumes summary plan titles', () => {
    const source = readClassicUserSubscriptionsModalSource()

    assert.match(source, /function getPlanDisplayTitle/)
    assert.match(source, /record\?\.plan\?\.title/)
    assert.match(source, /record\?\.plan_title/)
    assert.match(source, /getPlanDisplayTitle\(record, planTitleMap\)/)
  })
})
