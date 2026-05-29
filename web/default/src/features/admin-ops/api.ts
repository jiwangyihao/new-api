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
import { api } from '@/lib/api'
import type {
  AdminOpsApiResponse,
  AdminOpsConcurrencyResponse,
  AdminOpsSnapshot,
} from './types'

export type AdminOpsSnapshotParams = {
  window_seconds: number
  top: number
}

export type AdminOpsConcurrencyParams = {
  limit: number
  include_users: boolean
  min_active_or_queued: number
}

export async function getAdminOpsSnapshot(
  params: AdminOpsSnapshotParams
): Promise<AdminOpsApiResponse<AdminOpsSnapshot>> {
  const res = await api.get<AdminOpsApiResponse<AdminOpsSnapshot>>(
    '/api/admin-ops/snapshot',
    {
      params,
      disableDuplicate: true,
    } as Record<string, unknown>
  )
  return res.data
}

export async function getAdminOpsConcurrency(
  params: AdminOpsConcurrencyParams
): Promise<AdminOpsApiResponse<AdminOpsConcurrencyResponse>> {
  const res = await api.get<AdminOpsApiResponse<AdminOpsConcurrencyResponse>>(
    '/api/admin-ops/concurrency',
    {
      params,
      disableDuplicate: true,
    } as Record<string, unknown>
  )
  return res.data
}
