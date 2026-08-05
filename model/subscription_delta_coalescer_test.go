package model

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormlogger "gorm.io/gorm/logger"
)

type subscriptionDeltaUpdateLogger struct {
	gormlogger.Interface
	updates atomic.Int64
}

func (l *subscriptionDeltaUpdateLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	lower := strings.ToLower(sql)
	if strings.Contains(lower, "update") && strings.Contains(lower, "user_subscriptions") && strings.Contains(lower, "token_used") {
		l.updates.Add(1)
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

func TestPostConsumeUserSubscriptionTokenDeltaCoalescesConcurrentHotWrites(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.Create(&User{Id: 7510, Username: "delta_coalesce_user", Status: common.UserStatusEnabled, AffCode: "aff7510"}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7511, "delta-coalesce", 1000)
	seedUserSubscriptionForDistributorTest(t, 7512, 7510, 7511, 1000, 0, 1, "order")

	oldCoalescer := subscriptionTokenDeltaCoalescer
	subscriptionTokenDeltaCoalescer = newSubscriptionTokenDeltaCoalescer(subscriptionTokenDeltaCoalesceDelay)
	t.Cleanup(func() { subscriptionTokenDeltaCoalescer = oldCoalescer })

	counter := &subscriptionDeltaUpdateLogger{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	restoreLogger := DB.Config.Logger
	DB.Config.Logger = counter
	t.Cleanup(func() { DB.Config.Logger = restoreLogger })

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			errCh <- PostConsumeUserSubscriptionTokenDelta(7512, 26)
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	var got UserSubscription
	require.NoError(t, DB.First(&got, 7512).Error)
	assert.Equal(t, int64(workers*26), got.TokenUsed)
	require.LessOrEqual(t, counter.updates.Load(), int64(4), "post-consume token deltas should be coalesced under concurrent hot writes")
}

func TestCreditRequestTargetCoalescerPreservesEnqueueOrderAndResults(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const firstRequestID = "credit-coalescer-first"
	first, err := PreConsumeUserSubscriptionByUnits(firstRequestID, user.Id, "gpt-4o", 0, 0, 499)
	require.NoError(t, err)
	const secondRequestID = "credit-coalescer-second"
	second, err := PreConsumeUserSubscriptionByUnits(secondRequestID, user.Id, "gpt-4o", 0, 0, 500)
	require.NoError(t, err)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, first.UserSubscriptionId)
	require.Equal(t, first.UserSubscriptionId, second.UserSubscriptionId)

	oldCoalescer := subscriptionTokenDeltaCoalescer
	subscriptionTokenDeltaCoalescer = newSubscriptionTokenDeltaCoalescer(100 * time.Millisecond)
	t.Cleanup(func() { subscriptionTokenDeltaCoalescer = oldCoalescer })

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- SettleUserSubscriptionRequestTarget(firstRequestID, first.UserSubscriptionId, 500, false)
	}()
	select {
	case err := <-firstDone:
		require.NoError(t, err)
		t.Fatal("Credit request target bypassed the coalescer")
	case <-time.After(20 * time.Millisecond):
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- SettleUserSubscriptionRequestTarget(secondRequestID, second.UserSubscriptionId, 501, false)
	}()
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, first.UserSubscriptionId).Error)
	require.Equal(t, int64(1_001), subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Zero(t, state.AvailableCredit)
	require.Zero(t, state.ExactCostMicros)

	var firstRecord SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", firstRequestID).First(&firstRecord).Error)
	require.Equal(t, int64(500), firstRecord.AppliedCredit)
	require.Equal(t, int64(500), firstRecord.DeductedAvailableCredit)
	require.Zero(t, firstRecord.DebtFormedCredit)
	require.Equal(t, int64(20_000_000), firstRecord.DeductedExactCostMicros)
	var secondRecord SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", secondRequestID).First(&secondRecord).Error)
	require.Equal(t, int64(501), secondRecord.AppliedCredit)
	require.Equal(t, int64(500), secondRecord.DeductedAvailableCredit)
	require.Equal(t, int64(1), secondRecord.DebtFormedCredit)
	require.Equal(t, int64(20_000_000), secondRecord.DeductedExactCostMicros)
}
