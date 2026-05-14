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

export type OAuthOnboardingPendingData = {
  pending_token: string
  provider?: string
  login?: string
  email?: string
}

export type OAuthOnboardingRequiredResponse = {
  success?: boolean
  message?: string
  data?: OAuthOnboardingPendingData | null
}

export function isOAuthOnboardingRequiredResponse(
  response: unknown
): response is OAuthOnboardingRequiredResponse {
  if (!response || typeof response !== 'object') return false
  const candidate = response as OAuthOnboardingRequiredResponse
  return (
    candidate.success === true &&
    candidate.message === 'oauth_onboarding_required' &&
    typeof candidate.data?.pending_token === 'string' &&
    candidate.data.pending_token.length > 0
  )
}

export function buildOAuthOnboardingUrl(
  pendingToken: string,
  redirect?: string
): string {
  const params = new URLSearchParams({ pending_token: pendingToken })
  if (redirect) params.set('redirect', redirect)
  return `/oauth-onboarding?${params.toString()}`
}
