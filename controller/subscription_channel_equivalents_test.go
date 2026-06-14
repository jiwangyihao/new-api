package controller

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type subscriptionChannelEquivalentResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		Plan struct {
			Id                      int                                `json:"id"`
			MonthlyTokenLimit       int64                              `json:"monthly_token_limit"`
			ChannelTokenEquivalents []model.PlanChannelTokenEquivalent `json:"channel_token_equivalents"`
		} `json:"plan"`
	} `json:"data"`
}

func setupSubscriptionChannelEquivalentsTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}))
}

func seedSubscriptionEquivalentPlan(t *testing.T, id int, title string, tokenLimit int64) {
	t.Helper()
	plan := &model.SubscriptionPlan{
		Id:                id,
		Title:             title,
		DurationUnit:      model.SubscriptionDurationMonth,
		DurationValue:     1,
		Enabled:           true,
		PublicVisible:     true,
		MonthlyTokenLimit: tokenLimit,
		ConcurrencyLimit:  1,
		RewardEligible:    true,
	}
	require.NoError(t, model.DB.Create(plan).Error)
}

func seedSubscriptionEquivalentChannel(t *testing.T, id int, channelType int, multiplier float64, enabled bool) {
	t.Helper()
	status := common.ChannelStatusManuallyDisabled
	if enabled {
		status = common.ChannelStatusEnabled
	}
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:                     id,
		Type:                   channelType,
		Key:                    "sk-equivalent",
		Status:                 status,
		Name:                   "channel-equivalent",
		Models:                 "gpt-test",
		TokenBillingMultiplier: multiplier,
	}).Error)
}

func performSubscriptionPlansForEquivalentTest(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil)
	GetSubscriptionPlans(ctx)
	return recorder
}

func performPublicSubscriptionPlansForEquivalentTest(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/public/plans", nil)
	GetPublicSubscriptionPlans(ctx)
	return recorder
}

func decodeSubscriptionChannelEquivalentPlans(t *testing.T, recorder *httptest.ResponseRecorder) subscriptionChannelEquivalentResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload subscriptionChannelEquivalentResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	return payload
}

func requireEquivalentByType(t *testing.T, equivalents []model.PlanChannelTokenEquivalent, channelType int) model.PlanChannelTokenEquivalent {
	t.Helper()
	for _, equivalent := range equivalents {
		if equivalent.ChannelType == channelType {
			return equivalent
		}
	}
	require.FailNowf(t, "missing equivalent", "channel type %d not found in %#v", channelType, equivalents)
	return model.PlanChannelTokenEquivalent{}
}

func requireInt64PtrValue(t *testing.T, ptr *int64, want int64) {
	t.Helper()
	require.NotNil(t, ptr)
	assert.Equal(t, want, *ptr)
}

func requireFloat64PtrValue(t *testing.T, ptr *float64, want float64) {
	t.Helper()
	require.NotNil(t, ptr)
	assert.InDelta(t, want, *ptr, 1e-9)
}

