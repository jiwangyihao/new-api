package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	appLogger "github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/creditbilling"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormlogger "gorm.io/gorm/logger"
)

func ensureSubscriptionBillingTables(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Migrator().DropTable(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}, &model.TokenLimitPreConsumeRecord{}))
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}, &model.TokenLimitPreConsumeRecord{}))
}

func seedDistributorPlan(t *testing.T, id int, code string, tokenLimit int64) {
	t.Helper()
	ensureSubscriptionBillingTables(t)
	plan := &model.SubscriptionPlan{
		Id:                id,
		Title:             code,
		Enabled:           true,
		MonthlyTokenLimit: tokenLimit,
		ConcurrencyLimit:  1,
		BusinessCode:      &code,
	}
	require.NoError(t, model.DB.Create(plan).Error)
	model.InvalidateSubscriptionPlanCache(id)
}

func seedLegacySubscriptionPlan(t *testing.T, id int, title string) {
	t.Helper()
	ensureSubscriptionBillingTables(t)
	plan := &model.SubscriptionPlan{
		Id:      id,
		Title:   title,
		Enabled: true,
	}
	require.NoError(t, model.DB.Create(plan).Error)
	model.InvalidateSubscriptionPlanCache(id)
}

func seedLegacySubscription(t *testing.T, id int, userId int, planId int, amountTotal int64, amountUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		PlanId:      planId,
		AmountTotal: amountTotal,
		AmountUsed:  amountUsed,
		Status:      "active",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedDistributorSubscription(t *testing.T, id int, userId int, planId int, tokenLimit int64, tokenUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		PlanId:      planId,
		AmountTotal: 1,
		TokenLimit:  tokenLimit,
		TokenUsed:   tokenUsed,
		Status:      "active",
		GrantReason: "order",
		StartTime:   time.Now().Add(-time.Minute).Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func getSubscriptionTokenUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("token_used").Where("id = ?", id).First(&sub).Error)
	return sub.TokenUsed
}

func newBillingTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	return ctx
}

func newBillingTestRelayInfo(userId, tokenId int, tokenKey string, requestId string, pref string) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		TokenId:         tokenId,
		TokenKey:        tokenKey,
		UserId:          userId,
		OriginModelName: "gpt-4o",
		RequestId:       requestId,
		StartTime:       time.Now(),
		UserSetting: dto.UserSetting{
			BillingPreference: pref,
		},
		UserQuota: 10_000,
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			CacheRatio:      1,
			QuotaMultiplierInfo: types.QuotaMultiplierInfo{
				Ratio: 1,
			},
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
		RelayMode:   relayconstant.RelayModeChatCompletions,
	}
	return info
}

func preConsumeForBillingTest(t *testing.T, ctx *gin.Context, relayInfo *relaycommon.RelayInfo, estimated int) {
	t.Helper()
	apiErr := PreConsumeBilling(ctx, estimated, relayInfo)
	require.Nil(t, apiErr)
}

func assertSubscriptionBillingSettle(t *testing.T, name string, usage *dto.Usage, wantTokens int64) {
	t.Helper()
	truncate(t)
	const userID = 8001
	const tokenID = 8002
	const planID = 8003
	const subID = 8004
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-subscription-"+name, 10_000)
	seedDistributorPlan(t, planID, "plan-"+name, 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-subscription-"+name, "req-"+name, "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{
		WalletQuota:        999,
		SubscriptionTokens: wantTokens,
		UsageEstimated:     false,
	}))

	assert.Equal(t, wantTokens, getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getUserQuota(t, userID), "subscription billing must not deduct wallet quota")
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not deduct token key quota")
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID), "subscription billing must not update token key used quota")
	assert.Equal(t, int64(1_000), relayInfo.SubscriptionAmountTotal, "compat field must expose token limit for distributor subscription")
	assert.Equal(t, wantTokens, relayInfo.SubscriptionAmountUsedAfterPreConsume+relayInfo.SubscriptionPostDelta, "compat field must expose token used for distributor subscription")
	assert.Equal(t, wantTokens, relayInfo.SubscriptionPreConsumed+relayInfo.SubscriptionPostDelta, "subscription consumed compatibility field must use token units")
}

func TestSubscriptionBillingPreConsumesEstimatedTokens(t *testing.T) {
	truncate(t)
	const userID = 8041
	const tokenID = 8042
	const planID = 8043
	const subID = 8044
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-estimated-preconsume", 10_000)
	seedDistributorPlan(t, planID, "plan-estimated-preconsume", 5_000)
	seedDistributorSubscription(t, subID, userID, planID, 5_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-estimated-preconsume", "req-estimated-preconsume", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 1000)

	assert.Equal(t, int64(10), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
}

type subscriptionPlanInfoReadLogger struct {
	gormlogger.Interface
	reads atomic.Int64
}

func (l *subscriptionPlanInfoReadLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	if strings.Contains(sql, "FROM `user_subscriptions`") && strings.Contains(sql, "WHERE id = 8044") {
		l.reads.Add(1)
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

type subscriptionSettleSQLLogger struct {
	gormlogger.Interface
	reads atomic.Int64
}

func (l *subscriptionSettleSQLLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	if strings.Contains(sql, "FROM `user_subscriptions`") && strings.Contains(sql, "WHERE id = 8054") {
		l.reads.Add(1)
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

func TestSubscriptionBillingSettleUsesSingleEntitlementGuardRead(t *testing.T) {
	truncate(t)
	const userID = 8051
	const tokenID = 8052
	const planID = 8053
	const subID = 8054
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-settle-no-read", 10_000)
	seedDistributorPlan(t, planID, "plan-settle-no-read", 5_000)
	seedDistributorSubscription(t, subID, userID, planID, 5_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-settle-no-read", "req-settle-no-read", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)

	readLogger := &subscriptionSettleSQLLogger{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	restoreLogger := model.DB.Config.Logger
	model.DB.Config.Logger = readLogger
	t.Cleanup(func() { model.DB.Config.Logger = restoreLogger })

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{SubscriptionTokens: 28}))

	assert.Equal(t, int64(1), readLogger.reads.Load(), "timed settlement performs one non-locking entitlement guard read")
	assert.Equal(t, int64(28), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(10), relayInfo.SubscriptionTokenUsedAfterPreConsume)
	assert.Equal(t, int64(18), relayInfo.SubscriptionPostDelta)
}

func TestBillingSessionSettlesAndRefundsRequestsStartedBeforeSubscriptionConversion(t *testing.T) {
	tests := []struct {
		name       string
		settle     func(t *testing.T, session *BillingSession)
		wantLimit  int64
		wantUsed   int64
		wantStatus string
	}{
		{
			name: "positive settlement creates debt and rejects a new request",
			settle: func(t *testing.T, session *BillingSession) {
				require.NoError(t, session.SettleWithInput(BillingSettleInput{SubscriptionTokens: 250}))
			},
			wantLimit: 90,
			wantUsed:  240,
		},
		{
			name: "refund restores Credit without rewriting source usage",
			settle: func(t *testing.T, session *BillingSession) {
				session.refundSync()
			},
			wantLimit:  100,
			wantUsed:   0,
			wantStatus: "refunded",
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncate(t)
			userID := 8_061 + index*10
			tokenID := userID + 1
			timedPlanID := userID + 2
			creditPlanID := userID + 3
			sourceID := userID + 4
			requestID := fmt.Sprintf("converted-billing-session-%d", index)
			tokenKey := fmt.Sprintf("sk-converted-billing-session-%d", index)
			seedUser(t, userID, 10_000)
			seedToken(t, tokenID, userID, tokenKey, 10_000)
			ensureSubscriptionBillingTables(t)
			timedCode := fmt.Sprintf("converted-billing-timed-%d", index)
			creditCode := fmt.Sprintf("converted-billing-credit-%d", index)
			require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
				Id: timedPlanID, Title: "Convertible billing plan", Enabled: true,
				EntitlementType: model.SubscriptionEntitlementTimed, BusinessCode: &timedCode,
				DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
				QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100,
				TimedConversionEnabled: true,
			}).Error)
			require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
				Id: creditPlanID, Title: "Credit balance", Enabled: true,
				EntitlementType: model.SubscriptionEntitlementCreditBalance, BusinessCode: &creditCode,
				CreditBalanceConfigured: true, CreditBalanceConversionEnabled: true,
			}).Error)
			now := model.GetDBTimestamp()
			basis := int64(100)
			require.NoError(t, model.DB.Create(&model.UserSubscription{
				Id: sourceID, UserId: userID, PlanId: timedPlanID,
				EntitlementType: model.SubscriptionEntitlementTimed, TokenLimit: 100,
				GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
				StartTime: now - 48*60*60, EndTime: now + 60*60,
				Status: model.SubscriptionStatusActive, LastGrantedAt: now - 48*60*60,
				LastGrantCreditSnapshot: &basis,
				LastGrantTimeSource:     model.SubscriptionGrantTimeSourceLive,
				LastGrantSource:         model.SubscriptionGrantOrder,
			}).Error)
			require.NoError(t, model.SetUserActiveSubscription(userID, sourceID))

			ctx := newBillingTestContext(t)
			relayInfo := newBillingTestRelayInfo(userID, tokenID, tokenKey, requestID, "subscription_only")
			relayInfo.SetEstimatePromptTokens(10)
			preConsumeForBillingTest(t, ctx, relayInfo, 10)
			session, ok := relayInfo.Billing.(*BillingSession)
			require.True(t, ok)
			require.Equal(t, sourceID, session.funding.(*SubscriptionFunding).subscriptionId)

			conversion, err := model.ConfirmTimedSubscriptionConversion(userID, sourceID, fmt.Sprintf("billing-session-conversion-%d", index))
			require.NoError(t, err)
			targetID := conversion.Conversion.TargetSubscriptionId
			test.settle(t, session)

			var source model.UserSubscription
			require.NoError(t, model.DB.First(&source, sourceID).Error)
			assert.Equal(t, model.SubscriptionStatusConverted, source.Status)
			assert.Equal(t, int64(10), source.TokenUsed)
			var target model.UserSubscription
			require.NoError(t, model.DB.First(&target, targetID).Error)
			assert.Equal(t, test.wantLimit, target.TokenLimit)
			assert.Equal(t, test.wantUsed, target.TokenUsed)
			var preConsume model.SubscriptionPreConsumeRecord
			require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&preConsume).Error)
			assert.Equal(t, sourceID, preConsume.UserSubscriptionId)
			if test.wantStatus != "" {
				assert.Equal(t, test.wantStatus, preConsume.Status)
			}

			if target.TokenUsed > target.TokenLimit {
				newRelayInfo := newBillingTestRelayInfo(userID, tokenID, tokenKey, requestID+"-new", "subscription_only")
				newRelayInfo.SetEstimatePromptTokens(1)
				apiErr := PreConsumeBilling(ctx, 1, newRelayInfo)
				require.NotNil(t, apiErr)
				var unchangedTarget model.UserSubscription
				require.NoError(t, model.DB.First(&unchangedTarget, targetID).Error)
				assert.Equal(t, target.TokenLimit, unchangedTarget.TokenLimit)
				assert.Equal(t, target.TokenUsed, unchangedTarget.TokenUsed)
				var newRecordCount int64
				require.NoError(t, model.DB.Model(&model.SubscriptionPreConsumeRecord{}).
					Where("request_id = ?", requestID+"-new").Count(&newRecordCount).Error)
				assert.Zero(t, newRecordCount)
			}
		})
	}
}

