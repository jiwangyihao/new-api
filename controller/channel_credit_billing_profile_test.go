package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/creditbilling"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAddChannelCreditBillingProfileDefaults(t *testing.T) {
	db := setupChannelTokenMultiplierControllerTestDB(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/channel/", addChannelTokenMultiplierBody("default-credit-profile", nil), 1)
	AddChannel(ctx)
	response := decodeAPIResponse(t, recorder)

	require.True(t, response.Success, response.Message)
	profile := assertChannelCreditBillingProfile(t, db, "default-credit-profile")
	require.Equal(t, creditbilling.ModeUsageTokens, profile.CreditBillingMode)
	require.Equal(t, int64(0), profile.FixedRequestCredits)
	require.False(t, profile.DynamicBillingMultiplierEnabled)
}

func TestAddChannelFixedRequestRequiresPositiveCredits(t *testing.T) {
	db := setupChannelTokenMultiplierControllerTestDB(t)
	body := addChannelTokenMultiplierBody("fixed-without-credits", nil)
	channel := body["channel"].(map[string]any)
	channel["credit_billing_mode"] = creditbilling.ModeFixedRequest

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/channel/", body, 1)
	AddChannel(ctx)
	response := decodeAPIResponse(t, recorder)

	require.False(t, response.Success)
	var count int64
	require.NoError(t, db.Model(&model.Channel{}).Where("name = ?", "fixed-without-credits").Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestUpdateChannelCreditBillingProfilePresenceAndExplicitZeroValues(t *testing.T) {
	db := setupChannelTokenMultiplierControllerTestDB(t)
	seedChannelTokenMultiplier(t, db, &model.Channel{
		Id:                              7301,
		Type:                            constant.ChannelTypeOpenAI,
		Key:                             "sk-credit-profile",
		Status:                          common.ChannelStatusEnabled,
		Name:                            "credit-profile-original",
		Models:                          "gpt-test",
		TokenBillingMultiplier:          2,
		CreditBillingMode:               creditbilling.ModeFixedRequest,
		FixedRequestCredits:             80000,
		DynamicBillingMultiplierEnabled: true,
	})

	preserveBody := updateChannelTokenMultiplierBody(7301, "credit-profile-preserve", nil)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/channel/7301", preserveBody, 1)
	UpdateChannel(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	preserved := assertChannelCreditBillingProfile(t, db, "credit-profile-preserve")
	require.Equal(t, creditbilling.ModeFixedRequest, preserved.CreditBillingMode)
	require.Equal(t, int64(80000), preserved.FixedRequestCredits)
	require.True(t, preserved.DynamicBillingMultiplierEnabled)

	usageBody := updateChannelTokenMultiplierBody(7301, "credit-profile-zeroed", nil)
	usageBody["credit_billing_mode"] = creditbilling.ModeUsageTokens
	usageBody["fixed_request_credits"] = 0
	usageBody["dynamic_billing_multiplier_enabled"] = false
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/channel/7301", usageBody, 1)
	UpdateChannel(ctx)
	response = decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	zeroed := assertChannelCreditBillingProfile(t, db, "credit-profile-zeroed")
	require.Equal(t, creditbilling.ModeUsageTokens, zeroed.CreditBillingMode)
	require.Equal(t, int64(0), zeroed.FixedRequestCredits)
	require.False(t, zeroed.DynamicBillingMultiplierEnabled)
}

func assertChannelCreditBillingProfile(t *testing.T, db *gorm.DB, name string) model.Channel {
	t.Helper()
	var channel model.Channel
	require.NoError(t, db.First(&channel, "name = ?", name).Error)
	return channel
}
