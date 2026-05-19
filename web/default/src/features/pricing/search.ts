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
import type { TokenUnit } from './types'
import type { ViewMode } from './constants'

export type PricingSearch = {
  search?: string
  sort?: string
  vendor?: string
  group?: string
  quotaType?: string | number
  endpointType?: string
  tag?: string
  tokenUnit?: TokenUnit
  view?: ViewMode
  rechargePrice?: boolean | number
}

const COST_SORT_PREFIX = 'price-'

export function sanitizePricingSearchForRole(
  search: PricingSearch,
  isAdmin: boolean
): PricingSearch {
  if (isAdmin) {
    return { ...search }
  }

  const sanitized: PricingSearch = { ...search }

  if (sanitized.sort?.startsWith(COST_SORT_PREFIX)) {
    delete sanitized.sort
  }

  delete sanitized.quotaType
  delete sanitized.tokenUnit
  delete sanitized.rechargePrice

  for (const key of Object.keys(sanitized) as Array<keyof PricingSearch>) {
    if (sanitized[key] === undefined) {
      delete sanitized[key]
    }
  }

  return sanitized
}
