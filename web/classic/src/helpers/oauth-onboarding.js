/*
Copyright (C) 2025 QuantumNous

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

export function isOAuthOnboardingRequiredResponse(response) {
  return (
    response?.success === true &&
    response?.message === 'oauth_onboarding_required' &&
    typeof response?.data?.pending_token === 'string' &&
    response.data.pending_token.length > 0
  );
}

export function buildOAuthOnboardingUrl(pendingToken, redirect) {
  const params = new URLSearchParams({ pending_token: pendingToken });
  if (redirect) params.set('redirect', redirect);
  return `/oauth-onboarding?${params.toString()}`;
}
