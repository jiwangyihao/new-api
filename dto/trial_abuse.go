package dto

const (
	TrialAbuseRiskLevelHigh   = "high"
	TrialAbuseRiskLevelMedium = "medium"
	TrialAbuseRiskLevelLow    = "low"

	TrialAbuseWarningLogUnavailable            = "log_unavailable"
	TrialAbuseWarningRegistrationIPUnavailable = "registration_ip_unavailable"
	TrialAbuseWarningCandidateLimitExceeded    = "candidate_limit_exceeded"
	TrialAbuseWarningLogLimitExceeded          = "log_limit_exceeded"

	TrialAbuseSectionOverview          = "overview"
	TrialAbuseSectionUsageDistribution = "usage_distribution"
	TrialAbuseSectionRiskUsers         = "risk_users"
	TrialAbuseSectionRiskCounts        = "risk_counts"
	TrialAbuseSectionIPClusters        = "ip_clusters"
	TrialAbuseSectionInviterClusters   = "inviter_clusters"
	TrialAbuseSectionSelfInviteChains  = "self_invite_chains"

	TrialAbuseRiskReasonSameRegistrationIPCluster         = "sameRegistrationIpCluster"
	TrialAbuseRiskReasonSameRegistrationIPSelfInviteChain = "sameRegistrationIpSelfInviteChain"
	TrialAbuseRiskReasonInviterLowPaidConversion          = "inviterLowPaidConversion"
	TrialAbuseRiskReasonManagedInviterDisplayOnly         = "managedInviterDisplayOnly"
	TrialAbuseRiskReasonRegistrationIPUnavailable         = "registrationIpUnavailable"
	TrialAbuseRiskReasonLogUnavailable                    = "logUnavailable"
	TrialAbuseRiskReasonCandidateLimitExceeded            = "candidateLimitExceeded"
	TrialAbuseRiskReasonLogLimitExceeded                  = "logLimitExceeded"
)

type TrialAbuseSummaryResponse struct {
	GeneratedAt       int64                       `json:"generated_at"`
	Criteria          TrialAbuseCriteria          `json:"criteria"`
	Warnings          []TrialAbuseWarning         `json:"warnings"`
	PartialSections   map[string][]string         `json:"partial_sections"`
	Overview          TrialAbuseOverview          `json:"overview"`
	RiskCounts        TrialAbuseRiskCounts        `json:"risk_counts"`
	UsageDistribution TrialAbuseUsageDistribution `json:"usage_distribution"`
	IPClusters        []TrialAbuseIPCluster       `json:"ip_clusters"`
	InviterClusters   []TrialAbuseInviterCluster  `json:"inviter_clusters"`
	SelfInviteChains  []TrialAbuseSelfInviteChain `json:"self_invite_chains"`
	RiskUsers         []TrialAbuseRiskUser        `json:"risk_users"`
}

type TrialAbuseCriteria struct {
	TrialEndStart   int64 `json:"trial_end_start"`
	TrialEndEnd     int64 `json:"trial_end_end"`
	RegisteredStart int64 `json:"registered_start,omitempty"`
	RegisteredEnd   int64 `json:"registered_end,omitempty"`
	SnapshotAt      int64 `json:"snapshot_at"`
	MinConsumeCount int   `json:"min_consume_count"`
	MinClusterSize  int   `json:"min_cluster_size"`
	RiskLimit       int   `json:"risk_limit"`
	GroupLimit      int   `json:"group_limit"`
}

