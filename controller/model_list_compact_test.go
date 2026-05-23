package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withModelListCompactPricing(t *testing.T) {
	t.Helper()
	oldRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.5":1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
		model.InvalidatePricingCache()
	})
}

func listModelEndpoints(t *testing.T, recorder *httptest.ResponseRecorder, modelID string) []constant.EndpointType {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload listModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	for _, item := range payload.Data {
		if item.Id == modelID {
			return item.SupportedEndpointTypes
		}
	}
	t.Fatalf("model %s not found in response", modelID)
	return nil
}

func TestListModelsIncludesCodexCompactEndpoint(t *testing.T) {
	withSelfUseModeDisabled(t)
	withModelListCompactPricing(t)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 2101, Username: "codex_user", Group: "default", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 2102, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "codex"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2102, Enabled: true}).Error)
	model.InvalidatePricingCache()
	model.GetPricing()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 2101)

	ListModels(ctx, constant.ChannelTypeOpenAI)

	endpoints := listModelEndpoints(t, recorder, "gpt-5.5")
	assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponse)
	assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
}

func TestListModelsTokenLimitIncludesCodexCompactEndpoint(t *testing.T) {
	withSelfUseModeDisabled(t)
	withModelListCompactPricing(t)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 2112, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "codex"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2112, Enabled: true}).Error)
	model.InvalidatePricingCache()
	model.GetPricing()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 2111)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{"gpt-5.5": true})

	ListModels(ctx, constant.ChannelTypeOpenAI)

	endpoints := listModelEndpoints(t, recorder, "gpt-5.5")
	assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
}
