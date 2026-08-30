package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func testBillingInfoContext(t *testing.T) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return ctx
}

func testRelayInfoStartTimes(info *relaycommon.RelayInfo) {
	info.StartTime = time.UnixMilli(1000)
	info.FirstResponseTime = time.UnixMilli(1250)
	info.ChannelMeta = &relaycommon.ChannelMeta{}
}

func int64FromOtherValue(t *testing.T, value interface{}) int64 {
	t.Helper()
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		t.Fatalf("unexpected numeric type %T", value)
		return 0
	}
}

func TestGenerateTextOtherInfoWritesRequestBufferTiming(t *testing.T) {
	ctx := testBillingInfoContext(t)
	common.SetContextKey(ctx, constant.ContextKeyRequestBufferTimeMs, 1250)
	relayInfo := &relaycommon.RelayInfo{}
	testRelayInfoStartTimes(relayInfo)

	other := GenerateTextOtherInfo(ctx, relayInfo, 0, 0, 0, 0, 0)

	assert.Equal(t, int64(250), int64FromOtherValue(t, other["frt"]))
	assert.Equal(t, int64(1250), int64FromOtherValue(t, other["request_buffer_time_ms"]))
}

func TestGenerateTextOtherInfoOmitsFirstResponseTimingBeforeResponse(t *testing.T) {
	ctx := testBillingInfoContext(t)
	common.SetContextKey(ctx, constant.ContextKeyRequestBufferTimeMs, 1250)
	startTime := time.UnixMilli(1000)
	relayInfo := &relaycommon.RelayInfo{
		StartTime:         startTime,
		FirstResponseTime: startTime.Add(-time.Second),
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}

	other := GenerateTextOtherInfo(ctx, relayInfo, 0, 0, 0, 0, 0)

	assert.NotContains(t, other, "frt")
	assert.Equal(t, int64(1250), int64FromOtherValue(t, other["request_buffer_time_ms"]))
}

func TestAppendBillingInfoWritesSubscriptionTokenFields(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		BillingSource:                        "subscription",
		SubscriptionId:                       10,
		SubscriptionPlanId:                   2,
		SubscriptionPlanTitle:                "Basic",
		SubscriptionPreConsumed:              3000,
		SubscriptionPostDelta:                -952,
		SubscriptionTokenLimit:               1000000000,
		SubscriptionTokenUsedAfterPreConsume: 123000,
		SubscriptionTokenUnlimited:           false,
		SubscriptionDistributorTokenBilling:  true,
	}
	testRelayInfoStartTimes(relayInfo)
	other := GenerateTextOtherInfo(testBillingInfoContext(t), relayInfo, 0, 0, 0, 0, 0)

	assert.Equal(t, int64(1000000000), int64FromOtherValue(t, other["subscription_token_limit"]))
	assert.Equal(t, int64(2048), int64FromOtherValue(t, other["subscription_tokens_consumed"]))
	assert.Equal(t, int64(122048), int64FromOtherValue(t, other["subscription_token_used"]))
	assert.Equal(t, int64(999877952), int64FromOtherValue(t, other["subscription_token_remaining"]))
	assert.Equal(t, false, other["subscription_token_unlimited"])
}

func TestAppendBillingInfoClampsNegativeSubscriptionTokenConsumption(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		BillingSource:                        "subscription",
		SubscriptionId:                       10,
		SubscriptionPreConsumed:              10,
		SubscriptionPostDelta:                -50,
		SubscriptionTokenLimit:               1000,
		SubscriptionTokenUsedAfterPreConsume: 20,
		SubscriptionDistributorTokenBilling:  true,
	}
	testRelayInfoStartTimes(relayInfo)

	other := GenerateTextOtherInfo(testBillingInfoContext(t), relayInfo, 0, 0, 0, 0, 0)

	assert.Equal(t, int64(0), int64FromOtherValue(t, other["subscription_tokens_consumed"]))
	assert.Equal(t, int64(0), int64FromOtherValue(t, other["subscription_token_used"]))
	assert.Equal(t, int64(1000), int64FromOtherValue(t, other["subscription_token_remaining"]))
}

func TestAppendBillingInfoDoesNotWriteTokenFieldsForLegacyAmountSubscription(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		BillingSource:                         "subscription",
		SubscriptionPreConsumed:               30,
		SubscriptionPostDelta:                 20,
		SubscriptionAmountTotal:               100,
		SubscriptionAmountUsedAfterPreConsume: 50,
	}
	testRelayInfoStartTimes(relayInfo)

	other := GenerateTextOtherInfo(testBillingInfoContext(t), relayInfo, 0, 0, 0, 0, 0)

	for _, key := range []string{
		"subscription_tokens_consumed",
		"subscription_token_limit",
		"subscription_token_used",
		"subscription_token_remaining",
		"subscription_token_unlimited",
	} {
		_, exists := other[key]
		assert.False(t, exists, key)
	}
}

func TestTimedSubscriptionBillingLogsUseCreditUnit(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		BillingSource:                        BillingSourceSubscription,
		SubscriptionId:                       3427,
		SubscriptionEntitlementType:          model.SubscriptionEntitlementTimed,
		SubscriptionPreConsumed:              160_000,
		SubscriptionBillableTokens:           160_000,
		SubscriptionTokenLimit:               1_000_000,
		SubscriptionTokenUsedAfterPreConsume: 360_000,
		SubscriptionDistributorTokenBilling:  true,
	}
	testRelayInfoStartTimes(relayInfo)

	other := GenerateTextOtherInfo(testBillingInfoContext(t), relayInfo, 0, 0, 0, 0, 0)

	assert.Equal(t, "credit", other["billing_unit"])
	assert.Equal(t, 2, other["billing_schema_version"])
	assert.Equal(t, int64(160_000), int64FromOtherValue(t, other["pre_consumed_credits"]))
	assert.Equal(t, int64(0), int64FromOtherValue(t, other["settlement_delta_credits"]))
	assert.Equal(t, int64(640_000), int64FromOtherValue(t, other["remaining_credits"]))
	assert.Equal(t, int64(160_000), int64FromOtherValue(t, other["final_credits"]))
}
