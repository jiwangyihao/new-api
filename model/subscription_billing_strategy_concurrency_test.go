package model

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConcurrentUsageResetDoesNotOverwritePreConsume(t *testing.T) {
	originalDB := DB
	originalLogDB := LOG_DB
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled

	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "subscription-reset.db")) + "?cache=shared&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}))

	DB = db
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove("test:block_usage_subscription_read")
		ClearPrimaryBillableSubscriptionCacheForTest()
		ClearSubscriptionPlanCacheForTest()
		DB = originalDB
		LOG_DB = originalLogDB
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		common.RedisEnabled = originalRedisEnabled
		_ = sqlDB.Close()
	})

	const userID = 8081
	const planID = 8082
	const subscriptionID = 8083
	user := User{Id: userID, Username: "concurrent-reset-usage", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = subscriptionID
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	code := "concurrent-reset-usage"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: planID, Title: "Concurrent reset usage", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code, QuotaResetPeriod: SubscriptionResetDaily}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: subscriptionID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed, Status: "active", StartTime: now - 2*86400, EndTime: now + 3*86400, TokenLimit: 100, TokenUsed: 90, LastResetTime: now - 2*86400, NextResetTime: now - 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	usageRead := make(chan struct{})
	allowUsageWrite := make(chan struct{})
	var blocked atomic.Bool
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:block_usage_subscription_read", func(tx *gorm.DB) {
		if tx.Statement.Table != "user_subscriptions" || !blocked.CompareAndSwap(false, true) {
			return
		}
		close(usageRead)
		<-allowUsageWrite
	}))

	type usageResult struct {
		usage *ActiveSubscriptionUsage
		err   error
	}
	usageDone := make(chan usageResult, 1)
	go func() {
		usage, usageErr := GetActiveDistributorSubscriptionUsage(userID)
		usageDone <- usageResult{usage: usage, err: usageErr}
	}()

	select {
	case <-usageRead:
	case <-time.After(5 * time.Second):
		t.Fatal("usage query did not reach the locked subscription read")
	}

	type consumeResult struct {
		result *SubscriptionPreConsumeResult
		err    error
	}
	consumeDone := make(chan consumeResult, 1)
	go func() {
		result, consumeErr := PreConsumeUserSubscription("concurrent-reset-preconsume", userID, "gpt-4o", 0, 5)
		consumeDone <- consumeResult{result: result, err: consumeErr}
	}()

	var consumed consumeResult
	preConsumeFinishedEarly := false
	select {
	case consumed = <-consumeDone:
		preConsumeFinishedEarly = true
	case <-time.After(200 * time.Millisecond):
	}
	close(allowUsageWrite)
	if !preConsumeFinishedEarly {
		select {
		case consumed = <-consumeDone:
		case <-time.After(10 * time.Second):
			t.Fatal("pre-consume did not finish after releasing the usage transaction")
		}
	}
	assert.False(t, preConsumeFinishedEarly, "pre-consume must serialize behind the in-flight quota reset")
	require.NoError(t, consumed.err)
	require.NotNil(t, consumed.result)
	assert.Equal(t, subscriptionID, consumed.result.UserSubscriptionId)

	var observed usageResult
	select {
	case observed = <-usageDone:
	case <-time.After(5 * time.Second):
		t.Fatal("usage reset did not retry after the concurrent pre-consume")
	}
	require.NoError(t, observed.err)
	require.NotNil(t, observed.usage)
	assert.Equal(t, int64(5), observed.usage.TokenUsed)

	var persisted UserSubscription
	require.NoError(t, DB.First(&persisted, subscriptionID).Error)
	assert.Equal(t, int64(5), persisted.TokenUsed)
	assert.Greater(t, persisted.NextResetTime, now)
}