func TestSubscriptionPlansChannelTokenEquivalentsSingleAndRange(t *testing.T) {
	setupSubscriptionChannelEquivalentsTestDB(t)
	seedSubscriptionEquivalentPlan(t, 99101, "Equivalent Basic", 1_000_000)
	seedSubscriptionEquivalentChannel(t, 99111, constant.ChannelTypeOpenAI, 1, true)
	seedSubscriptionEquivalentChannel(t, 99112, constant.ChannelTypeGemini, 2, true)
	seedSubscriptionEquivalentChannel(t, 99113, constant.ChannelTypeAnthropic, 0.5, true)
	seedSubscriptionEquivalentChannel(t, 99114, constant.ChannelTypeAzure, 1.5, true)
	seedSubscriptionEquivalentChannel(t, 99115, constant.ChannelTypeAzure, 2, true)
	seedSubscriptionEquivalentChannel(t, 99116, constant.ChannelTypeCohere, 3, false)

	payload := decodeSubscriptionChannelEquivalentPlans(t, performSubscriptionPlansForEquivalentTest(t))

	require.Len(t, payload.Data, 1)
	equivalents := payload.Data[0].Plan.ChannelTokenEquivalents
	require.Len(t, equivalents, 4)
	openai := requireEquivalentByType(t, equivalents, constant.ChannelTypeOpenAI)
	assert.Equal(t, model.ChannelCreditEquivalentKindUsageTokens, openai.Kind)
	assert.Equal(t, model.ChannelCreditEquivalentValueTypeSingle, openai.ValueType)
	assert.Equal(t, "OpenAI", openai.ChannelTypeName)
	assert.Equal(t, 1, openai.VariantCount)
	requireFloat64PtrValue(t, openai.Multiplier, 1)
	requireInt64PtrValue(t, openai.EquivalentTokenLimit, 1_000_000)

	gemini := requireEquivalentByType(t, equivalents, constant.ChannelTypeGemini)
	assert.Equal(t, model.ChannelCreditEquivalentKindUsageTokens, gemini.Kind)
	assert.Equal(t, model.ChannelCreditEquivalentValueTypeSingle, gemini.ValueType)
	requireFloat64PtrValue(t, gemini.Multiplier, 2)
	requireInt64PtrValue(t, gemini.EquivalentTokenLimit, 500_000)

	anthropic := requireEquivalentByType(t, equivalents, constant.ChannelTypeAnthropic)
	assert.Equal(t, model.ChannelCreditEquivalentKindUsageTokens, anthropic.Kind)
	assert.Equal(t, model.ChannelCreditEquivalentValueTypeSingle, anthropic.ValueType)
	requireFloat64PtrValue(t, anthropic.Multiplier, 0.5)
	requireInt64PtrValue(t, anthropic.EquivalentTokenLimit, 2_000_000)

	azure := requireEquivalentByType(t, equivalents, constant.ChannelTypeAzure)
	assert.Equal(t, model.ChannelCreditEquivalentKindUsageTokens, azure.Kind)
	assert.Equal(t, model.ChannelCreditEquivalentValueTypeRange, azure.ValueType)
	assert.Equal(t, 2, azure.VariantCount)
	requireFloat64PtrValue(t, azure.MinMultiplier, 1.5)
	requireFloat64PtrValue(t, azure.MaxMultiplier, 2)
	requireInt64PtrValue(t, azure.EquivalentTokenLimitMin, 500_000)
	requireInt64PtrValue(t, azure.EquivalentTokenLimitMax, 666_666)
}

func TestSubscriptionPlansChannelTokenEquivalentsEmptyAndUnlimited(t *testing.T) {
	setupSubscriptionChannelEquivalentsTestDB(t)
	seedSubscriptionEquivalentPlan(t, 99201, "No Channel Basic", 1_000_000)

	emptyRecorder := performSubscriptionPlansForEquivalentTest(t)
	emptyPayload := decodeSubscriptionChannelEquivalentPlans(t, emptyRecorder)
	require.Len(t, emptyPayload.Data, 1)
	assert.NotNil(t, emptyPayload.Data[0].Plan.ChannelTokenEquivalents)
	assert.Empty(t, emptyPayload.Data[0].Plan.ChannelTokenEquivalents)
	assert.Contains(t, emptyRecorder.Body.String(), `"channel_token_equivalents":[]`)

	seedSubscriptionEquivalentChannel(t, 99211, constant.ChannelTypeOpenAI, 2, true)
	seedSubscriptionEquivalentPlan(t, 99202, "Unlimited Trial", 0)

	payload := decodeSubscriptionChannelEquivalentPlans(t, performSubscriptionPlansForEquivalentTest(t))
	require.Len(t, payload.Data, 2)
	sort.Slice(payload.Data, func(i, j int) bool { return payload.Data[i].Plan.Id < payload.Data[j].Plan.Id })
	unlimited := requireEquivalentByType(t, payload.Data[1].Plan.ChannelTokenEquivalents, constant.ChannelTypeOpenAI)
	assert.Equal(t, "unlimited", unlimited.Kind)
	assert.Equal(t, model.ChannelCreditEquivalentValueTypeUnlimited, unlimited.ValueType)
	assert.True(t, unlimited.TokenUnlimited)
	assert.Nil(t, unlimited.Multiplier)
	assert.Nil(t, unlimited.EquivalentTokenLimit)
}

func TestSubscriptionPublicPlansChannelTokenEquivalentsMatchAdminPlans(t *testing.T) {
	setupSubscriptionChannelEquivalentsTestDB(t)
	seedSubscriptionEquivalentPlan(t, 99301, "Equivalent Public", 1_000_000)
	seedSubscriptionEquivalentChannel(t, 99311, constant.ChannelTypeOpenAI, 1, true)
	seedSubscriptionEquivalentChannel(t, 99312, constant.ChannelTypeGemini, 2, true)

	adminPayload := decodeSubscriptionChannelEquivalentPlans(t, performSubscriptionPlansForEquivalentTest(t))
	publicPayload := decodeSubscriptionChannelEquivalentPlans(t, performPublicSubscriptionPlansForEquivalentTest(t))

	require.Len(t, adminPayload.Data, 1)
	require.Len(t, publicPayload.Data, 1)
	assert.Equal(t, adminPayload.Data[0].Plan.ChannelTokenEquivalents, publicPayload.Data[0].Plan.ChannelTokenEquivalents)
}

