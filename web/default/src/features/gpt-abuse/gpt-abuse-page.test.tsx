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
import { readFileSync } from 'node:fs'
import {
  gptAbuseQueryKeys,
  invalidateGPTAbuseUserDetailQueries,
} from './index'

describe('gpt abuse page contract', () => {
  test('reset success mutation invalidates user list and current detail queries', async () => {
    const invalidated: unknown[] = []
    const queryClient = {
      invalidateQueries: (options: unknown) => {
        invalidated.push(options)
        return Promise.resolve()
      },
    }

    await invalidateGPTAbuseUserDetailQueries(queryClient, 1958)

    assert.deepEqual(invalidated, [
      { queryKey: gptAbuseQueryKeys.users() },
      { queryKey: gptAbuseQueryKeys.logs(1958) },
      { queryKey: gptAbuseQueryKeys.repeatBlocks(1958) },
    ])
  })

  test('renders raw warning detail collapsed and truncated by default', () => {
    const source = readFileSync(new URL('./components/gpt-abuse-log-drawer.tsx', import.meta.url), 'utf8')

    assert.match(source, /formatRawWarningSummary\(rawWarning\)/)
    assert.match(source, /expanded \? \(/)
    assert.match(source, /formatRawWarning\(rawWarning\)/)
  })

  test('reset and clear dialogs keep failure paths open by closing only on success', () => {
    const pageSource = readFileSync(new URL('./index.tsx', import.meta.url), 'utf8')
    const resetDialogSource = readFileSync(
      new URL('./components/gpt-abuse-reset-dialog.tsx', import.meta.url),
      'utf8'
    )

    assert.match(pageSource, /if \(!response\.success\) \{[\s\S]*?return[\s\S]*?\}/)
    assert.match(pageSource, /setResetUser\(null\)/)
    assert.match(pageSource, /setClearUser\(null\)/)
    assert.match(resetDialogSource, /clear_suspension/)
  })
})
