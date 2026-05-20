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

import { formatLatencyMs, formatUsagePercent, formatUsageTokens } from './format'

test('formats latency without hiding zero', () => {
  assert.equal(formatLatencyMs(0), '0 ms')
  assert.equal(formatLatencyMs(999), '999 ms')
  assert.equal(formatLatencyMs(1500), '1.5 s')
})

test('formats percent with zero request semantics', () => {
  assert.equal(formatUsagePercent(0), '0%')
  assert.equal(formatUsagePercent(0.0156), '1.6%')
})

test('formats tokens preserving zero', () => {
  assert.equal(formatUsageTokens(0), '0')
  assert.equal(formatUsageTokens(1220000), '1.22M')
})
