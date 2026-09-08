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
import type { AvailabilityRange, AvailabilityReport } from './types'

export async function getAvailability(
  range: AvailabilityRange,
  signal: AbortSignal
): Promise<AvailabilityReport> {
  // React Query owns deduplication and cancellation for this signal.
  const config = { params: { range }, signal, disableDuplicate: true }
  const response = await api.get<{
    success: boolean
    message: string
    data: AvailabilityReport
  }>('/api/availability', config)
  if (!response.data.success) throw new Error('Availability report unavailable')
  return response.data.data
}
