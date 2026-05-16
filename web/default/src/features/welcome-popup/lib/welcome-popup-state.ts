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
import type { WelcomePopupClosedState, WelcomePopupFrequency } from '../types'

type UserId = number | string | null | undefined

export type ShouldShowWelcomePopupInput = {
  userId: UserId
  enabled: boolean
  content: string
  frequency: WelcomePopupFrequency
  closedState: WelcomePopupClosedState | null
  shownThisSession: boolean
  now: Date
}

export type BuildClosedStateInput = {
  content: string
  now: Date
}

export function hashWelcomePopupContent(content: string): string {
  const normalized = content.trim()
  let hash = 0

  for (let i = 0; i < normalized.length; i += 1) {
    hash = (hash << 5) - hash + normalized.charCodeAt(i)
    hash |= 0
  }

  return hash.toString(36)
}

export function createWelcomePopupStorageKey(userId: number | string): string {
  return `welcome-popup-state:v1:${userId}`
}

export function getLocalDateKey(now: Date): string {
  const year = now.getFullYear()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')

  return `${year}-${month}-${day}`
}

export function buildClosedState(
  input: BuildClosedStateInput
): WelcomePopupClosedState {
  return {
    lastContentHash: hashWelcomePopupContent(input.content),
    lastClosedDate: getLocalDateKey(input.now),
  }
}

export function shouldShowWelcomePopup(
  input: ShouldShowWelcomePopupInput
): boolean {
  if (!input.userId) return false
  if (!input.enabled) return false
  if (!input.content.trim()) return false

  if (input.frequency === 'every_session') return !input.shownThisSession

  const currentHash = hashWelcomePopupContent(input.content)
  if (!input.closedState) return true
  if (input.closedState.lastContentHash !== currentHash) return true
  if (input.frequency === 'once_per_day') {
    return input.closedState.lastClosedDate !== getLocalDateKey(input.now)
  }

  return false
}
