export type AdminAnalyticsGranularity = 'hour' | 'day' | 'week' | 'month'
export type AdminAnalyticsSortOrder = 'asc' | 'desc'
export type AdminAnalyticsSource =
  | 'order'
  | 'trial_code'
  | 'invite_trial'
  | 'monthly_invite_entitlement'
  | 'admin'
  | 'redemption'
  | 'system'
  | 'unknown'
export type AdminUsageGroupBy =
  | 'user'
  | 'plan'
  | 'model'
  | 'user_group'
  | 'request_group'
  | 'stream'
  | 'status'
  | 'channel'
  | 'endpoint'
  | 'billing_source'
  | 'token'
  | 'subscription_source'
export type AdminUsageMetric =
  | 'request_count'
  | 'total_tokens'
  | 'quota'
  | 'error_rate'
  | 'avg_latency_ms'
  | 'p95_latency_ms'
  | 'active_users'
  | 'active_api_keys'
export type AdminPlanAttribution = 'current' | 'event_time'
export type AdminAnalyticsTab =
  | 'overview'
  | 'plans'
  | 'quota'
  | 'users'
  | 'conversion'
  | 'invitations'
  | 'usage'
  | 'risks'

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface AdminAnalyticsSearch {
  tab: AdminAnalyticsTab
  start_timestamp: number
  end_timestamp: number
  granularity: AdminAnalyticsGranularity
  user_ids: number[]
  token_ids: number[]
  channel_ids: number[]
  user_groups: string[]
  request_groups: string[]
  plan_ids: number[]
  sources: AdminAnalyticsSource[]
  subscription_statuses: string[]
  user_statuses: string[]
  log_statuses: string[]
  grant_reasons: string[]
  business_codes: string[]
  statuses: string[]
  group_by: AdminUsageGroupBy
  metric: AdminUsageMetric
  plan_attribution: AdminPlanAttribution
  top_n: number
  limit: number
  offset: number
  sort_by?: string
  sort_order: AdminAnalyticsSortOrder
}

export type AdminAnalyticsCanonicalFilters = AdminAnalyticsSearch

export interface AdminAnalyticsRangeMeta {
  start_timestamp: number
  end_timestamp: number
  snapshot_at: number
}

export interface AdminAnalyticsPage {
  limit: number
  offset: number
  total: number
  has_more: boolean
}

export interface AdminAnalyticsList<T> {
  page: AdminAnalyticsPage
  items: T[]
  sort_by: string
  sort_order: AdminAnalyticsSortOrder
}

export interface AdminAnalyticsAvailabilityWarning {
  section: string
  reason: string
  message: string
}

export interface AdminAnalyticsPanelResponse<T> {
  range: AdminAnalyticsRangeMeta
  data: T
  warnings?: AdminAnalyticsAvailabilityWarning[]
}

export interface AdminAnalyticsDrilldownTarget {
  kind: string
  user_id?: number
  user_ids?: number[]
  username?: string
  user_group?: string
  user_status?: string
  plan_id?: number
  inviter_id?: number
  token_id?: number
  model?: string
  request_group?: string
  channel_id?: number
  status?: string
  start_timestamp?: number
  end_timestamp?: number
  tab?: string
}

export interface AdminAnalyticsOverviewUsers {
  total_users: number
  active_users: number
  new_users: number
  disabled_users: number
}

export interface AdminAnalyticsOverviewPlans {
  total_plans: number
  enabled_plans: number
  trial_plans: number
  public_plans: number
}

export interface AdminAnalyticsOverviewQuota {
  token_limit: number
  token_used: number
  remaining_tokens: number
  usage_rate: number | null
}

export interface AdminAnalyticsOverviewConversion {
  trial_users: number
  paid_users: number
  trial_to_paid_users: number
  trial_to_paid_rate: number
  renewal_users: number
}

export interface AdminAnalyticsOverviewInvitations {
  users_with_inviter: number
  inviters_count: number
  direct_invite_count: number
  qualified_invite_count: number
  qualified_inviter_count: number
  reward_users: number
  reward_subscriptions: number
  reward_active_subscription_count: number
  reward_expired_subscription_count: number
}

export interface AdminAnalyticsOverviewUsage {
  request_count: number
  success_count: number
  error_count: number
  error_rate: number
  total_tokens: number
  quota: number
  active_users: number
  active_api_keys: number
}

export interface AdminAnalyticsOverviewRisks {
  critical_count: number
  warning_count: number
  info_count: number
}

export interface AdminAnalyticsOverviewSubscriptions {
  active_count: number
  expired_count: number
  trial_count: number
  paid_count: number
  reward_count: number
}

