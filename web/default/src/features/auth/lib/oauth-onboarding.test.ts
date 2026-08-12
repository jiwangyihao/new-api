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
  buildOAuthOnboardingUrl,
  handleOAuthOnboardingRequired,
  isOAuthOnboardingRequiredResponse,
  type OAuthOnboardingRequiredResponse,
} from './oauth-onboarding'

describe('OAuth onboarding response helpers', () => {
  test('detects onboarding_required responses with pending token', () => {
    const response: OAuthOnboardingRequiredResponse = {
      success: true,
      message: 'oauth_onboarding_required',
      data: {
        pending_token: 'pending-123',
        provider: 'github',
        login: 'octocat',
        email: 'octo@example.com',
      },
    }

    assert.equal(isOAuthOnboardingRequiredResponse(response), true)
  })

  test('rejects successful login payloads without onboarding token', () => {
    assert.equal(
      isOAuthOnboardingRequiredResponse({
        success: true,
        message: '',
        data: { id: 1001 },
      }),
      false
    )
  })

  test('builds onboarding URL with encoded token and redirect', () => {
    assert.equal(
      buildOAuthOnboardingUrl('pending token', '/dashboard?tab=keys'),
      '/oauth-onboarding?pending_token=pending+token&redirect=%2Fdashboard%3Ftab%3Dkeys'
    )
  })

  test('dispatches onboarding_required responses to onboarding page', () => {
    const visited: string[] = []

    const handled = handleOAuthOnboardingRequired(
      {
        success: true,
        message: 'oauth_onboarding_required',
        data: { pending_token: 'pending token' },
      },
      (target) => visited.push(target),
      '/dashboard'
    )

    assert.equal(handled, true)
    assert.deepEqual(visited, [
      '/oauth-onboarding?pending_token=pending+token&redirect=%2Fdashboard',
    ])
  })

  test('does not navigate for non-onboarding responses', () => {
    const visited: string[] = []

    const handled = handleOAuthOnboardingRequired(
      { success: true, data: { id: 1 } },
      (target) => visited.push(target)
    )

    assert.equal(handled, false)
    assert.deepEqual(visited, [])
  })
})