func TestSubscriptionBillingPreConsumeUsesReturnedPlanMetadata(t *testing.T) {
	truncate(t)
	const userID = 8041
	const tokenID = 8042
	const planID = 8043
	const subID = 8044
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-plan-metadata", 10_000)
	seedDistributorPlan(t, planID, "plan-metadata", 5_000)
	seedDistributorSubscription(t, subID, userID, planID, 5_000, 0)

	readLogger := &subscriptionPlanInfoReadLogger{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	restoreLogger := model.DB.Config.Logger
	model.DB.Config.Logger = readLogger
	t.Cleanup(func() { model.DB.Config.Logger = restoreLogger })

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-plan-metadata", "req-plan-metadata", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 1000)

	assert.Equal(t, planID, relayInfo.SubscriptionPlanId)
	assert.Equal(t, "plan-metadata", relayInfo.SubscriptionPlanTitle)
	assert.Equal(t, int64(0), readLogger.reads.Load(), "preconsume should not reread the chosen subscription only to get plan metadata")
}

func TestPreConsumeBillingSyncsSubscriptionTrialMarker(t *testing.T) {
	truncate(t)
	const userID = 9061
	const tokenID = 9062
	const planID = 9063
	const subID = 9064
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-trial-marker", 10_000)
	ensureSubscriptionBillingTables(t)
	trialCode := "trial-marker-billing"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Trial Marker", Enabled: true, IsTrial: true, BusinessCode: &trialCode}).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subID, UserId: userID, PlanId: planID, EntitlementType: model.SubscriptionEntitlementTimed, AmountTotal: 1, TokenLimit: 0, TokenUsed: 0, Status: "active", GrantReason: "trial_code", StartTime: now - 60, EndTime: now + int64(24*time.Hour/time.Second)}).Error)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-trial-marker", "req-trial-marker", "subscription_only")

	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.Nil(t, apiErr)
	assert.Equal(t, "trial", relayInfo.SubscriptionTrialMarker)
}

func TestClearRelayBillingStateClearsSubscriptionTrialMarker(t *testing.T) {
	info := &relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"}

	clearRelayBillingState(info)

	assert.Empty(t, info.SubscriptionTrialMarker)
}

func TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields(t *testing.T) {
	truncate(t)
	const userID = 8051
	const tokenID = 8052
	const planID = 8053
	const subID = 8054
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-reserve", 10_000)
	seedDistributorPlan(t, planID, "plan-reserve", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-reserve", "req-reserve", "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	require.NotNil(t, relayInfo.Billing)
	require.NoError(t, relayInfo.Billing.Reserve(100))
	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 100, SubscriptionTokens: 100}))

	assert.Equal(t, int64(100), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(100), relayInfo.SubscriptionAmountUsedAfterPreConsume+relayInfo.SubscriptionPostDelta)
	assert.Equal(t, int64(100), relayInfo.SubscriptionPreConsumed+relayInfo.SubscriptionPostDelta)
}

func TestSettleBillingWrapperDoesNotSynthesizeSubscriptionTokens(t *testing.T) {
	truncate(t)
	const userID = 8071
	const tokenID = 8072
	const planID = 8073
	const subID = 8074
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-wrapper", 10_000)
	seedDistributorPlan(t, planID, "plan-wrapper", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-wrapper", "req-wrapper", "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	require.NoError(t, SettleBilling(ctx, relayInfo, 999))

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID), "legacy/non-text settlement wrapper must not synthesize distributor token usage from wallet quota")
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not settle token key quota")
	assert.Equal(t, -relayInfo.SubscriptionPreConsumed, relayInfo.SubscriptionPostDelta)
}

func TestSettleBillingWrapperRejectsLegacySubscriptionQuota(t *testing.T) {
	truncate(t)
	const userID = 8081
	const tokenID = 8082
	const planID = 8083
	const subID = 8084
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-legacy-wrapper", 10_000)
	seedLegacySubscriptionPlan(t, planID, "legacy-wrapper")
	seedLegacySubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-legacy-wrapper", "req-legacy-wrapper", "subscription_only")

	apiErr := PreConsumeBilling(ctx, 10, relayInfo)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "subscription")
	require.NoError(t, SettleBilling(ctx, relayInfo, 0))

	assert.Equal(t, int64(0), getSubscriptionUsed(t, subID), "legacy amount subscription must not participate in request billing")
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not settle token key quota")
	assert.Empty(t, relayInfo.BillingSource)
}

func TestSubscriptionBillingAllowsCompletionsTextRelay(t *testing.T) {
	truncate(t)
	const userID = 8085
	const tokenID = 8086
	const planID = 8087
	const subID = 8088
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-completions", 10_000)
	seedDistributorPlan(t, planID, "plan-completions", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-completions", "req-completions", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeCompletions
	relayInfo.SetEstimatePromptTokens(7)

	preConsumeForBillingTest(t, ctx, relayInfo, 7)
	session, ok := relayInfo.Billing.(*BillingSession)
	require.True(t, ok)
	require.NoError(t, session.SettleWithInput(BillingSettleInput{SubscriptionTokens: 11}))

	assert.Equal(t, int64(11), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not consume token key quota")
}

func TestSubscriptionBillingSettlesNativeClaudeTextRelay(t *testing.T) {
	truncate(t)
	const userID = 8089
	const tokenID = 8090
	const planID = 8096
	const subID = 8097
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-claude-native", 10_000)
	seedDistributorPlan(t, planID, "plan-claude-native", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-claude-native", "req-claude-native", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatClaude
	relayInfo.RelayMode = relayconstant.RelayModeUnknown
	relayInfo.SetEstimatePromptTokens(5)

	preConsumeForBillingTest(t, ctx, relayInfo, 5)
	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, TotalTokens: 9}, nil))

	assert.Equal(t, int64(9), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not consume token key quota")
}

func TestSubscriptionBillingSettlesNativeGeminiTextRelay(t *testing.T) {
	truncate(t)
	const userID = 8098
	const tokenID = 8099
	const planID = 8100
	const subID = 8105
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-gemini-native", 10_000)
	seedDistributorPlan(t, planID, "plan-gemini-native", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	ctx.Request = httptest.NewRequest("POST", "/v1beta/models/gemini-pro:generateContent", nil)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-gemini-native", "req-gemini-native", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatGemini
	relayInfo.RelayMode = relayconstant.RelayModeGemini
	relayInfo.RequestURLPath = "/v1beta/models/gemini-pro:generateContent"
	relayInfo.SetEstimatePromptTokens(5)

	preConsumeForBillingTest(t, ctx, relayInfo, 5)
	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{UsageSemantic: "gemini", PromptTokens: 5, TotalTokens: 9}, nil))

	assert.Equal(t, int64(9), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not consume token key quota")
}

func TestPostTextConsumeQuotaResponsesAndChatSettleUsageTotalTokens(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode int
		path string
	}{
		{name: "chat", mode: relayconstant.RelayModeChatCompletions, path: "/v1/chat/completions"},
		{name: "responses", mode: relayconstant.RelayModeResponses, path: "/v1/responses"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			const userID = 8121
			const tokenID = 8122
			const planID = 8123
			const subID = 8124
			seedUser(t, userID, 10_000)
			seedToken(t, tokenID, userID, "sk-usage-"+tc.name, 10_000)
			seedDistributorPlan(t, planID, "plan-usage-"+tc.name, 1_000)
			seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

			ctx := newBillingTestContext(t)
			ctx.Request = httptest.NewRequest("POST", tc.path, nil)
			relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-usage-"+tc.name, "req-usage-"+tc.name, "subscription_only")
			relayInfo.RelayMode = tc.mode
			relayInfo.RequestURLPath = tc.path
			relayInfo.SetEstimatePromptTokens(11)

			preConsumeForBillingTest(t, ctx, relayInfo, 11)
			require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}, nil))

			assert.Equal(t, int64(28), getSubscriptionTokenUsed(t, subID))
		})
	}
}

