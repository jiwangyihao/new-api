package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestPostConsumeQuotaCreditUsesStableRequestTarget(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subscriptionID, channelID = 82_951, 82_952, 82_953, 82_954, 82_955
	seedCreditBillingRuntime(t, userID, tokenID, planID, subscriptionID, channelID, "sk-credit-post-consume", 1_000, 0)
	require.NoError(t, model.DB.Migrator().DropTable(&model.CreditValuationState{}, &model.CreditValuationMigration{}))
	require.NoError(t, model.DB.AutoMigrate(&model.CreditValuationState{}, &model.CreditValuationMigration{}))
	t.Cleanup(func() {
		_ = model.DB.Migrator().DropTable(&model.CreditValuationState{}, &model.CreditValuationMigration{})
	})
	require.NoError(t, model.DB.Create(&model.CreditValuationMigration{
		Version: 1, Status: model.CreditValuationMigrationReady, ValuationCurrency: "CNY",
	}).Error)
	require.NoError(t, model.DB.Create(&model.CreditValuationState{
		UserSubscriptionId: subscriptionID,
		UserId:             userID,
		AvailableCredit:    1_000,
		ExactCostMicros:    40_000_000,
		Currency:           "CNY",
		RuleVersion:        model.CreditValuationRuleVersion,
		StateVersion:       1,
	}).Error)

	const requestID = "req-credit-post-consume-target"
	preConsumed, err := model.PreConsumeUserSubscriptionByUnits(requestID, userID, "gpt-4o", 0, 0, 100)
	require.NoError(t, err)
	require.Equal(t, subscriptionID, preConsumed.UserSubscriptionId)

	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-post-consume", requestID, "subscription_only")
	relayInfo.BillingSource = BillingSourceSubscription
	relayInfo.SubscriptionId = subscriptionID
	relayInfo.SubscriptionPreConsumed = 100
	relayInfo.SubscriptionDistributorTokenBilling = true

	require.NoError(t, PostConsumeQuota(relayInfo, 50, 100, false))

	var record model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(150), record.AppliedCredit)
	require.Equal(t, int64(150), record.DeductedAvailableCredit)
	require.Equal(t, int64(2), record.SettlementVersion)
	var subscription model.UserSubscription
	require.NoError(t, model.DB.First(&subscription, subscriptionID).Error)
	require.Equal(t, int64(150), subscription.TokenUsed)
	var state model.CreditValuationState
	require.NoError(t, model.DB.Where("user_subscription_id = ?", subscriptionID).First(&state).Error)
	require.Equal(t, int64(850), state.AvailableCredit)
	require.Equal(t, int64(34_000_000), state.ExactCostMicros)
	require.Equal(t, int64(3), state.StateVersion)
	require.Equal(t, int64(50), relayInfo.SubscriptionPostDelta)
	var requestCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionPreConsumeRecord{}).Where("request_id = ?", requestID).Count(&requestCount).Error)
	require.Equal(t, int64(1), requestCount)

	recordBeforeReplay := record
	subscriptionBeforeReplay := subscription
	stateBeforeReplay := state
	require.NoError(t, PostConsumeQuota(relayInfo, 50, 100, false))
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, recordBeforeReplay, record)
	require.NoError(t, model.DB.First(&subscription, subscriptionID).Error)
	require.Equal(t, subscriptionBeforeReplay, subscription)
	require.NoError(t, model.DB.Where("user_subscription_id = ?", subscriptionID).First(&state).Error)
	require.Equal(t, stateBeforeReplay, state)
}
