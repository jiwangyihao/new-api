package model

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	TokenLimitPreConsumeStatusConsumed     = "consumed"
	TokenLimitPreConsumeStatusRefunded     = "refunded"
	TokenLimitPreConsumeStatusSettled      = "settled"
	TokenLimitPreConsumeStatusSettleFailed = "settle_failed"
)

const tokenLimitFailureUsageReset = "usage_reset"

type TokenLimitPreConsumeRecord struct {
	Id                int    `json:"id"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	UserId            int    `json:"user_id" gorm:"index;not null"`
	TokenId           int    `json:"token_id" gorm:"index;not null"`
	PreConsumedTokens int64  `json:"pre_consumed_tokens" gorm:"type:bigint;not null;default:0"`
	ActualTokens      int64  `json:"actual_tokens" gorm:"type:bigint;not null;default:0"`
	DeltaTokens       int64  `json:"delta_tokens" gorm:"type:bigint;not null;default:0"`
	FailureCode       string `json:"failure_code" gorm:"type:varchar(64);not null;default:''"`
	Status            string `json:"status" gorm:"type:varchar(16);not null;default:'consumed'"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt         int64  `json:"updated_at" gorm:"bigint"`
}

func (record *TokenLimitPreConsumeRecord) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if record.CreatedAt == 0 {
		record.CreatedAt = now
	}
	if record.UpdatedAt == 0 {
		record.UpdatedAt = now
	}
	return nil
}

func (record *TokenLimitPreConsumeRecord) BeforeUpdate(tx *gorm.DB) error {
	record.UpdatedAt = common.GetTimestamp()
	return nil
}

func PreConsumeTokenLimit(tokenId int, userId int, requestId string, tokens int64) (bool, error) {
	if tokens <= 0 {
		return true, nil
	}
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return false, errors.New("request_id is required")
	}
	ok, changedTokenId, err := consumeTokenLimitRecord(tokenId, userId, requestId, tokens, TokenLimitPreConsumeStatusConsumed)
	if err != nil {
		return false, err
	}
	if changedTokenId > 0 {
		_ = invalidateTokenCacheById(changedTokenId)
	}
	return ok, nil
}

func ConsumeTokenLimitIncrement(tokenId int, userId int, idempotencyKey string, tokens int64) (bool, error) {
	if tokens <= 0 {
		return true, nil
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return false, errors.New("idempotency key is required")
	}
	ok, changedTokenId, err := consumeTokenLimitRecord(tokenId, userId, idempotencyKey, tokens, TokenLimitPreConsumeStatusSettled)
	if err != nil {
		return false, err
	}
	if changedTokenId > 0 {
		_ = invalidateTokenCacheById(changedTokenId)
	}
	return ok, nil
}

func SettleTokenLimitPreConsume(requestId string, actualTokens int64) error {
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return errors.New("request_id is required")
	}
	if actualTokens < 0 {
		actualTokens = 0
	}
	var changedTokenId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		record, err := getTokenLimitRecordTx(tx, requestId)
		if err != nil {
			return err
		}
		if record.Status != TokenLimitPreConsumeStatusConsumed {
			return nil
		}
		delta := actualTokens - record.PreConsumedTokens
		if delta > 0 {
			ok, err := incrementTokenLimitUsedTx(tx, record.TokenId, record.UserId, delta)
			if err != nil || !ok {
				return errTokenLimitInsufficient(ok, err)
			}
		} else if delta < 0 {
			if err := refundTokenLimitDeltaTx(tx, record.TokenId, record.UserId, -delta); err != nil {
				return err
			}
		}
		if err := tx.Model(&TokenLimitPreConsumeRecord{}).Where("id = ?", record.Id).Updates(map[string]interface{}{
			"actual_tokens": actualTokens,
			"delta_tokens":  delta,
			"status":        TokenLimitPreConsumeStatusSettled,
			"failure_code":  "",
			"updated_at":    common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		changedTokenId = record.TokenId
		return nil
	})
	if err != nil {
		return err
	}
	if changedTokenId > 0 {
		_ = invalidateTokenCacheById(changedTokenId)
	}
	return nil
}