func TestSubscriptionBillingRejectsNativeGeminiEmbeddingRelay(t *testing.T) {
	truncate(t)
	const userID = 8106
	const tokenID = 8107
	const planID = 8108
	const subID = 8109
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-gemini-embed", 10_000)
	seedDistributorPlan(t, planID, "plan-gemini-embed", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	ctx.Request = httptest.NewRequest("POST", "/v1beta/models/text-embedding-004:embedContent", nil)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-gemini-embed", "req-gemini-embed", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatGemini
	relayInfo.RelayMode = relayconstant.RelayModeGemini
	relayInfo.RequestURLPath = "/v1beta/models/text-embedding-004:embedContent"
	relayInfo.SetEstimatePromptTokens(5)

	apiErr := PreConsumeBilling(ctx, 5, relayInfo)
	require.NotNil(t, apiErr)
	assert.Empty(t, relayInfo.BillingSource)
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not consume token key quota")
}

func TestLegacySubscriptionPreConsumeUsesQuotaUnits(t *testing.T) {
	truncate(t)
	const userID = 8101
	const tokenID = 8102
	const planID = 8103
	const subID = 8104
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-legacy-preconsume", 10_000)
	seedLegacySubscriptionPlan(t, planID, "legacy-preconsume")
	seedLegacySubscription(t, subID, userID, planID, 200, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-legacy-preconsume", "req-legacy-preconsume", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	apiErr := PreConsumeBilling(ctx, 1_000, relayInfo)

	require.NotNil(t, apiErr)
	assert.Equal(t, int64(0), getSubscriptionUsed(t, subID))
}

func TestPostTextConsumeQuotaRejectsLegacySubscriptionQuota(t *testing.T) {
	truncate(t)
	const userID = 8111
	const tokenID = 8112
	const planID = 8113
	const subID = 8114
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-legacy-text", 10_000)
	seedChannel(t, 8115)
	seedLegacySubscriptionPlan(t, planID, "legacy-text")
	seedLegacySubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-legacy-text", "req-legacy-text", "subscription_only")
	relayInfo.ChannelId = 8115
	relayInfo.RelayMode = relayconstant.RelayModeChatCompletions
	relayInfo.PriceData.ModelRatio = 2
	apiErr := PreConsumeBilling(ctx, 10, relayInfo)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeSubscriptionRequired, apiErr.GetErrorCode())

	if relayInfo.Billing != nil {
		require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 10, TotalTokens: 10}, nil))
	}

	assert.Equal(t, int64(0), getSubscriptionUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not update token key quota")
}

func TestLegacySubscriptionNotificationUsesQuotaFormatting(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		BillingSource:                         BillingSourceSubscription,
		SubscriptionId:                        1,
		SubscriptionPreConsumed:               0,
		SubscriptionAmountTotal:               100,
		SubscriptionAmountUsedAfterPreConsume: 99,
		SubscriptionPostDelta:                 0,
	}
	remaining := relayInfo.SubscriptionAmountTotal - (relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta)
	remainingText := subscriptionRemainingText(false, remaining)
	assert.Equal(t, appLogger.FormatQuota(1), remainingText)
}

func TestDistributorSubscriptionNotificationUsesTokenFormatting(t *testing.T) {
	truncate(t)
	const userID = 8061
	const tokenID = 8062
	const planID = 8063
	const subID = 8064
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-notify", 10_000)
	seedDistributorPlan(t, planID, "plan-notify", 100)
	seedDistributorSubscription(t, subID, userID, planID, 100, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-notify", "req-notify", "subscription_only")
	relayInfo.SetEstimatePromptTokens(99)
	preConsumeForBillingTest(t, ctx, relayInfo, 99)

	remaining := relayInfo.SubscriptionAmountTotal - relayInfo.SubscriptionAmountUsedAfterPreConsume
	assert.Equal(t, "1 tokens", subscriptionRemainingText(true, remaining))
}

func TestSubscriptionBillingUsesMeteredTokens(t *testing.T) {
	tests := []struct {
		name  string
		usage *dto.Usage
		want  int64
	}{
		{
			name:  "openai_total_tokens",
			usage: &dto.Usage{TotalTokens: 8},
			want:  8,
		},
		{
			name: "responses_cached_tokens",
			usage: &dto.Usage{
				PromptTokens:        100,
				CompletionTokens:    50,
				TotalTokens:         150,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 40},
			},
			want: 150,
		},
		{
			name: "gemini_cached_content",
			usage: &dto.Usage{
				UsageSemantic:       "gemini",
				PromptTokens:        100,
				CompletionTokens:    50,
				TotalTokens:         150,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 40},
			},
			want: 150,
		},
		{
			name: "claude_cache_creation_remainder",
			usage: &dto.Usage{
				UsageSemantic:    "anthropic",
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
				PromptTokensDetails: dto.InputTokenDetails{
					CachedTokens:         30,
					CachedCreationTokens: 50,
				},
				ClaudeCacheCreation5mTokens: 7,
				ClaudeCacheCreation1hTokens: 11,
			},
			want: 230,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSubscriptionBillingSettle(t, tt.name, tt.usage, tt.want)
		})
	}
}

func TestSubscriptionBillingDoesNotChargeWhenUsageMissing(t *testing.T) {
	truncate(t)
	const userID = 8011
	const tokenID = 8012
	const planID = 8013
	const subID = 8014
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-missing-usage", 10_000)
	seedChannel(t, 8015)
	seedDistributorPlan(t, planID, "plan-missing-usage", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-missing-usage", "req-missing-usage", "subscription_only")
	relayInfo.ChannelId = 8015
	relayInfo.RelayMode = relayconstant.RelayModeChatCompletions
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, nil, nil))

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getUserQuota(t, userID), "missing usage must not deduct wallet quota")
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "missing usage must not use token key quota")
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, 0, log.Quota)
	assert.Equal(t, 0, log.PromptTokens)
	assert.Equal(t, 0, log.CompletionTokens)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, true, other["usage_unavailable"])
	assert.Nil(t, other["usage_estimated"])
}

func TestTokenLimitDoesNotChargeWhenUsageMissing(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 80121, 80122, 80123, 80124
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-missing-usage-key-cap", 10_000)
	seedChannel(t, 80125)
	seedDistributorPlan(t, planID, "plan-missing-usage-key-cap", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(100),
		"token_used":          int64(0),
	}).Error)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-missing-usage-key-cap", "req-missing-usage-key-cap", "subscription_only")
	relayInfo.ChannelId = 80125
	relayInfo.RelayMode = relayconstant.RelayModeChatCompletions
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, nil, nil))

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(0), getTokenUsed(t, tokenID))
	record := getTokenLimitRecordForTest(t, "req-missing-usage-key-cap")
	assert.Equal(t, model.TokenLimitPreConsumeStatusSettled, record.Status)
	assert.Equal(t, int64(0), record.ActualTokens)
}

func TestSubscriptionBillingDoesNotChargeLocalCountUsage(t *testing.T) {
	truncate(t)
	const userID = 8021
	const tokenID = 8022
	const planID = 8023
	const subID = 8024
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-local-usage", 10_000)
	seedChannel(t, 8025)
	seedDistributorPlan(t, planID, "plan-local-usage", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-local-usage", "req-local-usage", "subscription_only")
	relayInfo.ChannelId = 8025
	relayInfo.RelayMode = relayconstant.RelayModeChatCompletions
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	usage := ResponseText2Usage(ctx, "locally counted response", relayInfo.OriginModelName, relayInfo.GetEstimatePromptTokens())
	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, usage, nil))

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, 0, log.Quota)
	assert.Equal(t, 0, log.PromptTokens)
	assert.Equal(t, 0, log.CompletionTokens)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, true, other["usage_estimated"])
	assert.Equal(t, true, other["usage_unavailable"])
}

