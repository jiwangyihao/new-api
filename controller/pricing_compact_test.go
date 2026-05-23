package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCompactPricingTest(t *testing.T) {
	t.Helper()
	setupPricingDirectoryTestDB(t)
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	oldUsableGroups := setting.UserUsableGroups2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.5":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatio))
		model.InvalidatePricingCache()
	})
}

func seedCompactPricingUser(t *testing.T, id int, group string) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{Id: id, Username: group + "_pricing_user", Group: group, Status: common.UserStatusEnabled, AffCode: group + "_pricing_aff"}).Error)
}

func seedCompactPricingCodex(t *testing.T, group string, channelID int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.Channel{Id: channelID, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: group, Name: "codex-" + group}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: group, Model: "gpt-5.5", ChannelId: channelID, Enabled: true}).Error)
}

func supportedEndpointMap(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	supported, ok := payload["supported_endpoint"].(map[string]any)
	require.True(t, ok)
	return supported
}

func assertPricingItemHasEndpoint(t *testing.T, item map[string]any, endpoint constant.EndpointType) {
	t.Helper()
	endpoints, ok := item["supported_endpoint_types"].([]any)
	require.True(t, ok)
	for _, raw := range endpoints {
		if raw == string(endpoint) {
			return
		}
	}
	require.Failf(t, "endpoint not found", "%s not in %v", endpoint, endpoints)
}

func assertPricingItemLacksEndpoint(t *testing.T, item map[string]any, endpoint constant.EndpointType) {
	t.Helper()
	endpoints, ok := item["supported_endpoint_types"].([]any)
	require.True(t, ok)
	for _, raw := range endpoints {
		if raw == string(endpoint) {
			require.Failf(t, "endpoint found", "%s unexpectedly in %v", endpoint, endpoints)
		}
	}
}

func TestGetPricingIncludesCodexCompactSupportedEndpoint(t *testing.T) {
	setupCompactPricingTest(t)
	seedCompactPricingUser(t, 2201, "default")
	seedCompactPricingCodex(t, "default", 2202)
	model.InvalidatePricingCache()
	model.GetPricing()

	payload := performGetPricingForDirectoryTest(t, common.GetPointer(2201))
	item := pricingDirectoryItemByName(t, payload, "gpt-5.5")

	assertPricingItemHasEndpoint(t, item, constant.EndpointTypeOpenAIResponseCompact)
	supported := supportedEndpointMap(t, payload)
	compact, ok := supported[string(constant.EndpointTypeOpenAIResponseCompact)].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/v1/responses/compact", compact["path"])
	assert.Equal(t, "POST", compact["method"])
}

func TestGetPricingCustomEndpointsDoNotGateCompactExposure(t *testing.T) {
	setupCompactPricingTest(t)
	seedCompactPricingUser(t, 2211, "default")
	seedCompactPricingCodex(t, "default", 2212)
	require.NoError(t, model.DB.Create(&model.Model{ModelName: "gpt-5.5", NameRule: model.NameRuleExact, Status: 1, Endpoints: endpointStringsForControllerTest(t, map[string]any{
		string(constant.EndpointTypeOpenAIResponse): "/custom/responses",
	})}).Error)
	model.InvalidatePricingCache()
	model.GetPricing()

	payload := performGetPricingForDirectoryTest(t, common.GetPointer(2211))
	item := pricingDirectoryItemByName(t, payload, "gpt-5.5")
	assertPricingItemHasEndpoint(t, item, constant.EndpointTypeOpenAIResponseCompact)
	supported := supportedEndpointMap(t, payload)
	response, ok := supported[string(constant.EndpointTypeOpenAIResponse)].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/custom/responses", response["path"])
	compact, ok := supported[string(constant.EndpointTypeOpenAIResponseCompact)].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/v1/responses/compact", compact["path"])
}

func TestGetPricingCustomCompactEndpointInfo(t *testing.T) {
	setupCompactPricingTest(t)
	seedCompactPricingUser(t, 2221, "default")
	seedCompactPricingCodex(t, "default", 2222)
	require.NoError(t, model.DB.Create(&model.Model{ModelName: "gpt-5.5", NameRule: model.NameRuleExact, Status: 1, Endpoints: endpointStringsForControllerTest(t, map[string]any{
		string(constant.EndpointTypeOpenAIResponseCompact): map[string]any{"path": "/custom/compact", "method": "post"},
	})}).Error)
	model.InvalidatePricingCache()
	model.GetPricing()

	payload := performGetPricingForDirectoryTest(t, common.GetPointer(2221))
	compact, ok := supportedEndpointMap(t, payload)[string(constant.EndpointTypeOpenAIResponseCompact)].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/custom/compact", compact["path"])
	assert.Equal(t, "POST", compact["method"])
}

func TestGetPricingCustomCompactEndpointInfoRequiresChannelCapability(t *testing.T) {
	setupCompactPricingTest(t)
	seedCompactPricingUser(t, 2223, "default")
	require.NoError(t, model.DB.Create(&model.Channel{Id: 2224, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "openai-default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2224, Enabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.Model{ModelName: "gpt-5.5", NameRule: model.NameRuleExact, Status: 1, Endpoints: endpointStringsForControllerTest(t, map[string]any{
		string(constant.EndpointTypeOpenAIResponseCompact): map[string]any{"path": "/custom/compact", "method": "post"},
	})}).Error)
	model.InvalidatePricingCache()
	model.GetPricing()

	payload := performGetPricingForDirectoryTest(t, common.GetPointer(2223))
	item := pricingDirectoryItemByName(t, payload, "gpt-5.5")
	assertPricingItemLacksEndpoint(t, item, constant.EndpointTypeOpenAIResponseCompact)
	_, ok := supportedEndpointMap(t, payload)[string(constant.EndpointTypeOpenAIResponseCompact)]
	assert.False(t, ok)
}

func TestGetPricingSupportedEndpointMapReflectsAllEnabledRoutesAfterBusinessGroupRemoval(t *testing.T) {
	setupCompactPricingTest(t)
	seedCompactPricingUser(t, 2231, "default")
	seedCompactPricingUser(t, 2232, "vip")
	require.NoError(t, model.DB.Create(&model.Channel{Id: 2233, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Name: "openai-default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2233, Enabled: true}).Error)
	seedCompactPricingCodex(t, "vip", 2234)
	model.InvalidatePricingCache()
	model.GetPricing()

	defaultPayload := performGetPricingForDirectoryTest(t, common.GetPointer(2231))
	defaultItem := pricingDirectoryItemByName(t, defaultPayload, "gpt-5.5")
	assertPricingItemHasEndpoint(t, defaultItem, constant.EndpointTypeOpenAIResponseCompact)
	_, defaultHasCompact := supportedEndpointMap(t, defaultPayload)[string(constant.EndpointTypeOpenAIResponseCompact)]
	assert.True(t, defaultHasCompact)

	vipPayload := performGetPricingForDirectoryTest(t, common.GetPointer(2232))
	vipItem := pricingDirectoryItemByName(t, vipPayload, "gpt-5.5")
	assertPricingItemHasEndpoint(t, vipItem, constant.EndpointTypeOpenAIResponseCompact)
	_, vipHasCompact := supportedEndpointMap(t, vipPayload)[string(constant.EndpointTypeOpenAIResponseCompact)]
	assert.True(t, vipHasCompact)
}
