package dto

type AdminAnalyticsGranularity string

const (
	AdminAnalyticsGranularityHour  AdminAnalyticsGranularity = "hour"
	AdminAnalyticsGranularityDay   AdminAnalyticsGranularity = "day"
	AdminAnalyticsGranularityWeek  AdminAnalyticsGranularity = "week"
	AdminAnalyticsGranularityMonth AdminAnalyticsGranularity = "month"
)

type AdminAnalyticsSortOrder string

const (
	AdminAnalyticsSortAsc  AdminAnalyticsSortOrder = "asc"
	AdminAnalyticsSortDesc AdminAnalyticsSortOrder = "desc"
)

type AdminAnalyticsSource string

const (
	AdminAnalyticsSourceOrder                    AdminAnalyticsSource = "order"
	AdminAnalyticsSourceTrialCode                AdminAnalyticsSource = "trial_code"
	AdminAnalyticsSourceInviteTrial              AdminAnalyticsSource = "invite_trial"
	AdminAnalyticsSourceMonthlyInviteEntitlement AdminAnalyticsSource = "monthly_invite_entitlement"
	AdminAnalyticsSourceAdmin                    AdminAnalyticsSource = "admin"
	AdminAnalyticsSourceRedemption               AdminAnalyticsSource = "redemption"
	AdminAnalyticsSourceSystem                   AdminAnalyticsSource = "system"
	AdminAnalyticsSourceUnknown                  AdminAnalyticsSource = "unknown"
)

type AdminUsageGroupBy string

const (
	AdminUsageGroupByUser               AdminUsageGroupBy = "user"
	AdminUsageGroupByPlan               AdminUsageGroupBy = "plan"
	AdminUsageGroupByModel              AdminUsageGroupBy = "model"
	AdminUsageGroupByStream             AdminUsageGroupBy = "stream"
	AdminUsageGroupByStatus             AdminUsageGroupBy = "status"
	AdminUsageGroupByChannel            AdminUsageGroupBy = "channel"
	AdminUsageGroupByEndpoint           AdminUsageGroupBy = "endpoint"
	AdminUsageGroupByBillingSource      AdminUsageGroupBy = "billing_source"
	AdminUsageGroupByToken              AdminUsageGroupBy = "token"
	AdminUsageGroupBySubscriptionSource AdminUsageGroupBy = "subscription_source"
)

type AdminUsageMetric string

const (
	AdminUsageMetricRequestCount  AdminUsageMetric = "request_count"
	AdminUsageMetricTotalTokens   AdminUsageMetric = "total_tokens"
	AdminUsageMetricQuota         AdminUsageMetric = "quota"
	AdminUsageMetricErrorRate     AdminUsageMetric = "error_rate"
	AdminUsageMetricAvgLatencyMs  AdminUsageMetric = "avg_latency_ms"
	AdminUsageMetricP95LatencyMs  AdminUsageMetric = "p95_latency_ms"
	AdminUsageMetricActiveUsers   AdminUsageMetric = "active_users"
	AdminUsageMetricActiveAPIKeys AdminUsageMetric = "active_api_keys"
)

type AdminPlanAttribution string

const (
	AdminPlanAttributionCurrent   AdminPlanAttribution = "current"
	AdminPlanAttributionEventTime AdminPlanAttribution = "event_time"
)

type AdminAnalyticsRiskSeverity string

const (
	AdminAnalyticsRiskSeverityInfo     AdminAnalyticsRiskSeverity = "info"
	AdminAnalyticsRiskSeverityWarning  AdminAnalyticsRiskSeverity = "warning"
	AdminAnalyticsRiskSeverityCritical AdminAnalyticsRiskSeverity = "critical"
)

type AdminAnalyticsRiskCategory string

const (
	AdminAnalyticsRiskCategoryPlan       AdminAnalyticsRiskCategory = "plan"
	AdminAnalyticsRiskCategoryUser       AdminAnalyticsRiskCategory = "user"
	AdminAnalyticsRiskCategoryInvitation AdminAnalyticsRiskCategory = "invitation"
	AdminAnalyticsRiskCategorySystem     AdminAnalyticsRiskCategory = "system"
)

type AdminAnalyticsRangeMeta struct {
	StartTimestamp int64 `json:"start_timestamp"`
	EndTimestamp   int64 `json:"end_timestamp"`
	SnapshotAt     int64 `json:"snapshot_at"`
}

