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

export const trialCodeSchema = z.object({
  id: z.number(),
  code: z.string(),
  plan_id: z.number(),
  enabled: z.boolean(),
  max_redemptions: z.number(),
  redeemed_count: z.number(),
  expires_at: z.number(),
  created_at: z.number(),
  updated_at: z.number(),
})

export type TrialCode = z.infer<typeof trialCodeSchema>

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetTrialCodesParams {
  p?: number
  page_size?: number
  filter?: string
}

export interface GetTrialCodesResponse {
  success: boolean
  message?: string
  data?: {
    items: TrialCode[]
    total: number
    page: number
    page_size: number
  }
}

export interface TrialCodePayload {
  code: string
  plan_id: number
  enabled: boolean
  max_redemptions: number
  expires_at: number
}

export type TrialCodesDialogType = 'create' | 'update' | 'delete'
