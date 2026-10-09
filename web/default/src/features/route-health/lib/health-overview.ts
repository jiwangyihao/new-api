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
  routes: RouteHealthRow[]
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
    )
  )
}

export function routeHasObservations(route: RouteHealthRow): boolean {
  // Shared channel/key successes do not prove this model was observed.
  return route.resources.some(
    (resource) =>
      resource.scope === 'route' &&
      (resource.successes > 0 ||
        resource.failures > 0 ||
        resource.inflight > 0 ||
        resource.probe)
  )
}

export function groupRouteHealth(rows: RouteHealthRow[]): ChannelHealthGroup[] {
  const channels = new Map<number, ChannelHealthGroup>()
  const modelsByChannel = new Map<number, Map<string, ModelHealthGroup>>()
  const seenRoutes = new Set<string>()
  for (const row of rows) {
    if (seenRoutes.has(row.id)) continue
    seenRoutes.add(row.id)
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
      modelsByChannel.set(channel.id, new Map())
    }
    const modelMap = modelsByChannel.get(channel.id)!
    // Invalid mappings must not collapse unrelated configured aliases together.
    const key = row.upstream_model
      ? `model:${row.upstream_model}`
      : `aliases:${JSON.stringify([...row.models].sort())}`
    let model = modelMap.get(key)
    if (!model) {
      model = {
        id: key,
        name: row.upstream_model || row.models.join(', '),
        aliases: [],
        routes: [],
        attention: false,
        observed: false,
      }
      modelMap.set(key, model)
      channel.models.push(model)
    }
    model.routes.push(row)
    for (const alias of row.models) {
      if (!model.aliases.includes(alias)) model.aliases.push(alias)
    }
    model.attention ||= routeNeedsAttention(row)
    model.observed ||= routeHasObservations(row)
  }
  for (const channel of channels.values()) {
    for (const model of channel.models) {
      model.aliases.sort()
      model.routes.sort(
        (a, b) =>
          Number(routeNeedsAttention(b)) - Number(routeNeedsAttention(a)) ||
          Number(routeHasObservations(b)) - Number(routeHasObservations(a)) ||
          a.operation.localeCompare(b.operation)
      )
    }
    channel.models.sort(
      (a, b) =>
        Number(b.attention) - Number(a.attention) ||
        Number(b.observed) - Number(a.observed) ||
        a.name.localeCompare(b.name)
    )
    channel.attentionModels = channel.models.filter(
      (model) => model.attention
    ).length
    channel.observedModels = channel.models.filter(
      (model) => model.observed
    ).length
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
  options: {
    search: string
    view: HealthView
    showManuallyDisabled: boolean
  }
): { channel: ChannelHealthGroup; models: ModelHealthGroup[] }[] {
  const terms = options.search.trim().toLowerCase().split(/\s+/).filter(Boolean)
  const result = []
  for (const channel of channels) {
    if (channel.status === 2 && !options.showManuallyDisabled) continue
    const models = channel.models.filter((model) => {
      if (options.view === 'attention' && !model.attention) return false
      if (options.view === 'unobserved' && (model.observed || model.attention))
        return false
      const text = [
        channel.id,
        channel.name,
        model.name,
        ...model.aliases,
        ...model.routes.map((route) => route.operation),
      ]
        .join(' ')
        .toLowerCase()
      return terms.every((term) => text.includes(term))
    })
    if (models.length) result.push({ channel, models })
  }
  return result
}