func RefundTokenLimitPreConsume(requestId string, failureCode string) error {
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return errors.New("request_id is required")
	}
	var changedTokenId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		record, err := getTokenLimitRecordTx(tx, requestId)
		if err != nil {
			return err
		}
		if record.Status == TokenLimitPreConsumeStatusRefunded {
			return nil
		}
		if record.Status == TokenLimitPreConsumeStatusSettleFailed {
			return nil
		}
		charged := record.PreConsumedTokens + record.DeltaTokens
		if charged < 0 {
			charged = 0
		}
		if charged > 0 {
			if err := refundTokenLimitDeltaTx(tx, record.TokenId, record.UserId, charged); err != nil {
				return err
			}
		}
		if err := tx.Model(&TokenLimitPreConsumeRecord{}).Where("id = ?", record.Id).Updates(map[string]interface{}{
			"status":       TokenLimitPreConsumeStatusRefunded,
			"failure_code": failureCode,
			"updated_at":   common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		changedTokenId = record.TokenId
		return nil
	})
	if err != nil {
		return err
	}
	if changedTokenId > 0 {
		_ = invalidateTokenCacheById(changedTokenId)
	}
	return nil
}

func MarkTokenLimitSettleFailed(requestId string, actualTokens int64, failureCode string) error {
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return errors.New("request_id is required")
	}
	if actualTokens < 0 {
		actualTokens = 0
	}
	var changedTokenId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		record, err := getTokenLimitRecordTx(tx, requestId)
		if err != nil {
			return err
		}
		if record.Status != TokenLimitPreConsumeStatusConsumed {
			return nil
		}
		delta := actualTokens - record.PreConsumedTokens
		if delta < 0 {
			if err := refundTokenLimitDeltaTx(tx, record.TokenId, record.UserId, -delta); err != nil {
				return err
			}
		} else if delta > 0 {
			delta = 0
		}
		if err := tx.Model(&TokenLimitPreConsumeRecord{}).Where("id = ?", record.Id).Updates(map[string]interface{}{
			"actual_tokens": actualTokens,
			"delta_tokens":  delta,
			"status":        TokenLimitPreConsumeStatusSettleFailed,
			"failure_code":  failureCode,
			"updated_at":    common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		changedTokenId = record.TokenId
		return nil
	})
	if err != nil {
		return err
	}
	if changedTokenId > 0 {
		_ = invalidateTokenCacheById(changedTokenId)
	}
	return nil
}

func ResetTokenUsage(tokenId int, userId int) (before int64, err error) {
	var changed bool
	err = DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := tx.Select("id", "user_id", "token_used").Where("id = ? AND user_id = ?", tokenId, userId).First(&token).Error; err != nil {
			return err
		}
		before = token.TokenUsed
		if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", tokenId, userId).Update("token_used", int64(0)).Error; err != nil {
			return err
		}
		if err := tx.Model(&TokenLimitPreConsumeRecord{}).
			Where("token_id = ? AND user_id = ? AND status = ?", tokenId, userId, TokenLimitPreConsumeStatusConsumed).
			Updates(map[string]interface{}{
				"status":       TokenLimitPreConsumeStatusRefunded,
				"failure_code": tokenLimitFailureUsageReset,
				"updated_at":   common.GetTimestamp(),
			}).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return 0, err
	}
	if changed {
		_ = invalidateTokenCacheById(tokenId)
	}
	return before, nil
}

func invalidateTokenCacheById(tokenId int) error {
	if !common.RedisEnabled {
		return nil
	}
	var token Token
	if err := DB.Unscoped().Select("id", commonKeyCol).Where("id = ?", tokenId).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if token.Key == "" {
		return nil
	}
	return cacheDeleteToken(token.Key)
}

func TokenLimitEnabled(tokenId int, userId int) (bool, error) {
	return tokenLimitEnabledTx(DB, tokenId, userId)
}