func TestPostTextConsumeQuotaReturnsSettleErrorWhenSubscriptionTokensExhausted(t *testing.T) {
	truncate(t)
	const userID = 8016
	const tokenID = 8017
	const planID = 8018
	const subID = 8019
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-settle-exhausted", 10_000)
	seedChannel(t, 8020)
	seedDistributorPlan(t, planID, "plan-settle-exhausted", 10)
	seedDistributorSubscription(t, subID, userID, planID, 10, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-settle-exhausted", "req-settle-exhausted", "subscription_only")
	relayInfo.ChannelId = 8020
	relayInfo.RelayMode = relayconstant.RelayModeChatCompletions
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	err := PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6, TotalTokens: 11}, nil)

	require.Error(t, err)
	assert.Equal(t, int64(6), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not use token key quota")
}

func TestPreWssConsumeQuotaUsesSubscriptionTokensOnly(t *testing.T) {
	truncate(t)
	const userID = 8021
	const tokenID = 8022
	const planID = 8023
	const subID = 8024
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-realtime-sub", 10_000)
	seedDistributorPlan(t, planID, "plan-realtime-sub", 100)
	seedDistributorSubscription(t, subID, userID, planID, 100, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-realtime-sub", "req-realtime-sub", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	relayInfo.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx, relayInfo, 1)

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 4, InputTokens: 2, OutputTokens: 2}))

	assert.Equal(t, int64(5), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "realtime subscription billing must not consume token key quota")
	assert.Equal(t, 10_000, getUserQuota(t, userID), "realtime subscription billing must not consume wallet quota")
}

func TestPreWssConsumeQuotaAccumulatesMultipleRealtimeChunks(t *testing.T) {
	truncate(t)
	const userID = 8027
	const tokenID = 8028
	const planID = 8029
	const subID = 8030
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-realtime-multi", 10_000)
	seedDistributorPlan(t, planID, "plan-realtime-multi", 100)
	seedDistributorSubscription(t, subID, userID, planID, 100, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-realtime-multi", "req-realtime-multi", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	relayInfo.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx, relayInfo, 1)

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 4}))
	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 3}))

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "realtime subscription billing must not consume token key quota")
	assert.Equal(t, 10_000, getUserQuota(t, userID), "realtime subscription billing must not consume wallet quota")
}

func TestPreWssConsumeQuotaRequiresSubscriptionBilling(t *testing.T) {
	truncate(t)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(8025, 8026, "sk-realtime-missing", "req-realtime-missing", "wallet_only")

	err := PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 4})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscription")
}

func TestPostTextConsumeQuotaSkipsDistributorTokensForNonTextRelay(t *testing.T) {
	truncate(t)
	const userID = 8091
	const tokenID = 8092
	const planID = 8093
	const subID = 8094
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-embedding", 10_000)
	seedChannel(t, 8095)
	seedDistributorPlan(t, planID, "plan-embedding", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-embedding", "req-embedding", "subscription_only")
	relayInfo.ChannelId = 8095
	relayInfo.RelayMode = relayconstant.RelayModeEmbeddings
	relayInfo.SetEstimatePromptTokens(6)
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)

	if relayInfo.Billing != nil {
		require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6, TotalTokens: 6}, nil))
	}

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "token key quota must be refunded when distributor subscription is rejected")
}

func TestPreConsumeBillingFreeNonTextModelOnlyRequiresActiveSubscription(t *testing.T) {
	truncate(t)
	const userID = 8096
	const tokenID = 8097
	const planID = 8098
	const subID = 8099
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-free-embedding", 10_000)
	seedDistributorPlan(t, planID, "plan-free-embedding", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-free-embedding", "req-free-embedding", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeEmbeddings
	relayInfo.FreeModel = true
	relayInfo.SetEstimatePromptTokens(6)

	apiErr := PreConsumeBilling(ctx, 0, relayInfo)

	require.Nil(t, apiErr)
	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
}

func TestPreConsumeBillingFreeModelIgnoresLegacyModelLimits(t *testing.T) {
	for index, modelLimits := range []string{"mapped-gpt-4o", "gpt-4o"} {
		t.Run(modelLimits, func(t *testing.T) {
			truncate(t)
			userID := 8100 + index*10
			tokenID := userID + 1
			planID := userID + 2
			subID := userID + 3
			seedUser(t, userID, 10_000)
			seedToken(t, tokenID, userID, fmt.Sprintf("sk-free-scope-%d", index), 10_000)
			seedDistributorPlan(t, planID, fmt.Sprintf("plan-free-scope-%d", index), 1_000)
			require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", planID).Update("model_limits", modelLimits).Error)
			model.InvalidateSubscriptionPlanCache(planID)
			seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

			ctx := newBillingTestContext(t)
			relayInfo := newBillingTestRelayInfo(userID, tokenID, fmt.Sprintf("sk-free-scope-%d", index), fmt.Sprintf("req-free-scope-%d", index), "subscription_only")
			relayInfo.FreeModel = true
			relayInfo.BillingModelName = "mapped-gpt-4o"
			relayInfo.OriginModelName = "gpt-4o"

			session, apiErr := NewBillingSession(ctx, relayInfo, 0)

			require.Nil(t, apiErr)
			assert.Nil(t, session)
			assert.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
		})
	}
}
func TestPostAudioConsumeQuotaDoesNotConsumeDistributorSubscription(t *testing.T) {
	truncate(t)
	const userID = 8121
	const tokenID = 8122
	const planID = 8123
	const subID = 8124
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-audio", 10_000)
	seedChannel(t, 8125)
	seedDistributorPlan(t, planID, "plan-audio", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audio", "req-audio", "subscription_only")
	relayInfo.ChannelId = 8125
	relayInfo.RelayMode = relayconstant.RelayModeAudioSpeech
	relayInfo.SetEstimatePromptTokens(6)
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)

	PostAudioConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6, TotalTokens: 6}, "")
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
}

func TestPostAudioConsumeQuotaRejectsDistributorSubscriptionFirstWithoutWalletFallback(t *testing.T) {
	truncate(t)
	const userID = 8131
	const tokenID = 8132
	const planID = 8133
	const subID = 8134
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-audio-first", 10_000)
	seedChannel(t, 8135)
	seedDistributorPlan(t, planID, "plan-audio-first", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audio-first", "req-audio-first", "subscription_first")
	relayInfo.ChannelId = 8135
	relayInfo.RelayMode = relayconstant.RelayModeAudioSpeech
	relayInfo.SetEstimatePromptTokens(6)
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)
	assert.Empty(t, relayInfo.BillingSource)
	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
}

func TestPostAudioConsumeQuotaRejectsExhaustedDistributorSubscriptionFirstWithoutWalletFallback(t *testing.T) {
	truncate(t)
	const userID = 8141
	const tokenID = 8142
	const planID = 8143
	const subID = 8144
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-audio-exhausted", 10_000)
	seedChannel(t, 8145)
	seedDistributorPlan(t, planID, "plan-audio-exhausted", 5)
	seedDistributorSubscription(t, subID, userID, planID, 5, 5)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audio-exhausted", "req-audio-exhausted", "subscription_first")
	relayInfo.ChannelId = 8145
	relayInfo.RelayMode = relayconstant.RelayModeAudioSpeech
	relayInfo.SetEstimatePromptTokens(6)
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)
	assert.Empty(t, relayInfo.BillingSource)
	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(5), getSubscriptionTokenUsed(t, subID))
}

func TestWalletOnlyPreferenceRequiresSubscriptionForRequestBilling(t *testing.T) {
	truncate(t)
	const userID = 8021
	const tokenID = 8022
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-wallet", 10_000)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-wallet", "req-wallet", "wallet_only")
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "active subscription is required")
	assert.Empty(t, relayInfo.BillingSource)
	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID))
}

func TestTokenKeyQuotaDoesNotChangeWhenSubscriptionUsesTokens(t *testing.T) {
	truncate(t)
	const userID = 8031
	const tokenID = 8032
	const planID = 8033
	const subID = 8034
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-token-key", 10_000)
	seedDistributorPlan(t, planID, "plan-token-key", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-token-key", "req-token-key", "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{
		WalletQuota:        999,
		SubscriptionTokens: 8,
		UsageEstimated:     false,
	}))

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID))
}

func TestPreConsumeBillingPaidSubscriptionCodexProEligibleSyncsRelayInfo(t *testing.T) {
	for _, tc := range []struct {
		name        string
		grantSource string
	}{
		{name: "order", grantSource: model.SubscriptionGrantOrder},
		{name: "redemption", grantSource: "redemption"},
		{name: "admin_after_sales", grantSource: "admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			const userID = 8211
			const tokenID = 8212
			const planID = 8213
			const subID = 8214
			seedUser(t, userID, 10_000)
			seedToken(t, tokenID, userID, "sk-codex-pro-eligible-"+tc.name, 10_000)
			seedCodexProBillingPlan(t, planID, "codex-pro-eligible-"+tc.name, 100, 80, false, false)
			seedCodexProBillingSubscription(t, subID, userID, planID, 100, 0, tc.grantSource, tc.grantSource, "active", time.Now().Add(24*time.Hour).Unix())

			ctx := newBillingTestContext(t)
			relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-eligible-"+tc.name, "req-codex-pro-eligible-"+tc.name, "subscription_only")
			relayInfo.RelayMode = relayconstant.RelayModeResponses
			relayInfo.SetEstimatePromptTokens(10)

			apiErr := PreConsumeBilling(ctx, 10, relayInfo)

			require.Nil(t, apiErr)
			requireCodexProRelayInfoEligibility(t, relayInfo, true)
		})
	}
}

