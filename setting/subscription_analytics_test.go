package setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionAnalyticsExcludedUsersReturnsCopy(t *testing.T) {
	old := subscriptionAnalyticsSetting.ExcludedUsers
	t.Cleanup(func() { subscriptionAnalyticsSetting.ExcludedUsers = old })
	subscriptionAnalyticsSetting.ExcludedUsers = []SubscriptionAnalyticsExcludedUser{{UserID: 10, Reason: "original"}}

	excluded := GetSubscriptionAnalyticsExcludedUsers()
	excluded[10] = SubscriptionAnalyticsExcludedUser{UserID: 10, Reason: "mutated"}

	again := GetSubscriptionAnalyticsExcludedUsers()
	require.Equal(t, "original", again[10].Reason)
}

func TestSubscriptionAnalyticsExcludedUsersLoadsFromGlobalConfig(t *testing.T) {
	old := subscriptionAnalyticsSetting.ExcludedUsers
	t.Cleanup(func() { subscriptionAnalyticsSetting.ExcludedUsers = old })
	subscriptionAnalyticsSetting.ExcludedUsers = nil

	err := config.GlobalConfig.LoadFromDB(map[string]string{
		"subscription_analytics.excluded_users": `[{"user_id":10,"reason":"ops","excluded_at":123,"excluded_by":7}]`,
	})
	require.NoError(t, err)

	excluded := GetSubscriptionAnalyticsExcludedUsers()
	require.Len(t, excluded, 1)
	require.Equal(t, 10, excluded[10].UserID)
	require.Equal(t, "ops", excluded[10].Reason)
	require.Equal(t, int64(123), excluded[10].ExcludedAt)
	require.Equal(t, 7, excluded[10].ExcludedBy)
}
