package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ensureChannelMultiplierSelectorTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}))
}

func seedChannelMultiplierSelectorTest(t *testing.T, id int, modelName string, priority int64, multiplier float64) {
	t.Helper()
	priorityPtr := priority
	require.NoError(t, DB.Create(&Channel{
		Id:                     id,
		Name:                   "channel-multiplier-selector",
		Key:                    "sk-selector",
		Status:                 common.ChannelStatusEnabled,
		Models:                 modelName,
		Type:                   constant.ChannelTypeOpenAI,
		Priority:               &priorityPtr,
		TokenBillingMultiplier: multiplier,
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "default",
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    0,
	}).Error)
}

func TestRetryChannelMultiplierMemoryCacheSelectsSameMultiplierUnusedCandidate(t *testing.T) {
	ensureChannelMultiplierSelectorTables(t)
	truncateTables(t)
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCache })
	const modelName = "gpt-retry-channel-multiplier-memory"
	seedChannelMultiplierSelectorTest(t, 97101, modelName, 100, 2)
	seedChannelMultiplierSelectorTest(t, 97102, modelName, 100, 1)
	seedChannelMultiplierSelectorTest(t, 97103, modelName, 100, 2)
	InitChannelCache()

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 0, "", []int{97101}, 2, true)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 97103, channel.Id)
	assert.InDelta(t, 2, channel.GetTokenBillingMultiplier(), 1e-9)
}

func TestRetryChannelMultiplierMemoryCacheNoSameMultiplierCandidateStopsRetry(t *testing.T) {
	ensureChannelMultiplierSelectorTables(t)
	truncateTables(t)
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCache })
	const modelName = "gpt-retry-channel-multiplier-memory-none"
	seedChannelMultiplierSelectorTest(t, 97111, modelName, 100, 2)
	seedChannelMultiplierSelectorTest(t, 97112, modelName, 100, 1)
	InitChannelCache()

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 0, "", []int{97111}, 2, true)

	require.NoError(t, err)
	assert.Nil(t, channel)
}

func TestAbilityRetryMultiplierDatabaseFallbackSelectsSameMultiplierUnusedCandidate(t *testing.T) {
	ensureChannelMultiplierSelectorTables(t)
	truncateTables(t)
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCache })
	const modelName = "gpt-ability-channel-multiplier-db"
	seedChannelMultiplierSelectorTest(t, 97121, modelName, 100, 2)
	seedChannelMultiplierSelectorTest(t, 97122, modelName, 100, 1)
	seedChannelMultiplierSelectorTest(t, 97123, modelName, 100, 2)

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 0, "", []int{97121}, 2, true)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 97123, channel.Id)
	assert.InDelta(t, 2, channel.GetTokenBillingMultiplier(), 1e-9)
}

func TestRetryChannelMultiplierMemoryCacheFiltersBeforePrioritySelection(t *testing.T) {
	ensureChannelMultiplierSelectorTables(t)
	truncateTables(t)
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCache })
	const modelName = "gpt-retry-channel-multiplier-memory-priority"
	seedChannelMultiplierSelectorTest(t, 97131, modelName, 100, 2)
	seedChannelMultiplierSelectorTest(t, 97132, modelName, 100, 2)
	seedChannelMultiplierSelectorTest(t, 97133, modelName, 50, 2)
	InitChannelCache()

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 1, "", []int{97131}, 2, true)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 97132, channel.Id)
}

func TestAbilityRetryMultiplierDatabaseFallbackFiltersBeforePrioritySelection(t *testing.T) {
	ensureChannelMultiplierSelectorTables(t)
	truncateTables(t)
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCache })
	const modelName = "gpt-ability-channel-multiplier-db-priority"
	seedChannelMultiplierSelectorTest(t, 97141, modelName, 100, 2)
	seedChannelMultiplierSelectorTest(t, 97142, modelName, 100, 2)
	seedChannelMultiplierSelectorTest(t, 97143, modelName, 50, 2)

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 1, "", []int{97141}, 2, true)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 97142, channel.Id)
}
