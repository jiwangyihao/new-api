package setting

import "github.com/QuantumNous/new-api/setting/config"

type SubscriptionAnalyticsExcludedUser struct {
	UserID     int    `json:"user_id"`
	Username   string `json:"username,omitempty"`
	Reason     string `json:"reason,omitempty"`
	ExcludedAt int64  `json:"excluded_at,omitempty"`
	ExcludedBy int    `json:"excluded_by,omitempty"`
}

type SubscriptionAnalyticsSetting struct {
	ExcludedUsers []SubscriptionAnalyticsExcludedUser `json:"excluded_users"`
}

var subscriptionAnalyticsSetting = SubscriptionAnalyticsSetting{ExcludedUsers: []SubscriptionAnalyticsExcludedUser{}}

func init() {
	config.GlobalConfig.Register("subscription_analytics", &subscriptionAnalyticsSetting)
}

func GetSubscriptionAnalyticsExcludedUsers() map[int]SubscriptionAnalyticsExcludedUser {
	result := make(map[int]SubscriptionAnalyticsExcludedUser, len(subscriptionAnalyticsSetting.ExcludedUsers))
	for _, item := range subscriptionAnalyticsSetting.ExcludedUsers {
		if item.UserID > 0 {
			result[item.UserID] = item
		}
	}
	return result
}
