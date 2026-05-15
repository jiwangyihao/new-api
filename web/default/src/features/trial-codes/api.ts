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
  ApiResponse,
  GetTrialCodesParams,
  GetTrialCodesResponse,
  TrialCode,
  TrialCodePayload,
} from './types'

const trialCodesAdminPath = '/api/trial-codes/admin'

export async function getTrialCodes(
  params: GetTrialCodesParams = {}
): Promise<GetTrialCodesResponse> {
  const res = await api.get(trialCodesAdminPath, { params })
  return res.data
}

export async function createTrialCode(
  data: TrialCodePayload
): Promise<ApiResponse<TrialCode>> {
  const res = await api.post(trialCodesAdminPath, data)
  return res.data
}

export async function updateTrialCode(
  id: number,
  data: TrialCodePayload
): Promise<ApiResponse<TrialCode>> {
  const res = await api.put(`${trialCodesAdminPath}/${id}`, data)
  return res.data
}

export async function updateTrialCodeStatus(
  id: number,
  enabled: boolean
): Promise<ApiResponse<{ id: number; enabled: boolean }>> {
  const res = await api.patch(`${trialCodesAdminPath}/${id}`, { enabled })
  return res.data
}

export async function deleteTrialCode(
  id: number
): Promise<ApiResponse<{ id: number }>> {
  const res = await api.delete(`${trialCodesAdminPath}/${id}`)
  return res.data
}
