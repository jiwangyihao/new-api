package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	GPTAbuseSignalSourceHTTPError         = "http_error"
	GPTAbuseSignalSourceSSEResponseFailed = "sse_response_failed"
	GPTAbuseSignalSourceSSEMetadata       = "sse_metadata"
	GPTAbuseSignalSourceModelReroute      = "model_reroute"
)

const (
	GPTAbuseSeverityHigh   = "high"
	GPTAbuseSeverityMedium = "medium"
)

const (
	GPTAbuseKindCyberPolicy                 = "cyber_policy"
	GPTAbuseKindHighRiskCyberReroute        = "high_risk_cyber_reroute"
	GPTAbuseKindInvalidPromptSafety         = "invalid_prompt_safety"
	GPTAbuseKindContentPolicyViolation      = "content_policy_violation"
	GPTAbuseKindGenericPolicyViolation      = "generic_policy_violation"
	GPTAbuseKindGenericAbuseSecurityWarning = "generic_abuse_security_warning"
)

const (
	GPTAbuseSuspensionStatusActive  = "active"
	GPTAbuseSuspensionStatusExpired = "expired"
	GPTAbuseSuspensionStatusCleared = "cleared"
)

type GPTAbuseSignalLog struct {
	Id                   int    `json:"id" gorm:"primaryKey"`
	CreatedAt            int64  `json:"created_at" gorm:"bigint;index;index:idx_gpt_abuse_user_created,priority:2"`
	UserId               int    `json:"user_id" gorm:"not null;index;index:idx_gpt_abuse_user_created,priority:1"`
	Username             string `json:"username" gorm:"type:varchar(255);default:''"`
	UserEmail            string `json:"user_email" gorm:"type:varchar(255);default:''"`
	TokenId              int    `json:"token_id" gorm:"default:0;index;index:idx_gpt_abuse_token_created,priority:1"`
	TokenName            string `json:"token_name" gorm:"type:varchar(255);default:''"`
	ChannelId            int    `json:"channel_id" gorm:"default:0;index;index:idx_gpt_abuse_channel_created,priority:1"`
	ChannelName          string `json:"channel_name" gorm:"type:varchar(255);default:''"`
	ChannelType          int    `json:"channel_type" gorm:"default:0"`
	ChannelMultiKeyIndex int    `json:"channel_multi_key_index" gorm:"default:0"`
	RequestId            string `json:"request_id" gorm:"type:varchar(128);default:'';index"`
	UpstreamRequestId    string `json:"upstream_request_id" gorm:"type:varchar(128);default:'';index"`
	Endpoint             string `json:"endpoint" gorm:"type:varchar(255);default:''"`
	RelayMode            int    `json:"relay_mode" gorm:"default:0"`
	RequestedModel       string `json:"requested_model" gorm:"type:varchar(255);default:''"`
	UpstreamModel        string `json:"upstream_model" gorm:"type:varchar(255);default:''"`
	IsStream             bool   `json:"is_stream" gorm:"default:false;index"`
	Source               string `json:"source" gorm:"type:varchar(64);default:'';index"`
	Kind                 string `json:"kind" gorm:"type:varchar(64);default:'';index"`
	Severity             string `json:"severity" gorm:"type:varchar(16);default:'';index"`
	StatusCode           int    `json:"status_code" gorm:"default:0"`
	ErrorCode            string `json:"error_code" gorm:"type:varchar(128);default:''"`
	ErrorType            string `json:"error_type" gorm:"type:varchar(128);default:''"`
	CountEligible        bool   `json:"count_eligible" gorm:"default:false;index"`
	DedupeKey            string `json:"dedupe_key" gorm:"type:varchar(255);not null;uniqueIndex"`
	Extra                string `json:"extra" gorm:"type:text"`
	UpdatedAt            int64  `json:"updated_at" gorm:"bigint"`
}

func (l *GPTAbuseSignalLog) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if l.CreatedAt == 0 {
		l.CreatedAt = now
	}
	if l.UpdatedAt == 0 {
		l.UpdatedAt = now
	}
	return nil
}

func (l *GPTAbuseSignalLog) BeforeUpdate(tx *gorm.DB) error {
	l.UpdatedAt = common.GetTimestamp()
	return nil
}