type TrialAbuseWarning struct {
	Section string `json:"section"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type TrialAbusePartial struct {
	Partial        bool     `json:"partial"`
	PartialReasons []string `json:"partial_reasons"`
}

type TrialAbuseOverview struct {
	TrialAbusePartial
	TotalTrialUsers            int `json:"total_trial_users"`
	ActiveTrialUsers           int `json:"active_trial_users"`
	ExpiredTrialUsers          int `json:"expired_trial_users"`
	ExpiredUnpaidTrialUsers    int `json:"expired_unpaid_trial_users"`
	HighUsageCandidateUsers    int `json:"high_usage_candidate_users"`
	RiskUserCount              int `json:"risk_user_count"`
	HighRiskUserCount          int `json:"high_risk_user_count"`
	MediumRiskUserCount        int `json:"medium_risk_user_count"`
	LowRiskUserCount           int `json:"low_risk_user_count"`
	ManagedInviterClusterCount int `json:"managed_inviter_cluster_count"`
}

type TrialAbuseRiskCounts struct {
	TrialAbusePartial
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
}

type TrialAbuseUsageDistribution struct {
	TrialAbusePartial
	SampleSize          int `json:"sample_size"`
	ZeroUsageCount      int `json:"zero_usage_count"`
	AboveThresholdCount int `json:"above_threshold_count"`
	P50                 int `json:"p50"`
	P75                 int `json:"p75"`
	P90                 int `json:"p90"`
	P95                 int `json:"p95"`
	P99                 int `json:"p99"`
}

type TrialAbuseRiskUser struct {
	TrialAbusePartial
	UserID                  int      `json:"user_id"`
	Username                string   `json:"username"`
	CreatedAt               int64    `json:"created_at"`
	TrialSource             string   `json:"trial_source"`
	TrialStartTime          int64    `json:"trial_start_time"`
	TrialEndTime            int64    `json:"trial_end_time"`
	InviterID               int      `json:"inviter_id"`
	InviterUsername         string   `json:"inviter_username"`
	ConsumeCount            int      `json:"consume_count"`
	UsedQuota               int64    `json:"used_quota"`
	MeteredTokens           int64    `json:"metered_tokens"`
	ObservedIP              string   `json:"observed_ip"`
	IPSource                string   `json:"ip_source"`
	RegistrationIPAvailable bool     `json:"registration_ip_available"`
	RiskLevel               string   `json:"risk_level"`
	RiskScore               int      `json:"risk_score"`
	RiskReasons             []string `json:"risk_reasons"`
	PaidEntitlementExcluded bool     `json:"paid_entitlement_excluded"`
}

type TrialAbuseIPCluster struct {
	TrialAbusePartial
	ObservedIP              string `json:"observed_ip"`
	IPSource                string `json:"ip_source"`
	RegistrationIPAvailable bool   `json:"registration_ip_available"`
	CandidateCount          int    `json:"candidate_count"`
	ExpiredUnpaidTrialCount int    `json:"expired_unpaid_trial_count"`
	PaidEntitlementCount    int    `json:"paid_entitlement_count"`
	TotalConsumeCount       int    `json:"total_consume_count"`
	SampleUserIDs           []int  `json:"sample_user_ids"`
}

type TrialAbuseInviterCluster struct {
	TrialAbusePartial
	InviterID                int     `json:"inviter_id"`
	InviterUsername          string  `json:"inviter_username"`
	Managed                  bool    `json:"managed"`
	CandidateCount           int     `json:"candidate_count"`
	ExpiredTrialInviteeCount int     `json:"expired_trial_invitee_count"`
	ExpiredUnpaidTrialCount  int     `json:"expired_unpaid_trial_count"`
	PaidEntitlementCount     int     `json:"paid_entitlement_count"`
	PaidConversionRate       float64 `json:"paid_conversion_rate"`
	TotalConsumeCount        int     `json:"total_consume_count"`
	RiskParticipation        string  `json:"risk_participation"`
	SampleUserIDs            []int   `json:"sample_user_ids"`
}

type TrialAbuseSelfInviteChain struct {
	TrialAbusePartial
	ChainID                 string                     `json:"chain_id"`
	RegistrationIPAvailable bool                       `json:"registration_ip_available"`
	RegistrationIP          string                     `json:"registration_ip"`
	CandidateCount          int                        `json:"candidate_count"`
	TotalConsumeCount       int                        `json:"total_consume_count"`
	Nodes                   []TrialAbuseSelfInviteNode `json:"nodes"`
}

type TrialAbuseSelfInviteNode struct {
	UserID    int    `json:"user_id"`
	Username  string `json:"username"`
	InviterID int    `json:"inviter_id"`
}
