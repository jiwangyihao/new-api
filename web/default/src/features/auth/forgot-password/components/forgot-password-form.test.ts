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

describe('forgot password form submission', () => {
  test('uses form submit semantics for the reset email action', () => {
    const source = readFileSync(
      'src/features/auth/forgot-password/components/forgot-password-form.tsx',
      'utf8'
    )
    const button = source.match(
      /<Button\b[\s\S]*?Send reset email[\s\S]*?<\/Button>/
    )

    assert.ok(button, 'reset email action should render a Button')
    assert.match(
      button[0],
      /type=['"]submit['"]/,
      'reset email action must submit the form instead of inheriting Base UI type=button'
    )
    assert.equal(
      button[0].includes('onClick='),
      false,
      'reset email action should use the form onSubmit handler'
    )
  })
})
