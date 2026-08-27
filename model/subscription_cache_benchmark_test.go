package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func BenchmarkCachePrimaryBillableSelectionSettingReuse(b *testing.B) {
	const userID = 8796
	_ = DB.Where("id = ?", userID).Delete(&User{}).Error
	b.Cleanup(func() {
		_ = DB.Where("id = ?", userID).Delete(&User{}).Error
		primaryBillableSubscriptionCache.Delete(primaryBillableSubscriptionCacheKey(userID))
	})
	user := User{
		Id:       userID,
		Username: "cached_selection_bench",
		Status:   common.UserStatusEnabled,
		AffCode:  "aff8796",
		Setting:  `{"subscription_billing_strategy":"single_active","active_subscription_id":8798}`,
	}
	if err := DB.Create(&user).Error; err != nil {
		b.Fatal(err)
	}
	plan := &SubscriptionPlan{Id: 8797, Enabled: true, MonthlyTokenLimit: 1_000_000}
	sub := &UserSubscription{
		Id:         8798,
		UserId:     userID,
		PlanId:     plan.Id,
		Status:     SubscriptionStatusActive,
		TokenLimit: 1_000_000,
	}

	b.Run("query_setting", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var loaded User
			if err := DB.Select("setting").Where("id = ?", userID).First(&loaded).Error; err != nil {
				b.Fatal(err)
			}
			cachePrimaryBillableSelection(userID, loaded.Setting, sub, plan, true)
		}
	})

	b.Run("reuse_setting", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			cachePrimaryBillableSelection(userID, user.Setting, sub, plan, true)
		}
	})
}
