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
import type { AdminOpsConcurrencyUser } from '../types'

export type AdminOpsConcurrencyPlanOption = {
  id: number
  label: string
}

export function buildAdminOpsConcurrencyPlanOptions(
  users: AdminOpsConcurrencyUser[],
  planOptions: AdminOpsConcurrencyPlanOption[]
): AdminOpsConcurrencyPlanOption[] {
  const labels = new Map<number, string>()
  for (const option of planOptions) {
    if (option.id > 0 && option.label.trim() !== '') {
      labels.set(option.id, option.label)
    }
  }
  for (const user of users) {
    if (user.plan_id <= 0 || labels.has(user.plan_id)) continue
    labels.set(
      user.plan_id,
      user.plan_title || user.plan_code || `#${user.plan_id}`
    )
  }
  return Array.from(labels, ([id, label]) => ({ id, label }))
}
