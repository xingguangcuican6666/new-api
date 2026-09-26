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
import { useLocation } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { resolveSidebarView } from '@/components/layout/lib/sidebar-view-registry'
import type {
  NavGroup,
  NavItem,
  ResolvedSidebarView,
} from '@/components/layout/types'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarConfig } from './use-sidebar-config'
import { useSidebarData } from './use-sidebar-data'

/** Sentinel key used for the root navigation in animation `key=` props */
const ROOT_VIEW_KEY = '__root'

/**
 * Resolve the active sidebar view for the current location.
 *
 * - Returns the matching nested {@link SidebarView} (with its nav
 *   groups) when the URL belongs to a registered drill-in workspace.
 * - Otherwise returns the root navigation, narrowed by:
 *     · admin-only group visibility (role-based);
 *     · `useSidebarConfig` (admin × user `sidebar_modules` overlay).
 *
 * Nested views are intentionally NOT passed through `useSidebarConfig`
 * — those filters target known dashboard URLs only, and gating is
 * already enforced at the route level (`beforeLoad` redirects).
 */
export function useSidebarView(): ResolvedSidebarView {
  const { t } = useTranslation()
  const pathname = useLocation({ select: (l) => l.pathname })
  const user = useAuthStore((s) => s.auth.user)
  const rootSidebarData = useSidebarData()
  const configFilteredRoot = useSidebarConfig(rootSidebarData.navGroups)

  const rootNavGroups = useMemo<NavGroup[]>(() => {
    const role = user?.role ?? ROLE.GUEST
    const isAdmin = role >= ROLE.ADMIN

    // An item is visible by role threshold, or by an explicit capability grant
    // (e.g. a common user allowed to create OAuth applications).
    const isItemVisible = (item: NavItem) =>
      item.requiredRole === undefined ||
      role >= item.requiredRole ||
      item.requiredCapability?.(user) === true

    return configFilteredRoot
      .map((group) => {
        // The admin group is admin-only, except that a capability grant can
        // surface individual items within it to a non-admin user.
        if (group.id === 'admin' && !isAdmin) {
          const items = group.items.filter(
            (item) => item.requiredCapability?.(user) === true
          )
          return items.length > 0 ? { ...group, items } : null
        }
        const items = group.items.filter(isItemVisible)
        return items.length === group.items.length ? group : { ...group, items }
      })
      .filter((group): group is NavGroup => group !== null)
  }, [configFilteredRoot, user])

  const view = resolveSidebarView(pathname)

  if (view) {
    return {
      key: view.id,
      view,
      navGroups: view.getNavGroups(t),
    }
  }

  return {
    key: ROOT_VIEW_KEY,
    view: null,
    navGroups: rootNavGroups,
  }
}
