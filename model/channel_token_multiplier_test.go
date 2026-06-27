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
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}, &ChannelGroup{}, &ChannelGroupChannel{}, &TokenGroupBinding{}))
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

func seedChannelBillingProfileSelectorTest(t *testing.T, id int, modelName string, priority int64, multiplier float64, mode string, fixedCredits int64, dynamicEnabled bool) {
	t.Helper()
	priorityPtr := priority
	require.NoError(t, DB.Create(&Channel{
		Id:                              id,
		Name:                            "channel-billing-profile-selector",
		Key:                             "sk-selector",
		Status:                          common.ChannelStatusEnabled,
		Models:                          modelName,
		Type:                            constant.ChannelTypeOpenAI,
		Priority:                        &priorityPtr,
		TokenBillingMultiplier:          multiplier,
		CreditBillingMode:               mode,
		FixedRequestCredits:             fixedCredits,
		DynamicBillingMultiplierEnabled: dynamicEnabled,
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

func TestRetryBillingProfileMemoryCacheDoesNotCrossProfile(t *testing.T) {
	ensureChannelMultiplierSelectorTables(t)
	truncateTables(t)
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCache })
	const modelName = "gpt-retry-billing-profile-memory"
	seedChannelBillingProfileSelectorTest(t, 97301, modelName, 100, 2, "fixed_request", 80000, true)
	seedChannelBillingProfileSelectorTest(t, 97302, modelName, 100, 2, "usage_tokens", 0, true)
	seedChannelBillingProfileSelectorTest(t, 97303, modelName, 100, 2, "fixed_request", 80000, false)
	seedChannelBillingProfileSelectorTest(t, 97304, modelName, 100, 2, "fixed_request", 80000, true)
	InitChannelCache()

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 0, "", []int{97301}, 2, true, ChannelBillingProfile{CreditBillingMode: "fixed_request", FixedRequestCredits: 80000, TokenBillingMultiplier: 2, DynamicBillingMultiplierEnabled: true}, true)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 97304, channel.Id)
}

func TestRetryBillingProfileDatabaseFallbackDoesNotCrossProfile(t *testing.T) {
	ensureChannelMultiplierSelectorTables(t)
	truncateTables(t)
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCache })
	const modelName = "gpt-retry-billing-profile-db"
	seedChannelBillingProfileSelectorTest(t, 97311, modelName, 100, 2, "fixed_request", 80000, true)
	seedChannelBillingProfileSelectorTest(t, 97312, modelName, 100, 2, "usage_tokens", 0, true)
	seedChannelBillingProfileSelectorTest(t, 97313, modelName, 100, 2, "fixed_request", 80000, false)
	seedChannelBillingProfileSelectorTest(t, 97314, modelName, 100, 2, "fixed_request", 80000, true)

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 0, "", []int{97311}, 2, true, ChannelBillingProfile{CreditBillingMode: "fixed_request", FixedRequestCredits: 80000, TokenBillingMultiplier: 2, DynamicBillingMultiplierEnabled: true}, true)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 97314, channel.Id)
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

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 0, "", []int{97101}, 2, true, ChannelBillingProfile{}, false)

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

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 0, "", []int{97111}, 2, true, ChannelBillingProfile{}, false)

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

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 0, "", []int{97121}, 2, true, ChannelBillingProfile{}, false)

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

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 1, "", []int{97131}, 2, true, ChannelBillingProfile{}, false)

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

	channel, err := GetRandomSatisfiedChannelForEndpointWithRetryConstraints("default", modelName, 1, "", []int{97141}, 2, true, ChannelBillingProfile{}, false)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 97142, channel.Id)
}
