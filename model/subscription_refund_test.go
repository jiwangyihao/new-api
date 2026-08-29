package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRefundSubscriptionPreConsumeLocksRequestBeforeRefund(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))

	const subscriptionID = 24_301
	const requestID = "refund-lock-contract"
	require.NoError(t, DB.Create(&UserSubscription{
		Id:              subscriptionID,
		UserId:          24_302,
		PlanId:          24_303,
		EntitlementType: SubscriptionEntitlementTimed,
		Status:          SubscriptionStatusActive,
		TokenLimit:      100,
		TokenUsed:       20,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId:          requestID,
		UserId:             24_302,
		UserSubscriptionId: subscriptionID,
		PreConsumed:        5,
		Status:             "consumed",
	}).Error)

	originalUsingSQLite := common.UsingSQLite
	common.UsingSQLite = false
	t.Cleanup(func() { common.UsingSQLite = originalUsingSQLite })

	const callbackName = "subscription_refund_test:observe_request_lock"
	lockedRequest := false
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "SubscriptionPreConsumeRecord" {
			return
		}
		_, lockedRequest = tx.Statement.Clauses["FOR"]
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callbackName) })

	require.NoError(t, RefundSubscriptionPreConsume(requestID))
	assert.True(t, lockedRequest, "refund must lock the request row before checking its terminal state")

	var subscription UserSubscription
	require.NoError(t, DB.First(&subscription, subscriptionID).Error)
	assert.Equal(t, int64(15), subscription.TokenUsed)
}
