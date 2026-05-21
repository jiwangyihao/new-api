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
import {
  createFileRoute,
  redirect,
  useNavigate,
  type NavigateOptions,
} from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { AdminAnalyticsPage } from '@/features/admin-analytics'
import { adminAnalyticsSearchSchema } from '@/features/admin-analytics/lib/filters'
import type { AdminAnalyticsCanonicalFilters } from '@/features/admin-analytics/types'

export const Route = createFileRoute('/_authenticated/admin-analytics/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: adminAnalyticsSearchSchema,
  component: AdminAnalyticsRoute,
})

function AdminAnalyticsRoute() {
  const search = Route.useSearch()
  const navigate = useNavigate()

  return (
    <AdminAnalyticsPage
      search={search}
      onSearchChange={(next: AdminAnalyticsCanonicalFilters) => {
        void navigate({
          to: '/admin-analytics',
          search: () => next,
        })
      }}
      onDrilldown={(target) => {
        void navigate({
          to: target.to,
          params: target.params,
          search: { ...target.search },
        } as unknown as NavigateOptions)
      }}
    />
  )
}