type GPTAbuseUserSuspension struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	UserId         int    `json:"user_id" gorm:"not null;index;index:idx_gpt_abuse_susp_user_status_until,priority:1"`
	ActiveUserId   *int   `json:"-" gorm:"uniqueIndex"`
	Status         string `json:"status" gorm:"type:varchar(16);not null;default:'active';index;index:idx_gpt_abuse_susp_user_status_until,priority:2"`
	Reason         string `json:"reason" gorm:"type:varchar(64);default:''"`
	SuspendedUntil int64  `json:"suspended_until" gorm:"bigint;not null;index;index:idx_gpt_abuse_susp_user_status_until,priority:3"`
	TriggerLogId   int    `json:"trigger_log_id" gorm:"default:0;index"`
	DailyCount     int    `json:"daily_count" gorm:"default:0"`
	DailyLimit     int    `json:"daily_limit" gorm:"default:0"`
	ClearedAt      int64  `json:"cleared_at" gorm:"bigint;default:0"`
	ClearedBy      int    `json:"cleared_by" gorm:"default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}

func (s *GPTAbuseUserSuspension) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if s.CreatedAt == 0 {
		s.CreatedAt = now
	}
	if s.UpdatedAt == 0 {
		s.UpdatedAt = now
	}
	if strings.TrimSpace(s.Status) == "" {
		s.Status = GPTAbuseSuspensionStatusActive
	}
	if s.Status == GPTAbuseSuspensionStatusActive && s.ActiveUserId == nil && s.UserId > 0 {
		userId := s.UserId
		s.ActiveUserId = &userId
	}
	return nil
}

func (s *GPTAbuseUserSuspension) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = common.GetTimestamp()
	return nil
}

type GPTAbuseWarningReset struct {
	Id                int    `json:"id" gorm:"primaryKey"`
	UserId            int    `json:"user_id" gorm:"not null;index;index:idx_gpt_abuse_reset_user_window,priority:1"`
	WindowStart       int64  `json:"window_start" gorm:"bigint;not null;index;index:idx_gpt_abuse_reset_user_window,priority:2"`
	WindowEnd         int64  `json:"window_end" gorm:"bigint;not null;index"`
	ResetAt           int64  `json:"reset_at" gorm:"bigint;not null;index"`
	ResetBy           int    `json:"reset_by" gorm:"default:0;index"`
	PreviousRawCount  int    `json:"previous_raw_count" gorm:"default:0"`
	PreviousCount     int    `json:"previous_count" gorm:"default:0"`
	CutoffSignalLogID int    `json:"cutoff_signal_log_id" gorm:"default:0;index"`
	Reason            string `json:"reason" gorm:"type:varchar(255);default:''"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint"`
}

func (r *GPTAbuseWarningReset) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if r.ResetAt == 0 {
		r.ResetAt = now
	}
	if r.CreatedAt == 0 {
		r.CreatedAt = now
	}
	return nil
}

type GPTAbuseRepeatBlockLog struct {
	Id                            int    `json:"id" gorm:"primaryKey"`
	CreatedAt                     int64  `json:"created_at" gorm:"bigint;index;index:idx_gpt_abuse_repeat_user_created,priority:2"`
	UserId                        int    `json:"user_id" gorm:"not null;index;index:idx_gpt_abuse_repeat_user_created,priority:1"`
	Username                      string `json:"username" gorm:"type:varchar(255);default:''"`
	TokenId                       int    `json:"token_id" gorm:"default:0;index"`
	TokenName                     string `json:"token_name" gorm:"type:varchar(255);default:''"`
	RequestId                     string `json:"request_id" gorm:"type:varchar(128);default:'';index"`
	Endpoint                      string `json:"endpoint" gorm:"type:varchar(255);default:''"`
	RelayMode                     int    `json:"relay_mode" gorm:"default:0"`
	RequestedModel                string `json:"requested_model" gorm:"type:varchar(255);default:''"`
	BodyFingerprint               string `json:"-" gorm:"type:varchar(128);default:'';index"`
	FirstWarningLogId             int    `json:"first_warning_log_id" gorm:"default:0;index"`
	FirstWarningAt                int64  `json:"first_warning_at" gorm:"bigint;default:0"`
	FirstWarningRequestId         string `json:"first_warning_request_id" gorm:"type:varchar(128);default:''"`
	FirstWarningUpstreamRequestId string `json:"first_warning_upstream_request_id" gorm:"type:varchar(128);default:''"`
	FirstWarningSource            string `json:"first_warning_source" gorm:"type:varchar(64);default:''"`
	FirstWarningKind              string `json:"first_warning_kind" gorm:"type:varchar(64);default:''"`
	FirstWarningSeverity          string `json:"first_warning_severity" gorm:"type:varchar(16);default:''"`
	ChannelId                     int    `json:"channel_id" gorm:"default:0;index"`
	ChannelName                   string `json:"channel_name" gorm:"type:varchar(255);default:''"`
	ChannelType                   int    `json:"channel_type" gorm:"default:0"`
}

