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
  ChannelGroup,
  ChannelGroupPayload,
  ChannelOption,
} from './types'

// List all channel groups (admin view, includes member channel ids).
export async function getChannelGroups(): Promise<ApiResponse<ChannelGroup[]>> {
  const res = await api.get('/api/channel_group/')
  return res.data
}

// Create a channel group.
export async function createChannelGroup(
  payload: ChannelGroupPayload
): Promise<ApiResponse<ChannelGroup>> {
  const res = await api.post('/api/channel_group/', payload)
  return res.data
}

// Update an existing channel group.
export async function updateChannelGroup(
  payload: ChannelGroupPayload & { id: number }
): Promise<ApiResponse<ChannelGroup>> {
  const res = await api.put('/api/channel_group/', payload)
  return res.data
}

// Delete a channel group by id (default group is rejected by the backend).
export async function deleteChannelGroup(
  id: number
): Promise<ApiResponse<null>> {
  const res = await api.delete(`/api/channel_group/${id}`)
  return res.data
}

// Channel options for the member picker (id + name only).
export async function getChannelOptions(): Promise<ChannelOption[]> {
  const res = await api.get('/api/channel', { params: { p: 1, size: 1000 } })
  const items: Array<{ id: number; name: string }> = res.data?.data?.items ?? []
  return items.map((c) => ({ id: c.id, name: c.name }))
}
