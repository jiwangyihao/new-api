package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionConcurrencyOptionsUpdateRuntimeBooleans(t *testing.T) {
	oldMap := common.OptionMap
	oldRequireRedis := common.SubscriptionConcurrencyRequireRedis
	oldFailOpen := common.SubscriptionConcurrencyFailOpen
	common.OptionMap = make(map[string]string)
	common.SubscriptionConcurrencyRequireRedis = false
	common.SubscriptionConcurrencyFailOpen = false
	t.Cleanup(func() {
		common.OptionMap = oldMap
		common.SubscriptionConcurrencyRequireRedis = oldRequireRedis
		common.SubscriptionConcurrencyFailOpen = oldFailOpen
	})

	require.NoError(t, updateOptionMap("SubscriptionConcurrencyRequireRedis", "true"))
	require.True(t, common.SubscriptionConcurrencyRequireRedis)
	require.NoError(t, updateOptionMap("SubscriptionConcurrencyFailOpen", "true"))
	require.True(t, common.SubscriptionConcurrencyFailOpen)
}
