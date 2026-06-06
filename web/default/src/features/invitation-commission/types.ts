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
import type {
  InvitationCommissionContact,
  PageEnvelope,
  PageParams,
} from '@/features/wallet/types'

export type { InvitationCommissionContact, PageEnvelope, PageParams }

export type AdminInvitationCommissionWithdrawalStatus =
  | 'pending'
  | 'completed'
  | 'rejected'

export interface AdminInvitationCommissionWithdrawal {
  id: number
  user_id: number
  username: string
  display_name: string
  amount_cents: number
  status: AdminInvitationCommissionWithdrawalStatus
  method: 'manual'
  contact: InvitationCommissionContact
  user_remark: string
  admin_remark: string
  reviewer_id: number
  completed_by: number
  reviewed_at: number
  completed_at: number
  created_at: number
  updated_at: number
}

export interface AdminTasksSummary {
  pending_commission_withdrawals: number
}

export interface AdminInvitationCommissionWithdrawalParams extends PageParams {
  status?: AdminInvitationCommissionWithdrawalStatus
  user_id?: number
}