export interface AdminAnalyticsOverviewSummary {
  users: AdminAnalyticsOverviewUsers
  plans: AdminAnalyticsOverviewPlans
  quota: AdminAnalyticsOverviewQuota
  conversion: AdminAnalyticsOverviewConversion
  invitations: AdminAnalyticsOverviewInvitations
  usage: AdminAnalyticsOverviewUsage
  risks: AdminAnalyticsOverviewRisks
  subscriptions: AdminAnalyticsOverviewSubscriptions
}

export interface AdminAnalyticsOverviewTrendPoint extends AdminAnalyticsOverviewUsage {
  timestamp: number
  new_users: number
  active_subscriptions: number
}

export interface AdminAnalyticsOverviewResponse {
  summary: AdminAnalyticsOverviewSummary
  trends: AdminAnalyticsOverviewTrendPoint[]
}

export interface AdminAnalyticsPlanGroup {
  plan_id: number
  plan_title: string
  plan_business_code: string
  source: AdminAnalyticsSource
  subscription_count: number
  user_count: number
  token_limit: number
  token_used: number
  remaining_tokens: number
  usage_rate: number | null
  token_unlimited: boolean
  share: number
  drilldown?: AdminAnalyticsDrilldownTarget
}

export interface AdminAnalyticsPlanLifecycleTrendPoint {
  timestamp: number
  plan_id: number
  started_count: number
  ended_count: number
  active_count: number
}

export interface AdminAnalyticsPlanHealth {
  plan_id: number
  plan_title: string
  active_subscriptions: number
  expiring_soon: number
  high_usage_count: number
  idle_count: number
  risk_keys: string[]
}

export interface AdminAnalyticsPlanDistributionResponse {
  groups: AdminAnalyticsList<AdminAnalyticsPlanGroup>
  other?: AdminAnalyticsPlanGroup | null
  lifecycle_trends: AdminAnalyticsPlanLifecycleTrendPoint[]
  health: AdminAnalyticsPlanHealth[]
}

export interface AdminAnalyticsQuotaBucket {
  bucket: string
  subscription_count: number
  user_count: number
  token_limit: number
  token_used: number
  usage_rate: number | null
}

export interface AdminAnalyticsQuotaTrendPoint {
  timestamp: number
  bucket: string
  token_limit: number
  token_used: number
  usage_rate: number | null
}

export interface AdminAnalyticsSubscriptionRankingItem {
  subscription_id: number
  user_id: number
  username: string
  user_group: string
  plan_id: number
  plan_title: string
  source: AdminAnalyticsSource
  status: string
  start_time: number
  end_time: number
  token_limit: number
  token_used: number
  remaining_tokens: number
  usage_rate: number | null
  request_count: number
  drilldown?: AdminAnalyticsDrilldownTarget
}

export interface AdminAnalyticsQuotaDistributionResponse {
  buckets: AdminAnalyticsQuotaBucket[]
  trends: AdminAnalyticsQuotaTrendPoint[]
  high_usage_users: AdminAnalyticsList<AdminAnalyticsSubscriptionRankingItem>
  idle_subscriptions: AdminAnalyticsList<AdminAnalyticsSubscriptionRankingItem>
  exhausting_subscriptions: AdminAnalyticsList<AdminAnalyticsSubscriptionRankingItem>
}

export interface AdminAnalyticsUserLifecycleSummary {
  total_users: number
  new_users: number
  active_users: number
  paid_users: number
  trial_users: number
  reward_users: number
  disabled_users: number
}

export interface AdminAnalyticsUserLifecycleTrendPoint {
  timestamp: number
  new_users: number
  active_users: number
  paid_users: number
  trial_users: number
}

export interface AdminAnalyticsUserGroupDistribution {
  group: string
  user_count: number
  share: number
}

export interface AdminAnalyticsRequestGroupDistribution {
  group: string
  request_count: number
  total_tokens: number
  share: number
}

export interface AdminAnalyticsUserLifecycleItem {
  user_id: number
  username: string
  display_name: string
  email: string
  user_group: string
  status: number
  created_at: number
  last_login_at: number
  active_plan_id: number
  active_plan_title: string
  active_source: AdminAnalyticsSource
  token_limit: number
  token_used: number
  request_count: number
  drilldown?: AdminAnalyticsDrilldownTarget
}

export interface AdminAnalyticsUserLifecycleResponse {
  summary: AdminAnalyticsUserLifecycleSummary
  trends: AdminAnalyticsUserLifecycleTrendPoint[]
  user_groups: AdminAnalyticsUserGroupDistribution[]
  request_groups: AdminAnalyticsRequestGroupDistribution[]
  users: AdminAnalyticsList<AdminAnalyticsUserLifecycleItem>
}

