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
import { buildRegisterRequestBody } from './api'

describe('register API payload', () => {
  test('maps affiliate code to backend aff_code and leaves Turnstile for query only', () => {
    const body = buildRegisterRequestBody({
      username: 'alice',
      password: 'password123',
      email: 'alice@example.com',
      verification_code: '123456',
      aff: 'INVITE',
      trial_code: 'TRIAL-24H',
      turnstile: 'turnstile-token',
    })

    assert.deepEqual(body, {
      username: 'alice',
      password: 'password123',
      email: 'alice@example.com',
      verification_code: '123456',
      aff_code: 'INVITE',
      trial_code: 'TRIAL-24H',
    })
  })

  test('keeps prefilled invite code in trial code field when it matches affiliate code', () => {
    const body = buildRegisterRequestBody({
      username: 'bob',
      password: 'password123',
      aff: 'INVITE42',
      trial_code: ' invite42 ',
    })

    assert.deepEqual(body, {
      username: 'bob',
      password: 'password123',
      aff_code: 'INVITE42',
      trial_code: 'invite42',
    })
  })

  test('keeps trial code as the invite candidate when no invite link was saved', () => {
    const body = buildRegisterRequestBody({
      username: 'carol',
      password: 'password123',
      trial_code: ' TrialOrInvite ',
    })

    assert.deepEqual(body, {
      username: 'carol',
      password: 'password123',
      trial_code: 'TrialOrInvite',
    })
  })
})
