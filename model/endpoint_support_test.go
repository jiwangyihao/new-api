package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupEndpointSupportTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	initCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &Model{}))

	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		InvalidatePricingCache()
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func endpointStringsForTest(endpoints map[string]any) string {
	data, err := common.Marshal(endpoints)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestChannelSupportsEndpointIgnoresModelEndpointMetadata(t *testing.T) {
	db := setupEndpointSupportTestDB(t)
	channel := &Channel{Id: 1, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "gpt-5.5", ChannelId: channel.Id, Enabled: true}).Error)
	require.NoError(t, db.Create(&Model{ModelName: "gpt-5.5", NameRule: NameRuleExact, Status: 1, Endpoints: endpointStringsForTest(map[string]any{
		string(constant.EndpointTypeOpenAIResponse): "/v1/responses",
	})}).Error)

	RefreshEndpointSupportCache()

	assert.True(t, ChannelSupportsEndpoint(channel, "gpt-5.5", constant.EndpointTypeOpenAIResponse))
	assert.True(t, ChannelSupportsEndpoint(channel, "gpt-5.5", constant.EndpointTypeOpenAIResponseCompact))
}

func TestChannelSupportsEndpointUsesChannelSettingsBeforeTypeDefaults(t *testing.T) {
	db := setupEndpointSupportTestDB(t)
	channel := &Channel{
		Id:     4,
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.5",
		Group:  "default",
		OtherSettings: endpointStringsForTest(map[string]any{
			"supported_endpoint_types": []string{string(constant.EndpointTypeOpenAI), string(constant.EndpointTypeOpenAIResponseCompact)},
		}),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "gpt-5.5", ChannelId: channel.Id, Enabled: true}).Error)
	RefreshEndpointSupportCache()

	assert.True(t, ChannelSupportsEndpoint(channel, "gpt-5.5", constant.EndpointTypeOpenAIResponseCompact))
	assert.False(t, ChannelSupportsEndpoint(channel, "gpt-5.5", constant.EndpointTypeOpenAIResponse))
}

func TestModelEndpointMetadataDoesNotGateChannelSupport(t *testing.T) {
	db := setupEndpointSupportTestDB(t)
	channel := &Channel{
		Id:     5,
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.5",
		Group:  "default",
		OtherSettings: endpointStringsForTest(map[string]any{
			"supported_endpoint_types": []string{string(constant.EndpointTypeOpenAIResponseCompact)},
		}),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "gpt-5.5", ChannelId: channel.Id, Enabled: true}).Error)
	require.NoError(t, db.Create(&Model{ModelName: "gpt-5.5", NameRule: NameRuleExact, Status: 1, Endpoints: endpointStringsForTest(map[string]any{
		string(constant.EndpointTypeOpenAIResponse): "/v1/responses",
	})}).Error)
	RefreshEndpointSupportCache()

	assert.True(t, ChannelSupportsEndpoint(channel, "gpt-5.5", constant.EndpointTypeOpenAIResponseCompact))
}

func TestEndpointDisplayTypesDoNotExposeUnsupportedModelMetadata(t *testing.T) {
	db := setupEndpointSupportTestDB(t)
	channel := &Channel{
		Id:     7,
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.5",
		Group:  "default",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "gpt-5.5", ChannelId: channel.Id, Enabled: true}).Error)
	require.NoError(t, db.Create(&Model{ModelName: "gpt-5.5", NameRule: NameRuleExact, Status: 1, Endpoints: endpointStringsForTest(map[string]any{
		string(constant.EndpointTypeOpenAIResponseCompact): "/custom/compact",
	})}).Error)
	RefreshEndpointSupportCache()

	endpoints := GetEndpointDisplayTypes(channel, "gpt-5.5")

	assert.Contains(t, endpoints, constant.EndpointTypeOpenAI)
	assert.NotContains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
}

func TestGetChannelForEndpointUsesChannelSettings(t *testing.T) {
	db := setupEndpointSupportTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 6, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", OtherSettings: endpointStringsForTest(map[string]any{
		"supported_endpoint_types": []string{string(constant.EndpointTypeOpenAIResponse), string(constant.EndpointTypeOpenAIResponseCompact)},
	})}).Error)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "gpt-5.5", ChannelId: 6, Enabled: true}).Error)
	RefreshEndpointSupportCache()

	channel, err := GetChannelForEndpoint("default", "gpt-5.5", 0, constant.EndpointTypeOpenAIResponseCompact)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 6, channel.Id)
}

func TestGetChannelForEndpointFiltersBeforePrioritySelection(t *testing.T) {
	db := setupEndpointSupportTestDB(t)
	openAIHighPriority := int64(100)
	codexLowPriority := int64(10)
	require.NoError(t, db.Create(&Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Priority: &openAIHighPriority}).Error)
	require.NoError(t, db.Create(&Channel{Id: 2, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Priority: &codexLowPriority}).Error)
	require.NoError(t, db.Create(&[]Ability{
		{Group: "default", Model: "gpt-5.5", ChannelId: 1, Enabled: true, Priority: &openAIHighPriority},
		{Group: "default", Model: "gpt-5.5", ChannelId: 2, Enabled: true, Priority: &codexLowPriority},
	}).Error)

	RefreshEndpointSupportCache()
	channel, err := GetChannelForEndpoint("default", "gpt-5.5", 0, constant.EndpointTypeOpenAIResponseCompact)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2, channel.Id)
}

func TestGetChannelForEndpointWithoutRequiredEndpointUsesLegacySelection(t *testing.T) {
	db := setupEndpointSupportTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 3, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default"}).Error)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "gpt-5.5", ChannelId: 3, Enabled: true}).Error)
	RefreshEndpointSupportCache()

	channel, err := GetChannelForEndpoint("default", "gpt-5.5", 0, "")

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 3, channel.Id)
}

func TestGetRandomSatisfiedChannelForEndpointMemoryCache(t *testing.T) {
	db := setupEndpointSupportTestDB(t)
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCache })

	require.NoError(t, db.Create(&Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default"}).Error)
	require.NoError(t, db.Create(&Channel{Id: 2, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default"}).Error)
	require.NoError(t, db.Create(&[]Ability{
		{Group: "default", Model: "gpt-5.5", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "gpt-5.5", ChannelId: 2, Enabled: true},
	}).Error)

	RefreshEndpointSupportCache()
	InitChannelCache()
	channel, err := GetRandomSatisfiedChannelForEndpoint("default", "gpt-5.5", 0, constant.EndpointTypeOpenAIResponseCompact)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2, channel.Id)
}