func (l *GPTAbuseRepeatBlockLog) BeforeCreate(tx *gorm.DB) error {
	if l.CreatedAt == 0 {
		l.CreatedAt = common.GetTimestamp()
	}
	return nil
}


func RecordGPTAbuseSignalLog(log *GPTAbuseSignalLog) (bool, error) {
	if log == nil {
		return false, nil
	}
	if strings.TrimSpace(log.DedupeKey) == "" {
		return false, errors.New("dedupe key is empty")
	}
	result := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "dedupe_key"}},
		DoNothing: true,
	}).Create(log)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func CountGPTAbuseSignalsForUser(userID int, start, end int64) (int, error) {
	count, _, err := CountEffectiveGPTAbuseSignalsForUser(userID, start, end)
	return count, err
}

func CountGPTAbuseSignalsForUserRaw(userID int, start, end int64) (int, error) {
	return CountGPTAbuseSignalsForUserRawTx(DB, userID, start, end)
}

func CountGPTAbuseSignalsForUserRawTx(tx *gorm.DB, userID int, start, end int64) (int, error) {
	return countGPTAbuseSignalsForUserTx(tx, userID, start, end, 0)
}

func CountEffectiveGPTAbuseSignalsForUser(userID int, start, end int64) (int, *GPTAbuseWarningReset, error) {
	return CountEffectiveGPTAbuseSignalsForUserTx(DB, userID, start, end)
}

func CountEffectiveGPTAbuseSignalsForUserTx(tx *gorm.DB, userID int, start, end int64) (int, *GPTAbuseWarningReset, error) {
	if userID <= 0 || end <= start {
		return 0, nil, nil
	}
	reset, err := latestGPTAbuseWarningResetTx(tx, userID, start)
	if err != nil {
		return 0, nil, err
	}
	cutoffID := 0
	if reset != nil {
		cutoffID = reset.CutoffSignalLogID
	}
	count, err := countGPTAbuseSignalsForUserTx(tx, userID, start, end, cutoffID)
	if err != nil {
		return 0, nil, err
	}
	return count, reset, nil
}

func LatestGPTAbuseWarningReset(userID int, windowStart int64) (*GPTAbuseWarningReset, error) {
	return latestGPTAbuseWarningResetTx(DB, userID, windowStart)
}

func MaxGPTAbuseSignalLogIDForUserWindow(userID int, start, end int64) (int, error) {
	return MaxGPTAbuseSignalLogIDForUserWindowTx(DB, userID, start, end)
}

func MaxGPTAbuseSignalLogIDForUserWindowTx(tx *gorm.DB, userID int, start, end int64) (int, error) {
	if tx == nil {
		return 0, errors.New("tx is nil")
	}
	if userID <= 0 || end <= start {
		return 0, nil
	}
	var maxID int
	err := tx.Model(&GPTAbuseSignalLog{}).
		Select("COALESCE(MAX(id), 0)").
		Where("user_id = ? AND count_eligible = ? AND created_at >= ? AND created_at < ?", userID, true, start, end).
		Scan(&maxID).Error
	return maxID, err
}

func CreateGPTAbuseWarningReset(reset *GPTAbuseWarningReset) error {
	return CreateGPTAbuseWarningResetTx(DB, reset)
}

func CreateGPTAbuseWarningResetTx(tx *gorm.DB, reset *GPTAbuseWarningReset) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if reset == nil {
		return nil
	}
	return tx.Create(reset).Error
}

