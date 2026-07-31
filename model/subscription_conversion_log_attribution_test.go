package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimedConversionPreservesCompletedLogOwnershipAndTargetsNewUsage(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &Log{}, &SubscriptionPreConsumeRecord{}))

	const userID = 10_201
	const sourceID = 10_202
	const sourcePlanID = 10_203
	require.NoError(t, DB.Create(&User{Id: userID, Username: "conversion-log-owner", Status: common.UserStatusEnabled}).Error)
	plan := seedConversionQuoteTimedPlan(t, sourcePlanID, 100)
	plan.ModelLimits = "gpt-4o"
	require.NoError(t, DB.Save(plan).Error)

	now := GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: sourceID, UserId: userID, PlanId: sourcePlanID, EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit: 100, TokenUsed: 10, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 48*60*60, EndTime: now + 60*60, Status: SubscriptionStatusActive,
		LastGrantedAt: now - 48*60*60, LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: SubscriptionGrantTimeSourceLive, LastGrantSource: SubscriptionGrantOrder,
	}).Error)

	sourceIDValue := sourceID
	preTokens := 11
	explicitHistory := Log{
		UserId: userID, Username: "conversion-log-owner", CreatedAt: now - 60, Type: LogTypeConsume,
		RequestId: "conversion-explicit-history", MeteredTokens: &preTokens, SubscriptionID: &sourceIDValue,
		Other: common.MapToJsonStr(map[string]interface{}{
			"billing_source":               "subscription",
			"subscription_id":              sourceID,
			"subscription_plan_id":         sourcePlanID,
			"subscription_tokens_consumed": 11,
		}),
	}
	require.NoError(t, LOG_DB.Create(&explicitHistory).Error)
	legacyTokens := 7
	legacyHistory := Log{
		UserId: userID, Username: "conversion-log-owner", CreatedAt: now - 30, Type: LogTypeConsume,
		RequestId: "conversion-legacy-history", MeteredTokens: &legacyTokens,
		Other: common.MapToJsonStr(map[string]interface{}{
			"billing_source": "subscription",
		}),
	}
	require.NoError(t, LOG_DB.Create(&legacyHistory).Error)
	historicalOther := explicitHistory.Other

	result, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "conversion-log-history-key")
	require.NoError(t, err)
	require.False(t, result.Replayed)
	targetID := result.Conversion.TargetSubscriptionId
	require.Positive(t, targetID)
	targetPlanID := result.Conversion.TargetPlanId
	preConsume, err := PreConsumeUserSubscription("conversion-target-request", userID, "gpt-4o", 0, 13)
	require.NoError(t, err)
	require.Equal(t, targetID, preConsume.UserSubscriptionId)

	targetIDValue := preConsume.UserSubscriptionId
	postTokens := 13
	postConversion := Log{
		UserId: userID, Username: "conversion-log-owner", CreatedAt: result.Conversion.ConvertedAt + 1, Type: LogTypeConsume,
		RequestId: "conversion-target-usage", MeteredTokens: &postTokens, SubscriptionID: &targetIDValue,
		Other: common.MapToJsonStr(map[string]interface{}{
			"billing_source":               "subscription",
			"subscription_id":              targetID,
			"subscription_plan_id":         targetPlanID,
			"subscription_tokens_consumed": 13,
		}),
	}
	require.NoError(t, LOG_DB.Create(&postConversion).Error)

	logs, warnings, err := loadAndEnrichAdminUsageLogs(AdminAnalyticsUsageQuery{
		AdminAnalyticsQuery: AdminAnalyticsQuery{
			StartTimestamp: now - 120,
			EndTimestamp:   result.Conversion.ConvertedAt + 2,
			SnapshotAt:     result.Conversion.ConvertedAt + 2,
			Limit:          20,
		},
		GroupBy:         dto.AdminUsageGroupByPlan,
		Metric:          dto.AdminUsageMetricTotalTokens,
		PlanAttribution: dto.AdminPlanAttributionEventTime,
	})
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Len(t, logs, 3)
	byRequest := make(map[string]adminUsageCandidateLog, len(logs))
	for _, log := range logs {
		byRequest[log.RequestId] = log
	}
	assert.Equal(t, sourceID, byRequest[explicitHistory.RequestId].AttributedSubscriptionID)
	assert.Equal(t, sourcePlanID, byRequest[explicitHistory.RequestId].AttributedPlanID)
	assert.Equal(t, sourceID, byRequest[legacyHistory.RequestId].AttributedSubscriptionID)
	assert.Equal(t, sourcePlanID, byRequest[legacyHistory.RequestId].AttributedPlanID)
	assert.NotEqual(t, targetID, byRequest[legacyHistory.RequestId].AttributedSubscriptionID)
	assert.Equal(t, targetID, byRequest[postConversion.RequestId].AttributedSubscriptionID)
	assert.Equal(t, targetPlanID, byRequest[postConversion.RequestId].AttributedPlanID)

	var persistedHistory Log
	require.NoError(t, LOG_DB.First(&persistedHistory, explicitHistory.Id).Error)
	require.NotNil(t, persistedHistory.SubscriptionID)
	assert.Equal(t, sourceID, *persistedHistory.SubscriptionID)
	assert.Equal(t, historicalOther, persistedHistory.Other)
	var source UserSubscription
	require.NoError(t, DB.First(&source, sourceID).Error)
	assert.Equal(t, SubscriptionStatusConverted, source.Status)
	assert.Equal(t, result.Conversion.Id, source.ConversionId)
	assert.Equal(t, targetID, source.ConvertedToSubscriptionId)
	assert.Equal(t, int64(10), source.TokenUsed)
	var target UserSubscription
	require.NoError(t, DB.First(&target, targetID).Error)
	assert.Equal(t, SubscriptionEntitlementCreditBalance, target.EntitlementType)
	assert.Equal(t, int64(13), target.TokenUsed)
	var conversion SubscriptionConversion
	require.NoError(t, DB.First(&conversion, result.Conversion.Id).Error)
	assert.Equal(t, sourceID, conversion.SourceSubscriptionId)
	assert.Equal(t, sourcePlanID, conversion.SourcePlanId)
	assert.Equal(t, targetID, conversion.TargetSubscriptionId)
	assert.Equal(t, targetPlanID, conversion.TargetPlanId)
}