func TestPreConsumeBillingUsesAPIKeyCodexProModeOverride(t *testing.T) {
	truncate(t)
	const userID = 8275
	const tokenID = 8276
	const planID = 8277
	const subID = 8278
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-token-mode", 10_000)
	seedCodexProBillingPlan(t, planID, "codex-pro-token-mode", 100, 80, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 100, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(24*time.Hour).Unix())

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-token-mode", "req-codex-pro-token-mode", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.TokenCodexProMode = common.CodexProModeOff
	relayInfo.UserSetting.CodexProMode = common.CodexProModeAll
	relayInfo.SetEstimatePromptTokens(10)

	apiErr := PreConsumeBilling(ctx, 10, relayInfo)

	require.Nil(t, apiErr)
	requireCodexProRelayInfoEligibility(t, relayInfo, true)
	assert.Equal(t, common.CodexProModeOff, relayInfo.CodexProMode)
}

func TestPreConsumeBillingInheritsUserCodexProModeWhenAPIKeyInherits(t *testing.T) {
	truncate(t)
	const userID = 8285
	const tokenID = 8286
	const planID = 8287
	const subID = 8288
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-token-inherit", 10_000)
	seedCodexProBillingPlan(t, planID, "codex-pro-token-inherit", 100, 80, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 100, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(24*time.Hour).Unix())

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-token-inherit", "req-codex-pro-token-inherit", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.TokenCodexProMode = common.CodexProModeInherit
	relayInfo.UserSetting.CodexProMode = common.CodexProModeAll
	relayInfo.SetEstimatePromptTokens(10)

	apiErr := PreConsumeBilling(ctx, 10, relayInfo)

	require.Nil(t, apiErr)
	requireCodexProRelayInfoEligibility(t, relayInfo, true)
	assert.Equal(t, common.CodexProModeAll, relayInfo.CodexProMode)
}

func TestPreConsumeBillingGlobalCodexProHiddenDisablesRelayPro(t *testing.T) {
	truncate(t)
	oldHidden := common.CodexProFeaturesHidden
	common.CodexProFeaturesHidden = true
	t.Cleanup(func() { common.CodexProFeaturesHidden = oldHidden })
	const userID = 8295
	const tokenID = 8296
	const planID = 8297
	const subID = 8298
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-hidden", 10_000)
	seedCodexProBillingPlan(t, planID, "codex-pro-hidden", 100, 80, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 100, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(24*time.Hour).Unix())

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-hidden", "req-codex-pro-hidden", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.TokenCodexProMode = common.CodexProModeAll
	relayInfo.UserSetting.CodexProMode = common.CodexProModeAll
	relayInfo.SetEstimatePromptTokens(10)

	apiErr := PreConsumeBilling(ctx, 10, relayInfo)

	require.Nil(t, apiErr)
	assert.Equal(t, common.CodexProModeOff, relayInfo.CodexProMode)
	assert.Equal(t, false, relayInfo.CodexProEligible)
	assert.Equal(t, "features_hidden", relayInfo.CodexProUnavailableReason)
	assert.False(t, relayInfo.CodexProRequestSent)
}

func TestPreConsumeBillingWalletOnlySettingWithPaidSubscriptionStillCodexProEligible(t *testing.T) {
	truncate(t)
	const userID = 8265
	const tokenID = 8266
	const planID = 8267
	const subID = 8268
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-wallet-only-paid", 10_000)
	seedCodexProBillingPlan(t, planID, "codex-pro-wallet-only-paid", 100, 80, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 100, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(24*time.Hour).Unix())

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-wallet-only-paid", "req-codex-pro-wallet-only-paid", "wallet_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.SetEstimatePromptTokens(10)

	apiErr := PreConsumeBilling(ctx, 10, relayInfo)

	require.Nil(t, apiErr)
	requireCodexProRelayInfoEligibility(t, relayInfo, true)
}

func TestPreConsumeBillingCodexProUnavailableSyncsActualSubscriptionReason(t *testing.T) {
	for _, tc := range []struct {
		name        string
		price       float64
		isTrial     bool
		inviteTrial bool
		grantReason string
		source      string
		tokenLimit  int64
	}{
		{name: "is_trial", price: 80, isTrial: true, grantReason: "trial_code", source: "trial_code", tokenLimit: 0},
		{name: "invite_trial", price: 80, inviteTrial: true, grantReason: "invite_trial", source: "invite_trial", tokenLimit: 0},
		{name: "trial_code_grant_reason", price: 80, grantReason: "trial_code", source: model.SubscriptionGrantOrder, tokenLimit: 100},
		{name: "invite_trial_source", price: 80, grantReason: model.SubscriptionGrantOrder, source: "invite_trial", tokenLimit: 100},
		{name: "monthly_invite_entitlement_grant_reason", price: 80, grantReason: model.SubscriptionGrantMonthlyInviteEntitlement, source: model.SubscriptionGrantOrder, tokenLimit: 100},
		{name: "monthly_invite_entitlement_source", price: 80, grantReason: model.SubscriptionGrantOrder, source: model.SubscriptionGrantMonthlyInviteEntitlement, tokenLimit: 100},
		{name: "zero_price", price: 0, grantReason: model.SubscriptionGrantOrder, source: model.SubscriptionGrantOrder, tokenLimit: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			const userID = 8221
			const tokenID = 8222
			const planID = 8223
			const subID = 8224
			seedUser(t, userID, 10_000)
			seedToken(t, tokenID, userID, "sk-codex-pro-unavailable-"+tc.name, 10_000)
			seedCodexProBillingPlan(t, planID, "codex-pro-unavailable-"+tc.name, tc.tokenLimit, tc.price, tc.isTrial, tc.inviteTrial)
			seedCodexProBillingSubscription(t, subID, userID, planID, tc.tokenLimit, 0, tc.grantReason, tc.source, "active", time.Now().Add(24*time.Hour).Unix())

			ctx := newBillingTestContext(t)
			relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-unavailable-"+tc.name, "req-codex-pro-unavailable-"+tc.name, "subscription_only")
			relayInfo.RelayMode = relayconstant.RelayModeResponses
			relayInfo.SetEstimatePromptTokens(10)

			apiErr := PreConsumeBilling(ctx, 10, relayInfo)

			require.Nil(t, apiErr)
			requireCodexProRelayInfoEligibility(t, relayInfo, false)
		})
	}
}

func TestPreConsumeBillingCodexProUnavailableForExpiredExhaustedAndWalletOnly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		endTime    int64
		tokenUsed  int64
		preference string
		wantError  string
	}{
		{name: "expired", status: "active", endTime: time.Now().Add(-time.Hour).Unix(), preference: "subscription_only", wantError: "active subscription"},
		{name: "exhausted", status: "active", endTime: time.Now().Add(time.Hour).Unix(), tokenUsed: 100, preference: "subscription_only", wantError: "subscription token quota insufficient"},
		{name: "wallet_only_without_subscription", preference: "wallet_only", wantError: "active subscription"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			const userID = 8231
			const tokenID = 8232
			const planID = 8233
			const subID = 8234
			seedUser(t, userID, 10_000)
			seedToken(t, tokenID, userID, "sk-codex-pro-unavailable-"+tc.name, 10_000)
			if tc.name != "wallet_only_without_subscription" {
				seedCodexProBillingPlan(t, planID, "codex-pro-unavailable-"+tc.name, 100, 80, false, false)
				seedCodexProBillingSubscription(t, subID, userID, planID, 100, tc.tokenUsed, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, tc.status, tc.endTime)
			}

			ctx := newBillingTestContext(t)
			relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-unavailable-"+tc.name, "req-codex-pro-unavailable-"+tc.name, tc.preference)
			relayInfo.RelayMode = relayconstant.RelayModeResponses
			relayInfo.SetEstimatePromptTokens(10)

			apiErr := PreConsumeBilling(ctx, 10, relayInfo)

			require.NotNil(t, apiErr)
			assert.Contains(t, apiErr.Error(), tc.wantError)
			requireCodexProRelayInfoEligibility(t, relayInfo, false)
		})
	}
}

