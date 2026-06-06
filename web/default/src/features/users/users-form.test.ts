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
import { test } from 'node:test'
import {
  transformFormDataToPayload,
  transformUserToFormDefaults,
} from './lib/user-form'

test('includes invitation_reward_mode when updating an admin-managed user', () => {
  const payload = transformFormDataToPayload(
    {
      username: 'agent',
      display_name: 'Agent',
      password: '',
      role: 1,
      quota_cny: '0.00',
      remark: '',
      invitation_reward_mode: 'commission',
    },
    100
  )

  assert.deepEqual(
    {
      id: payload.id,
      username: payload.username,
      invitation_reward_mode: payload.invitation_reward_mode,
    },
    {
      id: 100,
      username: 'agent',
      invitation_reward_mode: 'commission',
    }
  )
})

test('defaults missing invitation_reward_mode to subscription', () => {
  const defaults = transformUserToFormDefaults({
    id: 1,
    username: 'plain',
    display_name: 'Plain',
    quota: 0,
    status: 1,
    role: 1,
    used_quota: 0,
    request_count: 0,
  })

  assert.equal(defaults.invitation_reward_mode, 'subscription')
})

test('user drawer and columns expose invitation reward mode controls', () => {
  const drawer = readFileSync(
    'src/features/users/components/users-mutate-drawer.tsx',
    'utf8'
  )
  const columns = readFileSync(
    'src/features/users/components/users-columns.tsx',
    'utf8'
  )

  assert.match(drawer, /invitation_reward_mode/)
  assert.match(drawer, /Reward package/)
  assert.match(drawer, /Commission/)
  assert.match(
    drawer,
    /Commission is only available for invited special users enabled by administrators\./
  )
  assert.match(columns, /invitation_reward_mode/)
  assert.match(columns, /subscription/)
  assert.match(columns, /commission/)
})
