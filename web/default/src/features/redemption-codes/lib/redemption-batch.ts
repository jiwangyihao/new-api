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
import { formatAccountBalanceForPlanPurchase } from '@/features/subscriptions/lib'
import type { Redemption } from '../types'

export type RedemptionBatchRow = Redemption & {
  is_batch_row: boolean
  children?: RedemptionBatchRow[]
}

export function getRedemptionBatchKey(redemption: Redemption): string {
  const batchId = redemption.batch_id?.trim()
  return batchId || `single-${redemption.id}`
}

export function formatRedemptionWalletValue(quota: number): string {
  return formatAccountBalanceForPlanPurchase(quota)
}

function createBatchRow(
  batchKey: string,
  children: RedemptionBatchRow[]
): RedemptionBatchRow {
  const first = children[0]
  const enabledCount = children.filter((item) => item.status === 1).length
  const usedCount = children.filter((item) => item.status === 3).length
  const disabledCount = children.filter((item) => item.status === 2).length
  const latestRedeemedTime = children.reduce(
    (latest, item) => Math.max(latest, item.redeemed_time || 0),
    0
  )
  const usedUserId =
    children.find((item) => item.used_user_id > 0)?.used_user_id || 0

  let status = first.status
  if (enabledCount > 0) {
    status = 1
  } else if (usedCount > 0 && disabledCount === 0) {
    status = 3
  }
  return {
    ...first,
    id: first.id,
    key: batchKey,
    batch_id: first.batch_id || batchKey,
    status,
    redeemed_time: latestRedeemedTime,
    used_user_id: usedUserId,
    is_batch_row: true,
    children,
  }
}

export function aggregateRedemptionsByBatch(
  redemptions: Redemption[]
): RedemptionBatchRow[] {
  const groups = new Map<string, RedemptionBatchRow[]>()

  for (const redemption of redemptions) {
    const child: RedemptionBatchRow = { ...redemption, is_batch_row: false }
    const key = getRedemptionBatchKey(redemption)
    const group = groups.get(key)
    if (group) {
      group.push(child)
    } else {
      groups.set(key, [child])
    }
  }

  const rows: RedemptionBatchRow[] = []
  for (const [batchKey, children] of groups) {
    if (children.length === 1) {
      rows.push({ ...children[0], is_batch_row: false })
      continue
    }
    rows.push(createBatchRow(batchKey, children))
  }
  return rows
}

export function isRedemptionBatchRow(
  redemption: Redemption | RedemptionBatchRow
): boolean {
  return Boolean((redemption as RedemptionBatchRow).is_batch_row)
}

export function getRedemptionRowDeleteIds(
  redemption: RedemptionBatchRow
): number[] {
  if (isRedemptionBatchRow(redemption) && redemption.children?.length) {
    return redemption.children.map((item) => item.id)
  }
  return [redemption.id]
}

export function createFullRedemptionBatchRow(
  batchRow: RedemptionBatchRow,
  redemptions: Redemption[]
): RedemptionBatchRow {
  if (!isRedemptionBatchRow(batchRow)) {
    return batchRow
  }
  const children = redemptions.map((redemption) => ({
    ...redemption,
    is_batch_row: false,
  }))
  return createBatchRow(
    batchRow.batch_id || getRedemptionBatchKey(batchRow),
    children
  )
}

export function getRedemptionRowCopyItems(
  redemption: RedemptionBatchRow
): RedemptionBatchRow[] {
  if (isRedemptionBatchRow(redemption) && redemption.children?.length) {
    return redemption.children
  }
  return [redemption]
}

export function getRedemptionRowCopyCount(
  redemption: RedemptionBatchRow
): number {
  return getRedemptionRowCopyItems(redemption).length
}

export function getRedemptionRowCopyText(
  redemption: RedemptionBatchRow
): string {
  return getRedemptionRowCopyItems(redemption)
    .map((item) => `${item.name}\t${item.key}`)
    .join('\n')
}