func TestSettleBillingWithInputCodexProServedDoesNotDoubleSubscriptionTokens(t *testing.T) {
	truncate(t)
	const userID = 8241
	const tokenID = 8242
	const planID = 8243
	const subID = 8244
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-settle", 10_000)
	seedDistributorPlan(t, planID, "plan-codex-pro-settle", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-settle", "req-codex-pro-settle", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)
	setBoolFieldForTest(t, relayInfo, "CodexProServed", true)

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 999, SubscriptionTokens: 8}))

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(2), relayInfo.SubscriptionPostDelta, "legacy Codex Pro served marker must not add an extra subscription debit")
	assert.Equal(t, 10_000, getUserQuota(t, userID), "Codex Pro multiplier must not touch wallet quota")
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "Codex Pro multiplier must not touch token-key wallet quota")
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID), "Codex Pro multiplier must not record wallet usage")
}

func TestSettleBillingWithInputDoesNotUsePreConsumeQuotaWhenEstimateMissing(t *testing.T) {
	truncate(t)
	const userID = 8246
	const tokenID = 8247
	const planID = 8248
	const subID = 8249
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-missing-estimate-settle", 10_000)
	seedDistributorPlan(t, planID, "plan-missing-estimate-settle", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-missing-estimate-settle", "req-missing-estimate-settle", "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 999, SubscriptionTokens: 8}))

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(7), relayInfo.SubscriptionPostDelta)
}

func TestSettleBillingWithInputCodexProUnavailableUsesSingleSubscriptionTokens(t *testing.T) {
	truncate(t)
	const userID = 8251
	const tokenID = 8252
	const planID = 8253
	const subID = 8254
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-single", 10_000)
	seedDistributorPlan(t, planID, "plan-codex-pro-single", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-single", "req-codex-pro-single", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 999, SubscriptionTokens: 8}))

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(2), relayInfo.SubscriptionPostDelta)
	assert.Equal(t, 10_000, getUserQuota(t, userID))
}

func TestPostTextConsumeQuotaCodexProServedDoesNotDoubleApiKeyTokenLimit(t *testing.T) {
	truncate(t)
	const userID = 8251
	const tokenID = 8252
	const planID = 8253
	const subID = 8254
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-token-limit", 10_000)
	seedChannel(t, 8255)
	seedCodexProBillingPlan(t, planID, "codex-pro-token-limit", 1_000, 100, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 1_000, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(time.Hour).Unix())
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(100),
		"token_used":          int64(0),
	}).Error)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-token-limit", "req-codex-pro-token-limit", "subscription_only")
	relayInfo.ChannelId = 8255
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))
	setBoolFieldForTest(t, relayInfo, "CodexProServed", true)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(8), getTokenUsed(t, tokenID))
	record := getTokenLimitRecordForTest(t, "req-codex-pro-token-limit")
	assert.Equal(t, model.TokenLimitPreConsumeStatusSettled, record.Status)
	assert.Equal(t, int64(8), record.ActualTokens)
}

func TestPostTextConsumeQuotaLogsNewAPIBillingForCodexProServed(t *testing.T) {
	truncate(t)
	const userID = 8351
	const tokenID = 8352
	const planID = 8353
	const subID = 8354
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-billing-log", 10_000)
	seedChannel(t, 8355)
	seedCodexProBillingPlan(t, planID, "codex-pro-billing-log", 1_000, 100, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 1_000, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(time.Hour).Unix())

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-billing-log", "req-codex-pro-billing-log", "subscription_only")
	relayInfo.ChannelId = 8355
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)
	setBoolFieldForTest(t, relayInfo, "CodexProRequestSent", true)
	setBoolFieldForTest(t, relayInfo, "CodexProServed", true)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))
	model.FlushConsumeLogUpdates()

	log := getLastLog(t)
	require.NotNil(t, log)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(1), other["billing_multiplier"])
	assert.Equal(t, "default", other["billing_multiplier_source"])
	assert.Equal(t, "usage_tokens", other["credit_billing_mode"])
	assert.Equal(t, float64(8), other["base_credits"])
	assert.Equal(t, float64(8), other["api_key_credits"])
	assert.Equal(t, float64(8), other["subscription_credits"])
	assert.Equal(t, float64(8), other["final_credits"])
	assert.Equal(t, float64(1), other["dynamic_billing_multiplier"])
	assert.Equal(t, "default", other["dynamic_billing_multiplier_source"])
	assert.Equal(t, float64(8), other["metered_tokens"])
	assert.Equal(t, float64(8), other["billable_tokens"])
	assert.Equal(t, true, other["codex_pro_requested"])
	assert.Equal(t, true, other["codex_pro_served"])

	logs, _, err := model.GetUserLogs(userID, model.LogTypeConsume, 0, time.Now().Add(time.Hour).Unix(), "", "", 0, 1, "", "", "")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	userOther, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.Equal(t, "default", userOther["billing_multiplier_source"])
	assert.Equal(t, float64(1), userOther["billing_multiplier"])
	assert.Equal(t, float64(8), userOther["metered_tokens"])
	assert.Equal(t, float64(8), userOther["billable_tokens"])
}

func TestPostTextConsumeQuotaLogsCodexProSourceWithDiscountedChannelMultiplier(t *testing.T) {
	truncate(t)
	const userID = 8371
	const tokenID = 8372
	const planID = 8373
	const subID = 8374
	const channelID = 8375
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-discounted-channel", 10_000)
	seedChannel(t, channelID)
	seedCodexProBillingPlan(t, planID, "codex-pro-discounted-channel", 1_000, 100, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 1_000, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(time.Hour).Unix())

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-discounted-channel", "req-codex-pro-discounted-channel", "subscription_only")
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, channelID, constant.ChannelTypeOpenAI, 0.5)
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	setBoolFieldForTest(t, relayInfo, "CodexProRequestSent", true)
	setBoolFieldForTest(t, relayInfo, "CodexProServed", true)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6, CompletionTokens: 4, TotalTokens: 10}, nil))
	model.FlushConsumeLogUpdates()

	log := getLastLog(t)
	require.NotNil(t, log)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(10), other["metered_tokens"])
	assert.Equal(t, float64(5), other["billable_tokens"])
	assert.Equal(t, float64(5), other["channel_billable_tokens"])
	assert.Equal(t, float64(5), other["api_key_billable_tokens"])
	assert.Equal(t, float64(5), other["subscription_billable_tokens"])
	assert.Equal(t, float64(1), other["billing_multiplier"])
	assert.Equal(t, "default", other["billing_multiplier_source"])
}

func TestPostTextConsumeQuotaLogsNewAPIBillingForFreeAndMissingUsage(t *testing.T) {
	for _, tc := range []struct {
		name       string
		freeModel  bool
		usage      *dto.Usage
		wantSource string
	}{
		{name: "free_model", freeModel: true, usage: &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, wantSource: "free_model"},
		{name: "missing_usage", usage: nil, wantSource: "usage_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			userID := 8360 + len(tc.name)
			tokenID := userID + 100
			planID := userID + 200
			subID := userID + 300
			channelID := userID + 400
			seedUser(t, userID, 10_000)
			seedToken(t, tokenID, userID, "sk-billing-log-"+tc.name, 10_000)
			seedChannel(t, channelID)
			seedCodexProBillingPlan(t, planID, "codex-pro-billing-log-"+tc.name, 1_000, 100, false, false)
			seedCodexProBillingSubscription(t, subID, userID, planID, 1_000, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(time.Hour).Unix())

			ctx := newBillingTestContext(t)
			relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-billing-log-"+tc.name, "req-billing-log-"+tc.name, "subscription_only")
			relayInfo.ChannelId = channelID
			relayInfo.RelayMode = relayconstant.RelayModeResponses
			relayInfo.FreeModel = tc.freeModel
			relayInfo.SetEstimatePromptTokens(6)
			preConsumed := 6
			if tc.freeModel {
				preConsumed = 0
			}
			preConsumeForBillingTest(t, ctx, relayInfo, preConsumed)

			require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, tc.usage, nil))
			model.FlushConsumeLogUpdates()

			log := getLastLog(t)
			require.NotNil(t, log)
			other, err := common.StrToMap(log.Other)
			require.NoError(t, err)
			assert.Equal(t, float64(0), other["billing_multiplier"])
			assert.Equal(t, tc.wantSource, other["billing_multiplier_source"])
			assert.Equal(t, float64(0), other["metered_tokens"])
			assert.Equal(t, float64(0), other["billable_tokens"])
			assert.Equal(t, false, other["codex_pro_served"])
		})
	}
}

func TestPostTextConsumeQuotaFreeModelCodexProServedDoesNotConsumeSubscriptionTokens(t *testing.T) {
	truncate(t)
	const userID = 8265
	const tokenID = 8266
	const planID = 8267
	const subID = 8268
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-free-model", 10_000)
	seedCodexProBillingPlan(t, planID, "codex-pro-free-model", 1_000, 100, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 1_000, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(time.Hour).Unix())

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-free-model", "req-codex-pro-free-model", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.FreeModel = true

	relayInfo.TieredBillingSnapshot = makeSnapshot(`tier("default", p * 100 + c * 100)`, 1, 6, 0)

	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 0)
	setBoolFieldForTest(t, relayInfo, "CodexProServed", true)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))

	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
	assert.Equal(t, 0, relayInfo.FinalPreConsumedQuota)
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(0), relayInfo.SubscriptionBillableTokens)
	assert.Equal(t, int64(0), relayInfo.SubscriptionPostDelta)
}

