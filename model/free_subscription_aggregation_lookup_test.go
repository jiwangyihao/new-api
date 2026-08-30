package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func loadFreeSubscriptionAggregationIdentityLegacy(db *gorm.DB, subscriptionID int) (freeSubscriptionAggregationIdentity, bool, error) {
	var sub UserSubscription
	if err := db.Where("id = ?", subscriptionID).First(&sub).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return freeSubscriptionAggregationIdentity{}, false, nil
		}
		return freeSubscriptionAggregationIdentity{}, false, err
	}
	var user User
	if err := db.Select("id").Where("id = ?", sub.UserId).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return freeSubscriptionAggregationIdentity{}, false, nil
		}
		return freeSubscriptionAggregationIdentity{}, false, err
	}
	var plan SubscriptionPlan
	if err := db.Where("id = ?", sub.PlanId).First(&plan).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return freeSubscriptionAggregationIdentity{}, false, nil
		}
		return freeSubscriptionAggregationIdentity{}, false, err
	}
	return freeSubscriptionAggregationIdentity{
		SubscriptionID: sub.Id,
		UserID:         sub.UserId,
		PlanID:         sub.PlanId,
		StartTime:      sub.StartTime,
		EndTime:        sub.EndTime,
		GrantReason:    sub.GrantReason,
		Source:         sub.Source,
		PriceAmount:    plan.PriceAmount,
		IsTrial:        plan.IsTrial,
	}, true, nil
}

func TestLoadFreeSubscriptionAggregationIdentityJoinedMatchesLegacy(t *testing.T) {
	setupLogAggregationTestDBs(t)
	start := int64(1778716800)
	sub := seedLogAggregationFreeSubscription(t, 7101, 7201, start)

	legacy, legacyFound, err := loadFreeSubscriptionAggregationIdentityLegacy(DB, sub.Id)
	require.NoError(t, err)
	joined, joinedFound, err := loadFreeSubscriptionAggregationIdentity(DB, sub.Id)
	require.NoError(t, err)
	require.Equal(t, legacyFound, joinedFound)
	require.Equal(t, legacy, joined)

	_, legacyFound, err = loadFreeSubscriptionAggregationIdentityLegacy(DB, sub.Id+99999)
	require.NoError(t, err)
	_, joinedFound, err = loadFreeSubscriptionAggregationIdentity(DB, sub.Id+99999)
	require.NoError(t, err)
	require.Equal(t, legacyFound, joinedFound)

	require.NoError(t, DB.Delete(&User{}, sub.UserId).Error)
	_, legacyFound, err = loadFreeSubscriptionAggregationIdentityLegacy(DB, sub.Id)
	require.NoError(t, err)
	_, joinedFound, err = loadFreeSubscriptionAggregationIdentity(DB, sub.Id)
	require.NoError(t, err)
	require.Equal(t, legacyFound, joinedFound)
}

func TestApplyFreeSubscriptionUsageAggregationSkipsMonthlyInviteGrantReasonEvenWhenSourceLooksTrial(t *testing.T) {
	setupLogAggregationTestDBs(t)
	start := int64(1778716800)
	userID := 7401
	subscriptionID := 7501
	require.NoError(t, DB.Create(&User{Id: userID, Username: "monthly-invite-priority", Status: common.UserStatusEnabled, AffCode: "monthly-invite-priority"}).Error)
	plan := SubscriptionPlan{Id: 7601, Title: "trial-marked-monthly-invite", Enabled: true, PriceAmount: 0, IsTrial: true}
	require.NoError(t, DB.Create(&plan).Error)
	sub := UserSubscription{Id: subscriptionID, UserId: userID, PlanId: plan.Id, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: "trial_code", StartTime: start, EndTime: start + 24*3600, Status: "active"}
	require.NoError(t, DB.Create(&sub).Error)
	tokens := int64(17)
	log := &Log{UserId: userID, CreatedAt: start + 120, Type: LogTypeConsume, SubscriptionID: &sub.Id, SubscriptionTokensConsumed: &tokens}
	require.NoError(t, LOG_DB.Create(log).Error)
	require.NoError(t, queueLogAggregationEventsForLogs([]*Log{log}))

	require.NoError(t, ApplyPendingLogAggregationEvents(100))
	var count int64
	require.NoError(t, LOG_DB.Model(&FreeSubscriptionUsageHourly{}).Where("subscription_id = ?", sub.Id).Count(&count).Error)
	require.Zero(t, count)
}

func BenchmarkLoadFreeSubscriptionAggregationIdentity(b *testing.B) {
	dsn := fmt.Sprintf("file:identity_bench_%p?mode=memory&cache=shared", b)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}); err != nil {
		b.Fatal(err)
	}
	user := User{Id: 8101, Username: "identity-benchmark", Status: common.UserStatusEnabled, AffCode: "identity-benchmark"}
	if err := db.Create(&user).Error; err != nil {
		b.Fatal(err)
	}
	plan := SubscriptionPlan{Id: 8201, Title: "identity-plan", Enabled: true, PriceAmount: 0, IsTrial: true}
	if err := db.Create(&plan).Error; err != nil {
		b.Fatal(err)
	}
	sub := UserSubscription{Id: 8301, UserId: user.Id, PlanId: plan.Id, GrantReason: "trial_code", Source: "trial_code", StartTime: 1778716800, EndTime: 1778803200, Status: "active"}
	if err := db.Create(&sub).Error; err != nil {
		b.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		load func(*gorm.DB, int) (freeSubscriptionAggregationIdentity, bool, error)
	}{
		{name: "legacy_three_queries", load: loadFreeSubscriptionAggregationIdentityLegacy},
		{name: "joined_row_scan", load: loadFreeSubscriptionAggregationIdentity},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, found, err := tc.load(db, sub.Id)
				if err != nil || !found {
					b.Fatalf("found=%v err=%v", found, err)
				}
			}
		})
	}
}
