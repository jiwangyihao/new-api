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
// ----------------------------------------------------------------------------
// Rankings types
// ----------------------------------------------------------------------------
//
// Shape of the real data shown on the /rankings page.

export type RankingPeriod = 'today' | 'week' | 'month' | 'year' | 'all'

export type RankingCategoryId =
  | 'all'
  | 'programming'
  | 'roleplay'
  | 'marketing'
  | 'translation'
  | 'science'
  | 'finance'
  | 'health'
  | 'legal'
  | 'education'
  | 'productivity'
  | 'multimodal'

export type ModelRanking = {
  rank: number
  model_name: string
  vendor: string
  vendor_icon?: string
  category: RankingCategoryId
  /** Total tokens routed through this model in the period. */
  total_tokens: number
}

export type FreeUserRanking = {
  rank: number
  display_name: string
  total_tokens: number
  named: boolean
}

/**
 * One sample of a model's token usage at a given timestamp.
 * Flat shape ready to feed VChart's stacked-bar spec.
 */
export type ModelHistoryPoint = {
  ts: string
  /** Pre-formatted x-axis label (e.g. "May 5", "12:00"). */
  label: string
  /** Model display name shown in tooltip / legend. */
  model: string
  vendor: string
  /** Token count routed through the model in this bucket. */
  tokens: number
}

export type ModelHistorySeries = {
  /** Flat points ready for VChart, ordered oldest → newest. */
  points: ModelHistoryPoint[]
  /** Models that appear in the series, sorted by total tokens desc. */
  models: Array<{ name: string; vendor: string; total: number }>
  /** Bucket count (used for sizing axis ticks). */
  buckets: number
}

export type RankingsSnapshot = {
  // Overall (all categories) ------------------------------------------------
  models: ModelRanking[]
  /** Stacked-bar history of token usage by model over the period. */
  models_history: ModelHistorySeries
  /** Token usage ranking for users while they are on a free plan. */
  free_users: FreeUserRanking[]
  /** Aggregate token usage represented by the free-plan leaderboard. */
  free_user_total_tokens: number
}