export interface AdminAnalyticsSubscriptionConversionSummary {
  trial_users: number
  paid_users: number
  trial_to_paid_users: number
  trial_to_paid_rate: number
  renewal_users: number
  churned_users: number
}

export interface AdminAnalyticsConversionTrendPoint {
  timestamp: number
  users: number
  rate: number
}

export interface AdminAnalyticsPlanMigrationItem {
  from_plan_id: number
  from_plan_title: string
  to_plan_id: number
  to_plan_title: string
  user_count: number
}

export interface AdminAnalyticsSubscriptionConversionResponse {
  summary: AdminAnalyticsSubscriptionConversionSummary
  trial_to_paid: AdminAnalyticsConversionTrendPoint[]
  renewals: AdminAnalyticsConversionTrendPoint[]
  migration_matrix: AdminAnalyticsPlanMigrationItem[]
}

export interface AdminAnalyticsInvitationRewardsSummary {
  users_with_inviter: number
  inviters_count: number
  direct_invite_count: number
  qualified_invite_count: number
  reward_users: number
  reward_subscriptions: number
}

export interface AdminAnalyticsInviterItem {
  inviter_id: number
  inviter_username: string
  direct_invite_count: number
  qualified_invite_count: number
  reward_subscription_id: number
  reward_plan_id: number
  reward_plan_title: string
  reward_status: string
  drilldown?: AdminAnalyticsDrilldownTarget
}

export interface AdminAnalyticsInvitationRewardStatus {
  status: string
  inviter_count: number
  subscription_count: number
}

export interface AdminAnalyticsInvitationTrendPoint {
  timestamp: number
  direct_invite_count: number
  qualified_invite_count: number
  reward_subscription_count: number
}

export interface AdminAnalyticsInvitationRewardsResponse {
  summary: AdminAnalyticsInvitationRewardsSummary
  inviters: AdminAnalyticsList<AdminAnalyticsInviterItem>
  reward_status: AdminAnalyticsInvitationRewardStatus[]
  trends: AdminAnalyticsInvitationTrendPoint[]
}

export interface AdminUsageMetrics {
  request_count: number
  success_count: number
  error_count: number
  success_rate: number
  error_rate: number
  quota: number
  prompt_tokens: number
  completion_tokens: number
  metered_tokens: number
  total_tokens: number
  avg_latency_ms: number
  p95_latency_ms: number
  rpm: number
  tpm: number
  active_users: number
  active_api_keys: number
  first_used_at: number
  last_used_at: number
}

export interface AdminUsageGroup extends AdminUsageMetrics {
  group_by: AdminUsageGroupBy
  group_key: string
  group_value: string
  group_label: string
  share: number | null
  drilldown?: AdminAnalyticsDrilldownTarget
}

export interface AdminUsageConsumptionSummaryResponse {
  total: AdminUsageMetrics
  groups: AdminAnalyticsList<AdminUsageGroup>
  group_by: AdminUsageGroupBy
  other?: AdminUsageGroup | null
}

export interface AdminUsageTimeseriesPoint extends AdminUsageGroup {
  timestamp: number
  time_label: string
}

export interface AdminUsageTimeseriesResponse {
  points: AdminUsageTimeseriesPoint[]
  granularity: AdminAnalyticsGranularity
  group_by: AdminUsageGroupBy
}

export interface AdminUsageBreakdownResponse {
  groups: AdminAnalyticsList<AdminUsageGroup>
  group_by: AdminUsageGroupBy
  other?: AdminUsageGroup | null
}

export interface AdminAnalyticsRiskItem {
  risk_key: string
  severity: 'info' | 'warning' | 'critical'
  category: 'plan' | 'user' | 'invitation' | 'system'
  title: string
  description: string
  threshold: string
  sample_size: number
  value: number
  drilldown?: AdminAnalyticsDrilldownTarget
}

export interface AdminAnalyticsRisksResponse {
  plan_risks: AdminAnalyticsList<AdminAnalyticsRiskItem>
  user_risks: AdminAnalyticsList<AdminAnalyticsRiskItem>
  invitation_risks: AdminAnalyticsList<AdminAnalyticsRiskItem>
  system_risks: AdminAnalyticsList<AdminAnalyticsRiskItem>
}

export interface FrontendAdminAnalyticsDrilldownTarget {
  to: '/users' | '/usage-logs/$section' | '/admin-analytics'
  params?: { section: 'common' }
  search: Record<string, unknown>
}