func RecordGPTAbuseRepeatBlockLog(log *GPTAbuseRepeatBlockLog) error {
	if log == nil {
		return nil
	}
	if log.UserId <= 0 {
		return errors.New("invalid user id")
	}
	if strings.TrimSpace(log.BodyFingerprint) == "" {
		return errors.New("body fingerprint is empty")
	}
	if log.FirstWarningLogId <= 0 {
		return errors.New("first warning log id is empty")
	}
	return DB.Create(log).Error
}

func countGPTAbuseSignalsForUserTx(tx *gorm.DB, userID int, start, end int64, cutoffID int) (int, error) {
	if tx == nil {
		return 0, errors.New("tx is nil")
	}
	if userID <= 0 || end <= start {
		return 0, nil
	}
	query := tx.Model(&GPTAbuseSignalLog{}).
		Where("user_id = ? AND count_eligible = ? AND created_at >= ? AND created_at < ?", userID, true, start, end)
	if cutoffID > 0 {
		query = query.Where("id > ?", cutoffID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func latestGPTAbuseWarningResetTx(tx *gorm.DB, userID int, windowStart int64) (*GPTAbuseWarningReset, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if userID <= 0 {
		return nil, nil
	}
	var reset GPTAbuseWarningReset
	err := tx.Where("user_id = ? AND window_start = ?", userID, windowStart).
		Order("reset_at desc, id desc").
		First(&reset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &reset, nil
}

func GetActiveGPTAbuseSuspension(userID int, now int64) (*GPTAbuseUserSuspension, error) {
	if userID <= 0 {
		return nil, nil
	}
	var susp GPTAbuseUserSuspension
	err := DB.Where("user_id = ? AND status = ? AND suspended_until > ?", userID, GPTAbuseSuspensionStatusActive, now).
		Order("suspended_until desc, id desc").First(&susp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var expiredCount int64
		_ = DB.Model(&GPTAbuseUserSuspension{}).
			Where("user_id = ? AND status = ? AND suspended_until <= ?", userID, GPTAbuseSuspensionStatusActive, now).
			Count(&expiredCount).Error
		if expiredCount > 0 {
			_ = DB.Model(&GPTAbuseUserSuspension{}).
				Where("user_id = ? AND status = ? AND suspended_until <= ?", userID, GPTAbuseSuspensionStatusActive, now).
				Updates(map[string]any{"status": GPTAbuseSuspensionStatusExpired, "active_user_id": nil, "updated_at": now}).Error
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &susp, nil
}

func MarkExpiredGPTAbuseSuspensions(userID int, now int64) error {
	if userID <= 0 {
		return nil
	}
	return DB.Model(&GPTAbuseUserSuspension{}).
		Where("user_id = ? AND status = ? AND suspended_until <= ?", userID, GPTAbuseSuspensionStatusActive, now).
		Updates(map[string]any{"status": GPTAbuseSuspensionStatusExpired, "active_user_id": nil, "updated_at": now}).Error
}

func UpsertGPTAbuseSuspension(userID int, triggerLogID int, dailyCount int, dailyLimit int, suspendedUntil int64) error {
	if userID <= 0 {
		return errors.New("invalid user id")
	}
	now := common.GetTimestamp()
	activeUserId := userID
	suspension := &GPTAbuseUserSuspension{
		UserId:         userID,
		ActiveUserId:   &activeUserId,
		Status:         GPTAbuseSuspensionStatusActive,
		Reason:         "gpt_abuse_daily_limit",
		SuspendedUntil: suspendedUntil,
		TriggerLogId:   triggerLogID,
		DailyCount:     dailyCount,
		DailyLimit:     dailyLimit,
		ClearedAt:      0,
		ClearedBy:      0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "active_user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"user_id":         userID,
			"status":          GPTAbuseSuspensionStatusActive,
			"reason":          "gpt_abuse_daily_limit",
			"suspended_until": suspendedUntil,
			"trigger_log_id":  triggerLogID,
			"daily_count":     dailyCount,
			"daily_limit":     dailyLimit,
			"cleared_at":      0,
			"cleared_by":      0,
			"updated_at":      now,
		}),
	}).Create(suspension).Error
}

func GPTAbuseDayWindow(ts int64) (int64, int64) {
	if ts <= 0 {
		ts = common.GetTimestamp()
	}
	tm := time.Unix(ts, 0)
	startTime := time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, tm.Location())
	return startTime.Unix(), startTime.AddDate(0, 0, 1).Unix()
}
