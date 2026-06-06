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
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { useLayout } from '@/context/layout-provider'
import {
  filterNavGroupsByRole,
  isAdminCommissionNavVisible,
  resolvePendingCommissionWithdrawalsBadge,
  shouldFetchAdminTasksSummary,
  useSidebarConfig,
} from '@/hooks/use-sidebar-config'
import { useSidebarData } from '@/hooks/use-sidebar-data'
import { Sidebar, SidebarContent, SidebarRail } from '@/components/ui/sidebar'
import { getAdminTasksSummary } from '@/features/invitation-commission/api'
import { getNavGroupsForPath } from '../lib/workspace-registry'
import { NavGroup } from './nav-group'

/**
 * Application sidebar component
 * Fetches corresponding navigation menu from workspace registry based on current path
 * Dynamically filters navigation items based on backend SidebarModulesAdmin configuration
 *
 * Automatically matches workspace configuration for current path through workspace registry system
 * Adding new workspaces only requires registration in workspace-registry.ts
 */
export function AppSidebar() {
  const { t } = useTranslation()
  const { collapsible, variant } = useLayout()
  const { pathname } = useLocation()
  const userRole = useAuthStore((state) => state.auth.user?.role)
  const sidebarData = useSidebarData()

  // Get navigation group configuration corresponding to current path from workspace registry
  const allNavGroups = getNavGroupsForPath(pathname, t) || sidebarData.navGroups

  // Filter sidebar navigation items based on backend configuration
  const configFilteredNavGroups = useSidebarConfig(allNavGroups)

  const roleFilteredNavGroups = useMemo(
    () => filterNavGroupsByRole(configFilteredNavGroups, userRole),
    [configFilteredNavGroups, userRole]
  )

  const adminTasksSummaryQuery = useQuery({
    queryKey: ['admin', 'tasks', 'summary'],
    queryFn: () =>
      resolvePendingCommissionWithdrawalsBadge(
        getAdminTasksSummary,
        () => undefined
      ),
    enabled: shouldFetchAdminTasksSummary(userRole, roleFilteredNavGroups),
  })

  const pendingCommissionWithdrawals = Number(adminTasksSummaryQuery.data ?? 0)

  const currentNavGroups = useMemo(() => {
    const badge =
      pendingCommissionWithdrawals > 0
        ? String(pendingCommissionWithdrawals)
        : undefined
    if (!badge) return roleFilteredNavGroups
    if (!isAdminCommissionNavVisible(roleFilteredNavGroups)) {
      return roleFilteredNavGroups
    }

    return roleFilteredNavGroups.map((group) => {
      if (group.id !== 'admin') return group
      return {
        ...group,
        items: group.items.map((item) => {
          if (
            'url' in item &&
            item.url === '/invitation-commission/withdrawals'
          ) {
            return { ...item, badge }
          }
          return item
        }),
      }
    })
  }, [pendingCommissionWithdrawals, roleFilteredNavGroups])

  return (
    <Sidebar collapsible={collapsible} variant={variant}>
      <SidebarContent className='py-2'>
        {currentNavGroups.map((props) => {
          const key = props.id || props.title
          return <NavGroup key={key} {...props} />
        })}
      </SidebarContent>
      <SidebarRail />
    </Sidebar>
  )
}
