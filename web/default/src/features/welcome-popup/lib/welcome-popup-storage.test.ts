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
import { beforeEach, describe, test } from 'node:test'
import { buildClosedState } from './welcome-popup-state'
import {
  clearWelcomePopupMemoryClosedState,
  readWelcomePopupClosedState,
  type WelcomePopupStorage,
  writeWelcomePopupClosedState,
} from './welcome-popup-storage'

function createMemoryStorage(initial?: Record<string, string>): WelcomePopupStorage {
  const values = new Map(Object.entries(initial ?? {}))

  return {
    getItem(key: string): string | null {
      return values.get(key) ?? null
    },
    setItem(key: string, value: string): void {
      values.set(key, value)
    },
  }
}

describe('welcome popup storage', () => {
  beforeEach(() => {
    clearWelcomePopupMemoryClosedState()
  })

  test('reads and writes closed state through storage', () => {
    const storage = createMemoryStorage()
    const state = buildClosedState({
      content: 'hello',
      now: new Date('2026-05-16T10:00:00'),
    })

    writeWelcomePopupClosedState(1, state, storage)

    assert.deepEqual(readWelcomePopupClosedState(1, storage), state)
  })

  test('prefers newer storage state over memory cache', () => {
    const storage = createMemoryStorage()
    const oldState = buildClosedState({
      content: 'old',
      now: new Date('2026-05-16T10:00:00'),
    })
    const newState = buildClosedState({
      content: 'new',
      now: new Date('2026-05-17T10:00:00'),
    })

    writeWelcomePopupClosedState(1, oldState, storage)
    storage.setItem('welcome-popup-state:v1:1', JSON.stringify(newState))

    assert.deepEqual(readWelcomePopupClosedState(1, storage), newState)
  })

  test('falls back to module memory when storage throws', () => {
    const state = buildClosedState({
      content: 'fallback content',
      now: new Date('2026-05-16T10:00:00'),
    })
    const throwingStorage = {
      getItem(): string | null {
        throw new Error('storage unavailable')
      },
      setItem(): void {
        throw new Error('storage unavailable')
      },
    }

    writeWelcomePopupClosedState(1, state, throwingStorage)

    assert.deepEqual(readWelcomePopupClosedState(1, throwingStorage), state)
  })

  test('ignores corrupted storage entries', () => {
    const storage = createMemoryStorage({
      'welcome-popup-state:v1:1': '{bad json',
      'welcome-popup-state:v1:2': JSON.stringify({ lastContentHash: 123 }),
    })

    assert.equal(readWelcomePopupClosedState(1, storage), null)
    assert.equal(readWelcomePopupClosedState(2, storage), null)
  })

  test('isolates fallback memory by user id', () => {
    const storage = createMemoryStorage()
    const userOneState = buildClosedState({
      content: 'one',
      now: new Date('2026-05-16T10:00:00'),
    })
    const userTwoState = buildClosedState({
      content: 'two',
      now: new Date('2026-05-16T10:00:00'),
    })

    writeWelcomePopupClosedState(1, userOneState, storage)
    writeWelcomePopupClosedState(2, userTwoState, storage)

    assert.deepEqual(readWelcomePopupClosedState(1, storage), userOneState)
    assert.deepEqual(readWelcomePopupClosedState(2, storage), userTwoState)
  })
})
