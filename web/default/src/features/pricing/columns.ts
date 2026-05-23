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
import type { PricingModel } from './types'

export type ModelDirectoryColumn = {
  id?: string
  accessorKey?: keyof PricingModel | 'price'
}

const PUBLIC_MODEL_DIRECTORY_COLUMNS: readonly ModelDirectoryColumn[] = [
  { accessorKey: 'model_name' },
  { accessorKey: 'vendor_name' },
  { accessorKey: 'supported_endpoint_types' },
  { id: 'context_length' },
  { id: 'modalities' },
  { id: 'capabilities' },
  { accessorKey: 'tags' },
]

const ADMIN_COST_COLUMNS: readonly ModelDirectoryColumn[] = [
  { accessorKey: 'quota_type' },
  { accessorKey: 'price' },
  { id: 'cached_price' },
  { accessorKey: 'billing_mode' },
]

export function getModelDirectoryColumnKey(
  column: ModelDirectoryColumn
): string {
  return column.id || column.accessorKey || ''
}

export function buildModelDirectoryColumns(options: {
  isAdmin: boolean
}): ModelDirectoryColumn[] {
  if (!options.isAdmin) {
    return [...PUBLIC_MODEL_DIRECTORY_COLUMNS]
  }

  const [modelColumn, ...directoryColumns] = PUBLIC_MODEL_DIRECTORY_COLUMNS
  return [modelColumn, ...ADMIN_COST_COLUMNS, ...directoryColumns]
}
