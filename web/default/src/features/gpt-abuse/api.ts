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
  GPTAbuseApiResponse,
  GPTAbuseClearSuspensionResponse,
  GPTAbuseLogListResponse,
  GPTAbuseLogSearch,
  GPTAbuseReasonPayload,
  GPTAbuseRepeatBlockListResponse,
  GPTAbuseRepeatBlockSearch,
  GPTAbuseResetWarningsPayload,
  GPTAbuseResetWarningsResponse,
  GPTAbuseUserListResponse,
  GPTAbuseUserListSearch,
} from './types'

export async function getGPTAbuseUsers(
  params: GPTAbuseUserListSearch
): Promise<GPTAbuseApiResponse<GPTAbuseUserListResponse>> {
  const res = await api.get<GPTAbuseApiResponse<GPTAbuseUserListResponse>>(
    '/api/gpt-abuse/users',
    {
      params,
      disableDuplicate: true,
    } as Record<string, unknown>
  )
  return res.data
}

export async function getGPTAbuseUserLogs(
  userId: number,
  params: GPTAbuseLogSearch
): Promise<GPTAbuseApiResponse<GPTAbuseLogListResponse>> {
  const res = await api.get<GPTAbuseApiResponse<GPTAbuseLogListResponse>>(
    `/api/gpt-abuse/users/${userId}/logs`,
    {
      params,
      disableDuplicate: true,
    } as Record<string, unknown>
  )
  return res.data
}

export async function getGPTAbuseRepeatBlocks(
  userId: number,
  params: GPTAbuseRepeatBlockSearch
): Promise<GPTAbuseApiResponse<GPTAbuseRepeatBlockListResponse>> {
  const res = await api.get<GPTAbuseApiResponse<GPTAbuseRepeatBlockListResponse>>(
    `/api/gpt-abuse/users/${userId}/repeat-blocks`,
    {
      params,
      disableDuplicate: true,
    } as Record<string, unknown>
  )
  return res.data
}

export async function clearGPTAbuseSuspension(
  userId: number,
  payload: GPTAbuseReasonPayload
): Promise<GPTAbuseApiResponse<GPTAbuseClearSuspensionResponse>> {
  const res = await api.post<GPTAbuseApiResponse<GPTAbuseClearSuspensionResponse>>(
    `/api/gpt-abuse/users/${userId}/clear-suspension`,
    payload
  )
  return res.data
}

export async function resetGPTAbuseWarnings(
  userId: number,
  payload: GPTAbuseResetWarningsPayload
): Promise<GPTAbuseApiResponse<GPTAbuseResetWarningsResponse>> {
  const res = await api.post<GPTAbuseApiResponse<GPTAbuseResetWarningsResponse>>(
    `/api/gpt-abuse/users/${userId}/reset-warnings`,
    payload
  )
  return res.data
}
