package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
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
