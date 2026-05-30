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
import type { AdminOpsConcurrencyFilters } from './components/concurrency-queue-card'

export const DEFAULT_ADMIN_OPS_CONCURRENCY_FILTERS: AdminOpsConcurrencyFilters =
  {
    search: '',
    planId: 0,
    status: '',
    minActiveOrQueued: 1,
  }

export function isDefaultAdminOpsConcurrencyFilters(
  filters: AdminOpsConcurrencyFilters
): boolean {
  return (
    filters.search === DEFAULT_ADMIN_OPS_CONCURRENCY_FILTERS.search &&
    filters.planId === DEFAULT_ADMIN_OPS_CONCURRENCY_FILTERS.planId &&
    filters.status === DEFAULT_ADMIN_OPS_CONCURRENCY_FILTERS.status &&
    filters.minActiveOrQueued ===
      DEFAULT_ADMIN_OPS_CONCURRENCY_FILTERS.minActiveOrQueued
  )
}
