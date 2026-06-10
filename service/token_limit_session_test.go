package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTokenUsed(t *testing.T, id int) int64 {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("token_used").Where("id = ?", id).First(&token).Error)
	return token.TokenUsed
}

func getTokenLimitRecordForTest(t *testing.T, requestID string) model.TokenLimitPreConsumeRecord {
	t.Helper()
	var record model.TokenLimitPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&record).Error)
	return record
}

func getSubscriptionPreConsumeRecordForTest(t *testing.T, requestID string) model.SubscriptionPreConsumeRecord {
	t.Helper()
	var record model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&record).Error)
	return record
}

func seedRuntimeDistributorBilling(t *testing.T, userID, tokenID, planID, subID int, tokenKey string, subLimit, subUsed int64) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPreConsumeRecord{}, &model.TokenLimitPreConsumeRecord{}))
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, tokenKey, 10_000)
	planCode := tokenKey + "-plan"
	seedDistributorPlan(t, planID, planCode, subLimit)
	seedDistributorSubscription(t, subID, userID, planID, subLimit, subUsed)
}

func TestTokenLimitSessionNoopWhenDisabled(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	require.NoError(t, model.DB.Create(&model.Token{Id: 96001, UserId: 96002, Key: "sk-no-cap", Status: common.TokenStatusEnabled, TokenLimitEnabled: false}).Error)
	relayInfo := subscriptionOnlyRelayInfo(96002, 96001, "sk-no-cap", "subscription_only")

	session := NewTokenLimitSession(relayInfo)
	apiErr := session.PreConsume(100)

	require.Nil(t, apiErr)
	require.Equal(t, int64(0), getTokenUsed(t, 96001))
}

func TestTokenLimitSessionRejectsWhenLimitExhausted(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	require.NoError(t, model.DB.Create(&model.Token{Id: 96011, UserId: 96012, Key: "sk-cap", Status: common.TokenStatusEnabled, TokenLimitEnabled: true, TokenLimit: 100, TokenUsed: 95}).Error)
	relayInfo := subscriptionOnlyRelayInfo(96012, 96011, "sk-cap", "subscription_only")

	session := NewTokenLimitSession(relayInfo)
	apiErr := session.PreConsume(10)

	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeAPIKeyTokenLimitExhausted, apiErr.GetErrorCode())
	require.Equal(t, int64(95), getTokenUsed(t, 96011))
}

func TestTokenLimitRejectRefundsSubscriptionPreConsume(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96021, 96022, 96023, 96024
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-cap-reject-refund", 100, 20)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(25),
		"token_used":          int64(24),
	}).Error)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-cap-reject-refund", "req-cap-reject-refund", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)

	apiErr := relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens())
	require.NotNil(t, apiErr)
	RefundBillingAfterTokenLimitReject(relayInfo.Billing)

	assert.Equal(t, types.ErrorCodeAPIKeyTokenLimitExhausted, apiErr.GetErrorCode())
	assert.Equal(t, int64(20), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(24), getTokenUsed(t, tokenID))
	record := getSubscriptionPreConsumeRecordForTest(t, "req-cap-reject-refund")
	assert.Equal(t, "refunded", record.Status)
}

func TestTokenLimitRejectReleasesSubscriptionConcurrencyLease(t *testing.T) {
	truncate(t)
	resetSubscriptionConcurrencyForTest(t)
	common.SubscriptionConcurrencyQueueCapacity = 0
	const userID, tokenID, planID, subID = 96031, 96032, 96033, 96034
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-cap-reject-lease", 100, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1),
		"token_used":          int64(1),
	}).Error)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-cap-reject-lease", "req-cap-reject-lease", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	lease, concurrencyErr := AcquireSubscriptionConcurrency(context.Background(), relayInfo)
	require.Nil(t, concurrencyErr)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)

	apiErr := relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens())
	require.NotNil(t, apiErr)
	require.NoError(t, lease.Release(context.Background()))
	RefundBillingAfterTokenLimitReject(relayInfo.Billing)
	nextLease, nextErr := AcquireSubscriptionConcurrency(context.Background(), relayInfo)
	require.Nil(t, nextErr)
	require.NoError(t, nextLease.Release(context.Background()))
}

