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
	AdminAnalyticsSourceCreditBalancePool        AdminAnalyticsSource = "credit_balance_pool"
	AdminAnalyticsSourceUnknown                  AdminAnalyticsSource = "unknown"
)

type AdminAnalyticsExcludedMode string

const (
	AdminAnalyticsExcludedModeIncludedOnly    AdminAnalyticsExcludedMode = "included_only"
	AdminAnalyticsExcludedModeIncludeExcluded AdminAnalyticsExcludedMode = "include_excluded"
	AdminAnalyticsExcludedModeExcludedOnly    AdminAnalyticsExcludedMode = "excluded_only"
)

type AdminAnalyticsMoneyAmount struct {
	Amount       float64 `json:"amount"`
	AmountMicros string  `json:"amount_micros,omitempty"`
	Currency     string  `json:"currency"`
}

type AdminAnalyticsMoneyBreakdown struct {
	Amount       float64 `json:"amount"`
	AmountMicros string  `json:"amount_micros,omitempty"`
	Currency     string  `json:"currency"`
}

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
	AvailableCredit int64    `json:"available_credit"`
	SettlementDebt  int64    `json:"settlement_debt"`
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
	ActiveCount          int `json:"active_count"`
	ExpiredCount         int `json:"expired_count"`
	TrialCount           int `json:"trial_count"`
	PaidCount            int `json:"paid_count"`
	RewardCount          int `json:"reward_count"`
	TimedActiveCount     int `json:"timed_active_count"`
	CreditBalanceCount   int `json:"credit_balance_count"`
	CreditAvailableCount int `json:"credit_available_count"`
	CreditExhaustedCount int `json:"credit_exhausted_count"`
	CreditDebtCount      int `json:"credit_debt_count"`
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
	EntitlementType string                         `json:"entitlement_type"`
	LifecycleState  string                         `json:"lifecycle_state"`
	AvailableCredit int64                          `json:"available_credit"`
	SettlementDebt  int64                          `json:"settlement_debt"`
}

