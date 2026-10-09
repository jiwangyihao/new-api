package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionConcurrencyOptionsUpdateRuntimeBooleans(t *testing.T) {
	oldMap := common.OptionMap
	oldRequireRedis := common.SubscriptionConcurrencyRequireRedis
	oldFailOpen := common.SubscriptionConcurrencyFailOpen
	oldQueueCapacity := common.SubscriptionConcurrencyQueueCapacity
	common.OptionMap = make(map[string]string)
	common.SubscriptionConcurrencyRequireRedis = false
	common.SubscriptionConcurrencyFailOpen = false
	common.SubscriptionConcurrencyQueueCapacity = 10
	t.Cleanup(func() {
		common.OptionMap = oldMap
		common.SubscriptionConcurrencyRequireRedis = oldRequireRedis
		common.SubscriptionConcurrencyFailOpen = oldFailOpen
		common.SubscriptionConcurrencyQueueCapacity = oldQueueCapacity
	})

	require.NoError(t, updateOptionMap("SubscriptionConcurrencyRequireRedis", "true"))
	require.True(t, common.SubscriptionConcurrencyRequireRedis)
	require.NoError(t, updateOptionMap("SubscriptionConcurrencyFailOpen", "true"))
	require.True(t, common.SubscriptionConcurrencyFailOpen)
	require.NoError(t, updateOptionMap("SubscriptionConcurrencyQueueCapacity", "25"))
	require.Equal(t, 25, common.SubscriptionConcurrencyQueueCapacity)
}

func TestUpdateOptionMapKyrenDoesNotOverwriteSecretsWithEmptyValue(t *testing.T) {
	oldMap := common.OptionMap
	oldAPIKey := setting.KyrenApiKey
	oldWebhookSecret := setting.KyrenWebhookSecret
	common.OptionMap = map[string]string{
		"KyrenApiKey":        "kyren_live_existing",
		"KyrenWebhookSecret": "whsec_existing",
	}
	setting.KyrenApiKey = "kyren_live_existing"
	setting.KyrenWebhookSecret = "whsec_existing"
	t.Cleanup(func() {
		common.OptionMap = oldMap
		setting.KyrenApiKey = oldAPIKey
		setting.KyrenWebhookSecret = oldWebhookSecret
	})

	require.NoError(t, updateOptionMap("KyrenApiKey", ""))
	require.NoError(t, updateOptionMap("KyrenWebhookSecret", ""))

	assert.Equal(t, "kyren_live_existing", setting.KyrenApiKey)
	assert.Equal(t, "whsec_existing", setting.KyrenWebhookSecret)
	assert.Equal(t, "kyren_live_existing", common.OptionMap["KyrenApiKey"])
	assert.Equal(t, "whsec_existing", common.OptionMap["KyrenWebhookSecret"])
}

func TestUpdateOptionMapKyrenRejectsInvalidRuntimeValuesBeforeOptionMapUpdate(t *testing.T) {
	oldMap := common.OptionMap
	oldBaseURL := setting.KyrenBaseURL
	common.OptionMap = map[string]string{}
	setting.KyrenBaseURL = "https://api.kyren.top"
	t.Cleanup(func() {
		common.OptionMap = oldMap
		setting.KyrenBaseURL = oldBaseURL
	})

	err := updateOptionMap("KyrenBaseURL", "https://evil.example.com")

	require.Error(t, err)
	assert.Equal(t, "https://api.kyren.top", setting.KyrenBaseURL)
	_, exists := common.OptionMap["KyrenBaseURL"]
	assert.False(t, exists)
}

func TestRouteHealthOptionRejectsInvalidValuesBeforeMutation(t *testing.T) {
	old := *operation_setting.GetRouteHealthSetting()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { *operation_setting.GetRouteHealthSetting() = old; common.OptionMap = oldMap })
	for _, pair := range [][2]string{{"lease_seconds", "0"}, {"failure_ratio", "NaN"}, {"max_inflight", "-1"}, {"enabled", "maybe"}, {"failure_threshold", "1.5"}, {"unknown", "1"}} {
		key := "route_health_setting." + pair[0]
		require.Error(t, updateOptionMap(key, pair[1]))
		require.Equal(t, old, *operation_setting.GetRouteHealthSetting())
		require.NotContains(t, common.OptionMap, key)
	}
	require.NoError(t, updateOptionMap("route_health_setting.failure_threshold", "4"))
	require.Equal(t, 4, operation_setting.GetRouteHealthSetting().FailureThreshold)
}

func TestRouteHealthPolicyReloadValidatesWholeConfiguration(t *testing.T) {
	db := setupChannelGroupSelectionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	old := *operation_setting.GetRouteHealthSetting()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { *operation_setting.GetRouteHealthSetting() = old; common.OptionMap = oldMap })
	// The base exceeds the default maximum, but the persisted pair is valid.
	require.NoError(t, db.Create(&[]Option{{Key: "route_health_setting.base_cooldown_seconds", Value: "400"}, {Key: "route_health_setting.max_cooldown_seconds", Value: "500"}}).Error)
	loadOptionsFromDatabase()
	require.Equal(t, 400, operation_setting.GetRouteHealthSetting().BaseCooldownSeconds)
	require.Equal(t, 500, operation_setting.GetRouteHealthSetting().MaxCooldownSeconds)
	loadOptionsFromDatabase()
	require.Equal(t, 400, operation_setting.GetRouteHealthSetting().BaseCooldownSeconds)
	require.NoError(t, db.Model(&Option{}).Where("key = ?", "route_health_setting.failure_ratio").FirstOrCreate(&Option{Key: "route_health_setting.failure_ratio", Value: "NaN"}).Error)
	loadOptionsFromDatabase()
	require.Equal(t, old.FailureRatio, operation_setting.GetRouteHealthSetting().FailureRatio)
}
