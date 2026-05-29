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
  formatAdminOpsBytes,
  formatAdminOpsCount,
  formatAdminOpsDuration,
  formatAdminOpsPercent,
  formatAdminOpsRate,
} from './format'

describe('admin ops format helpers', () => {
  test('formats counts compactly', () => {
    assert.equal(formatAdminOpsCount(999), '999')
    assert.equal(formatAdminOpsCount(1200), '1.2K')
  })

  test('formats percentage from ratio', () => {
    assert.equal(formatAdminOpsPercent(0.1234), '12.3%')
  })

  test('formats seconds as readable duration', () => {
    assert.equal(formatAdminOpsDuration(59), '59s')
    assert.equal(formatAdminOpsDuration(61), '1m 1s')
    assert.equal(formatAdminOpsDuration(3661), '1h 1m')
  })

  test('formats rate with unit', () => {
    assert.equal(formatAdminOpsRate(12.345, 'rpm'), '12.3 rpm')
  })

  test('formats bytes compactly', () => {
    assert.equal(formatAdminOpsBytes(1023), '1023 B')
    assert.equal(formatAdminOpsBytes(1024), '1.0 KB')
    assert.equal(formatAdminOpsBytes(1536 * 1024), '1.5 MB')
  })
})
