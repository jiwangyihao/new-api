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
import { test } from 'node:test'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zh from '@/i18n/locales/zh.json'

const requiredAccountBalanceKeys = [
  'Account balance',
  'Top-up credit',
  'Credited balance',
  'Credited balance must be at least ¥0.01',
  'Amount is in CNY',
  'New User Account Balance Reward (CNY)',
  'Inviter Account Balance Reward (CNY)',
  'Invitee Account Balance Reward (CNY)',
  'Initial CNY account balance credited to new users',
  'CNY account balance credited to users who invite others',
  'CNY account balance credited to invited users',
  'Configure daily check-in account balance rewards',
  'Allow users to check in daily for random CNY account balance rewards',
  'Minimum check-in account balance reward (CNY)',
  'Maximum check-in account balance reward (CNY)',
  'Minimum CNY account balance credited for check-in',
  'Maximum CNY account balance credited for check-in',
  'Credited account balance (CNY)',
  'Saved to the server as CNY cents.',
  'Unit price for credited balance (CNY)',
  'Channel payment amount charged per CNY credited balance',
  'Channel payment amount charged per CNY credited balance (Epay)',
  'Minimum credited balance (CNY)',
  'Minimum credited account balance in CNY',
  'Smallest credited account balance in CNY (Epay)',
  'Account balance CNY options',
  'Credited account balance CNY options (JSON array)',
  'Account balance CNY discount',
  'Credited account balance CNY discount by amount',
] as const

const locales = { en, zh, fr, ja, ru, vi } as const

for (const [name, locale] of Object.entries(locales)) {
  test(`${name} has account balance translations`, () => {
    assert.ok(Object.hasOwn(locale, 'translation'))
    for (const key of requiredAccountBalanceKeys) {
      assert.ok(Object.hasOwn(locale.translation, key), `${name}: ${key}`)
      assert.notEqual(locale.translation[key], '', `${name}: ${key} is empty`)
    }
  })
}