func tokenLimitEnabledTx(tx *gorm.DB, tokenId int, userId int) (bool, error) {
	var enabled bool
	var limit int64
	err := tx.Raw("SELECT token_limit_enabled, token_limit FROM tokens WHERE id = ? AND user_id = ? AND deleted_at IS NULL LIMIT 1", tokenId, userId).Row().Scan(&enabled, &limit)
	if errors.Is(err, sql.ErrNoRows) {
		return false, gorm.ErrRecordNotFound
	}
	return enabled && limit > 0, err
}

func consumeTokenLimitRecord(tokenId int, userId int, requestId string, tokens int64, status string) (ok bool, changedTokenId int, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		existing, err := getTokenLimitRecordTx(tx, requestId)
		if err == nil {
			switch existing.Status {
			case TokenLimitPreConsumeStatusConsumed, TokenLimitPreConsumeStatusSettled, TokenLimitPreConsumeStatusSettleFailed:
				ok = true
			case TokenLimitPreConsumeStatusRefunded:
				ok = false
			default:
				ok = false
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		enabled, err := tokenLimitEnabledTx(tx, tokenId, userId)
		if err != nil {
			return err
		}
		if !enabled {
			ok = true
			return nil
		}
		actualTokens := int64(0)
		if status == TokenLimitPreConsumeStatusSettled {
			actualTokens = tokens
		}
		record := TokenLimitPreConsumeRecord{
			RequestId:         requestId,
			UserId:            userId,
			TokenId:           tokenId,
			PreConsumedTokens: tokens,
			ActualTokens:      actualTokens,
			Status:            status,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		updated, err := incrementTokenLimitUsedTx(tx, tokenId, userId, tokens)
		if err != nil {
			return err
		}
		if !updated {
			ok = false
			return errTokenLimitCapExceeded
		}
		ok = true
		changedTokenId = tokenId
		return nil
	})
	if errors.Is(err, errTokenLimitCapExceeded) {
		return false, 0, nil
	}
	if err != nil && isUniqueConstraintError(err) {
		existing, readErr := getTokenLimitRecord(requestId)
		if readErr != nil {
			return false, 0, readErr
		}
		return tokenLimitRecordIdempotentOK(existing.Status), 0, nil
	}
	return ok, changedTokenId, err
}

var ErrTokenLimitExceeded = errors.New("api key token limit exceeded")

var errTokenLimitCapExceeded = ErrTokenLimitExceeded

func errTokenLimitInsufficient(ok bool, err error) error {
	if err != nil {
		return err
	}
	if !ok {
		return errTokenLimitCapExceeded
	}
	return nil
}

func tokenLimitRecordIdempotentOK(status string) bool {
	return status == TokenLimitPreConsumeStatusConsumed || status == TokenLimitPreConsumeStatusSettled || status == TokenLimitPreConsumeStatusSettleFailed
}

func getTokenLimitRecord(requestId string) (*TokenLimitPreConsumeRecord, error) {
	return getTokenLimitRecordTx(DB, requestId)
}

func getTokenLimitRecordTx(tx *gorm.DB, requestId string) (*TokenLimitPreConsumeRecord, error) {
	var record TokenLimitPreConsumeRecord
	if err := tx.Where("request_id = ?", requestId).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func incrementTokenLimitUsedTx(tx *gorm.DB, tokenId int, userId int, tokens int64) (bool, error) {
	if tokens <= 0 {
		return true, nil
	}
	result := tx.Model(&Token{}).
		Where("id = ? AND user_id = ? AND token_limit_enabled = ? AND token_limit > ? AND token_used + ? <= token_limit", tokenId, userId, true, 0, tokens).
		Update("token_used", gorm.Expr("token_used + ?", tokens))
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func refundTokenLimitDeltaTx(tx *gorm.DB, tokenId int, userId int, tokens int64) error {
	if tokens <= 0 {
		return nil
	}
	return tx.Model(&Token{}).Where("id = ? AND user_id = ?", tokenId, userId).Update(
		"token_used",
		gorm.Expr("CASE WHEN token_used >= ? THEN token_used - ? ELSE 0 END", tokens, tokens),
	).Error
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unique") || strings.Contains(text, "duplicate") || strings.Contains(text, "constraint")
}
