package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestGroupOptionsAreAcceptedAsNoopCompatibilityWrites(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)

	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalTopupRatio := common.TopupGroupRatio2JSONString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originalDefaultUseAutoGroup := setting.DefaultUseAutoGroup
	originalUserUsableGroups := setting.UserUsableGroups2JSONString()
	originalModelRateLimitGroup := setting.ModelRequestRateLimitGroup2JSONString()
	originalSpecialUsable := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString()

	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups))
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(originalModelRateLimitGroup))
		require.NoError(t, ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.UnmarshalJSON([]byte(originalSpecialUsable)))
		common.OptionMap = originalOptionMap
		setting.DefaultUseAutoGroup = originalDefaultUseAutoGroup
	})

	cases := []struct {
		key   string
		value string
	}{
		{"GroupRatio", `{"vip":9}`},
		{"GroupGroupRatio", `{"vip":{"svip":9}}`},
		{"AutoGroups", `["vip"]`},
		{"DefaultUseAutoGroup", `true`},
		{"UserUsableGroups", `{"vip":"VIP"}`},
		{"TopupGroupRatio", `{"vip":9}`},
		{"ModelRequestRateLimitGroup", `{"vip":[9,9]}`},
		{"group_ratio_setting.group_special_usable_group", `{"vip":{"special":"Special"}}`},
	}

	for _, tc := range cases {
		require.NoError(t, model.UpdateOption(tc.key, tc.value), tc.key)
		assertGroupRuntimeOptionsUnchanged(t, originalGroupRatio, originalAutoGroups, originalTopupRatio, originalGroupGroupRatio, originalDefaultUseAutoGroup, originalUserUsableGroups, originalModelRateLimitGroup, originalSpecialUsable)

		ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/option/", map[string]any{"key": tc.key, "value": tc.value}, 1)
		UpdateOption(ctx)
		require.Equal(t, http.StatusOK, recorder.Code, tc.key)
		require.Contains(t, recorder.Body.String(), `"success":true`, tc.key)
		assertGroupRuntimeOptionsUnchanged(t, originalGroupRatio, originalAutoGroups, originalTopupRatio, originalGroupGroupRatio, originalDefaultUseAutoGroup, originalUserUsableGroups, originalModelRateLimitGroup, originalSpecialUsable)
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/option/", nil, 1)
	GetOptions(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	for _, tc := range cases {
		require.NotContains(t, recorder.Body.String(), tc.key)
	}
	require.NotContains(t, recorder.Body.String(), "group_ratio_setting.group_special_usable_group")
}

func assertGroupRuntimeOptionsUnchanged(t *testing.T, originalGroupRatio, originalAutoGroups, originalTopupRatio, originalGroupGroupRatio string, originalDefaultUseAutoGroup bool, originalUserUsableGroups, originalModelRateLimitGroup, originalSpecialUsable string) {
	t.Helper()
	require.JSONEq(t, originalGroupRatio, ratio_setting.GroupRatio2JSONString())
	require.JSONEq(t, originalAutoGroups, setting.AutoGroups2JsonString())
	require.JSONEq(t, originalTopupRatio, common.TopupGroupRatio2JSONString())
	require.JSONEq(t, originalGroupGroupRatio, ratio_setting.GroupGroupRatio2JSONString())
	require.Equal(t, originalDefaultUseAutoGroup, setting.DefaultUseAutoGroup)
	require.JSONEq(t, originalUserUsableGroups, setting.UserUsableGroups2JSONString())
	require.JSONEq(t, originalModelRateLimitGroup, setting.ModelRequestRateLimitGroup2JSONString())
	require.JSONEq(t, originalSpecialUsable, ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString())
}