func TestPostTextConsumeQuotaFreeModelTokenLimitDoesNotSettleMissingPreconsume(t *testing.T) {
	truncate(t)
	const userID = 8281
	const tokenID = 8282
	const planID = 8283
	const subID = 8284
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-free-token-limit", 10_000)
	seedCodexProBillingPlan(t, planID, "codex-pro-free-token-limit", 1_000, 100, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 1_000, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(time.Hour).Unix())
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(100),
		"token_used":          int64(0),
	}).Error)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-free-token-limit", "req-free-token-limit", "subscription_only")
	relayInfo.ChannelId = 8285
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.FreeModel = true
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 0)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))
	setBoolFieldForTest(t, relayInfo, "CodexProServed", true)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))

	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(0), getTokenUsed(t, tokenID))
	assert.Equal(t, int64(0), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(0), relayInfo.SubscriptionBillableTokens)
}

func TestPostTextConsumeQuotaDoesNotChargeUsageModeWhenUsageObjectHasZeroTokens(t *testing.T) {
	truncate(t)
	const userID = 8286
	const tokenID = 8287
	const planID = 8288
	const subID = 8289
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-zero-total", 10_000)
	seedDistributorPlan(t, planID, "plan-zero-total", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-zero-total", "req-zero-total", "subscription_only")
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 0}, nil))

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(0), relayInfo.SubscriptionBillableTokens)
	assert.Equal(t, int64(0), relayInfo.RawMeteredTokens)
	assert.True(t, relayInfo.HasTrustedUsage)
	log := getLastLog(t)
	require.NotNil(t, log)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Nil(t, other["usage_unavailable"])
	assert.Equal(t, true, other["has_trusted_usage"])
}

func TestSettleBillingWithInputCodexProServedDoesNotDoubleWalletOnlyPreferenceOrFreeRequests(t *testing.T) {
	truncate(t)
	const userID = 8261
	const tokenID = 8262
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-codex-pro-wallet-only", 10_000)
	const planID = 8263
	const subID = 8264
	seedDistributorPlan(t, planID, "plan-codex-pro-wallet-only", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)

	walletOnly := newBillingTestRelayInfo(userID, tokenID, "sk-codex-pro-wallet-only", "req-codex-pro-wallet-only", "wallet_only")
	preConsumeForBillingTest(t, ctx, walletOnly, 6)
	setBoolFieldForTest(t, walletOnly, "CodexProServed", true)
	require.NoError(t, SettleBillingWithInput(ctx, walletOnly, BillingSettleInput{WalletQuota: 8, SubscriptionTokens: 8}))
	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID), "wallet-only preference is compatibility metadata; Codex Pro served marker must not add an extra subscription debit")
	assert.Equal(t, 10_000, getUserQuota(t, userID), "subscription settlement must not touch wallet quota")
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))

	freeRequest := &relaycommon.RelayInfo{}
	setBoolFieldForTest(t, freeRequest, "CodexProServed", true)
	require.NoError(t, SettleBillingWithInput(ctx, freeRequest, BillingSettleInput{WalletQuota: 0, SubscriptionTokens: 8}))
	assert.Empty(t, freeRequest.SubscriptionPostDelta)
}

func TestSettleBillingWithInputFreeModelIgnoresNonZeroSubscriptionTokens(t *testing.T) {
	truncate(t)
	const userID = 8271
	const tokenID = 8272
	const planID = 8273
	const subID = 8274
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-free-direct-settle", 10_000)
	seedCodexProBillingPlan(t, planID, "codex-pro-free-direct-settle", 1_000, 100, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 1_000, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", time.Now().Add(time.Hour).Unix())

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-free-direct-settle", "req-free-direct-settle", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	relayInfo.FreeModel = true
	preConsumeForBillingTest(t, ctx, relayInfo, 0)
	setBoolFieldForTest(t, relayInfo, "CodexProServed", true)
	relayInfo.SubscriptionId = subID
	relayInfo.SubscriptionDistributorTokenBilling = true
	relayInfo.Billing = &BillingSession{
		relayInfo: relayInfo,
		funding: &SubscriptionFunding{
			requestId:               relayInfo.RequestId,
			userId:                  relayInfo.UserId,
			subscriptionId:          subID,
			DistributorTokenBilling: true,
			preConsumed:             0,
			distributorAmount:       0,
			amount:                  0,
		},
	}
	assert.Equal(t, int64(0), codexProAdjustedSubscriptionTokens(relayInfo, 8, 0))

	err := SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{SubscriptionTokens: 8, ApiKeyTokens: 0, ResponseStarted: true})

	require.NoError(t, err)
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(0), relayInfo.SubscriptionBillableTokens)
	assert.Equal(t, int64(0), relayInfo.SubscriptionPostDelta)
}

func seedCodexProBillingPlan(t *testing.T, id int, code string, tokenLimit int64, price float64, isTrial bool, inviteTrial bool) {
	t.Helper()
	ensureSubscriptionBillingTables(t)
	plan := &model.SubscriptionPlan{Id: id, Title: code, Enabled: true, TotalAmount: 1, PriceAmount: price, MonthlyTokenLimit: tokenLimit, ConcurrencyLimit: 1, IsTrial: isTrial, InviteTrial: inviteTrial, BusinessCode: &code}
	require.NoError(t, model.DB.Create(plan).Error)
	model.InvalidateSubscriptionPlanCache(id)
}

func seedCodexProBillingSubscription(t *testing.T, id int, userId int, planId int, tokenLimit int64, tokenUsed int64, grantReason string, source string, status string, endTime int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: id, UserId: userId, PlanId: planId, AmountTotal: 1, TokenLimit: tokenLimit, TokenUsed: tokenUsed, Status: status, GrantReason: grantReason, Source: source, StartTime: time.Now().Add(-time.Minute).Unix(), EndTime: endTime}).Error)
}

func requireCodexProRelayInfoEligibility(t *testing.T, relayInfo *relaycommon.RelayInfo, wantEligible bool) {
	t.Helper()
	v := reflect.ValueOf(relayInfo).Elem()
	eligibleField := v.FieldByName("CodexProEligible")
	require.True(t, eligibleField.IsValid(), "RelayInfo must expose CodexProEligible")
	assert.Equal(t, wantEligible, eligibleField.Bool())
	if wantEligible {
		reasonField := v.FieldByName("CodexProUnavailableReason")
		require.True(t, reasonField.IsValid(), "RelayInfo must expose CodexProUnavailableReason")
		assert.Empty(t, reasonField.String())
	}
}

func setBoolFieldForTest(t *testing.T, target any, fieldName string, value bool) {
	t.Helper()
	field := reflect.ValueOf(target).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "%T must expose %s", target, fieldName)
	require.Truef(t, field.CanSet(), "%T.%s must be settable", target, fieldName)
	field.SetBool(value)
}

func TestCreditBalanceTaskBillingUsesTokenUnitsAndRefundsReserve(t *testing.T) {
	truncate(t)
	ensureSubscriptionBillingTables(t)
	const userID = 8291
	const tokenID = 8292
	const planID = 8293
	const subID = 8294
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-credit-task", 10_000)
	code := "credit-task"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:               planID,
		Title:            "Credit task",
		EntitlementType:  model.SubscriptionEntitlementCreditBalance,
		Enabled:          true,
		ModelLimits:      "video-model",
		ConcurrencyLimit: 2,
		BusinessCode:     &code,
	}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id:              subID,
		UserId:          userID,
		PlanId:          planID,
		EntitlementType: model.SubscriptionEntitlementCreditBalance,
		TokenLimit:      1_000,
		TokenUsed:       0,
		AmountUsed:      7,
		Status:          "active",
		EndTime:         0,
		GrantReason:     model.SubscriptionGrantOrder,
	}).Error)
	seedReadyCreditValuationForServiceTest(t, userID, subID, 1_000)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-task", "req-credit-task", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatTask
	relayInfo.RelayMode = relayconstant.RelayModeVideoSubmit
	relayInfo.OriginModelName = "video-model"
	relayInfo.SetEstimatePromptTokens(6)

	require.Nil(t, PreConsumeBilling(ctx, 100, relayInfo))
	require.Equal(t, subID, relayInfo.SubscriptionId)
	require.Equal(t, 100, relayInfo.FinalPreConsumedQuota)
	require.Equal(t, int64(100), getSubscriptionTokenUsed(t, subID))
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", subID).First(&sub).Error)
	require.Equal(t, int64(7), sub.AmountUsed)

	session, ok := relayInfo.Billing.(*BillingSession)
	require.True(t, ok)
	require.NoError(t, session.Reserve(150))
	require.Equal(t, int64(150), getSubscriptionTokenUsed(t, subID))
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", subID).First(&sub).Error)
	require.Equal(t, int64(7), sub.AmountUsed)

	RefundBillingAfterTokenLimitReject(relayInfo.Billing)
	require.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", subID).First(&sub).Error)
	require.Equal(t, int64(7), sub.AmountUsed)
}

