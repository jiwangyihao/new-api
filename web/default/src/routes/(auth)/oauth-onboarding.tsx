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
import { z } from 'zod'
import { createFileRoute, useNavigate, useSearch } from '@tanstack/react-router'
import { OAuthOnboarding } from '@/features/auth/oauth-onboarding'

const searchSchema = z.object({
  pending_token: z.string().optional(),
  redirect: z.string().optional(),
})

type OnboardingSearch = {
  pending_token?: string
  redirect?: string
}

function sanitizeRedirect(redirect: string | undefined): string | undefined {
  if (
    !redirect ||
    !redirect.startsWith('/') ||
    redirect.startsWith('//') ||
    /[\\\u0000-\u001F\u007F]/.test(redirect)
  ) {
    return undefined
  }
  return redirect
}

function OAuthOnboardingPage() {
  const navigate = useNavigate()
  const search = useSearch({
    from: '/(auth)/oauth-onboarding',
  }) as OnboardingSearch

  return (
    <OAuthOnboarding
      pendingToken={search?.pending_token ?? ''}
      redirect={sanitizeRedirect(search?.redirect)}
      onComplete={(target) => navigate({ to: target as never, replace: true })}
    />
  )
}

export const Route = createFileRoute('/(auth)/oauth-onboarding')({
  component: OAuthOnboardingPage,
  validateSearch: searchSchema,
})
