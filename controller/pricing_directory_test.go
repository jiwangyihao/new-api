package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPricingDirectoryTestDB(t *testing.T) {
	t.Helper()

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Model{}, &model.Vendor{}))
	require.NoError(t, db.Exec("DELETE FROM users").Error)
	require.NoError(t, db.Exec("DELETE FROM channels").Error)
	require.NoError(t, db.Exec("DELETE FROM abilities").Error)
	require.NoError(t, db.Exec("DELETE FROM models").Error)
	require.NoError(t, db.Exec("DELETE FROM vendors").Error)

	seedPricingDirectoryOptions(t)
	model.InvalidatePricingCache()
	t.Cleanup(model.InvalidatePricingCache)
}

func seedPricingDirectoryOptions(t *testing.T) {
	t.Helper()

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"pricing-visible-model":3}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"pricing-fixed-model":0.000001}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"pricing-visible-model":4}`))
	cacheRatio := `{"pricing-visible-model":0.25}`
	require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(cacheRatio))
	require.NoError(t, ratio_setting.UpdateCreateCacheRatioByJSONString(cacheRatio))
	require.NoError(t, ratio_setting.UpdateImageRatioByJSONString(cacheRatio))
	require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(cacheRatio))
	require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(cacheRatio))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": fmt.Sprintf(`{"pricing-visible-model":%q}`, billing_setting.BillingModeTieredExpr),
		"billing_setting.billing_expr": `{"pricing-visible-model":"max(p+c, 0)"}`,
	}))
}

func int64PtrForPricingDirectoryTest(value int64) *int64 {
	return &value
}

func seedPricingDirectoryData(t *testing.T) {
	t.Helper()

	require.NoError(t, model.DB.Create(&model.User{Id: 9101, Username: "pricing_common", Group: "", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "pricing_common"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9102, Username: "pricing_admin", Group: "", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AffCode: "pricing_admin"}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     9201,
		Type:   constant.ChannelTypeOpenAI,
		Name:   "pricing-openai",
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
		Models: "pricing-visible-model,pricing-fixed-model",
		Group:  "default",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "pricing-visible-model", ChannelId: 9201, Enabled: true, Priority: int64PtrForPricingDirectoryTest(1)}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "pricing-fixed-model", ChannelId: 9201, Enabled: true, Priority: int64PtrForPricingDirectoryTest(1)}).Error)
}

func performGetPricingForDirectoryTest(t *testing.T, userID *int) map[string]any {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
	if userID != nil {
		ctx.Set("id", *userID)
	}

	GetPricing(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, true, payload["success"])
	return payload
}

func firstPricingDirectoryItem(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	data, ok := payload["data"].([]any)
	require.True(t, ok, "pricing response data must be a list")
	require.NotEmpty(t, data)
	item, ok := data[0].(map[string]any)
	require.True(t, ok, "pricing item must be an object")
	return item
}

func pricingDirectoryItemByName(t *testing.T, payload map[string]any, modelName string) map[string]any {
	t.Helper()

	data, ok := payload["data"].([]any)
	require.True(t, ok, "pricing response data must be a list")
	for _, rawItem := range data {
		item, ok := rawItem.(map[string]any)
		require.True(t, ok, "pricing item must be an object")
		if item["model_name"] == modelName {
			return item
		}
	}
	require.Failf(t, "pricing item not found", "model %q not found", modelName)
	return nil
}

func assertNoPricingCostFields(t *testing.T, item map[string]any) {
	t.Helper()

	for _, key := range []string{
		"model_ratio",
		"model_price",
		"completion_ratio",
		"cache_ratio",
		"create_cache_ratio",
		"image_ratio",
		"audio_ratio",
		"audio_completion_ratio",
		"billing_mode",
		"billing_expr",
	} {
		assert.NotContains(t, item, key)
	}
}

func TestGetPricingRedactsCostFieldsForAnonymousAndUser(t *testing.T) {
	setupPricingDirectoryTestDB(t)
	seedPricingDirectoryData(t)

	anonymousPayload := performGetPricingForDirectoryTest(t, nil)
	assertNoPricingCostFields(t, firstPricingDirectoryItem(t, anonymousPayload))
	assert.NotContains(t, anonymousPayload, "group_ratio")

	commonUserID := 9101
	userPayload := performGetPricingForDirectoryTest(t, &commonUserID)
	assertNoPricingCostFields(t, firstPricingDirectoryItem(t, userPayload))
	assert.NotContains(t, userPayload, "group_ratio")
}

func TestGetPricingKeepsDirectoryFieldsForAnonymousAndUser(t *testing.T) {
	setupPricingDirectoryTestDB(t)
	seedPricingDirectoryData(t)

	payload := performGetPricingForDirectoryTest(t, nil)
	item := firstPricingDirectoryItem(t, payload)

	assert.Contains(t, item, "model_name")
	assert.Contains(t, item, "supported_endpoint_types")
	assert.Contains(t, payload, "vendors")
	assert.Contains(t, payload, "supported_endpoint")
	assert.Contains(t, payload, "pricing_version")
}

func TestGetPricingKeepsCostFieldsForAdmin(t *testing.T) {
	setupPricingDirectoryTestDB(t)
	seedPricingDirectoryData(t)

	adminID := 9102
	payload := performGetPricingForDirectoryTest(t, &adminID)
	item := pricingDirectoryItemByName(t, payload, "pricing-visible-model")

	assert.Contains(t, item, "model_ratio")
	assert.Contains(t, item, "model_price")
	assert.Contains(t, item, "billing_expr")
}

func TestRatioSyncDefaultEndpointUsesRatioConfig(t *testing.T) {
	requestBody := fmt.Sprintf(`{"upstreams":[{"name":"demo","base_url":"%s"}],"timeout":1}`, "http://127.0.0.1:1")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio-sync/fetch", strings.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	FetchUpstreamRatios(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			TestResults []dto.TestResult `json:"test_results"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.TestResults, 1)
	assert.Contains(t, payload.Data.TestResults[0].Error, "/api/ratio_config")
	assert.NotContains(t, payload.Data.TestResults[0].Error, "/api/pricing")
}

func TestRatioSyncRejectsRedactedPricingPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/pricing", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"pricing-visible-model","supported_endpoint_types":["chat"]}]}`))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio-sync/fetch", bytes.NewBufferString(fmt.Sprintf(`{"upstreams":[{"name":"redacted","base_url":%q,"endpoint":"/api/pricing"}],"timeout":2}`, server.URL)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	FetchUpstreamRatios(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Differences map[string]map[string]dto.DifferenceItem `json:"differences"`
			TestResults []dto.TestResult                         `json:"test_results"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.TestResults, 1)
	assert.Equal(t, "error", payload.Data.TestResults[0].Status)
	assert.Contains(t, payload.Data.TestResults[0].Error, "no sync fields")
	assert.Empty(t, payload.Data.Differences)
}
