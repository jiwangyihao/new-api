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
import { normalizeAdminAnalyticsUsersDrilldownResponse } from './lib/admin-analytics-drilldown'
import type {
  User,
  GetUsersParams,
  GetUsersResponse,
  SearchUsersParams,
  AdminAnalyticsUsersDrilldownParams,
  AdminAnalyticsUsersDrilldownResponse,
  AdminAnalyticsUsersDrilldownEnvelopeResponse,
  UserFormData,
  ManageUserAction,
  ManageUserQuotaPayload,
  ApiResponse,
} from './types'

// ============================================================================
// User Management APIs
// ============================================================================

/**
 * Get paginated users list
 */
export async function getUsers(
  params: GetUsersParams = {}
): Promise<GetUsersResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/user/?p=${p}&page_size=${page_size}`)
  return res.data
}

/**
 * Search users by keyword
 */
export async function searchUsers(
  params: SearchUsersParams
): Promise<GetUsersResponse> {
  const { keyword = '', p = 1, page_size = 10 } = params
  const queryParams = new URLSearchParams()
  queryParams.set('keyword', keyword)
  queryParams.set('p', String(p))
  queryParams.set('page_size', String(page_size))
  const res = await api.get(`/api/user/search?${queryParams.toString()}`)
  return res.data
}

function appendNumberParam(
  queryParams: URLSearchParams,
  key: string,
  value: number | undefined
): void {
  if (value !== undefined) queryParams.append(key, String(value))
}

function appendStringParam(
  queryParams: URLSearchParams,
  key: string,
  value: string | undefined
): void {
  const normalized = value?.trim()
  if (normalized) queryParams.append(key, normalized)
}

function appendUserIdsParam(
  queryParams: URLSearchParams,
  value: number | number[] | undefined
): void {
  if (Array.isArray(value)) {
    for (const userId of value) queryParams.append('user_id', String(userId))
    return
  }
  appendNumberParam(queryParams, 'user_id', value)
}

export async function getAdminAnalyticsUsersDrilldown(
  params: AdminAnalyticsUsersDrilldownParams
): Promise<AdminAnalyticsUsersDrilldownResponse> {
  const queryParams = new URLSearchParams()
  appendUserIdsParam(queryParams, params.user_id)
  appendNumberParam(queryParams, 'plan_id', params.plan_id)
  appendNumberParam(queryParams, 'inviter_id', params.inviter_id)
  appendStringParam(queryParams, 'user_status', params.user_status)
  appendNumberParam(queryParams, 'limit', params.limit)
  appendNumberParam(queryParams, 'offset', params.offset)
  appendStringParam(queryParams, 'sort_order', params.sort_order)

  const res = await api.get<AdminAnalyticsUsersDrilldownEnvelopeResponse>(
    `/api/admin-analytics/drilldown/users?${queryParams.toString()}`
  )
  return normalizeAdminAnalyticsUsersDrilldownResponse(res.data)
}

/**
 * Get single user by ID
 */
export async function getUser(id: number): Promise<ApiResponse<User>> {
  const res = await api.get(`/api/user/${id}`)
  return res.data
}

/**
 * Create a new user
 */
export async function createUser(
  data: UserFormData
): Promise<ApiResponse<User>> {
  const res = await api.post('/api/user/', data)
  return res.data
}

/**
 * Update an existing user
 */
export async function updateUser(
  data: UserFormData & { id: number }
): Promise<ApiResponse<Partial<User>>> {
  const res = await api.put('/api/user/', data)
  return res.data
}

/**
 * Delete a single user (hard delete)
 */
export async function deleteUser(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/user/${id}/`)
  return res.data
}

/**
 * Manage user (promote, demote, enable, disable, delete)
 */
export async function manageUser(
  id: number,
  action: ManageUserAction
): Promise<ApiResponse<Partial<User>>> {
  const res = await api.post('/api/user/manage', { id, action })
  return res.data
}

/**
 * Adjust user quota atomically (add/subtract/override)
 */
export async function adjustUserQuota(
  payload: ManageUserQuotaPayload
): Promise<ApiResponse<Partial<User>>> {
  const res = await api.post('/api/user/manage', payload)
  return res.data
}

/**
 * Reset user's Passkey registration
 */
export async function resetUserPasskey(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/user/${id}/reset_passkey`)
  return res.data
}

/**
 * Reset user's Two-Factor Authentication setup
 */
export async function resetUserTwoFA(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/user/${id}/2fa`)
  return res.data
}


// ============================================================================
// Admin Binding Management APIs
// ============================================================================

export interface OAuthBinding {
  provider_id: string
  provider_name: string
  user_id?: number
  external_id?: string
}

/**
 * Get user's custom OAuth bindings (admin)
 */
export async function getUserOAuthBindings(
  userId: number
): Promise<ApiResponse<OAuthBinding[]>> {
  const res = await api.get(`/api/user/${userId}/oauth/bindings`)
  return res.data
}

/**
 * Clear a user's built-in binding (admin)
 */
export async function adminClearUserBinding(
  userId: number,
  bindingType: string
): Promise<ApiResponse> {
  const res = await api.delete(`/api/user/${userId}/bindings/${bindingType}`)
  return res.data
}

/**
 * Unbind custom OAuth for a user (admin)
 */
export async function adminUnbindCustomOAuth(
  userId: number,
  providerId: string
): Promise<ApiResponse> {
  const res = await api.delete(
    `/api/user/${userId}/oauth/bindings/${providerId}`
  )
  return res.data
}
