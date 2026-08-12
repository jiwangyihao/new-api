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
import type {
  CodexProMode,
  CodexProUnavailableReason,
} from '@/features/subscriptions/types'
import {
  CODEX_PRO_HELP_GUIDANCE_ITEMS,
  CODEX_PRO_MODE_OPTIONS,
  CODEX_PRO_MODE_TITLE_KEY,
  canUseCodexProModeControl,
  getCodexProModeFailureRollback,
  getCodexProUnavailableMessageKey,
} from './codex-pro-mode-control'

describe('Codex Pro mode control contract', () => {
  test('offers a segmented three-state switch contract', () => {
    assert.equal(CODEX_PRO_MODE_TITLE_KEY, 'Codex Pro')
    assert.deepEqual(
      CODEX_PRO_MODE_OPTIONS.map((option) => option.value),
      ['all', 'flexible', 'off'] satisfies CodexProMode[]
    )
    assert.deepEqual(
      CODEX_PRO_MODE_OPTIONS.map((option) => option.labelKey),
      ['All', 'Flexible', 'Off']
    )
    assert.deepEqual(
      CODEX_PRO_MODE_OPTIONS.map((option) => option.descriptionKey),
      [
        'All eligible GPT-family Responses requests try Codex Pro without requiring the intent header.',
        'Only requests with X-NewAPI-Codex-Pro-Intent: codex-pro try Codex Pro in flexible mode.',
        'Codex Pro is disabled; eligible requests stay on the normal group.',
      ]
    )
  })

  test('documents downstream harness guidance without treating internal headers as request guidance', () => {
    const guidance = CODEX_PRO_HELP_GUIDANCE_ITEMS.join('\n')

    assert.match(guidance, /X-NewAPI-Codex-Pro-Intent: codex-pro/)
    assert.match(guidance, /newapi_billing/)
    assert.match(guidance, /X-NewAPI-Pro-Served/)
    assert.match(guidance, /Do not rely on X-NewAPI-Pro-Served/)
    assert.match(guidance, /do not send internal X-NewAPI-Pro-Request/)
    assert.doesNotMatch(
      guidance,
      /(?:^|\n|\. )send internal X-NewAPI-Pro-Request/
    )
  })

  test('keeps eligible off mode selectable so users can switch back', () => {
    assert.equal(
      canUseCodexProModeControl({
        codex_pro_mode: 'off',
        codex_pro_eligible: true,
        codex_pro_unavailable_reason: '',
      }),
      true
    )
  })

  test('hides mode control when global Codex Pro features are hidden', () => {
    assert.equal(
      canUseCodexProModeControl({
        codex_pro_mode: 'all',
        codex_pro_eligible: true,
        codex_pro_unavailable_reason: '',
        codex_pro_features_hidden: true,
      }),
      false
    )
  })

  test('maps ineligible reasons to action-oriented copy instead of raw enums', () => {
    const cases: Array<[CodexProUnavailableReason, string]> = [
      [
        'wallet_only',
        'Your current billing preference will not create a subscription billing session.',
      ],
      ['trial_subscription', 'Trial subscriptions do not support Codex Pro.'],
      [
        'reward_subscription',
        'Invitation reward subscriptions do not support Codex Pro.',
      ],
      [
        'no_paid_subscription',
        'Please purchase an eligible paid subscription first.',
      ],
    ]

    for (const [reason, expectedKey] of cases) {
      assert.equal(getCodexProUnavailableMessageKey(reason), expectedKey)
      assert.notEqual(getCodexProUnavailableMessageKey(reason), reason)
    }

    assert.equal(
      getCodexProUnavailableMessageKey('unexpected'),
      'Please purchase an eligible paid subscription first.'
    )
  })

  test('rolls optimistic mode changes back and exposes a translated failure message', () => {
    assert.deepEqual(
      getCodexProModeFailureRollback({
        previousMode: 'flexible',
        requestedMode: 'all',
      }),
      {
        mode: 'flexible',
        messageKey: 'Request failed',
      }
    )
  })
})
