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
import type { PublicPlanRecord } from '@/features/subscriptions/types'

export const HOME_PLANS_PREVIEW_LIMIT = 3

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isVisiblePlanRecord(record: unknown): record is PublicPlanRecord {
  if (!isObject(record)) return false

  const plan = record.plan
  if (!isObject(plan)) return false

  return plan.public_visible !== false
}

export function selectHomePlanRecords(
  records: readonly unknown[] = []
): PublicPlanRecord[] {
  return records
    .filter(isVisiblePlanRecord)
    .slice(0, HOME_PLANS_PREVIEW_LIMIT)
}

export function hasMoreHomePlans(records: readonly unknown[] = []): boolean {
  return records.filter(isVisiblePlanRecord).length > HOME_PLANS_PREVIEW_LIMIT
}
