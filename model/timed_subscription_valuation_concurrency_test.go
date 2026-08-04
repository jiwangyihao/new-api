package model

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTimedSubscriptionValuationGrantConcurrentReplayLinearizes(t *testing.T) {
	db, userID, plan := setupTimedSubscriptionValuationConcurrencyTestDB(t)
	request := TimedSubscriptionGrantRequest{
		UserId:         userID,
		PlanId:         plan.Id,
		IdempotencyKey: "timed-concurrent-21903",
		SourceType:     TimedSubscriptionGrantSourceAdmin,
		Reason:         "并发管理员授予测试",
	}

	type barrierEvent uint8
	const (
		barrierEventPlanGuard barrierEvent = iota + 1
		barrierEventReplayBeforeGuard
	)
	events := make(chan barrierEvent, 8)
	releaseReplayReads := make(chan struct{})
	var releaseOnce sync.Once
	var planGuardAttempted atomic.Bool

	guardCallback := "issue21:observe_timed_plan_guard"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(guardCallback, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "SubscriptionPlan" {
			return
		}
		planGuardAttempted.Store(true)
		events <- barrierEventPlanGuard
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(guardCallback) })

	replayCallback := "issue21:barrier_after_timed_grant_replay"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(replayCallback, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "TimedSubscriptionValuationGrant" || tx.Error != nil || tx.RowsAffected != 0 || planGuardAttempted.Load() {
			return
		}
		events <- barrierEventReplayBeforeGuard
		<-releaseReplayReads
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(replayCallback) })

	type grantOutcome struct {
		result *UserSubscriptionCreationResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan grantOutcome, 2)
	for range 2 {
		go func() {
			<-start
			var result *UserSubscriptionCreationResult
			err := db.Transaction(func(tx *gorm.DB) error {
				var grantErr error
				result, grantErr = GrantTimedSubscriptionTx(tx, request)
				return grantErr
			})
			outcomes <- grantOutcome{result: result, err: err}
		}()
	}
	close(start)

	preGuardReplayReads := 0
	for preGuardReplayReads < 2 {
		switch <-events {
		case barrierEventPlanGuard:
			releaseOnce.Do(func() { close(releaseReplayReads) })
			preGuardReplayReads = 2
		case barrierEventReplayBeforeGuard:
			preGuardReplayReads++
			if preGuardReplayReads == 2 {
				releaseOnce.Do(func() { close(releaseReplayReads) })
			}
		}
	}

	results := make([]grantOutcome, 0, 2)
	for range 2 {
		results = append(results, <-outcomes)
	}
	var subscriptions []UserSubscription
	require.NoError(t, db.Where("user_id = ? AND plan_id = ?", userID, plan.Id).Find(&subscriptions).Error)
	require.Len(t, subscriptions, 1)
	var grants []TimedSubscriptionValuationGrant
	require.NoError(t, db.Find(&grants).Error)
	require.Len(t, grants, 1)
	require.Equal(t, subscriptions[0].Id, grants[0].UserSubscriptionId)

	for _, outcome := range results {
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.result)
		require.NotNil(t, outcome.result.Subscription)
	}
	require.Equal(t, results[0].result.Subscription.Id, results[1].result.Subscription.Id)
	require.Equal(t, results[0].result.EventStartTime, results[1].result.EventStartTime)
	require.Equal(t, results[0].result.EventEndTime, results[1].result.EventEndTime)
	require.Equal(t, results[0].result.EventEndTime, subscriptions[0].EndTime)
}

func setupTimedSubscriptionValuationConcurrencyTestDB(t *testing.T) (*gorm.DB, int, SubscriptionPlan) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "timed-grant-concurrency.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	DB = db
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	resetDBTimestampCacheForTest()
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &TimedSubscriptionValuationGrant{}))

	const userID = 21_901
	priceMicros := int64(40_000_000)
	plan := SubscriptionPlan{
		Id:                21_902,
		Title:             "Concurrent Timed Grant",
		Enabled:           true,
		EntitlementType:   SubscriptionEntitlementTimed,
		PriceAmount:       40,
		PriceAmountMicros: &priceMicros,
		Currency:          "CNY",
		DurationUnit:      SubscriptionDurationHour,
		DurationValue:     1,
		MonthlyTokenLimit: 1000,
		QuotaResetPeriod:  SubscriptionResetNever,
	}
	require.NoError(t, db.Create(&User{Id: userID, Username: "timed-grant-concurrency", Status: common.UserStatusEnabled, AffCode: "timed-grant-concurrency-aff"}).Error)
	require.NoError(t, db.Create(&plan).Error)

	t.Cleanup(func() {
		_ = sqlDB.Close()
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		ClearSubscriptionPlanCacheForTest()
		resetDBTimestampCacheForTest()
	})
	return db, userID, plan
}
