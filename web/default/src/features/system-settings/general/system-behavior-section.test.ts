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

const behaviorSource = readFileSync(
  new URL('./system-behavior-section.tsx', import.meta.url),
  'utf8'
)
const operationsSource = readFileSync(
  new URL('../operations/index.tsx', import.meta.url),
  'utf8'
)
const updateOptionSource = readFileSync(
  new URL('../hooks/use-update-option.ts', import.meta.url),
  'utf8'
)

describe('system behavior Codex Pro feature visibility setting', () => {
  test('exposes a persisted global Codex Pro hide switch in system behavior settings', () => {
    assert.match(operationsSource, /CodexProFeaturesHidden:\s*false/)
    assert.match(behaviorSource, /CodexProFeaturesHidden:\s*z\.boolean\(\)/)
    assert.match(behaviorSource, /name='CodexProFeaturesHidden'/)
    assert.match(behaviorSource, /Hide Codex Pro features/)
    assert.match(behaviorSource, /Hide subscription, API key, and help entries for Codex Pro/)
  })

  test('refreshes public status after the Codex Pro hide switch changes', () => {
    assert.match(updateOptionSource, /'CodexProFeaturesHidden'/)
  })
})
