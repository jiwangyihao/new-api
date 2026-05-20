package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type RankingQuotaTotal struct {
	ModelName   string `json:"model_name"`
	TotalTokens int64  `json:"total_tokens"`
}

type RankingQuotaBucket struct {
	ModelName string `json:"model_name"`
	Bucket    int64  `json:"bucket"`
	Tokens    int64  `json:"tokens"`
}

type RankingFreeUserTotal struct {
	UserID      int    `json:"user_id"`
	Setting     string `json:"setting"`
	TotalTokens int64  `json:"total_tokens"`
}

func GetRankingQuotaTotals(startTime int64, endTime int64) ([]RankingQuotaTotal, error) {
	var rows []RankingQuotaTotal
	query := DB.Table("quota_data").
		Select("model_name, sum(token_used) as total_tokens").
		Where("model_name <> ''").
		Group("model_name").
		Having("sum(token_used) > 0").
		Order("total_tokens DESC")
	query = applyRankingQuotaTimeRange(query, startTime, endTime)
	err := query.Find(&rows).Error
	return rows, err
}

func GetRankingQuotaBuckets(startTime int64, endTime int64, bucketSize int64) ([]RankingQuotaBucket, error) {
	if bucketSize <= 0 {
		bucketSize = 3600
	}
	bucketExpr := rankingBucketExpr(bucketSize)
	var rows []RankingQuotaBucket
	query := DB.Table("quota_data").
		Select(fmt.Sprintf("model_name, %s as bucket, sum(token_used) as tokens", bucketExpr)).
		Where("model_name <> ''").
		Group(fmt.Sprintf("model_name, %s", bucketExpr)).
		Having("sum(token_used) > 0").
		Order("bucket ASC")
	query = applyRankingQuotaTimeRange(query, startTime, endTime)
	err := query.Find(&rows).Error
	return rows, err
}

func GetRankingFreeUserTotals(limit int) ([]RankingFreeUserTotal, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []RankingFreeUserTotal
	query := DB.Table("user_subscriptions").
		Select("user_subscriptions.user_id, users.setting, sum(user_subscriptions.token_used) as total_tokens").
		Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Joins("JOIN users ON users.id = user_subscriptions.user_id").
		Where("users.deleted_at IS NULL").
		Where("user_subscriptions.token_used > 0").
		Where("subscription_plans.price_amount = ?", 0).
		Where("(subscription_plans.is_trial = ? OR user_subscriptions.grant_reason IN ?)", true, []string{"trial_code", "invite_trial"}).
		Group("user_subscriptions.user_id, users.setting").
		Having("sum(user_subscriptions.token_used) > 0").
		Order("total_tokens DESC").
		Order("user_subscriptions.user_id ASC").
		Limit(limit)
	err := query.Find(&rows).Error
	return rows, err
}

func rankingBucketExpr(bucketSize int64) string {
	if common.UsingMySQL {
		return fmt.Sprintf("FLOOR(created_at / %d) * %d", bucketSize, bucketSize)
	}
	return fmt.Sprintf("(created_at / %d) * %d", bucketSize, bucketSize)
}

func applyRankingQuotaTimeRange(query *gorm.DB, startTime int64, endTime int64) *gorm.DB {
	if startTime > 0 {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("created_at <= ?", endTime)
	}
	return query
}
