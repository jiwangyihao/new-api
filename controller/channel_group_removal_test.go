package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSearchChannelsIgnoresLegacyGroupQuery(t *testing.T) {
	db := setupChannelControllerGroupRemovalTestDB(t)
	seedChannelForGroupRemoval(t, db, &model.Channel{Id: 2001, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "vip-only", Models: "gpt-test", Group: "vip", Tag: common.GetPointer("tag-a")})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/channel/search?keyword=vip-only&group=missing&model=gpt-test", nil, 1)
	SearchChannels(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Contains(t, recorder.Body.String(), "vip-only")
	require.NotContains(t, recorder.Body.String(), `"group"`)
}

func TestSearchTagsIgnoresLegacyGroupQuery(t *testing.T) {
	db := setupChannelControllerGroupRemovalTestDB(t)
	seedChannelForGroupRemoval(t, db, &model.Channel{Id: 2002, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "tagged", Models: "gpt-test", Group: "vip", Tag: common.GetPointer("legacy-tag")})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/channel/search?tag_mode=true&group=missing&model=gpt-test", nil, 1)
	SearchChannels(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Contains(t, recorder.Body.String(), "legacy-tag")
	require.NotContains(t, recorder.Body.String(), `"group"`)
}

func TestEditTagChannelsIgnoresLegacyGroupsPayload(t *testing.T) {
	db := setupChannelControllerGroupRemovalTestDB(t)
	seedChannelForGroupRemoval(t, db, &model.Channel{Id: 2003, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "tag-edit", Models: "gpt-test", Group: "default", Tag: common.GetPointer("edit-tag")})
	body := map[string]any{"tag": "edit-tag", "groups": "vip", "models": "gpt-test,gpt-next"}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/channel/tag", body, 1)
	EditTagChannels(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var updated model.Channel
	require.NoError(t, db.First(&updated, 2003).Error)
	require.Equal(t, "default", updated.Group)
	require.Equal(t, "gpt-test,gpt-next", updated.Models)
}

func TestAddAndUpdateChannelIgnoreLegacyGroupPayloads(t *testing.T) {
	db := setupChannelControllerGroupRemovalTestDB(t)
	addBody := map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"type":   constant.ChannelTypeOpenAI,
			"key":    "sk-test",
			"status": common.ChannelStatusEnabled,
			"name":   "legacy-add",
			"models": "gpt-test",
			"group":  "vip",
		},
	}
	addCtx, addRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/channel/", addBody, 1)
	AddChannel(addCtx)
	addResponse := decodeAPIResponse(t, addRecorder)
	require.True(t, addResponse.Success, addResponse.Message)

	var inserted model.Channel
	require.NoError(t, db.First(&inserted, "name = ?", "legacy-add").Error)
	require.Equal(t, "", inserted.Group)

	updateBody := map[string]any{
		"id":     inserted.Id,
		"type":   inserted.Type,
		"key":    inserted.Key,
		"status": inserted.Status,
		"name":   "legacy-update",
		"models": inserted.Models,
		"group":  "svip",
	}
	updateCtx, updateRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/channel/"+strconv.Itoa(inserted.Id), updateBody, 1)
	UpdateChannel(updateCtx)
	updateResponse := decodeAPIResponse(t, updateRecorder)
	require.True(t, updateResponse.Success, updateResponse.Message)
	require.NotContains(t, updateRecorder.Body.String(), `"group"`)

	var updated model.Channel
	require.NoError(t, db.First(&updated, inserted.Id).Error)
	require.Equal(t, "", updated.Group)
	require.Equal(t, "legacy-update", updated.Name)
}

func TestChannelUpstreamModelUpdateSelectFieldsOmitLegacyGroup(t *testing.T) {
	require.NotContains(t, channelUpstreamModelUpdateSelectFields, "group")
}

func setupChannelControllerGroupRemovalTestDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

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

func seedChannelForGroupRemoval(t *testing.T, db *gorm.DB, channel *model.Channel) {
	t.Helper()
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
}
