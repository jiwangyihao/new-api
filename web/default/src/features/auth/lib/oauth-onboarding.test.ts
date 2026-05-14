import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildOAuthOnboardingUrl,
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
})
