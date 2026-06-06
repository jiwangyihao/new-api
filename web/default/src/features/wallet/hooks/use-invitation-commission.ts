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
import {
  type UseMutationResult,
  type UseQueryResult,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useAuthStore } from '@/stores/auth-store'
import {
  getInvitationCommissionRecords,
  getInvitationCommissionSummary,
  getInvitationCommissionWithdrawals,
  requestInvitationCommissionWithdrawal,
  transferInvitationCommission,
} from '../api'
import type {
  InvitationCommissionRecord,
  InvitationCommissionSummary,
  InvitationCommissionTransferResult,
  InvitationCommissionWithdrawal,
  InvitationCommissionWithdrawalPayload,
  PageEnvelope,
  PageParams,
} from '../types'

function invalidateInvitationCommissionQueries(
  queryClient: ReturnType<typeof useQueryClient>,
  userId: number | undefined
): Promise<void[]> {
  return Promise.all([
    queryClient.invalidateQueries({
      queryKey: ['wallet', 'invitation-commission', 'summary', userId],
    }),
    queryClient.invalidateQueries({
      queryKey: ['wallet', 'invitation-commission', 'records', userId],
    }),
    queryClient.invalidateQueries({
      queryKey: ['wallet', 'invitation-commission', 'withdrawals', userId],
    }),
  ])
}

export function useInvitationCommissionSummary(): UseQueryResult<
  InvitationCommissionSummary,
  Error
> {
  const userId = useAuthStore((state) => state.auth.user?.id)

  return useQuery({
    queryKey: ['wallet', 'invitation-commission', 'summary', userId],
    queryFn: getInvitationCommissionSummary,
    enabled: Boolean(userId),
  })
}

export function useInvitationCommissionRecords(
  params: PageParams
): UseQueryResult<PageEnvelope<InvitationCommissionRecord>, Error> {
  const userId = useAuthStore((state) => state.auth.user?.id)

  return useQuery({
    queryKey: ['wallet', 'invitation-commission', 'records', userId, params],
    queryFn: () => getInvitationCommissionRecords(params),
    enabled: Boolean(userId),
  })
}

export function useInvitationCommissionWithdrawals(
  params: PageParams
): UseQueryResult<PageEnvelope<InvitationCommissionWithdrawal>, Error> {
  const userId = useAuthStore((state) => state.auth.user?.id)

  return useQuery({
    queryKey: [
      'wallet',
      'invitation-commission',
      'withdrawals',
      userId,
      params,
    ],
    queryFn: () => getInvitationCommissionWithdrawals(params),
    enabled: Boolean(userId),
  })
}

export function useTransferInvitationCommission(): UseMutationResult<
  InvitationCommissionTransferResult,
  Error,
  number
> {
  const queryClient = useQueryClient()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const auth = useAuthStore((state) => state.auth)

  return useMutation({
    mutationFn: transferInvitationCommission,
    onSuccess: async (result) => {
      await invalidateInvitationCommissionQueries(queryClient, userId)

      if (auth.user) {
        auth.setUser({ ...auth.user, quota: result.user_quota })
      }
    },
  })
}

export function useRequestInvitationCommissionWithdrawal(): UseMutationResult<
  InvitationCommissionWithdrawal,
  Error,
  InvitationCommissionWithdrawalPayload
> {
  const queryClient = useQueryClient()
  const userId = useAuthStore((state) => state.auth.user?.id)

  return useMutation({
    mutationFn: (payload: InvitationCommissionWithdrawalPayload) =>
      requestInvitationCommissionWithdrawal(payload),
    onSuccess: async () => {
      await invalidateInvitationCommissionQueries(queryClient, userId)
    },
  })
}
