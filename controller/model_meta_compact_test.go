package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeModelMetaEndpoints(t *testing.T, endpoints string) []constant.EndpointType {
	t.Helper()
	var decoded []constant.EndpointType
	require.NoError(t, common.Unmarshal([]byte(endpoints), &decoded))
	return decoded
}

func TestModelMetaExactCodexDefaultIncludesCompactEndpoint(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 2301, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "codex"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2301, Enabled: true}).Error)
	m := &model.Model{ModelName: "gpt-5.5", NameRule: model.NameRuleExact, Status: 1}

	enrichModels([]*model.Model{m})

	endpoints := decodeModelMetaEndpoints(t, m.Endpoints)
	assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
}

func TestModelMetaCustomEndpointsOverrideCompactEndpoint(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 2311, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "codex"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2311, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.Model{ModelName: "gpt-5.5", NameRule: model.NameRuleExact, Status: 1, Endpoints: endpointStringsForControllerTest(t, map[string]any{
		string(constant.EndpointTypeOpenAIResponse): "/v1/responses",
	})}).Error)
	model.InvalidatePricingCache()
	model.GetPricing()
	m := &model.Model{ModelName: "gpt-5.5", NameRule: model.NameRuleExact, Status: 1}

	enrichModels([]*model.Model{m})

	endpoints := decodeModelMetaEndpoints(t, m.Endpoints)
	assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponse)
	assert.NotContains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
}

func TestModelMetaRuleModelUsesEffectiveEndpointSemantics(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 2321, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "codex"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2321, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.Model{ModelName: "gpt-5", NameRule: model.NameRulePrefix, Status: 1, Endpoints: endpointStringsForControllerTest(t, map[string]any{
		string(constant.EndpointTypeOpenAIResponse): "/v1/responses",
	})}).Error)
	model.InvalidatePricingCache()
	model.GetPricing()
	m := &model.Model{ModelName: "gpt-5", NameRule: model.NameRulePrefix, Status: 1}

	enrichModels([]*model.Model{m})

	endpoints := decodeModelMetaEndpoints(t, m.Endpoints)
	assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponse)
	assert.NotContains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
}

func TestGetModelMetaUsesCommonJSONForEndpoints(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 2331, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "codex"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2331, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.Model{Id: 2332, ModelName: "gpt-5.5", NameRule: model.NameRuleExact, Status: 1}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/model/2332", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "2332"}}

	GetModelMeta(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, data["endpoints"])
}
