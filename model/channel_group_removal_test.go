package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelGroupRemovalTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := DB
	originalLOGDB := LOG_DB
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRedisEnabled := common.RedisEnabled
	originalModel2Channels := model2channels
	originalChannelsIDM := channelsIDM

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	model2channels = nil
	channelsIDM = nil

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		DB = originalDB
		LOG_DB = originalLOGDB
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RedisEnabled = originalRedisEnabled
		model2channels = originalModel2Channels
		channelsIDM = originalChannelsIDM
	})

	return db
}

func TestChannelAbilitiesIgnoreBusinessGroups(t *testing.T) {
	db := setupChannelGroupRemovalTestDB(t)
	priority := int64(10)
	weight := uint(5)
	channel := &Channel{Id: 1001, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "groupless", Models: "gpt-test,gpt-test", Group: "default,vip", Priority: &priority, Weight: &weight}
	require.NoError(t, db.Create(channel).Error)

	require.NoError(t, channel.UpdateAbilities(nil))

	var abilities []Ability
	require.NoError(t, db.Where("channel_id = ? AND model = ?", channel.Id, "gpt-test").Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.Equal(t, legacyAbilityGroup, abilities[0].Group)

	selected, err := GetRandomSatisfiedChannel("gpt-test", 0)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, channel.Id, selected.Id)
}

func TestLegacyDuplicateAbilitiesAreDeduplicatedByChannel(t *testing.T) {
	db := setupChannelGroupRemovalTestDB(t)
	priority := int64(3)
	weight := uint(7)
	channel := &Channel{Id: 1002, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "legacy-dupe", Models: "gpt-test", Group: "legacy", Priority: &priority, Weight: &weight}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "gpt-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
	require.NoError(t, db.Create(&Ability{Group: "vip", Model: "gpt-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)

	selected, err := GetRandomSatisfiedChannel("gpt-test", 0)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, channel.Id, selected.Id)
}

func TestChannelCacheDeduplicatesRepeatedModelsPerChannel(t *testing.T) {
	db := setupChannelGroupRemovalTestDB(t)
	common.MemoryCacheEnabled = true
	channel := &Channel{Id: 1004, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "cache-dupe-model", Models: "gpt-test,gpt-test", Group: "default"}
	require.NoError(t, db.Create(channel).Error)

	InitChannelCache()
	require.Equal(t, []int{channel.Id}, model2channels["gpt-test"])
}

func TestChannelCacheCompatibilityWrappersIgnoreGroups(t *testing.T) {
	db := setupChannelGroupRemovalTestDB(t)
	common.MemoryCacheEnabled = true
	priority := int64(1)
	channel := &Channel{Id: 1003, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "cache-groupless", Models: "gpt-test", Group: "default"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{Group: "vip", Model: "gpt-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: 1}).Error)

	InitChannelCache()
	require.True(t, IsChannelEnabledForGroupModel("missing", "gpt-test", channel.Id))
	require.True(t, IsChannelEnabledForAnyGroupModel([]string{"missing"}, "gpt-test", channel.Id))

	CacheUpdateChannelStatus(channel.Id, common.ChannelStatusManuallyDisabled)
	require.False(t, IsChannelEnabledForGroupModel("missing", "gpt-test", channel.Id))
	require.False(t, IsChannelEnabledForAnyGroupModel([]string{"missing"}, "gpt-test", channel.Id))
}