func TestPostTextConsumeQuotaSettlesApiKeyLimitWithSubscriptionMeteredTokens(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96041, 96042, 96043, 96044
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-text-metered", 1000, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1000),
		"token_used":          int64(0),
		"remain_quota":        123,
		"used_quota":          456,
	}).Error)
	usage := &dto.Usage{
		PromptTokens:                100,
		CompletionTokens:            20,
		TotalTokens:                 50,
		UsageSemantic:               "anthropic",
		PromptTokensDetails:         dto.InputTokenDetails{CachedTokens: 10},
		ClaudeCacheCreation5mTokens: 30,
		ClaudeCacheCreation1hTokens: 40,
	}
	expectedTokens := SubscriptionMeteredTokens(usage)
	require.Equal(t, int64(200), expectedTokens)
	require.NotEqual(t, int64(usage.TotalTokens), expectedTokens)
	require.NotEqual(t, int64(usage.PromptTokens+usage.CompletionTokens), expectedTokens)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-text-metered", "req-text-metered", "subscription_only")
	relayInfo.ChannelId = 96045
	relayInfo.SetEstimatePromptTokens(100)
	seedChannel(t, 96045)
	preConsumeForBillingTest(t, ctx, relayInfo, 100)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, usage, nil))

	assert.Equal(t, expectedTokens, getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, expectedTokens, getTokenUsed(t, tokenID))
	assert.Equal(t, 123, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 456, getTokenUsedQuota(t, tokenID))
}

func TestPostSettleErrorToOpenAIErrorPreservesAPIKeyTokenLimitError(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{Billing: &recordingBillingSettler{commitCount: 0}}
	origin := types.NewOpenAIError(errors.New("api key token limit exhausted"), types.ErrorCodeAPIKeyTokenLimitExhausted, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())

	apiErr := PostSettleErrorToOpenAIError(relayInfo, origin)

	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeAPIKeyTokenLimitExhausted, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, 0, relayInfo.Billing.(*recordingBillingSettler).commitCount)
}

func TestSettleBillingRefundsSubscriptionWhenApiKeySettleRejects(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96051, 96052, 96053, 96054
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-settle-reject", 100, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(12),
		"token_used":          int64(0),
	}).Error)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-settle-reject", "req-settle-reject", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	err := SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 20, SubscriptionTokens: 20, ApiKeyTokens: 20})

	require.Error(t, err)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, types.ErrorCodeAPIKeyTokenLimitExhausted, apiErr.GetErrorCode())
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(0), getTokenUsed(t, tokenID))
	assert.Equal(t, "refunded", getSubscriptionPreConsumeRecordForTest(t, "req-settle-reject").Status)
	assert.Equal(t, model.TokenLimitPreConsumeStatusRefunded, getTokenLimitRecordForTest(t, "req-settle-reject").Status)
}

func TestSettleBillingRefundsApiKeyWhenSubscriptionSettleFails(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96061, 96062, 96063, 96064
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-sub-settle-fail", 15, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(100),
		"token_used":          int64(0),
	}).Error)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-sub-settle-fail", "req-sub-settle-fail", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	err := SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 20, SubscriptionTokens: 20, ApiKeyTokens: 12})

	require.Error(t, err)
	assert.Equal(t, int64(0), getTokenUsed(t, tokenID))
	assert.Equal(t, model.TokenLimitPreConsumeStatusRefunded, getTokenLimitRecordForTest(t, "req-sub-settle-fail").Status)
}

func TestSettleBillingUsesAuditWhenGinWriterAlreadyWritten(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96071, 96072, 96073, 96074
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-audit-written", 100, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(12),
		"token_used":          int64(0),
	}).Error)
	ctx := newBillingTestContext(t)
	ctx.Writer.WriteHeaderNow()
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audit-written", "req-audit-written", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	err := SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 20, SubscriptionTokens: 20, ApiKeyTokens: 20})

	require.NoError(t, err)
	assert.Equal(t, int64(20), getSubscriptionTokenUsed(t, subID))
	assert.LessOrEqual(t, getTokenUsed(t, tokenID), int64(12))
	record := getTokenLimitRecordForTest(t, "req-audit-written")
	assert.Equal(t, model.TokenLimitPreConsumeStatusSettleFailed, record.Status)
	assert.Equal(t, "api_key_token_limit_settle_failed", record.FailureCode)
}

func TestSettleBillingAuditSettleUsesActualTokensWhenWithinLimit(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96076, 96077, 96078, 96079
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-audit-within", 100, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(30),
		"token_used":          int64(0),
	}).Error)
	ctx := newBillingTestContext(t)
	ctx.Writer.WriteHeaderNow()
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audit-within", "req-audit-within", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	err := SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 20, SubscriptionTokens: 20, ApiKeyTokens: 20})

	require.NoError(t, err)
	assert.Equal(t, int64(20), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(20), getTokenUsed(t, tokenID))
	record := getTokenLimitRecordForTest(t, "req-audit-within")
	assert.Equal(t, model.TokenLimitPreConsumeStatusSettled, record.Status)
	assert.Equal(t, int64(20), record.ActualTokens)
	assert.Equal(t, int64(10), record.DeltaTokens)
	assert.Equal(t, "", record.FailureCode)
}