func TestChannelTokenMultiplierEndToEndSubscriptionPlansReflectCurrentChannelMultiplier(t *testing.T) {
	setupSubscriptionChannelEquivalentsTestDB(t)
	const planID = 99501
	const channelAID = 99511
	const channelBID = 99512
	seedSubscriptionEquivalentPlan(t, planID, "Multiplier Snapshot Basic", 1_000_000)
	seedSubscriptionEquivalentChannel(t, channelAID, constant.ChannelTypeOpenAI, 1, true)
	seedSubscriptionEquivalentChannel(t, channelBID, constant.ChannelTypeGemini, 2, true)

	payload := decodeSubscriptionChannelEquivalentPlans(t, performSubscriptionPlansForEquivalentTest(t))

	require.Len(t, payload.Data, 1)
	assert.Equal(t, int64(1_000_000), payload.Data[0].Plan.MonthlyTokenLimit)
	openai := requireEquivalentByType(t, payload.Data[0].Plan.ChannelTokenEquivalents, constant.ChannelTypeOpenAI)
	assert.Equal(t, model.ChannelCreditEquivalentKindUsageTokens, openai.Kind)
	assert.Equal(t, model.ChannelCreditEquivalentValueTypeSingle, openai.ValueType)
	requireFloat64PtrValue(t, openai.Multiplier, 1)
	requireInt64PtrValue(t, openai.EquivalentTokenLimit, 1_000_000)
	gemini := requireEquivalentByType(t, payload.Data[0].Plan.ChannelTokenEquivalents, constant.ChannelTypeGemini)
	assert.Equal(t, model.ChannelCreditEquivalentKindUsageTokens, gemini.Kind)
	assert.Equal(t, model.ChannelCreditEquivalentValueTypeSingle, gemini.ValueType)
	requireFloat64PtrValue(t, gemini.Multiplier, 2)
	requireInt64PtrValue(t, gemini.EquivalentTokenLimit, 500_000)

	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelBID).Update("token_billing_multiplier", 1.5).Error)
	updatedPayload := decodeSubscriptionChannelEquivalentPlans(t, performSubscriptionPlansForEquivalentTest(t))

	require.Len(t, updatedPayload.Data, 1)
	updatedGemini := requireEquivalentByType(t, updatedPayload.Data[0].Plan.ChannelTokenEquivalents, constant.ChannelTypeGemini)
	assert.Equal(t, model.ChannelCreditEquivalentKindUsageTokens, updatedGemini.Kind)
	assert.Equal(t, model.ChannelCreditEquivalentValueTypeSingle, updatedGemini.ValueType)
	requireFloat64PtrValue(t, updatedGemini.Multiplier, 1.5)
	requireInt64PtrValue(t, updatedGemini.EquivalentTokenLimit, 666_666)
}

func TestSubscriptionSelfSummaryChannelTokenEquivalentsIncludesZeroRemaining(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	seedSubscriptionEquivalentChannel(t, 99411, constant.ChannelTypeOpenAI, 2, true)
	const userID = 99401
	const planID = 99402
	seedSubscriptionSelfSummaryUser(t, userID, "self_summary_equivalent")
	seedSubscriptionSelfSummaryPlan(t, planID, "Self Equivalent", "self-equivalent", 1_000_000, 1, false, false)
	seedSubscriptionSelfSummarySubscription(t, 99403, userID, planID, 1_000_000, 999_999, 1, "order", 3600)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	assert.Contains(t, recorder.Body.String(), `"equivalent_token_remaining":0`)
	summary := requireSubscriptionSelfSummary(t, subscriptionSelfSummaryData(t, recorder))
	creditValue, ok := summary["channel_credit_equivalents"].([]interface{})
	require.True(t, ok, "channel_credit_equivalents must be present as array")
	require.Len(t, creditValue, 1)
	creditEquivalent, ok := creditValue[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, model.ChannelCreditEquivalentKindUsageTokens, creditEquivalent["kind"])
	assert.Equal(t, model.ChannelCreditEquivalentValueTypeSingle, creditEquivalent["value_type"])
	assert.Equal(t, float64(constant.ChannelTypeOpenAI), creditEquivalent["channel_type"])
	assert.Equal(t, float64(500_000), creditEquivalent["equivalent_token_limit"])
	assert.Equal(t, float64(0), creditEquivalent["equivalent_token_remaining"])

	legacyValue, ok := summary["channel_token_equivalents"].([]interface{})
	require.True(t, ok, "channel_token_equivalents must be present as array")
	assert.Equal(t, creditValue, legacyValue)
}
