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
import { accountBalanceCentsToCnyAmount } from '@/features/subscriptions/lib'
import { type UserFormData, type User } from '../types'

// ============================================================================
// Form Schema
// ============================================================================

export const userFormSchema = z.object({
  username: z.string().min(1, 'Username is required'),
  display_name: z.string().optional(),
  password: z.string().optional(),
  role: z.number().optional(),
  quota_cny: z.string().optional(),
  remark: z.string().optional(),
  invitation_reward_mode: z.enum(['subscription', 'commission']).optional(),
})

export type UserFormValues = z.infer<typeof userFormSchema>
type UserFormDefaultsInput = Pick<User, 'quota'> & Partial<Omit<User, 'quota'>>

type InvitationCommissionEstimateInput = Pick<
  User,
  | 'invitation_reward_mode'
  | 'invitation_commission_estimated_cents'
  | 'invitation_commission_estimated_source_amount_cents'
  | 'invitation_commission_estimated_event_count'
>

export function shouldShowInvitationCommissionEstimateForUser(
  user?: Partial<InvitationCommissionEstimateInput>
): boolean {
  if (!user || user.invitation_reward_mode === 'commission') {
    return false
  }
  return (
    (user.invitation_commission_estimated_cents ?? 0) > 0 ||
    (user.invitation_commission_estimated_source_amount_cents ?? 0) > 0 ||
    (user.invitation_commission_estimated_event_count ?? 0) > 0
  )
}
// ============================================================================
// Form Defaults
// ============================================================================

export const USER_FORM_DEFAULT_VALUES: UserFormValues = {
  username: '',
  display_name: '',
  password: '',
  role: 1, // Default to common user
  quota_cny: '0.00',
  remark: '',
  invitation_reward_mode: 'subscription',
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: UserFormValues,
  userId?: number
): UserFormData & { id?: number } {
  const payload: UserFormData & { id?: number } = {
    username: data.username,
    display_name: data.display_name || data.username,
  }

  if (userId === undefined || data.password) {
    payload.password = data.password || ''
  }

  // For create: only send required fields
  if (userId === undefined) {
    payload.role = data.role || 1 // Default to common user
  } else {
    // For update: quota is adjusted atomically via /api/user/manage, not sent here
    payload.remark = data.remark || undefined
    payload.id = userId
    payload.invitation_reward_mode =
      data.invitation_reward_mode || 'subscription'
  }

  return payload
}

/**
 * Transform user data to form defaults
 */
export function transformUserToFormDefaults(
  user: UserFormDefaultsInput
): UserFormValues {
  const username = user.username ?? ''
  return {
    username,
    display_name: user.display_name ?? username,
    password: '',
    role: user.role ?? 1,
    quota_cny: accountBalanceCentsToCnyAmount(user.quota).toFixed(2),
    remark: user.remark || '',
    invitation_reward_mode: user.invitation_reward_mode ?? 'subscription',
  }
}
