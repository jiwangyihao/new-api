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
  Redemption,
  ApiResponse,
  GetRedemptionsParams,
  GetRedemptionsResponse,
  SearchRedemptionsParams,
  RedemptionFormData,
} from './types'

function buildRedemptionsQuery(
  params: GetRedemptionsParams | SearchRedemptionsParams
): string {
  const searchParams = new URLSearchParams()
  searchParams.set('p', String(params.p ?? 1))
  searchParams.set('page_size', String(params.page_size ?? 10))
  if (params.keyword?.trim()) searchParams.set('keyword', params.keyword.trim())
  if (params.type) searchParams.set('type', params.type)
  if (params.status !== undefined)
    searchParams.set('status', String(params.status))
  if (params.batch_id?.trim())
    searchParams.set('batch_id', params.batch_id.trim())
  return searchParams.toString()
}

// ============================================================================
// Redemption Code Management
// ============================================================================

// Get paginated redemption codes list
export async function getRedemptions(
  params: GetRedemptionsParams = {}
): Promise<GetRedemptionsResponse> {
  const res = await api.get(`/api/redemption/?${buildRedemptionsQuery(params)}`)
  return res.data
}

// Search redemption codes by keyword
export async function searchRedemptions(
  params: SearchRedemptionsParams
): Promise<GetRedemptionsResponse> {
  const res = await api.get(
    `/api/redemption/search?${buildRedemptionsQuery(params)}`
  )
  return res.data
}

// Get single redemption code by ID
export async function getRedemption(
  id: number
): Promise<ApiResponse<Redemption>> {
  const res = await api.get(`/api/redemption/${id}`)
  return res.data
}

// Create redemption code(s)
export async function createRedemption(
  data: RedemptionFormData
): Promise<ApiResponse<string[]>> {
  const res = await api.post('/api/redemption/', data)
  return res.data
}

// Update redemption code
export async function updateRedemption(
  data: RedemptionFormData & { id: number }
): Promise<ApiResponse<Redemption>> {
  const res = await api.put('/api/redemption/', data)
  return res.data
}

// Update redemption code status (enable/disable)
export async function updateRedemptionStatus(
  id: number,
  status: number
): Promise<ApiResponse<Redemption>> {
  const res = await api.put('/api/redemption/?status_only=true', { id, status })
  return res.data
}

// Delete a single redemption code
export async function deleteRedemption(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/redemption/${id}/`)
  return res.data
}

// Delete invalid redemption codes (used, disabled, expired)
export async function deleteInvalidRedemptions(): Promise<ApiResponse<number>> {
  const res = await api.delete('/api/redemption/invalid')
  return res.data
}

// Delete selected redemption codes in any status
export async function batchDeleteRedemptions(
  ids: number[]
): Promise<ApiResponse<number>> {
  const res = await api.post('/api/redemption/batch', { ids })
  return res.data
}

// Delete all redemption codes in any status
export async function deleteAllRedemptions(): Promise<ApiResponse<number>> {
  const res = await api.delete('/api/redemption/all')
  return res.data
}
