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
import type { RouteHealthRow, RouteHealthState } from '../types'

export type HealthView = 'all' | 'attention' | 'unobserved'

export interface ModelHealthGroup {
  id: string
  name: string
  aliases: string[]
  row: RouteHealthRow
  attention: boolean
  observed: boolean
}

export interface ChannelHealthGroup {
  id: number
  name: string
  status: number
  models: ModelHealthGroup[]
  attentionModels: number
  observedModels: number
}

const attentionStates: Partial<Record<RouteHealthState, true>> = {
  cooling: true,
  recovering: true,
  saturated: true,
  automatically_disabled: true,
  no_credentials: true,
  invalid_mapping: true,
  credential_invalid: true,
}

export function routeNeedsAttention(route: RouteHealthRow): boolean {
  if (route.channel_status === 2 || route.state === 'manually_disabled')
    return false
  return (
    attentionStates[route.state] === true ||
    route.resources.some(
      (resource) =>
        attentionStates[resource.state] === true ||
        resource.failures > 0 ||
        resource.degraded
    ) ||
    route.attempts.some((attempt) => attempt.verdict === 'failure')
  )
}

export function routeHasObservations(route: RouteHealthRow): boolean {
  return route.resources.some(
    (resource) =>
      resource.scope === 'model' &&
      (resource.successes > 0 ||
        resource.failures > 0 ||
        resource.inflight > 0 ||
        resource.probe)
  )
}

export function groupRouteHealth(
  rows: readonly RouteHealthRow[]
): ChannelHealthGroup[] {
  const channels = new Map<number, ChannelHealthGroup>()
  for (const row of rows) {
    let channel = channels.get(row.channel_id)
    if (!channel) {
      channel = {
        id: row.channel_id,
        name: row.channel_name,
        status: row.channel_status,
        models: [],
        attentionModels: 0,
        observedModels: 0,
      }
      channels.set(channel.id, channel)
    }
    const attention = routeNeedsAttention(row)
    const observed = routeHasObservations(row)
    channel.models.push({
      id: row.id,
      name: row.upstream_model || row.models.join(', '),
      aliases: [...new Set(row.models)].sort(),
      row,
      attention,
      observed,
    })
    channel.attentionModels += Number(attention)
    channel.observedModels += Number(observed)
  }
  for (const channel of channels.values()) {
    channel.models.sort(
      (a, b) =>
        Number(b.attention) - Number(a.attention) ||
        Number(b.observed) - Number(a.observed) ||
        a.name.localeCompare(b.name)
    )
  }
  return [...channels.values()].sort(
    (a, b) =>
      Number(a.status === 2) - Number(b.status === 2) ||
      Number(b.attentionModels > 0) - Number(a.attentionModels > 0) ||
      Number(b.observedModels > 0) - Number(a.observedModels > 0) ||
      a.id - b.id
  )
}

export function selectChannelHealth(
  channels: ChannelHealthGroup[],
  options: { search: string; view: HealthView; showManuallyDisabled: boolean }
): { channel: ChannelHealthGroup; models: ModelHealthGroup[] }[] {
  const terms = options.search.trim().toLowerCase().split(/\s+/).filter(Boolean)
  const result: { channel: ChannelHealthGroup; models: ModelHealthGroup[] }[] =
    []
  for (const channel of channels) {
    if (channel.status === 2 && !options.showManuallyDisabled) continue
    const models = channel.models.filter((model) => {
      if (options.view === 'attention' && !model.attention) return false
      if (options.view === 'unobserved' && (model.observed || model.attention))
        return false
      const attempts = model.row.attempts
      const text = [
        channel.id,
        channel.name,
        model.name,
        ...model.aliases,
        ...attempts.flatMap((attempt) => [attempt.operation, attempt.reason]),
      ]
        .join(' ')
        .toLowerCase()
      return terms.every((term) => text.includes(term))
    })
    if (models.length) result.push({ channel, models })
  }
  return result
}
