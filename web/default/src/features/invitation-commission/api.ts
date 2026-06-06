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
  AdminInvitationCommissionWithdrawal,
  AdminInvitationCommissionWithdrawalParams,
  AdminTasksSummary,
  PageEnvelope,
} from './types'

type ApiPayloadResponse<T> = {
  success: boolean
  message?: string
  data: T
}

function unwrapAdminPayload<T>(payload: ApiPayloadResponse<T>): T {
  if (!payload.success) {
    throw new Error(payload.message || 'Request failed')
  }
  return payload.data
}

export async function listAdminInvitationCommissionWithdrawals(
  params: AdminInvitationCommissionWithdrawalParams
): Promise<PageEnvelope<AdminInvitationCommissionWithdrawal>> {
  const res = await api.get<
    ApiPayloadResponse<PageEnvelope<AdminInvitationCommissionWithdrawal>>
  >('/api/admin/invitation-commission/withdrawals', { params })
  return unwrapAdminPayload(res.data)
}

export async function completeInvitationCommissionWithdrawal(
  id: number,
  admin_remark: string
): Promise<void> {
  const res = await api.post<ApiPayloadResponse<unknown>>(
    `/api/admin/invitation-commission/withdrawals/${id}/complete`,
    { admin_remark }
  )
  void unwrapAdminPayload(res.data)
}

export async function rejectInvitationCommissionWithdrawal(
  id: number,
  admin_remark: string
): Promise<void> {
  const res = await api.post<ApiPayloadResponse<unknown>>(
    `/api/admin/invitation-commission/withdrawals/${id}/reject`,
    { admin_remark }
  )
  void unwrapAdminPayload(res.data)
}

export async function getAdminTasksSummary(): Promise<AdminTasksSummary> {
  const res = await api.get<ApiPayloadResponse<AdminTasksSummary>>(
    '/api/admin/tasks/summary',
    {
      skipErrorHandler: true,
      skipBusinessError: true,
      disableDuplicate: true,
    } as Record<string, unknown>
  )
  return {
    pending_commission_withdrawals: unwrapAdminPayload(res.data)
      .pending_commission_withdrawals,
  }
}