type AdminAnalyticsPage struct {
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	Total   int  `json:"total"`
	HasMore bool `json:"has_more"`
}

type AdminAnalyticsList[T any] struct {
	Page      AdminAnalyticsPage      `json:"page"`
	Items     []T                     `json:"items"`
	SortBy    string                  `json:"sort_by"`
	SortOrder AdminAnalyticsSortOrder `json:"sort_order"`
}

type AdminAnalyticsAvailabilityWarning struct {
	Section string `json:"section"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type AdminAnalyticsPanelResponse[T any] struct {
	Range    AdminAnalyticsRangeMeta             `json:"range"`
	Data     T                                   `json:"data"`
	Warnings []AdminAnalyticsAvailabilityWarning `json:"warnings,omitempty"`
}

type AdminAnalyticsOverviewResponse struct {
	Summary AdminAnalyticsOverviewSummary      `json:"summary"`
	Trends  []AdminAnalyticsOverviewTrendPoint `json:"trends"`
}

type AdminAnalyticsOverviewSummary struct {
	Users         AdminAnalyticsOverviewUsers         `json:"users"`
	Plans         AdminAnalyticsOverviewPlans         `json:"plans"`
	Quota         AdminAnalyticsOverviewQuota         `json:"quota"`
	Conversion    AdminAnalyticsOverviewConversion    `json:"conversion"`
	Invitations   AdminAnalyticsOverviewInvitations   `json:"invitations"`
	Usage         AdminAnalyticsOverviewUsage         `json:"usage"`
	Risks         AdminAnalyticsOverviewRisks         `json:"risks"`
	Subscriptions AdminAnalyticsOverviewSubscriptions `json:"subscriptions"`
}

type AdminAnalyticsOverviewUsers struct {
	TotalUsers    int `json:"total_users"`
	ActiveUsers   int `json:"active_users"`
	NewUsers      int `json:"new_users"`
	DisabledUsers int `json:"disabled_users"`
}

type AdminAnalyticsOverviewPlans struct {
	TotalPlans   int `json:"total_plans"`
	EnabledPlans int `json:"enabled_plans"`
	TrialPlans   int `json:"trial_plans"`
	PublicPlans  int `json:"public_plans"`
}

type AdminAnalyticsOverviewQuota struct {
	TokenLimit      int64    `json:"token_limit"`
	TokenUsed       int64    `json:"token_used"`
	RemainingTokens int64    `json:"remaining_tokens"`
	UsageRate       *float64 `json:"usage_rate"`
}

type AdminAnalyticsOverviewConversion struct {
	TrialUsers       int     `json:"trial_users"`
	PaidUsers        int     `json:"paid_users"`
	TrialToPaidUsers int     `json:"trial_to_paid_users"`
	TrialToPaidRate  float64 `json:"trial_to_paid_rate"`
	RenewalUsers     int     `json:"renewal_users"`
}

type AdminAnalyticsOverviewInvitations struct {
	UsersWithInviter               int `json:"users_with_inviter"`
	InvitersCount                  int `json:"inviters_count"`
	DirectInviteCount              int `json:"direct_invite_count"`
	QualifiedInviteCount           int `json:"qualified_invite_count"`
	QualifiedInviterCount          int `json:"qualified_inviter_count"`
	RewardUsers                    int `json:"reward_users"`
	RewardSubscriptions            int `json:"reward_subscriptions"`
	RewardActiveSubscriptionCount  int `json:"reward_active_subscription_count"`
	RewardExpiredSubscriptionCount int `json:"reward_expired_subscription_count"`
}

type AdminAnalyticsOverviewUsage struct {
	RequestCount  int     `json:"request_count"`
	SuccessCount  int     `json:"success_count"`
	ErrorCount    int     `json:"error_count"`
	ErrorRate     float64 `json:"error_rate"`
	TotalTokens   int64   `json:"total_tokens"`
	Quota         int64   `json:"quota"`
	ActiveUsers   int     `json:"active_users"`
	ActiveAPIKeys int     `json:"active_api_keys"`
}

type AdminAnalyticsOverviewRisks struct {
	CriticalCount int `json:"critical_count"`
	WarningCount  int `json:"warning_count"`
	InfoCount     int `json:"info_count"`
}

type AdminAnalyticsOverviewSubscriptions struct {
	ActiveCount  int `json:"active_count"`
	ExpiredCount int `json:"expired_count"`
	TrialCount   int `json:"trial_count"`
	PaidCount    int `json:"paid_count"`
	RewardCount  int `json:"reward_count"`
}

type AdminAnalyticsOverviewTrendPoint struct {
	Timestamp int64 `json:"timestamp"`
	AdminAnalyticsOverviewUsage
	NewUsers            int `json:"new_users"`
	ActiveSubscriptions int `json:"active_subscriptions"`
}

type AdminAnalyticsPlanDistributionResponse struct {
	Groups          AdminAnalyticsList[AdminAnalyticsPlanGroup] `json:"groups"`
	Other           *AdminAnalyticsPlanGroup                    `json:"other,omitempty"`
	LifecycleTrends []AdminAnalyticsPlanLifecycleTrendPoint     `json:"lifecycle_trends"`
	Health          []AdminAnalyticsPlanHealth                  `json:"health"`
}

type AdminAnalyticsPlanGroup struct {
	PlanID            int                            `json:"plan_id"`
	PlanTitle         string                         `json:"plan_title"`
	PlanBusinessCode  string                         `json:"plan_business_code"`
	Source            AdminAnalyticsSource           `json:"source"`
	SubscriptionCount int                            `json:"subscription_count"`
	UserCount         int                            `json:"user_count"`
	TokenLimit        int64                          `json:"token_limit"`
	TokenUsed         int64                          `json:"token_used"`
	RemainingTokens   int64                          `json:"remaining_tokens"`
	UsageRate         *float64                       `json:"usage_rate"`
	TokenUnlimited    bool                           `json:"token_unlimited"`
	Share             float64                        `json:"share"`
	Drilldown         *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminAnalyticsPlanLifecycleTrendPoint struct {
	Timestamp    int64 `json:"timestamp"`
	PlanID       int   `json:"plan_id"`
	StartedCount int   `json:"started_count"`
	EndedCount   int   `json:"ended_count"`
	ActiveCount  int   `json:"active_count"`
}

type AdminAnalyticsPlanHealth struct {
	PlanID              int      `json:"plan_id"`
	PlanTitle           string   `json:"plan_title"`
	ActiveSubscriptions int      `json:"active_subscriptions"`
	ExpiringSoon        int      `json:"expiring_soon"`
	HighUsageCount      int      `json:"high_usage_count"`
	IdleCount           int      `json:"idle_count"`
	RiskKeys            []string `json:"risk_keys"`
}

type AdminAnalyticsQuotaDistributionResponse struct {
	Buckets                 []AdminAnalyticsQuotaBucket                               `json:"buckets"`
	Trends                  []AdminAnalyticsQuotaTrendPoint                           `json:"trends"`
	HighUsageUsers          AdminAnalyticsList[AdminAnalyticsSubscriptionRankingItem] `json:"high_usage_users"`
	IdleSubscriptions       AdminAnalyticsList[AdminAnalyticsSubscriptionRankingItem] `json:"idle_subscriptions"`
	ExhaustingSubscriptions AdminAnalyticsList[AdminAnalyticsSubscriptionRankingItem] `json:"exhausting_subscriptions"`
}

type AdminAnalyticsQuotaBucket struct {
	Bucket            string   `json:"bucket"`
	SubscriptionCount int      `json:"subscription_count"`
	UserCount         int      `json:"user_count"`
	TokenLimit        int64    `json:"token_limit"`
	TokenUsed         int64    `json:"token_used"`
	UsageRate         *float64 `json:"usage_rate"`
}

type AdminAnalyticsQuotaTrendPoint struct {
	Timestamp  int64    `json:"timestamp"`
	Bucket     string   `json:"bucket"`
	TokenLimit int64    `json:"token_limit"`
	TokenUsed  int64    `json:"token_used"`
	UsageRate  *float64 `json:"usage_rate"`
}

type AdminAnalyticsSubscriptionRankingItem struct {
	SubscriptionID  int                            `json:"subscription_id"`
	UserID          int                            `json:"user_id"`
	Username        string                         `json:"username"`
	UserGroup       string                         `json:"-"`
	PlanID          int                            `json:"plan_id"`
	PlanTitle       string                         `json:"plan_title"`
	Source          AdminAnalyticsSource           `json:"source"`
	Status          string                         `json:"status"`
	StartTime       int64                          `json:"start_time"`
	EndTime         int64                          `json:"end_time"`
	TokenLimit      int64                          `json:"token_limit"`
	TokenUsed       int64                          `json:"token_used"`
	RemainingTokens int64                          `json:"remaining_tokens"`
	UsageRate       *float64                       `json:"usage_rate"`
	RequestCount    int                            `json:"request_count"`
	Drilldown       *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminAnalyticsUserLifecycleResponse struct {
	Summary       AdminAnalyticsUserLifecycleSummary                  `json:"summary"`
	Trends        []AdminAnalyticsUserLifecycleTrendPoint             `json:"trends"`
	UserGroups    []AdminAnalyticsUserGroupDistribution               `json:"-"`
	RequestGroups []AdminAnalyticsRequestGroupDistribution            `json:"-"`
	Users         AdminAnalyticsList[AdminAnalyticsUserLifecycleItem] `json:"users"`
}

type AdminAnalyticsUserLifecycleSummary struct {
	TotalUsers    int `json:"total_users"`
	NewUsers      int `json:"new_users"`
	ActiveUsers   int `json:"active_users"`
	PaidUsers     int `json:"paid_users"`
	TrialUsers    int `json:"trial_users"`
	RewardUsers   int `json:"reward_users"`
	DisabledUsers int `json:"disabled_users"`
}

type AdminAnalyticsUserLifecycleTrendPoint struct {
	Timestamp   int64 `json:"timestamp"`
	NewUsers    int   `json:"new_users"`
	ActiveUsers int   `json:"active_users"`
	PaidUsers   int   `json:"paid_users"`
	TrialUsers  int   `json:"trial_users"`
}

type AdminAnalyticsUserGroupDistribution struct {
	Group     string  `json:"group"`
	UserCount int     `json:"user_count"`
	Share     float64 `json:"share"`
}

type AdminAnalyticsRequestGroupDistribution struct {
	Group        string  `json:"group"`
	RequestCount int     `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	Share        float64 `json:"share"`
}