type AdminAnalyticsUserLifecycleResponse struct {
	Summary AdminAnalyticsUserLifecycleSummary                  `json:"summary"`
	Trends  []AdminAnalyticsUserLifecycleTrendPoint             `json:"trends"`
	Users   AdminAnalyticsList[AdminAnalyticsUserLifecycleItem] `json:"users"`
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

type AdminAnalyticsUserLifecycleItem struct {
	UserID          int                            `json:"user_id"`
	Username        string                         `json:"username"`
	DisplayName     string                         `json:"display_name"`
	Email           string                         `json:"email"`
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

type AdminPaidSubscriptionValueResponse struct {
	Summary       AdminPaidSubscriptionValueSummary                          `json:"summary"`
	Users         AdminAnalyticsList[AdminPaidSubscriptionValueUser]         `json:"users"`
	Subscriptions AdminAnalyticsList[AdminPaidSubscriptionValueSubscription] `json:"subscriptions"`
	Plans         AdminAnalyticsList[AdminPaidSubscriptionValuePlanGroup]    `json:"plans"`
	Sources       AdminAnalyticsList[AdminPaidSubscriptionValueSourceGroup]  `json:"sources"`
}

type AdminPaidSubscriptionValueSummary struct {
	RecognizedRemainingValueByCurrency []AdminAnalyticsMoneyBreakdown `json:"recognized_remaining_value_by_currency"`
	TokenBasedValueByCurrency          []AdminAnalyticsMoneyBreakdown `json:"token_based_value_by_currency"`
	TimeBasedValueByCurrency           []AdminAnalyticsMoneyBreakdown `json:"time_based_value_by_currency"`
	ExactRemainingValueByCurrency      []AdminAnalyticsMoneyBreakdown `json:"exact_remaining_value_by_currency"`
	EstimatedRemainingValueByCurrency  []AdminAnalyticsMoneyBreakdown `json:"estimated_remaining_value_by_currency"`
	ExcludedRemainingValueByCurrency   []AdminAnalyticsMoneyBreakdown `json:"excluded_remaining_value_by_currency"`
	ActivePaidSubscriptionCount        int                            `json:"active_paid_subscription_count"`
	ActivePaidUserCount                int                            `json:"active_paid_user_count"`
	TokenValueUnavailableCount         int                            `json:"token_value_unavailable_count"`
	UnknownCostCredit                  int64                          `json:"unknown_cost_credit"`
	UnknownTimedSubscriptionCount      int                            `json:"unknown_timed_subscription_count"`
	CreditValuationStateMissingCount   int                            `json:"credit_valuation_state_missing_count"`
}

type AdminPaidSubscriptionValuePlanGroup struct {
	PlanID                             int                            `json:"plan_id"`
	PlanName                           string                         `json:"plan_name"`
	PlanBusinessCode                   string                         `json:"plan_business_code"`
	ActiveUserCount                    int                            `json:"active_user_count"`
	ActiveSubscriptionCount            int                            `json:"active_subscription_count"`
	RecognizedRemainingValueByCurrency []AdminAnalyticsMoneyBreakdown `json:"recognized_remaining_value_by_currency"`
	TokenBasedValueByCurrency          []AdminAnalyticsMoneyBreakdown `json:"token_based_value_by_currency"`
	TimeBasedValueByCurrency           []AdminAnalyticsMoneyBreakdown `json:"time_based_value_by_currency"`
	ExactRemainingValueByCurrency      []AdminAnalyticsMoneyBreakdown `json:"exact_remaining_value_by_currency"`
	EstimatedRemainingValueByCurrency  []AdminAnalyticsMoneyBreakdown `json:"estimated_remaining_value_by_currency"`
	UnknownCostCredit                  int64                          `json:"unknown_cost_credit"`
	ExcludedRemainingValueByCurrency   []AdminAnalyticsMoneyBreakdown `json:"excluded_remaining_value_by_currency"`
	AverageTokenUsageRatio             *float64                       `json:"average_token_usage_ratio"`
}

type AdminPaidSubscriptionValueSourceGroup struct {
	Source                             AdminAnalyticsSource           `json:"source"`
	GrantReason                        string                         `json:"grant_reason"`
	UserCount                          int                            `json:"user_count"`
	SubscriptionCount                  int                            `json:"subscription_count"`
	RecognizedRemainingValueByCurrency []AdminAnalyticsMoneyBreakdown `json:"recognized_remaining_value_by_currency"`
	ExactRemainingValueByCurrency      []AdminAnalyticsMoneyBreakdown `json:"exact_remaining_value_by_currency"`
	EstimatedRemainingValueByCurrency  []AdminAnalyticsMoneyBreakdown `json:"estimated_remaining_value_by_currency"`
	UnknownCostCredit                  int64                          `json:"unknown_cost_credit"`
	ExcludedRemainingValueByCurrency   []AdminAnalyticsMoneyBreakdown `json:"excluded_remaining_value_by_currency"`
	SourceAttribution                  string                         `json:"source_attribution"`
}

type AdminPaidSubscriptionValueUser struct {
	UserID                             int                            `json:"user_id"`
	Username                           string                         `json:"username"`
	DisplayName                        string                         `json:"display_name"`
	ActivePaidPlanCount                int                            `json:"active_paid_plan_count"`
	RecognizedRemainingValueByCurrency []AdminAnalyticsMoneyBreakdown `json:"recognized_remaining_value_by_currency"`
	TokenBasedValueByCurrency          []AdminAnalyticsMoneyBreakdown `json:"token_based_value_by_currency"`
	TimeBasedValueByCurrency           []AdminAnalyticsMoneyBreakdown `json:"time_based_value_by_currency"`
	ExactRemainingValueByCurrency      []AdminAnalyticsMoneyBreakdown `json:"exact_remaining_value_by_currency"`
	EstimatedRemainingValueByCurrency  []AdminAnalyticsMoneyBreakdown `json:"estimated_remaining_value_by_currency"`
	UnknownCostCredit                  int64                          `json:"unknown_cost_credit"`
	EarliestEndTime                    int64                          `json:"earliest_end_time"`
	Excluded                           bool                           `json:"excluded"`
	ExcludedReason                     string                         `json:"excluded_reason"`
	ExcludedAt                         int64                          `json:"excluded_at"`
	ExcludedBy                         int                            `json:"excluded_by"`
	WouldHaveRemainingValueByCurrency  []AdminAnalyticsMoneyBreakdown `json:"would_have_remaining_value_by_currency"`
	Drilldown                          *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminPaidSubscriptionValueSubscription struct {
	SubscriptionID                     int                            `json:"subscription_id"`
	UserID                             int                            `json:"user_id"`
	Username                           string                         `json:"username"`
	PlanID                             int                            `json:"plan_id"`
	PlanName                           string                         `json:"plan_name"`
	EntitlementType                    string                         `json:"entitlement_type"`
	Source                             AdminAnalyticsSource           `json:"source"`
	GrantReason                        string                         `json:"grant_reason"`
	PlanPrice                          AdminAnalyticsMoneyAmount      `json:"plan_price"`
	StartTime                          int64                          `json:"start_time"`
	EndTime                            int64                          `json:"end_time"`
	RemainingSeconds                   int64                          `json:"remaining_seconds"`
	TokenLimit                         int64                          `json:"token_limit"`
	TokenUsed                          int64                          `json:"token_used"`
	AvailableCredit                    int64                          `json:"available_credit"`
	UnknownCostCredit                  int64                          `json:"unknown_cost_credit"`
	NextResetTime                      int64                          `json:"next_reset_time"`
	TokenBasedValue                    *AdminAnalyticsMoneyAmount     `json:"token_based_value"`
	TimeBasedValue                     *AdminAnalyticsMoneyAmount     `json:"time_based_value"`
	RecognizedRemainingValue           *AdminAnalyticsMoneyAmount     `json:"recognized_remaining_value"`
	TokenBasedValueByCurrency          []AdminAnalyticsMoneyBreakdown `json:"token_based_value_by_currency"`
	TimeBasedValueByCurrency           []AdminAnalyticsMoneyBreakdown `json:"time_based_value_by_currency"`
	RecognizedRemainingValueByCurrency []AdminAnalyticsMoneyBreakdown `json:"recognized_remaining_value_by_currency"`
	ExactRemainingValue                AdminAnalyticsMoneyAmount      `json:"exact_remaining_value"`
	EstimatedRemainingValue            AdminAnalyticsMoneyAmount      `json:"estimated_remaining_value"`
	ValuationBasis                     string                         `json:"valuation_basis"`
	ValuationConfidence                string                         `json:"valuation_confidence"`
	ValuationWarnings                  []string                       `json:"valuation_warnings,omitempty"`
	ValuationStateVersion              int64                          `json:"valuation_state_version"`
	ValuationUpdatedAt                 int64                          `json:"valuation_updated_at"`
	SnapshotSemantics                  string                         `json:"snapshot_semantics"`
	SourceAttribution                  string                         `json:"source_attribution"`
	Excluded                           bool                           `json:"excluded"`
	ExcludedReason                     string                         `json:"excluded_reason"`
	Drilldown                          *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
	PossibleOrderID                    *int                           `json:"possible_order_id"`
	PaymentProvider                    string                         `json:"payment_provider"`
	PaymentMethod                      string                         `json:"payment_method"`
	OrderRecordedAmount                *AdminAnalyticsMoneyAmount     `json:"order_recorded_amount"`
}

type AdminInvitationPaidSubscriptionsResponse struct {
	Summary       AdminInvitationPaidSubscriptionsSummary                   `json:"summary"`
	Inviters      AdminAnalyticsList[AdminInvitationPaidInviter]            `json:"inviters"`
	Invitees      AdminAnalyticsList[AdminInvitationPaidInvitee]            `json:"invitees"`
	Subscriptions AdminAnalyticsList[AdminInvitationPaidSubscriptionRecord] `json:"subscriptions"`
}

type AdminInvitationPaidSubscriptionsSummary struct {
	RecognizedInvitationPaidAmountByCurrency []AdminAnalyticsMoneyBreakdown `json:"recognized_invitation_paid_amount_by_currency"`
	ActiveInvitationPaidAmountByCurrency     []AdminAnalyticsMoneyBreakdown `json:"active_invitation_paid_amount_by_currency"`
	ActiveInvitationRemainingValueByCurrency []AdminAnalyticsMoneyBreakdown `json:"active_invitation_remaining_value_by_currency"`
	ExcludedInvitationPaidAmountByCurrency   []AdminAnalyticsMoneyBreakdown `json:"excluded_invitation_paid_amount_by_currency"`
	ExcludedActiveRemainingValueByCurrency   []AdminAnalyticsMoneyBreakdown `json:"excluded_active_remaining_value_by_currency"`
	InviterCount                             int                            `json:"inviter_count"`
	InviteeCount                             int                            `json:"invitee_count"`
	PaidInviteeCount                         int                            `json:"paid_invitee_count"`
	ActivePaidInviteeCount                   int                            `json:"active_paid_invitee_count"`
}

type AdminInvitationPaidInviter struct {
	InviterUserID                            int                            `json:"inviter_user_id"`
	InviterUsername                          string                         `json:"inviter_username"`
	InviteeCount                             int                            `json:"invitee_count"`
	PaidInviteeCount                         int                            `json:"paid_invitee_count"`
	ActivePaidInviteeCount                   int                            `json:"active_paid_invitee_count"`
	RecognizedInvitationPaidAmountByCurrency []AdminAnalyticsMoneyBreakdown `json:"recognized_invitation_paid_amount_by_currency"`
	ActiveInvitationPaidAmountByCurrency     []AdminAnalyticsMoneyBreakdown `json:"active_invitation_paid_amount_by_currency"`
	ActiveInvitationRemainingValueByCurrency []AdminAnalyticsMoneyBreakdown `json:"active_invitation_remaining_value_by_currency"`
	ExcludedInvitationPaidAmountByCurrency   []AdminAnalyticsMoneyBreakdown `json:"excluded_invitation_paid_amount_by_currency"`
	ExcludedActiveRemainingValueByCurrency   []AdminAnalyticsMoneyBreakdown `json:"excluded_active_remaining_value_by_currency"`
	LatestPaidSubscriptionTime               int64                          `json:"latest_paid_subscription_time"`
	Drilldown                                *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminInvitationPaidInvitee struct {
	InviteeUserID                           int                            `json:"invitee_user_id"`
	InviteeUsername                         string                         `json:"invitee_username"`
	InviterUserID                           int                            `json:"inviter_user_id"`
	RegisteredAt                            int64                          `json:"registered_at"`
	PaidSubscriptionSnapshotCount           int                            `json:"paid_subscription_snapshot_count"`
	RecognizedPaidUnits                     float64                        `json:"recognized_paid_units"`
	ActivePaidSubscriptionCount             int                            `json:"active_paid_subscription_count"`
	RecognizedPaidAmountByCurrency          []AdminAnalyticsMoneyBreakdown `json:"recognized_paid_amount_by_currency"`
	ActiveRemainingValueByCurrency          []AdminAnalyticsMoneyBreakdown `json:"active_remaining_value_by_currency"`
	ActivePaidAmountByCurrency              []AdminAnalyticsMoneyBreakdown `json:"active_paid_amount_by_currency"`
	Excluded                                bool                           `json:"excluded"`
	ExcludedReason                          string                         `json:"excluded_reason"`
	ExcludedAt                              int64                          `json:"excluded_at"`
	ExcludedBy                              int                            `json:"excluded_by"`
	WouldHavePaidAmountByCurrency           []AdminAnalyticsMoneyBreakdown `json:"would_have_paid_amount_by_currency"`
	WouldHaveActiveRemainingValueByCurrency []AdminAnalyticsMoneyBreakdown `json:"would_have_active_remaining_value_by_currency"`
	Drilldown                               *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
}

type AdminInvitationPaidSubscriptionRecord struct {
	SubscriptionID           int                            `json:"subscription_id"`
	InviteeUserID            int                            `json:"invitee_user_id"`
	InviterUserID            int                            `json:"inviter_user_id"`
	PlanID                   int                            `json:"plan_id"`
	PlanName                 string                         `json:"plan_name"`
	PlanPrice                AdminAnalyticsMoneyAmount      `json:"plan_price"`
	RecognizedPaidUnits      float64                        `json:"recognized_paid_units"`
	RecognizedPaidAmount     AdminAnalyticsMoneyAmount      `json:"recognized_paid_amount"`
	UnitInferenceBasis       string                         `json:"unit_inference_basis"`
	Source                   AdminAnalyticsSource           `json:"source"`
	GrantReason              string                         `json:"grant_reason"`
	SourceAttribution        string                         `json:"source_attribution"`
	StartTime                int64                          `json:"start_time"`
	EndTime                  int64                          `json:"end_time"`
	Status                   string                         `json:"status"`
	RecognizedRemainingValue *AdminAnalyticsMoneyAmount     `json:"recognized_remaining_value"`
	Excluded                 bool                           `json:"excluded"`
	ExcludedReason           string                         `json:"excluded_reason"`
	PossibleOrderID          *int                           `json:"possible_order_id"`
	PaymentProvider          string                         `json:"payment_provider"`
	PaymentMethod            string                         `json:"payment_method"`
	OrderRecordedAmount      *AdminAnalyticsMoneyAmount     `json:"order_recorded_amount"`
	OrderStatus              string                         `json:"order_status"`
	CompleteTime             int64                          `json:"complete_time"`
	Drilldown                *AdminAnalyticsDrilldownTarget `json:"drilldown,omitempty"`
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
	UserStatus     string `json:"user_status,omitempty"`
	PlanID         *int   `json:"plan_id,omitempty"`
	SubscriptionID *int   `json:"subscription_id,omitempty"`
	InviteeID      *int   `json:"invitee_id,omitempty"`
	InviterID      *int   `json:"inviter_id,omitempty"`
	TokenID        *int   `json:"token_id,omitempty"`
	Model          string `json:"model,omitempty"`
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
	SubscriptionID        int                  `json:"subscription_id"`
	UserID                int                  `json:"user_id"`
	Username              string               `json:"username"`
	PlanID                int                  `json:"plan_id"`
	PlanTitle             string               `json:"plan_title"`
	Source                AdminAnalyticsSource `json:"source"`
	Status                string               `json:"status"`
	StartTime             int64                `json:"start_time"`
	EndTime               int64                `json:"end_time"`
	TokenLimit            int64                `json:"token_limit"`
	TokenUsed             int64                `json:"token_used"`
	RemainingTokens       int64                `json:"remaining_tokens"`
	UsageRate             *float64             `json:"usage_rate"`
	EntitlementType       string               `json:"entitlement_type"`
	LifecycleState        string               `json:"lifecycle_state"`
	AvailableCredit       int64                `json:"available_credit"`
	SettlementDebt        int64                `json:"settlement_debt"`
	GraceRemainingSeconds int64                `json:"grace_remaining_seconds"`
	ConversionID          int                  `json:"conversion_id"`
	TargetSubscriptionID  int                  `json:"target_subscription_id"`
	TargetUserID          int                  `json:"target_user_id"`
	TargetPlanID          int                  `json:"target_plan_id"`
	TargetPlanTitle       string               `json:"target_plan_title"`
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
