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
  buildClosedState,
  createWelcomePopupStorageKey,
  getLocalDateKey,
  hashWelcomePopupContent,
  shouldShowWelcomePopup,
} from './welcome-popup-state'

const today = new Date('2026-05-16T10:00:00')
const tomorrow = new Date('2026-05-17T10:00:00')

describe('welcome popup state', () => {
  test('does not show when disabled, empty, or missing user', () => {
    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: false,
        content: 'x',
        frequency: 'once_per_version',
        closedState: null,
        shownThisSession: false,
        now: today,
      }),
      false
    )
    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content: ' ',
        frequency: 'once_per_version',
        closedState: null,
        shownThisSession: false,
        now: today,
      }),
      false
    )
    assert.equal(
      shouldShowWelcomePopup({
        userId: null,
        enabled: true,
        content: 'x',
        frequency: 'once_per_version',
        closedState: null,
        shownThisSession: false,
        now: today,
      }),
      false
    )
  })

  test('isolates storage by user id', () => {
    assert.equal(createWelcomePopupStorageKey(1), 'welcome-popup-state:v1:1')
    assert.equal(createWelcomePopupStorageKey(2), 'welcome-popup-state:v1:2')
  })

  test('once_per_version shows first time and again when content changes', () => {
    const content = 'hello'
    const closedState = buildClosedState({ content, now: today })

    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content,
        frequency: 'once_per_version',
        closedState: null,
        shownThisSession: true,
        now: today,
      }),
      true
    )
    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content,
        frequency: 'once_per_version',
        closedState,
        shownThisSession: false,
        now: today,
      }),
      false
    )
    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content: 'changed',
        frequency: 'once_per_version',
        closedState,
        shownThisSession: true,
        now: today,
      }),
      true
    )
  })

  test('once_per_day respects local date but new content wins', () => {
    const closedState = buildClosedState({ content: 'hello', now: today })

    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content: 'hello',
        frequency: 'once_per_day',
        closedState,
        shownThisSession: false,
        now: today,
      }),
      false
    )
    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content: 'hello',
        frequency: 'once_per_day',
        closedState,
        shownThisSession: false,
        now: tomorrow,
      }),
      true
    )
    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content: 'changed',
        frequency: 'once_per_day',
        closedState,
        shownThisSession: true,
        now: today,
      }),
      true
    )
  })

  test('every_session shows only once per mounted session', () => {
    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content: 'x',
        frequency: 'every_session',
        closedState: null,
        shownThisSession: false,
        now: today,
      }),
      true
    )
    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content: 'changed',
        frequency: 'every_session',
        closedState: null,
        shownThisSession: true,
        now: today,
      }),
      false
    )
  })

  test('different user ids can compare different closed states', () => {
    const userOneClosedState = buildClosedState({
      content: 'hello',
      now: today,
    })
    const userTwoClosedState = null

    assert.equal(
      shouldShowWelcomePopup({
        userId: 1,
        enabled: true,
        content: 'hello',
        frequency: 'once_per_version',
        closedState: userOneClosedState,
        shownThisSession: false,
        now: today,
      }),
      false
    )
    assert.equal(
      shouldShowWelcomePopup({
        userId: 2,
        enabled: true,
        content: 'hello',
        frequency: 'once_per_version',
        closedState: userTwoClosedState,
        shownThisSession: false,
        now: today,
      }),
      true
    )
  })

  test('hash and date are deterministic', () => {
    assert.equal(
      hashWelcomePopupContent(' hello '),
      hashWelcomePopupContent('hello')
    )
    assert.equal(getLocalDateKey(new Date('2026-05-16T23:59:00')), '2026-05-16')
  })
})
