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
import { z } from 'zod'

// ============================================================================
// User Schema & Types
// ============================================================================

/** User status: 1 = enabled, 2 = disabled, 3+ = other states */
export const userStatusSchema = z.number()
export type UserStatus = z.infer<typeof userStatusSchema>

/** User role: 1 = common user, 10 = admin, 100 = root */
export const userRoleSchema = z.number()
export type UserRole = z.infer<typeof userRoleSchema>

export const userSchema = z.object({
  id: z.number(),
  username: z.string(),
  display_name: z.string(),
  password: z.string().optional(),
  github_id: z.string().optional(),
  oidc_id: z.string().optional(),
  wechat_id: z.string().optional(),
  telegram_id: z.string().optional(),
  email: z.string().optional(),
  quota: z.number(),
  used_quota: z.number(),
  request_count: z.number(),
  group: z.string(),
  aff_code: z.string().optional(),
  aff_count: z.number().optional(),
  aff_quota: z.number().optional(),
  aff_history_quota: z.number().optional(),
  direct_invite_count: z.number().optional(),
  qualified_paid_invite_count: z.number().optional(),
  invitation_reward_status: z.string().optional(),
  invitation_reward_plan_title: z.string().optional(),
  reward_plan_id: z.number().optional(),
  reward_plan_title: z.string().optional(),
  reward_plan_business_code: z.string().optional(),
  reward_tier_rank: z.number().optional(),
  reward_tier_qualified_count: z.number().optional(),
  downgrade_reward_plan_id: z.number().optional(),
  downgrade_reward_plan_title: z.string().optional(),
  downgrade_reward_plan_business_code: z.string().optional(),
  downgrade_tier_rank: z.number().optional(),
  downgrade_tier_qualified_count: z.number().optional(),
  downgrade_entitlement_end_time: z.number().optional(),
  inviter_id: z.number().optional(),
  linux_do_id: z.string().optional(),
  status: userStatusSchema,
  role: userRoleSchema,
  created_at: z.number().optional(),
  updated_at: z.number().optional(),
  last_login_at: z.number().optional(),
  DeletedAt: z.any().nullable().optional(),
  remark: z.string().optional(),
})
export type User = z.infer<typeof userSchema>

export const userListSchema = z.array(userSchema)

// ============================================================================
// API Request/Response Types
// ============================================================================

/** Generic API response */
export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetUsersParams {
  p?: number
  page_size?: number
}

export interface GetUsersResponse {
  success: boolean
  message?: string
  data?: {
    items: User[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchUsersParams {
  keyword?: string
  group?: string
  p?: number
  page_size?: number
}

export type AdminAnalyticsUsersDrilldownSortOrder = 'asc' | 'desc'

export interface AdminAnalyticsUsersDrilldownParams {
  user_id?: number | number[]
  plan_id?: number
  inviter_id?: number
  user_group?: string
  user_status?: string
  limit?: number
  offset?: number
  sort_order?: AdminAnalyticsUsersDrilldownSortOrder
}

export interface AdminAnalyticsUsersDrilldownItem {
  user_id: number
  username: string
  display_name: string
  email: string
  user_group: string
  status: number
  role: number
  created_at: number
  last_login_at: number
  inviter_id: number
  active_plan_id: number
  active_plan_title: string
}

export interface AdminAnalyticsUsersDrilldownPage {
  limit: number
  offset: number
  total: number
  has_more: boolean
}

export interface AdminAnalyticsUsersDrilldownList {
  items: AdminAnalyticsUsersDrilldownItem[]
  page: AdminAnalyticsUsersDrilldownPage
  sort_by?: string
  sort_order: AdminAnalyticsUsersDrilldownSortOrder
}

export interface AdminAnalyticsUsersDrilldownResponse extends ApiResponse<{
  users: AdminAnalyticsUsersDrilldownList
}> {}

export interface AdminAnalyticsUsersDrilldownEnvelopeResponse extends ApiResponse<{
  range: {
    start_timestamp: number
    end_timestamp: number
    snapshot_at: number
  }
  data: {
    users: AdminAnalyticsUsersDrilldownList
  }
  warnings?: Array<{
    section: string
    reason: string
    message: string
  }>
}> {}
export interface UserFormData {
  username: string
  display_name: string
  password?: string
  role?: number // Only used when creating user
  quota?: number // Only used when updating user
  group?: string // Only used when updating user
  remark?: string // Only used when updating user
}

export type ManageUserAction =
  | 'promote'
  | 'demote'
  | 'enable'
  | 'disable'
  | 'delete'
  | 'add_quota'

export type QuotaAdjustMode = 'add' | 'subtract' | 'override'

export interface ManageUserQuotaPayload {
  id: number
  action: 'add_quota'
  mode: QuotaAdjustMode
  value: number
}

// ============================================================================
// Dialog Types
// ============================================================================

export type UsersDialogType = 'create' | 'update' | 'delete'
