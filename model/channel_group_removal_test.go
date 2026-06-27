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
	originalGroupModel2Channels := groupModel2channels
	originalChannelsIDM := channelsIDM
	originalDefaultExplicit := defaultGroupHasExplicitMembersCache

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	model2channels = nil
	groupModel2channels = nil
	channelsIDM = nil

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelGroup{}, &ChannelGroupChannel{}, &TokenGroupBinding{}))
	require.NoError(t, ensureDefaultChannelGroup())

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
		groupModel2channels = originalGroupModel2Channels
		channelsIDM = originalChannelsIDM
		defaultGroupHasExplicitMembersCache = originalDefaultExplicit
	})

	return db
}

func TestChannelAbilitiesUseDefaultGroupWhenNoMembership(t *testing.T) {
	db := setupChannelGroupRemovalTestDB(t)
	priority := int64(10)
	weight := uint(5)
	channel := &Channel{Id: 1001, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "groupless", Models: "gpt-test,gpt-test", Priority: &priority, Weight: &weight}
	require.NoError(t, db.Create(channel).Error)

	require.NoError(t, channel.UpdateAbilities(nil))

	// 无显式分组成员时，渠道隐式属于默认分组，abilities 写入默认分组名（每个 model 一行）。
	var abilities []Ability
	require.NoError(t, db.Where("channel_id = ? AND model = ?", channel.Id, "gpt-test").Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.Equal(t, DefaultChannelGroupName, abilities[0].Group)

	// 分组无关选择仍可命中该渠道。
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

func TestChannelCacheCompatibilityWrappersRespectGroups(t *testing.T) {
	db := setupChannelGroupRemovalTestDB(t)
	common.MemoryCacheEnabled = true
	priority := int64(1)
	channel := &Channel{Id: 1003, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Status: common.ChannelStatusEnabled, Name: "cache-grouped", Models: "gpt-test", Priority: &priority}
	require.NoError(t, db.Create(channel).Error)
	// 显式写入 vip 分组的 ability 行（模拟渠道属于 vip 分组）。
	require.NoError(t, db.Create(&Ability{Group: "vip", Model: "gpt-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: 1}).Error)

	InitChannelCache()
	// 渠道在 vip 分组内可命中。
	require.True(t, IsChannelEnabledForGroupModel("vip", "gpt-test", channel.Id))
	require.True(t, IsChannelEnabledForAnyGroupModel([]string{"vip"}, "gpt-test", channel.Id))
	// 不在 missing 分组内不可命中（分组过滤生效）。
	require.False(t, IsChannelEnabledForGroupModel("missing", "gpt-test", channel.Id))

	CacheUpdateChannelStatus(channel.Id, common.ChannelStatusManuallyDisabled)
	require.False(t, IsChannelEnabledForGroupModel("vip", "gpt-test", channel.Id))
	require.False(t, IsChannelEnabledForAnyGroupModel([]string{"vip"}, "gpt-test", channel.Id))
}