func TestStreamingTokenLimitSettleFailureIsAuditedWithoutOverLimit(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96081, 96082, 96083, 96084
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-audit-stream", 100, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(12),
		"token_used":          int64(0),
	}).Error)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audit-stream", "req-audit-stream", "subscription_only")
	relayInfo.IsStream = true
	relayInfo.ChannelId = 96085
	relayInfo.SetEstimatePromptTokens(10)
	seedChannel(t, 96085)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 20, SubscriptionTokens: 20, ApiKeyTokens: 20}))
	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 0, 0, 0)

	assert.LessOrEqual(t, getTokenUsed(t, tokenID), int64(12))
	assert.Equal(t, true, other["api_key_token_limit_settle_failed"])
	assert.Equal(t, int64(20), other["api_key_token_limit_actual_tokens"])
	assert.Equal(t, int64(10), other["api_key_token_limit_pre_consumed"])
	assert.Equal(t, "api_key_token_limit_settle_failed", other["api_key_token_limit_failure_code"])
}

func TestStreamingSettleUsesAuditButStreamingCleanupBeforeFirstChunkRefunds(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96091, 96092, 96093, 96094
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-stream-cleanup", 100, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(12),
		"token_used":          int64(0),
	}).Error)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-stream-cleanup", "req-stream-cleanup", "subscription_only")
	relayInfo.IsStream = true
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))
	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 20, SubscriptionTokens: 20, ApiKeyTokens: 20}))
	assert.Equal(t, model.TokenLimitPreConsumeStatusSettleFailed, getTokenLimitRecordForTest(t, "req-stream-cleanup").Status)

	ctx2 := newBillingTestContext(t)
	relayInfo2 := newBillingTestRelayInfo(userID, tokenID, "sk-stream-cleanup", "req-stream-cleanup-before-first", "subscription_only")
	relayInfo2.IsStream = true
	relayInfo2.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx2, relayInfo2, 1)
	relayInfo2.TokenLimit = NewTokenLimitSession(relayInfo2)
	require.Nil(t, relayInfo2.TokenLimit.PreConsume(relayInfo2.SubscriptionPreConsumedTokens()))
	assert.False(t, ResponseAlreadyWritten(ctx2, relayInfo2, false), "stream flag alone is not a written response")
	RefundTokenLimitOnRelayFailure(relayInfo2, "client_gone_before_response")
	RefundBillingAfterTokenLimitReject(relayInfo2.Billing)
	assert.Equal(t, model.TokenLimitPreConsumeStatusRefunded, getTokenLimitRecordForTest(t, "req-stream-cleanup-before-first").Status)
}

func TestPreWssConsumeQuotaSettlesApiKeyLimitIncrementally(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96101, 96102, 96103, 96104
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-wss-increment", 100, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(10),
		"token_used":          int64(0),
	}).Error)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-wss-increment", "req-wss-increment", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	relayInfo.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx, relayInfo, 1)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 7, InputTokens: 3, OutputTokens: 4}))
	err := PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 4})

	require.Error(t, err)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, types.ErrorCodeAPIKeyTokenLimitExhausted, apiErr.GetErrorCode())
	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(7), getTokenUsed(t, tokenID))
}

func TestPreWssConsumeQuotaRefundsApiKeyIncrementWhenSubscriptionIncrementFails(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 96111, 96112, 96113, 96114
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-wss-sub-fail", 10, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(100),
		"token_used":          int64(0),
	}).Error)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-wss-sub-fail", "req-wss-sub-fail", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	relayInfo.SetEstimatePromptTokens(9)
	preConsumeForBillingTest(t, ctx, relayInfo, 9)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)

	err := PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 5})

	require.Error(t, err)
	assert.Equal(t, int64(0), getTokenUsed(t, tokenID))
	assert.Equal(t, model.TokenLimitPreConsumeStatusRefunded, getTokenLimitRecordForTest(t, "req-wss-sub-fail:realtime:1").Status)
}

type recordingBillingSettler struct {
	refundCount int
	commitCount int
}

func (s *recordingBillingSettler) Settle(actualQuota int) error  { return nil }
func (s *recordingBillingSettler) Refund(c *gin.Context)         { s.refundCount++ }
func (s *recordingBillingSettler) CommitPreConsumedOnFailure()   { s.commitCount++ }
func (s *recordingBillingSettler) NeedsRefund() bool             { return true }
func (s *recordingBillingSettler) GetPreConsumedQuota() int      { return 0 }
func (s *recordingBillingSettler) Reserve(targetQuota int) error { return nil }
