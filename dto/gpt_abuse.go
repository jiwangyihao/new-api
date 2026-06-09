package dto

type GPTAbuseUserListQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	Keyword        string
	UserID         int
	Status         string
	Kind           string
	Severity       string
	Source         string
	Limit          int
	Offset         int
	SortBy         string
	SortOrder      string
}

type GPTAbuseUserListResponse struct {
	Items          []GPTAbuseUserListItem `json:"items"`
	Total          int                     `json:"total"`
	Limit          int                     `json:"limit"`
	Offset         int                     `json:"offset"`
	StartTimestamp int64                   `json:"start_timestamp"`
	EndTimestamp   int64                   `json:"end_timestamp"`
}

type GPTAbuseUserListItem struct {
	UserID                int                        `json:"user_id"`
	Username              string                     `json:"username"`
	UserEmail             string                     `json:"user_email"`
	WarningCount          int                        `json:"warning_count"`
	EffectiveWarningCount int                        `json:"effective_warning_count"`
	DailyLimit            int                        `json:"daily_limit"`
	RemainingWarningCount int                        `json:"remaining_warning_count"`
	HighCount             int                        `json:"high_count"`
	MediumCount           int                        `json:"medium_count"`
	MaxSeverity           string                     `json:"max_severity"`
	LatestWarningAt       int64                      `json:"latest_warning_at"`
	LatestKind            string                     `json:"latest_kind"`
	LatestSource          string                     `json:"latest_source"`
	LatestRequestedModel  string                     `json:"latest_requested_model"`
	LatestUpstreamModel   string                     `json:"latest_upstream_model"`
	LatestChannelID       int                        `json:"latest_channel_id"`
	LatestChannelName     string                     `json:"latest_channel_name"`
	SuspensionStatus      string                     `json:"suspension_status"`
	ActiveSuspension      *GPTAbuseActiveSuspension  `json:"active_suspension"`
	LastResetAt           int64                      `json:"last_reset_at"`
	LastResetBy           int                        `json:"last_reset_by"`
	RepeatBlockCount      int                        `json:"repeat_block_count"`
	LatestRepeatBlockAt   int64                      `json:"latest_repeat_block_at"`
}

type GPTAbuseActiveSuspension struct {
	ID             int    `json:"id"`
	Reason         string `json:"reason"`
	SuspendedUntil int64  `json:"suspended_until"`
	DailyCount     int    `json:"daily_count"`
	DailyLimit     int    `json:"daily_limit"`
}

type GPTAbuseLogQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	Source         string
	Kind           string
	Severity       string
	CountEligible  string
	Limit          int
	Offset         int
}

type GPTAbuseLogListResponse struct {
	Items          []GPTAbuseSignalLogItem `json:"items"`
	Total          int                     `json:"total"`
	Limit          int                     `json:"limit"`
	Offset         int                     `json:"offset"`
	StartTimestamp int64                   `json:"start_timestamp"`
	EndTimestamp   int64                   `json:"end_timestamp"`
}

type GPTAbuseSignalLogItem struct {
	ID                   int    `json:"id"`
	CreatedAt            int64  `json:"created_at"`
	UserID               int    `json:"user_id"`
	Username             string `json:"username"`
	UserEmail            string `json:"user_email"`
	TokenID              int    `json:"token_id"`
	TokenName            string `json:"token_name"`
	ChannelID            int    `json:"channel_id"`
	ChannelName          string `json:"channel_name"`
	ChannelType          int    `json:"channel_type"`
	ChannelMultiKeyIndex int    `json:"channel_multi_key_index"`
	RequestID            string `json:"request_id"`
	UpstreamRequestID    string `json:"upstream_request_id"`
	Endpoint             string `json:"endpoint"`
	RelayMode            int    `json:"relay_mode"`
	RequestedModel       string `json:"requested_model"`
	UpstreamModel        string `json:"upstream_model"`
	IsStream             bool   `json:"is_stream"`
	Source               string `json:"source"`
	Kind                 string `json:"kind"`
	Severity             string `json:"severity"`
	StatusCode           int    `json:"status_code"`
	ErrorCode            string `json:"error_code"`
	ErrorType            string `json:"error_type"`
	CountEligible        bool   `json:"count_eligible"`
	Extra                any    `json:"extra"`
}

type GPTAbuseRepeatBlockQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	Limit          int
	Offset         int
}

type GPTAbuseRepeatBlockListResponse struct {
	Items          []GPTAbuseRepeatBlockItem `json:"items"`
	Total          int                       `json:"total"`
	Limit          int                       `json:"limit"`
	Offset         int                       `json:"offset"`
	StartTimestamp int64                     `json:"start_timestamp"`
	EndTimestamp   int64                     `json:"end_timestamp"`
}

type GPTAbuseRepeatBlockItem struct {
	ID                              int    `json:"id"`
	CreatedAt                       int64  `json:"created_at"`
	UserID                          int    `json:"user_id"`
	Username                        string `json:"username"`
	TokenID                         int    `json:"token_id"`
	TokenName                       string `json:"token_name"`
	RequestID                       string `json:"request_id"`
	Endpoint                        string `json:"endpoint"`
	RelayMode                       int    `json:"relay_mode"`
	RequestedModel                  string `json:"requested_model"`
	BodyFingerprintPrefix           string `json:"body_fingerprint_prefix"`
	FirstWarningLogID               int    `json:"first_warning_log_id"`
	FirstWarningAt                  int64  `json:"first_warning_at"`
	FirstWarningRequestID           string `json:"first_warning_request_id"`
	FirstWarningUpstreamRequestID   string `json:"first_warning_upstream_request_id"`
	FirstWarningSource              string `json:"first_warning_source"`
	FirstWarningKind                string `json:"first_warning_kind"`
	FirstWarningSeverity            string `json:"first_warning_severity"`
	ChannelID                       int    `json:"channel_id"`
	ChannelName                     string `json:"channel_name"`
	ChannelType                     int    `json:"channel_type"`
}

type GPTAbuseClearSuspensionRequest struct {
	Reason string `json:"reason"`
}

type GPTAbuseClearSuspensionResponse struct {
	UserID              int  `json:"user_id"`
	HadActiveSuspension bool `json:"had_active_suspension"`
	SuspensionCleared   bool `json:"suspension_cleared"`
	ClearedSuspensionID int  `json:"cleared_suspension_id"`
}

type GPTAbuseResetWarningsRequest struct {
	Reason          string `json:"reason"`
	ClearSuspension bool   `json:"clear_suspension"`
}

type GPTAbuseResetWarningsResponse struct {
	ResetID                int   `json:"reset_id"`
	UserID                 int   `json:"user_id"`
	WindowStart            int64 `json:"window_start"`
	WindowEnd              int64 `json:"window_end"`
	ResetAt                int64 `json:"reset_at"`
	PreviousRawCount       int   `json:"previous_raw_count"`
	PreviousEffectiveCount int   `json:"previous_effective_count"`
	EffectiveWarningCount  int   `json:"effective_warning_count"`
	CutoffSignalLogID      int   `json:"cutoff_signal_log_id"`
	HadActiveSuspension    bool  `json:"had_active_suspension"`
	SuspensionCleared      bool  `json:"suspension_cleared"`
	ClearedSuspensionID    int   `json:"cleared_suspension_id"`
}
