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
import type { RouteHealthReport } from './types'

export async function getRouteHealth(
  signal: AbortSignal
): Promise<RouteHealthReport> {
  // React Query owns deduplication and cancellation for this read-only snapshot.
  const config = {
    signal,
    disableDuplicate: true,
    skipBusinessError: true,
    skipErrorHandler: true,
  } as Record<string, unknown>
  const response = await api.get<{ success: boolean; data: RouteHealthReport }>(
    '/api/channel/route_health',
    config
  )
  if (!response.data.success || !response.data.data) {
    throw new Error('Route health snapshot unavailable')
  }
  return response.data.data
}
