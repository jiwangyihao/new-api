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
