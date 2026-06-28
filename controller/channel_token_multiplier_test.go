package controller

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAddChannelTokenBillingMultiplierDefaultsAndValidation(t *testing.T) {
	db := setupChannelTokenMultiplierControllerTestDB(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/channel/", addChannelTokenMultiplierBody("default-multiplier", nil), 1)
	AddChannel(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	assertChannelTokenMultiplier(t, db, "default-multiplier", 1)

	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "zero", value: 0},
		{name: "null", value: nil},
		{name: "string", value: "2"},
		{name: "boolean", value: true},
		{name: "object", value: map[string]any{"value": 2}},
		{name: "array", value: []any{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := addChannelTokenMultiplierBody("invalid-"+tc.name, tc.value)
			body["channel"].(map[string]any)["token_billing_multiplier"] = tc.value
			ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/channel/", body, 1)
			AddChannel(ctx)
			response := decodeAPIResponse(t, recorder)
			require.False(t, response.Success)

			var count int64
			require.NoError(t, db.Model(&model.Channel{}).Where("name = ?", "invalid-"+tc.name).Count(&count).Error)
			require.Equal(t, int64(0), count)
		})
	}

	ctx, recorder = newAuthenticatedContext(t, http.MethodPost, "/api/channel/", addChannelTokenMultiplierBody("explicit-multiplier", 2), 1)
	AddChannel(ctx)
	response = decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	assertChannelTokenMultiplier(t, db, "explicit-multiplier", 2)
}

func TestAddChannelTokenBillingMultiplierBatchDefaults(t *testing.T) {
	db := setupChannelTokenMultiplierControllerTestDB(t)
	body := addChannelTokenMultiplierBody("batch-default", nil)
	body["mode"] = "batch"
	body["channel"].(map[string]any)["key"] = "sk-batch-a\nsk-batch-b"

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/channel/", body, 1)
	AddChannel(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var channels []model.Channel
	require.NoError(t, db.Where("name = ?", "batch-default").Order("id asc").Find(&channels).Error)
	require.Len(t, channels, 2)
	for _, channel := range channels {
		require.InDelta(t, 1, channel.TokenBillingMultiplier, 1e-9)
	}
}

func TestUpdateChannelTokenBillingMultiplierPresenceAndValidation(t *testing.T) {
	db := setupChannelTokenMultiplierControllerTestDB(t)
	seedChannelTokenMultiplier(t, db, &model.Channel{Id: 7101, Type: constant.ChannelTypeOpenAI, Key: "sk-update", Status: common.ChannelStatusEnabled, Name: "update-multiplier", Models: "gpt-test", TokenBillingMultiplier: 2})

	updateBody := updateChannelTokenMultiplierBody(7101, "update-rename", nil)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/channel/7101", updateBody, 1)
	UpdateChannel(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	assertChannelTokenMultiplier(t, db, "update-rename", 2)

	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "null", value: nil},
		{name: "zero", value: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := updateChannelTokenMultiplierBody(7101, "invalid-update-"+tc.name, tc.value)
			body["token_billing_multiplier"] = tc.value
			ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/channel/7101", body, 1)
			UpdateChannel(ctx)
			response := decodeAPIResponse(t, recorder)
			require.False(t, response.Success)
			assertChannelTokenMultiplier(t, db, "update-rename", 2)
		})
	}

	body := updateChannelTokenMultiplierBody(7101, "updated-multiplier", 1.5)
	body["token_billing_multiplier"] = 1.5
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/channel/7101", body, 1)
	UpdateChannel(ctx)
	response = decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	assertChannelTokenMultiplier(t, db, "updated-multiplier", 1.5)
}

func TestChannelTokenBillingMultiplierCopyInheritsValue(t *testing.T) {
	db := setupChannelTokenMultiplierControllerTestDB(t)
	seedChannelTokenMultiplier(t, db, &model.Channel{Id: 7201, Type: constant.ChannelTypeOpenAI, Key: "sk-copy", Status: common.ChannelStatusEnabled, Name: "copy-source", Models: "gpt-test", TokenBillingMultiplier: 2.5})

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/channel/copy/7201?suffix=-copy", nil, 1)
	ctx.Params = append(ctx.Params, ginParam("id", "7201"))
	CopyChannel(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	assertChannelTokenMultiplier(t, db, "copy-source-copy", 2.5)
}

func addChannelTokenMultiplierBody(name string, multiplier any) map[string]any {
	channel := map[string]any{
		"type":   constant.ChannelTypeOpenAI,
		"key":    "sk-" + strings.ReplaceAll(name, "_", "-"),
		"status": common.ChannelStatusEnabled,
		"name":   name,
		"models": "gpt-test",
	}
	if multiplier != nil {
		channel["token_billing_multiplier"] = multiplier
	}
	return map[string]any{
		"mode":    "single",
		"channel": channel,
	}
}

func updateChannelTokenMultiplierBody(id int, name string, multiplier any) map[string]any {
	body := map[string]any{
		"id":     id,
		"type":   constant.ChannelTypeOpenAI,
		"key":    "sk-update",
		"status": common.ChannelStatusEnabled,
		"name":   name,
		"models": "gpt-test",
	}
	if multiplier != nil {
		body["token_billing_multiplier"] = multiplier
	}
	return body
}

func assertChannelTokenMultiplier(t *testing.T, db *gorm.DB, name string, want float64) {
	t.Helper()
	var channel model.Channel
	require.NoError(t, db.First(&channel, "name = ?", name).Error)
	require.InDelta(t, want, channel.TokenBillingMultiplier, 1e-9)
}

func seedChannelTokenMultiplier(t *testing.T, db *gorm.DB, channel *model.Channel) {
	t.Helper()
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
}

func setupChannelTokenMultiplierControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	originalLOGDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelGroup{}, &model.ChannelGroupChannel{}, &model.TokenGroupBinding{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		model.LOG_DB = originalLOGDB
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RedisEnabled = originalRedisEnabled
	})

	return db
}

func ginParam(key string, value string) gin.Param {
	return gin.Param{Key: key, Value: value}
}
