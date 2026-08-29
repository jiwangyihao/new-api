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

export interface CreditBillingData {
  billing_unit?: string
  final_credits?: number
}

export type CreditSettlement =
  | { kind: 'released'; credits: number }
  | { kind: 'charged'; credits: number }
  | { kind: 'none'; credits: 0 }

export function isCreditBillingLog(
  billing: CreditBillingData | null | undefined
): boolean {
  return billing?.billing_unit === 'credit'
}

export function formatCredits(value: number | bigint, locale?: string): string {
  if (typeof value === 'number' && !Number.isFinite(value)) return '— Credit'
  const credits = typeof value === 'bigint' ? value : Math.trunc(value)
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(credits)} Credit`
}

export function getCreditSettlement(delta: number): CreditSettlement {
  if (delta < 0)
    return { kind: 'released', credits: Math.abs(Math.trunc(delta)) }
  if (delta > 0) return { kind: 'charged', credits: Math.trunc(delta) }
  return { kind: 'none', credits: 0 }
}