type AdminAnalyticsUserLifecycleItem struct {
	UserID          int                            `json:"user_id"`
	Username        string                         `json:"username"`
	DisplayName     string                         `json:"display_name"`
	Email           string                         `json:"email"`
	UserGroup       string                         `json:"-"`
	Status          int                            `json:"status"`
	CreatedAt       int64                          `json:"created_at"`
	LastLoginAt     int64                          `json:"last_login_at"`
	ActivePlanID    int                            `json:"active_plan_id"`
	ActivePlanTitle string                         `json:"active_plan_title"`
	ActiveSource    AdminAnalyticsSource           `json:"active_source"`
	TokenLimit      int64                          `json:"token_limit"`
	TokenUsed       int64                          `json:"token_used"`
	RequestCount    int                            `json:"request_count"`
	Drilldown       *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminAnalyticsSubscriptionConversionResponse struct {
	Summary         AdminAnalyticsSubscriptionConversionSummary `json:"summary"`
	TrialToPaid     []AdminAnalyticsConversionTrendPoint        `json:"trial_to_paid"`
	Renewals        []AdminAnalyticsConversionTrendPoint        `json:"renewals"`
	MigrationMatrix []AdminAnalyticsPlanMigrationItem           `json:"migration_matrix"`
}

type AdminAnalyticsSubscriptionConversionSummary struct {
	TrialUsers       int     `json:"trial_users"`
	PaidUsers        int     `json:"paid_users"`
	TrialToPaidUsers int     `json:"trial_to_paid_users"`
	TrialToPaidRate  float64 `json:"trial_to_paid_rate"`
	RenewalUsers     int     `json:"renewal_users"`
	ChurnedUsers     int     `json:"churned_users"`
}

type AdminAnalyticsConversionTrendPoint struct {
	Timestamp int64   `json:"timestamp"`
	Users     int     `json:"users"`
	Rate      float64 `json:"rate"`
}

type AdminAnalyticsPlanMigrationItem struct {
	FromPlanID    int    `json:"from_plan_id"`
	FromPlanTitle string `json:"from_plan_title"`
	ToPlanID      int    `json:"to_plan_id"`
	ToPlanTitle   string `json:"to_plan_title"`
	UserCount     int    `json:"user_count"`
}

type AdminAnalyticsInvitationRewardsResponse struct {
	Summary      AdminAnalyticsInvitationRewardsSummary        `json:"summary"`
	Inviters     AdminAnalyticsList[AdminAnalyticsInviterItem] `json:"inviters"`
	RewardStatus []AdminAnalyticsInvitationRewardStatus        `json:"reward_status"`
	Trends       []AdminAnalyticsInvitationTrendPoint          `json:"trends"`
}

type AdminAnalyticsInvitationRewardsSummary struct {
	UsersWithInviter     int `json:"users_with_inviter"`
	InvitersCount        int `json:"inviters_count"`
	DirectInviteCount    int `json:"direct_invite_count"`
	QualifiedInviteCount int `json:"qualified_invite_count"`
	RewardUsers          int `json:"reward_users"`
	RewardSubscriptions  int `json:"reward_subscriptions"`
}

type AdminAnalyticsInviterItem struct {
	InviterID            int                            `json:"inviter_id"`
	InviterUsername      string                         `json:"inviter_username"`
	DirectInviteCount    int                            `json:"direct_invite_count"`
	QualifiedInviteCount int                            `json:"qualified_invite_count"`
	RewardSubscriptionID int                            `json:"reward_subscription_id"`
	RewardPlanID         int                            `json:"reward_plan_id"`
	RewardPlanTitle      string                         `json:"reward_plan_title"`
	RewardStatus         string                         `json:"reward_status"`
	Drilldown            *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminAnalyticsInvitationRewardStatus struct {
	Status            string `json:"status"`
	InviterCount      int    `json:"inviter_count"`
	SubscriptionCount int    `json:"subscription_count"`
}

type AdminAnalyticsInvitationTrendPoint struct {
	Timestamp               int64 `json:"timestamp"`
	DirectInviteCount       int   `json:"direct_invite_count"`
	QualifiedInviteCount    int   `json:"qualified_invite_count"`
	RewardSubscriptionCount int   `json:"reward_subscription_count"`
}

type AdminUsageMetrics struct {
	RequestCount     int     `json:"request_count"`
	SuccessCount     int     `json:"success_count"`
	ErrorCount       int     `json:"error_count"`
	SuccessRate      float64 `json:"success_rate"`
	ErrorRate        float64 `json:"error_rate"`
	Quota            int64   `json:"quota"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	MeteredTokens    int64   `json:"metered_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	AvgLatencyMs     int     `json:"avg_latency_ms"`
	P95LatencyMs     int     `json:"p95_latency_ms"`
	Rpm              int     `json:"rpm"`
	Tpm              int     `json:"tpm"`
	ActiveUsers      int     `json:"active_users"`
	ActiveAPIKeys    int     `json:"active_api_keys"`
	FirstUsedAt      int64   `json:"first_used_at"`
	LastUsedAt       int64   `json:"last_used_at"`
}

type AdminUsageGroup struct {
	AdminUsageMetrics
	GroupBy    AdminUsageGroupBy              `json:"group_by"`
	GroupKey   string                         `json:"group_key"`
	GroupValue string                         `json:"group_value"`
	GroupLabel string                         `json:"group_label"`
	Share      *float64                       `json:"share"`
	Drilldown  *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminUsageConsumptionSummaryResponse struct {
	Total   AdminUsageMetrics                   `json:"total"`
	Groups  AdminAnalyticsList[AdminUsageGroup] `json:"groups"`
	GroupBy AdminUsageGroupBy                   `json:"group_by"`
	Other   *AdminUsageGroup                    `json:"other,omitempty"`
}

type AdminUsageTimeseriesResponse struct {
	Points      []AdminUsageTimeseriesPoint `json:"points"`
	Granularity AdminAnalyticsGranularity   `json:"granularity"`
	GroupBy     AdminUsageGroupBy           `json:"group_by"`
}

type AdminUsageTimeseriesPoint struct {
	Timestamp int64  `json:"timestamp"`
	TimeLabel string `json:"time_label"`
	AdminUsageGroup
}

type AdminUsageBreakdownResponse struct {
	Groups  AdminAnalyticsList[AdminUsageGroup] `json:"groups"`
	GroupBy AdminUsageGroupBy                   `json:"group_by"`
	Other   *AdminUsageGroup                    `json:"other,omitempty"`
}

type AdminAnalyticsRisksResponse struct {
	PlanRisks       AdminAnalyticsList[AdminAnalyticsRiskItem] `json:"plan_risks"`
	UserRisks       AdminAnalyticsList[AdminAnalyticsRiskItem] `json:"user_risks"`
	InvitationRisks AdminAnalyticsList[AdminAnalyticsRiskItem] `json:"invitation_risks"`
	SystemRisks     AdminAnalyticsList[AdminAnalyticsRiskItem] `json:"system_risks"`
}

type AdminAnalyticsRiskItem struct {
	RiskKey     string                         `json:"risk_key"`
	Severity    AdminAnalyticsRiskSeverity     `json:"severity"`
	Category    AdminAnalyticsRiskCategory     `json:"category"`
	Title       string                         `json:"title"`
	Description string                         `json:"description"`
	Threshold   string                         `json:"threshold"`
	SampleSize  int                            `json:"sample_size"`
	Value       float64                        `json:"value"`
	Drilldown   *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminAnalyticsDrilldownTarget struct {
	Kind           string `json:"kind"`
	UserID         *int   `json:"user_id,omitempty"`
	UserIDs        []int  `json:"user_ids,omitempty"`
	Username       string `json:"username,omitempty"`
	UserGroup      string `json:"-"`
	UserStatus     string `json:"user_status,omitempty"`
	PlanID         *int   `json:"plan_id,omitempty"`
	InviterID      *int   `json:"inviter_id,omitempty"`
	TokenID        *int   `json:"token_id,omitempty"`
	Model          string `json:"model,omitempty"`
	RequestGroup   string `json:"-"`
	ChannelID      *int   `json:"channel_id,omitempty"`
	Status         string `json:"status,omitempty"`
	StartTimestamp int64  `json:"start_timestamp,omitempty"`
	EndTimestamp   int64  `json:"end_timestamp,omitempty"`
	Tab            string `json:"tab,omitempty"`
}

type AdminAnalyticsDrilldownUsersResponse struct {
	Users AdminAnalyticsList[AdminAnalyticsDrilldownUserItem] `json:"users"`
}

type AdminAnalyticsDrilldownUserItem struct {
	UserID          int                            `json:"user_id"`
	Username        string                         `json:"username"`
	DisplayName     string                         `json:"display_name"`
	Email           string                         `json:"email"`
	UserGroup       string                         `json:"-"`
	Status          int                            `json:"status"`
	Role            int                            `json:"role"`
	CreatedAt       int64                          `json:"created_at"`
	LastLoginAt     int64                          `json:"last_login_at"`
	InviterID       int                            `json:"inviter_id"`
	ActivePlanID    int                            `json:"active_plan_id"`
	ActivePlanTitle string                         `json:"active_plan_title"`
	Drilldown       *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminAnalyticsDrilldownSubscriptionsResponse struct {
	Subscriptions AdminAnalyticsList[AdminAnalyticsDrilldownSubscriptionItem] `json:"subscriptions"`
}

type AdminAnalyticsDrilldownSubscriptionItem struct {
	SubscriptionID  int                  `json:"subscription_id"`
	UserID          int                  `json:"user_id"`
	Username        string               `json:"username"`
	PlanID          int                  `json:"plan_id"`
	PlanTitle       string               `json:"plan_title"`
	Source          AdminAnalyticsSource `json:"source"`
	Status          string               `json:"status"`
	StartTime       int64                `json:"start_time"`
	EndTime         int64                `json:"end_time"`
	TokenLimit      int64                `json:"token_limit"`
	TokenUsed       int64                `json:"token_used"`
	RemainingTokens int64                `json:"remaining_tokens"`
	UsageRate       *float64             `json:"usage_rate"`
}

type AdminAnalyticsDrilldownInvitationsResponse struct {
	Invitations AdminAnalyticsList[AdminAnalyticsDrilldownInvitationItem] `json:"invitations"`
}

type AdminAnalyticsDrilldownInvitationItem struct {
	InviterID       int    `json:"inviter_id"`
	InviterUsername string `json:"inviter_username"`
	InviteeID       int    `json:"invitee_id"`
	InviteeUsername string `json:"invitee_username"`
	InviteeStatus   int    `json:"invitee_status"`
	Qualified       bool   `json:"qualified"`
	RewardMonth     string `json:"reward_month"`
	RewardStatus    string `json:"reward_status"`
	CreatedAt       int64  `json:"created_at"`
}
