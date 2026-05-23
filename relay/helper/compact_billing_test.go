package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func snapshotCompactEndpointTestGlobals(t *testing.T) func() {
	t.Helper()

	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldModelPrice := ratio_setting.ModelPrice2JSONString()
	oldCompletionRatio := ratio_setting.CompletionRatio2JSONString()
	oldCacheRatio := ratio_setting.CacheRatio2JSONString()
	oldCreateCacheRatio := ratio_setting.CreateCacheRatio2JSONString()
	oldImageRatio := ratio_setting.ImageRatio2JSONString()
	oldAudioRatio := ratio_setting.AudioRatio2JSONString()
	oldAudioCompletionRatio := ratio_setting.AudioCompletionRatio2JSONString()
	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		savedConfig[key] = value
		return nil
	}))

	return func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrice))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletionRatio))
		require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(oldCacheRatio))
		require.NoError(t, ratio_setting.UpdateCreateCacheRatioByJSONString(oldCreateCacheRatio))
		require.NoError(t, ratio_setting.UpdateImageRatioByJSONString(oldImageRatio))
		require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(oldAudioRatio))
		require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(oldAudioCompletionRatio))
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		model.InvalidatePricingCache()
	}
}

func TestModelPriceHelperResponsesCompactUsesBillingModelName(t *testing.T) {
	restore := snapshotCompactEndpointTestGlobals(t)
	t.Cleanup(restore)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.5":1,"gpt-5.5-openai-compact":9}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		RelayMode:        constant.RelayModeResponsesCompact,
		OriginModelName:  "gpt-5.5",
		BillingModelName: "gpt-5.5-openai-compact",
		UserGroup:        "default",
		UsingGroup:       "default",
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})

	require.NoError(t, err)
	require.Equal(t, 9.0, priceData.ModelRatio)
}

func TestModelPriceHelperResponsesCompactTieredSnapshotUsesBillingModelName(t *testing.T) {
	restore := snapshotCompactEndpointTestGlobals(t)
	t.Cleanup(restore)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"gpt-5.5-openai-compact":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"gpt-5.5-openai-compact":"tier(\"compact\", p * 9)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		RelayMode:        constant.RelayModeResponsesCompact,
		OriginModelName:  "gpt-5.5",
		BillingModelName: "gpt-5.5-openai-compact",
		UserGroup:        "default",
		UsingGroup:       "default",
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{},
			Body:    []byte(`{}`),
		},
	}

	_, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})

	require.NoError(t, err)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, "gpt-5.5-openai-compact", info.TieredBillingSnapshot.ModelName)
}

func TestResolveBillingModelFromMappingForResponsesCompact(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeResponsesCompact, OriginModelName: "gpt-5.5"}

	require.NoError(t, SetCompactBillingModelFromMapping(info, `{"gpt-5.5":"upstream-gpt"}`))

	require.Equal(t, "gpt-5.5", info.OriginModelName)
	require.Equal(t, "upstream-gpt-openai-compact", info.BillingModelName)
}
