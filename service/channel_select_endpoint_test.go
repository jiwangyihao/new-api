package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectEndpointTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldMemoryCache := common.MemoryCacheEnabled
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Model{}))
	t.Cleanup(func() {
		model.DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.MemoryCacheEnabled = oldMemoryCache
		model.InvalidatePricingCache()
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestCacheGetRandomSatisfiedChannelFiltersEndpointOnRetry(t *testing.T) {
	db := setupChannelSelectEndpointTestDB(t)
	openAIHigh := int64(100)
	codexLow := int64(10)
	require.NoError(t, db.Create(&model.Channel{Id: 2501, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Priority: &openAIHigh}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 2502, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Priority: &codexLow}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "gpt-5.5", ChannelId: 2501, Enabled: true, Priority: &openAIHigh},
		{Group: "default", Model: "gpt-5.5", ChannelId: 2502, Enabled: true, Priority: &codexLow},
	}).Error)
	model.RefreshEndpointSupportCache()
	ctx := gin.Context{}

	channel, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: &ctx, TokenGroup: "default", ModelName: "gpt-5.5", Retry: common.GetPointer(0), EndpointType: constant.EndpointTypeOpenAIResponseCompact})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "default", group)
	assert.Equal(t, 2502, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelWithoutEndpointUsesLegacySelection(t *testing.T) {
	db := setupChannelSelectEndpointTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 2503, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2503, Enabled: true}).Error)
	model.RefreshEndpointSupportCache()
	ctx := gin.Context{}

	channel, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: &ctx, TokenGroup: "default", ModelName: "gpt-5.5", Retry: common.GetPointer(0)})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "default", group)
	assert.Equal(t, 2503, channel.Id)
}
