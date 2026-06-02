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

export type TrialAbuseRiskLevel = 'high' | 'medium' | 'low'

export type TrialAbuseWarningReason =
  | 'log_unavailable'
  | 'registration_ip_unavailable'
  | 'candidate_limit_exceeded'
  | 'log_limit_exceeded'

export type TrialAbuseSection =
  | 'overview'
  | 'usage_distribution'
  | 'risk_users'
  | 'risk_counts'
  | 'ip_clusters'
  | 'inviter_clusters'
  | 'self_invite_chains'

export type TrialAbuseRiskReason =
  | 'sameRegistrationIpCluster'
  | 'sameRegistrationIpSelfInviteChain'
  | 'inviterLowPaidConversion'
  | 'managedInviterDisplayOnly'
  | 'registrationIpUnavailable'
  | 'logUnavailable'
  | 'candidateLimitExceeded'
  | 'logLimitExceeded'

export type TrialAbuseSummaryParams = {
  trial_end_start: number
  trial_end_end: number
  registered_start?: number
  registered_end?: number
  snapshot_at?: number
  min_consume_count: number
  min_cluster_size: number
  risk_limit?: number
  group_limit?: number
}

export type TrialAbuseCriteria = {
  trial_end_start: number
  trial_end_end: number
  registered_start?: number
  registered_end?: number
  snapshot_at: number
  min_consume_count: number
  min_cluster_size: number
  risk_limit: number
  group_limit: number
}

export type TrialAbuseWarning = {
  section: TrialAbuseSection
  reason: TrialAbuseWarningReason
  message: string
}

export type TrialAbusePartial = {
  partial: boolean
  partial_reasons: TrialAbuseWarningReason[]
}

export type TrialAbuseOverview = TrialAbusePartial & {
  total_trial_users: number
  active_trial_users: number
  expired_trial_users: number
  expired_unpaid_trial_users: number
  high_usage_candidate_users: number
  risk_user_count: number
  high_risk_user_count: number
  medium_risk_user_count: number
  low_risk_user_count: number
  managed_inviter_cluster_count: number
}

export type TrialAbuseRiskCounts = TrialAbusePartial & {
  high: number
  medium: number
  low: number
}

export type TrialAbuseUsageDistribution = TrialAbusePartial & {
  sample_size: number
  zero_usage_count: number
  above_threshold_count: number
  p50: number
  p75: number
  p90: number
  p95: number
  p99: number
}

export type TrialAbuseRiskUser = TrialAbusePartial & {
  user_id: number
  username: string
  created_at: number
  trial_source: string
  trial_start_time: number
  trial_end_time: number
  inviter_id: number
  inviter_username: string
  consume_count: number
  used_quota: number
  metered_tokens: number
  observed_ip: string
  ip_source: string
  registration_ip_available: boolean
  risk_level: TrialAbuseRiskLevel
  risk_score: number
  risk_reasons: TrialAbuseRiskReason[]
  paid_entitlement_excluded: boolean
}

export type TrialAbuseIPCluster = TrialAbusePartial & {
  observed_ip: string
  ip_source: string
  registration_ip_available: boolean
  candidate_count: number
  expired_unpaid_trial_count: number
  paid_entitlement_count: number
  total_consume_count: number
  sample_user_ids: number[]
}

export type TrialAbuseInviterCluster = TrialAbusePartial & {
  inviter_id: number
  inviter_username: string
  managed: boolean
  candidate_count: number
  expired_trial_invitee_count: number
  expired_unpaid_trial_count: number
  paid_entitlement_count: number
  paid_conversion_rate: number
  total_consume_count: number
  risk_participation: string
  sample_user_ids: number[]
}

export type TrialAbuseSelfInviteNode = {
  user_id: number
  username: string
  inviter_id: number
}

export type TrialAbuseSelfInviteChain = TrialAbusePartial & {
  chain_id: string
  registration_ip_available: boolean
  registration_ip: string
  candidate_count: number
  total_consume_count: number
  nodes: TrialAbuseSelfInviteNode[]
}

export type TrialAbuseSummaryResponse = {
  generated_at: number
  criteria: TrialAbuseCriteria
  warnings: TrialAbuseWarning[]
  partial_sections: Partial<Record<TrialAbuseSection, TrialAbuseWarningReason[]>>
  overview: TrialAbuseOverview
  risk_counts: TrialAbuseRiskCounts
  usage_distribution: TrialAbuseUsageDistribution
  ip_clusters: TrialAbuseIPCluster[]
  inviter_clusters: TrialAbuseInviterCluster[]
  self_invite_chains: TrialAbuseSelfInviteChain[]
  risk_users: TrialAbuseRiskUser[]
}

export type TrialAbuseApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}
