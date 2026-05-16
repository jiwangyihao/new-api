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
import { MESSAGE_ROLES, MESSAGE_STATUS } from '../constants'
import {
  formatPlaygroundErrorMessage,
  updateAssistantMessageWithError,
} from './message-utils'
import type { Message } from '../types'

function createAssistantMessage(): Message {
  return {
    key: 'assistant-1',
    from: MESSAGE_ROLES.ASSISTANT,
    versions: [{ id: 'v1', content: 'partial response' }],
    status: MESSAGE_STATUS.STREAMING,
  }
}

describe('playground error messages', () => {
  test('maps system resource overloads to an actionable message', () => {
    assert.equal(
      formatPlaygroundErrorMessage(
        'system cpu overloaded (current: 99.1%, threshold: 90%)',
        'system_cpu_overloaded'
      ),
      'Server load is high. Please try again later. Administrators can adjust or disable Performance monitoring in System settings.'
    )
  })

  test('does not prepend the generic request error to resource overloads', () => {
    const updated = updateAssistantMessageWithError(
      [createAssistantMessage()],
      'system cpu overloaded (current: 99.1%, threshold: 90%)',
      'system_cpu_overloaded'
    )

    assert.equal(updated[0]?.versions[0]?.content, 'Server load is high. Please try again later. Administrators can adjust or disable Performance monitoring in System settings.')
    assert.equal(updated[0]?.errorCode, 'system_cpu_overloaded')
  })

  test('keeps generic request prefix for non-overload errors', () => {
    const updated = updateAssistantMessageWithError(
      [createAssistantMessage()],
      'upstream request failed',
      'upstream_error'
    )

    assert.equal(
      updated[0]?.versions[0]?.content,
      'Request error occurred: upstream request failed'
    )
  })
})
