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
import type { WelcomePopupClosedState } from '../types'
import { createWelcomePopupStorageKey } from './welcome-popup-state'

export type WelcomePopupStorage = Pick<Storage, 'getItem' | 'setItem'>

const memoryClosedState = new Map<string, WelcomePopupClosedState>()

function getBrowserStorage(): WelcomePopupStorage | null {
  if (typeof window === 'undefined') return null

  try {
    return window.localStorage
  } catch {
    return null
  }
}

function parseWelcomePopupClosedState(
  raw: string | null
): WelcomePopupClosedState | null {
  if (!raw) return null

  try {
    const parsed = JSON.parse(raw) as Partial<WelcomePopupClosedState> | null
    if (
      parsed &&
      typeof parsed === 'object' &&
      typeof parsed.lastContentHash === 'string' &&
      typeof parsed.lastClosedDate === 'string'
    ) {
      return {
        lastContentHash: parsed.lastContentHash,
        lastClosedDate: parsed.lastClosedDate,
      }
    }
  } catch {
    return null
  }

  return null
}

export function readWelcomePopupClosedState(
  userId: number | string,
  storage: WelcomePopupStorage | null = getBrowserStorage()
): WelcomePopupClosedState | null {
  const key = createWelcomePopupStorageKey(userId)

  if (storage) {
    try {
      const parsed = parseWelcomePopupClosedState(storage.getItem(key))
      if (parsed) {
        memoryClosedState.set(key, parsed)
        return parsed
      }
    } catch {
      // Fall through to the current JS session fallback.
    }
  }

  return memoryClosedState.get(key) ?? null
}

export function writeWelcomePopupClosedState(
  userId: number | string,
  state: WelcomePopupClosedState,
  storage: WelcomePopupStorage | null = getBrowserStorage()
): void {
  const key = createWelcomePopupStorageKey(userId)
  memoryClosedState.set(key, state)
  if (!storage) return

  try {
    storage.setItem(key, JSON.stringify(state))
  } catch {
    // The module-level Map above is the current JS session fallback.
  }
}

export function clearWelcomePopupMemoryClosedState(): void {
  memoryClosedState.clear()
}