func TestCreditBillingSessionRefundUsesStableRequestTarget(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 82911, 82912, 82913, 82914, 82915
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-request-target", 1_000, 0)
	require.NoError(t, model.DB.Migrator().DropTable(&model.CreditValuationState{}, &model.CreditValuationMigration{}))
	require.NoError(t, model.DB.AutoMigrate(&model.CreditValuationState{}, &model.CreditValuationMigration{}))
	t.Cleanup(func() {
		_ = model.DB.Migrator().DropTable(&model.CreditValuationState{}, &model.CreditValuationMigration{})
	})
	require.NoError(t, model.DB.Create(&model.CreditValuationMigration{Version: 1, Status: model.CreditValuationMigrationReady, ValuationCurrency: "CNY"}).Error)
	require.NoError(t, model.DB.Create(&model.CreditValuationState{
		UserSubscriptionId: subID,
		UserId:             userID,
		Currency:           "CNY",
		AvailableCredit:    1_000,
		ExactCostMicros:    40_000_000,
		RuleVersion:        model.CreditValuationRuleVersion,
		StateVersion:       1,
	}).Error)

	const requestID = "req-credit-target-refund"
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-request-target", requestID, "subscription_only")
	freezeCreditBillingForServiceTest(t, ctx, relayInfo, channelID, creditbilling.ModeUsageTokens, 0, false)
	relayInfo.SetEstimatePromptTokens(100)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)
	session, ok := relayInfo.Billing.(*BillingSession)
	require.True(t, ok)

	loadRecord := func() model.SubscriptionPreConsumeRecord {
		var record model.SubscriptionPreConsumeRecord
		require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&record).Error)
		return record
	}
	require.Equal(t, int64(100), loadRecord().AppliedCredit)
	require.NoError(t, session.Reserve(150))
	require.Equal(t, int64(150), loadRecord().AppliedCredit)
	require.NoError(t, session.SettleSubscriptionIncrement(25))
	require.Equal(t, int64(175), loadRecord().AppliedCredit)

	session.refundSync()
	refunded := loadRecord()
	require.Zero(t, refunded.AppliedCredit)
	require.Zero(t, refunded.DeductedAvailableCredit)
	require.Zero(t, refunded.DeductedExactCostMicros)
	require.Equal(t, "refunded", refunded.Status)
	require.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	var state model.CreditValuationState
	require.NoError(t, model.DB.Where("user_subscription_id = ?", subID).First(&state).Error)
	require.Equal(t, int64(1_000), state.AvailableCredit)
	require.Equal(t, int64(40_000_000), state.ExactCostMicros)
	var requestCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionPreConsumeRecord{}).Where("request_id = ?", requestID).Count(&requestCount).Error)
	require.Equal(t, int64(1), requestCount)
}

func TestCreditTaskInitialSettlementPersistsNonFinalRequestIdentity(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 82921, 82922, 82923, 82924, 82925
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-task-lifecycle", 1_000, 0)
	require.NoError(t, model.DB.AutoMigrate(&model.CreditValuationState{}, &model.CreditValuationMigration{}))
	require.NoError(t, model.DB.Create(&model.CreditValuationMigration{Version: 1, Status: model.CreditValuationMigrationReady, ValuationCurrency: "CNY"}).Error)
	require.NoError(t, model.DB.Create(&model.CreditValuationState{
		UserSubscriptionId: subID,
		UserId:             userID,
		Currency:           "CNY",
		AvailableCredit:    1_000,
		ExactCostMicros:    40_000_000,
		RuleVersion:        model.CreditValuationRuleVersion,
		StateVersion:       1,
	}).Error)

	const requestID = "req-credit-task-real-lifecycle"
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-task-lifecycle", requestID, "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatTask
	relayInfo.RelayMode = relayconstant.RelayModeVideoSubmit
	relayInfo.OriginModelName = "video-model"
	relayInfo.SetEstimatePromptTokens(100)
	preConsumeForBillingTest(t, ctx, relayInfo, 100)

	session, ok := relayInfo.Billing.(*BillingSession)
	require.True(t, ok)
	require.NoError(t, session.Reserve(150))
	require.NoError(t, SettleBilling(ctx, relayInfo, 150))
	var active model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&active).Error)
	require.Equal(t, int64(150), active.AppliedCredit)
	require.Equal(t, "consumed", active.Status)
	require.Zero(t, active.FinalizedAt)

	task := model.InitTask("video", relayInfo)
	task.TaskID = "task-credit-real-lifecycle"
	task.Quota = 150
	task.PrivateData.BillingSource = relayInfo.BillingSource
	task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
	require.NoError(t, task.Insert())
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	require.Equal(t, requestID, reloaded.PrivateData.SubscriptionRequestId)

	RecalculateTaskQuota(context.Background(), &reloaded, 175, "task poll final")
	var settled model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&settled).Error)
	require.Equal(t, int64(175), settled.AppliedCredit)
	require.Equal(t, "settled", settled.Status)
	require.Positive(t, settled.FinalizedAt)
	require.Equal(t, int64(175), getSubscriptionTokenUsed(t, subID))

	var replayedTask model.Task
	require.NoError(t, model.DB.First(&replayedTask, task.ID).Error)
	RecalculateTaskQuota(context.Background(), &replayedTask, 175, "task poll final replay")
	var replayed model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&replayed).Error)
	require.Equal(t, settled.SettlementVersion, replayed.SettlementVersion)
	require.Equal(t, settled.FinalizedAt, replayed.FinalizedAt)
}

func TestCreditTaskFailureRefundReusesInitialBillingRequestIdentity(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 82931, 82932, 82933, 82934, 82935
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-task-refund", 1_000, 0)
	require.NoError(t, model.DB.AutoMigrate(&model.CreditValuationState{}, &model.CreditValuationMigration{}))
	require.NoError(t, model.DB.Create(&model.CreditValuationMigration{Version: 1, Status: model.CreditValuationMigrationReady, ValuationCurrency: "CNY"}).Error)
	require.NoError(t, model.DB.Create(&model.CreditValuationState{
		UserSubscriptionId: subID,
		UserId:             userID,
		Currency:           "CNY",
		AvailableCredit:    1_000,
		ExactCostMicros:    40_000_000,
		RuleVersion:        model.CreditValuationRuleVersion,
		StateVersion:       1,
	}).Error)

	const requestID = "req-credit-task-real-refund"
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-task-refund", requestID, "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatTask
	relayInfo.RelayMode = relayconstant.RelayModeVideoSubmit
	relayInfo.OriginModelName = "video-model"
	relayInfo.SetEstimatePromptTokens(100)
	preConsumeForBillingTest(t, ctx, relayInfo, 100)
	require.NoError(t, SettleBilling(ctx, relayInfo, 100))
	var active model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&active).Error)
	require.Equal(t, "consumed", active.Status)
	require.Zero(t, active.FinalizedAt)

	task := model.InitTask("video", relayInfo)
	task.TaskID = "task-credit-real-refund"
	task.Quota = 100
	task.PrivateData.BillingSource = relayInfo.BillingSource
	task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
	require.NoError(t, task.Insert())
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	RefundTaskQuota(context.Background(), &reloaded, "task failed")

	var refunded model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&refunded).Error)
	require.Equal(t, "refunded", refunded.Status)
	require.Zero(t, refunded.AppliedCredit)
	require.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))

	var replayedTask model.Task
	require.NoError(t, model.DB.First(&replayedTask, task.ID).Error)
	RefundTaskQuota(context.Background(), &replayedTask, "task failed replay")
	var replayed model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&replayed).Error)
	require.Equal(t, refunded.SettlementVersion, replayed.SettlementVersion)
	require.Equal(t, refunded.FinalizedAt, replayed.FinalizedAt)
}

func TestSettleBillingKeepsTimedDistributorTaskTokenInputUnchanged(t *testing.T) {
	ctx := newBillingTestContext(t)
	relayInfo := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatTask}
	session := &BillingSession{funding: &SubscriptionFunding{
		DistributorTokenBilling: true,
		EntitlementType:         model.SubscriptionEntitlementTimed,
	}}
	relayInfo.Billing = session

	require.NoError(t, SettleBilling(ctx, relayInfo, 175))
	require.True(t, session.settled)
	require.Zero(t, session.preConsumedSubscription)
	require.Zero(t, session.funding.(*SubscriptionFunding).targetAppliedCredit)
}

func TestTaskBillingMapsMissingSubscriptionWithStructuredError(t *testing.T) {
	truncate(t)
	const userID = 8295
	const tokenID = 8296
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-task-missing-subscription", 10_000)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-task-missing-subscription", "req-task-missing-subscription", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatTask
	relayInfo.RelayMode = relayconstant.RelayModeVideoSubmit

	session, apiErr := NewBillingSession(ctx, relayInfo, 100)

	require.Nil(t, session)
	require.NotNil(t, apiErr)
	require.True(t, errors.Is(apiErr, model.ErrNoActiveSubscription))
	require.Equal(t, types.ErrorCodeSubscriptionRequired, apiErr.GetErrorCode())
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.Contains(t, apiErr.Error(), "非文本异步任务不支持分销订阅扣费")
}
