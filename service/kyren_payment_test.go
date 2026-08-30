package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConcurrentKyrenPaidAndFailedEventsReachOneTerminalOutcome(t *testing.T) {
	db := setupKyrenPaymentServiceTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 9701, Username: "kyren-concurrent-user", Status: common.UserStatusEnabled, AffCode: "kyren-concurrent-user"}).Error)
	snapshot, err := model.MarshalKyrenTopUpSnapshot(model.KyrenTopUpSnapshot{
		LocalTopUpID: "concurrent-topup", ProductID: "prod_concurrent_topup",
		Amount: "50.00", Currency: "CNY", Quota: 5000,
	})
	require.NoError(t, err)
	order := &model.TopUp{
		UserId: 9701, Amount: 5000, AmountUnit: model.TopUpAmountUnitAccountBalanceCents,
		Money: 50, TradeNo: "kyren-concurrent-paid-failed", PaymentMethod: model.PaymentMethodKyren,
		PaymentProvider: model.PaymentProviderKyren, CreateTime: common.GetTimestamp(),
		Status: common.TopUpStatusPending, KyrenSnapshot: snapshot,
	}
	require.NoError(t, CreateKyrenTopUpPaymentOrder(order))

	paid := KyrenPaymentEventRequest{
		EventID: "evt_concurrent_paid", EventType: KyrenPaymentEventPaid,
		PayloadHash: strings.Repeat("a", 64), TradeNo: order.TradeNo,
		OrderKind: model.PaymentOrderKindTopUp, ProviderOrderID: "order_concurrent_paid_failed",
		ProductID: "prod_concurrent_topup", Amount: "50.00", Currency: "CNY",
		ProviderPayload: `{"event_type":"order.paid"}`,
	}
	failed := KyrenPaymentEventRequest{
		EventID: "evt_concurrent_failed", EventType: KyrenPaymentEventFailed,
		PayloadHash: strings.Repeat("b", 64), TradeNo: order.TradeNo,
		OrderKind: model.PaymentOrderKindTopUp, ProviderOrderID: "order_concurrent_paid_failed",
		ProductID: "prod_concurrent_topup", Amount: "50.00", Currency: "CNY",
		ProviderPayload: `{"event_type":"order.failed"}`,
	}

	start := make(chan struct{})
	errorsByType := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for _, request := range []KyrenPaymentEventRequest{paid, failed} {
		waitGroup.Add(1)
		go func(request KyrenPaymentEventRequest) {
			defer waitGroup.Done()
			<-start
			_, processErr := ProcessKyrenPaymentEvent(request)
			errorsByType <- processErr
		}(request)
	}
	close(start)
	waitGroup.Wait()
	close(errorsByType)

	successfulAttempts := 0
	for processErr := range errorsByType {
		if processErr == nil {
			successfulAttempts++
		}
	}
	assert.GreaterOrEqual(t, successfulAttempts, 1)

	_, err = ProcessKyrenPaymentEvent(paid)
	require.NoError(t, err)
	_, err = ProcessKyrenPaymentEvent(failed)
	require.NoError(t, err)

	var persisted model.TopUp
	require.NoError(t, db.First(&persisted, order.Id).Error)
	assert.Contains(t, []string{common.TopUpStatusSuccess, common.TopUpStatusFailed}, persisted.Status)
	var user model.User
	require.NoError(t, db.First(&user, order.UserId).Error)
	if persisted.Status == common.TopUpStatusSuccess {
		assert.Equal(t, 5000, user.Quota)
	} else {
		assert.Zero(t, user.Quota)
	}

	var events []model.PaymentProviderEvent
	require.NoError(t, db.Where("trade_no = ?", order.TradeNo).Order("event_id").Find(&events).Error)
	require.Len(t, events, 2)
	applied := 0
	conflicted := 0
	for _, event := range events {
		assert.Empty(t, event.ProcessingToken)
		assert.Zero(t, event.ProcessingStartedAt)
		switch event.Status {
		case model.PaymentProviderEventApplied:
			applied++
		case model.PaymentProviderEventConflict:
			conflicted++
		default:
			t.Fatalf("unexpected event status %q", event.Status)
		}
	}
	assert.Equal(t, 1, applied)
	assert.Equal(t, 1, conflicted)
}

func TestClaimKyrenSubscriptionPaymentOrderReusesSinglePendingCheckout(t *testing.T) {
	db := setupKyrenPaymentServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionOrder{}, &model.SubscriptionPlan{}))
	userID := 9711
	planID := 9712
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "kyren-claim-user", Status: common.UserStatusEnabled, AffCode: "kyren-claim-user"}).Error)
	snapshot, err := model.MarshalSubscriptionEntitlementSnapshot(model.SubscriptionEntitlementSnapshot{
		PurchaseMode: model.SubscriptionPurchaseModeTimed, PlanID: planID,
	})
	require.NoError(t, err)
	paymentSnapshot, err := model.MarshalKyrenPaymentSnapshot(model.KyrenPaymentSnapshot{ProductID: "prod_claim", Amount: "40.00", Currency: "CNY"})
	require.NoError(t, err)
	newOrder := func(tradeNo string) *model.SubscriptionOrder {
		return &model.SubscriptionOrder{
			UserId: userID, PlanId: planID, TradeNo: tradeNo,
			PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
			Status: common.TopUpStatusPending, KyrenSnapshot: paymentSnapshot, EntitlementSnapshot: snapshot,
		}
	}

	first, err := ClaimKyrenSubscriptionPaymentOrder(newOrder("kyren-claim-first"), model.SubscriptionPurchaseModeTimed)
	require.NoError(t, err)
	require.True(t, first.Created)
	require.NoError(t, BindKyrenPaymentCheckout(model.PaymentOrderKindSubscription, first.Order.TradeNo, "cs_claim_reuse"))
	second, err := ClaimKyrenSubscriptionPaymentOrder(newOrder("kyren-claim-second"), model.SubscriptionPurchaseModeTimed)
	require.NoError(t, err)

	assert.False(t, second.Created)
	assert.False(t, second.InProgress)
	assert.Equal(t, first.Order.Id, second.Order.Id)
	assert.Equal(t, "cs_claim_reuse", second.CheckoutID)
	var count int64
	require.NoError(t, db.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ? AND status = ?", userID, planID, common.TopUpStatusPending).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestConcurrentKyrenSubscriptionClaimsCreateOnePendingOrder(t *testing.T) {
	db := setupKyrenPaymentServiceTestDB(t)
	userID := 9721
	planID := 9722
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "kyren-concurrent-claim", Status: common.UserStatusEnabled, AffCode: "kyren-concurrent-claim"}).Error)
	snapshot, err := model.MarshalSubscriptionEntitlementSnapshot(model.SubscriptionEntitlementSnapshot{
		PurchaseMode: model.SubscriptionPurchaseModeTimed, PlanID: planID,
	})
	require.NoError(t, err)
	paymentSnapshot, err := model.MarshalKyrenPaymentSnapshot(model.KyrenPaymentSnapshot{ProductID: "prod_concurrent_claim", Amount: "40.00", Currency: "CNY"})
	require.NoError(t, err)
	start := make(chan struct{})
	claims := make(chan *KyrenSubscriptionPaymentOrderClaim, 2)
	errorsByClaim := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for index := range 2 {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			<-start
			claim, claimErr := ClaimKyrenSubscriptionPaymentOrder(&model.SubscriptionOrder{
				UserId: userID, PlanId: planID, TradeNo: fmt.Sprintf("kyren-concurrent-claim-%d", index),
				PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
				Status: common.TopUpStatusPending, KyrenSnapshot: paymentSnapshot, EntitlementSnapshot: snapshot,
			}, model.SubscriptionPurchaseModeTimed)
			claims <- claim
			errorsByClaim <- claimErr
		}(index)
	}
	close(start)
	waitGroup.Wait()
	close(claims)
	close(errorsByClaim)

	for claimErr := range errorsByClaim {
		require.NoError(t, claimErr)
	}
	created := 0
	inProgress := 0
	for claim := range claims {
		require.NotNil(t, claim)
		if claim.Created {
			created++
		}
		if claim.InProgress {
			inProgress++
		}
	}
	assert.Equal(t, 1, created)
	assert.Equal(t, 1, inProgress)
	var count int64
	require.NoError(t, db.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ? AND status = ?", userID, planID, common.TopUpStatusPending).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestConcurrentKyrenSubscriptionRetriesCreateOneSuccessor(t *testing.T) {
	db := setupKyrenPaymentServiceTestDB(t)
	userID := 9731
	planID := 9732
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "kyren-concurrent-retry", Status: common.UserStatusEnabled, AffCode: "kyren-concurrent-retry"}).Error)
	entitlementSnapshot, err := model.MarshalSubscriptionEntitlementSnapshot(model.SubscriptionEntitlementSnapshot{
		PurchaseMode: model.SubscriptionPurchaseModeTimed, PlanID: planID,
	})
	require.NoError(t, err)
	paymentSnapshot, err := model.MarshalKyrenPaymentSnapshot(model.KyrenPaymentSnapshot{ProductID: "prod_concurrent_retry", Amount: "40.00", Currency: "CNY"})
	require.NoError(t, err)
	newOrder := func(tradeNo string) *model.SubscriptionOrder {
		return &model.SubscriptionOrder{
			UserId: userID, PlanId: planID, Money: 40, AmountCents: 4000, Currency: "CNY",
			TradeNo: tradeNo, PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
			Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
			KyrenSnapshot: paymentSnapshot, EntitlementSnapshot: entitlementSnapshot,
		}
	}
	predecessor := newOrder("kyren-concurrent-retry-predecessor")
	require.NoError(t, db.Create(predecessor).Error)

	start := make(chan struct{})
	claims := make(chan *KyrenSubscriptionPaymentOrderClaim, 2)
	errorsByRetry := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for index := range 2 {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			<-start
			claim, retryErr := SupersedeKyrenSubscriptionPaymentOrder(KyrenSubscriptionPaymentOrderRetryRequest{
				PredecessorTradeNo: predecessor.TradeNo,
				Successor:          newOrder(fmt.Sprintf("kyren-concurrent-retry-successor-%d", index)),
				PurchaseMode:       model.SubscriptionPurchaseModeTimed,
			})
			claims <- claim
			errorsByRetry <- retryErr
		}(index)
	}
	close(start)
	waitGroup.Wait()
	close(claims)
	close(errorsByRetry)

	for retryErr := range errorsByRetry {
		require.NoError(t, retryErr)
	}
	created := 0
	inProgress := 0
	successorID := 0
	for claim := range claims {
		require.NotNil(t, claim)
		require.NotNil(t, claim.Order)
		if successorID == 0 {
			successorID = claim.Order.Id
		}
		assert.Equal(t, successorID, claim.Order.Id)
		if claim.Created {
			created++
		}
		if claim.InProgress {
			inProgress++
		}
	}
	assert.Equal(t, 1, created)
	assert.Equal(t, 1, inProgress)

	var persistedPredecessor model.SubscriptionOrder
	require.NoError(t, db.First(&persistedPredecessor, predecessor.Id).Error)
	assert.Equal(t, common.TopUpStatusExpired, persistedPredecessor.Status)
	assert.Equal(t, successorID, persistedPredecessor.SupersededByOrderID)
	var pendingCount, totalCount int64
	require.NoError(t, db.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ? AND status = ?", userID, planID, common.TopUpStatusPending).Count(&pendingCount).Error)
	require.NoError(t, db.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", userID, planID).Count(&totalCount).Error)
	assert.Equal(t, int64(1), pendingCount)
	assert.Equal(t, int64(2), totalCount)
}

func TestKyrenSubscriptionRetryRejectsMismatchedPaymentIdentity(t *testing.T) {
	db := setupKyrenPaymentServiceTestDB(t)
	userID := 9741
	planID := 9742
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "kyren-retry-identity", Status: common.UserStatusEnabled, AffCode: "kyren-retry-identity"}).Error)
	entitlementSnapshot, err := model.MarshalSubscriptionEntitlementSnapshot(model.SubscriptionEntitlementSnapshot{
		PurchaseMode: model.SubscriptionPurchaseModeTimed, PlanID: planID,
	})
	require.NoError(t, err)
	originalPayment, err := model.MarshalKyrenPaymentSnapshot(model.KyrenPaymentSnapshot{ProductID: "prod_retry_identity", Amount: "40.00", Currency: "CNY"})
	require.NoError(t, err)
	differentPayment, err := model.MarshalKyrenPaymentSnapshot(model.KyrenPaymentSnapshot{ProductID: "prod_retry_identity", Amount: "41.00", Currency: "CNY"})
	require.NoError(t, err)
	predecessor := &model.SubscriptionOrder{
		UserId: userID, PlanId: planID, TradeNo: "kyren-retry-identity-predecessor",
		PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
		Status: common.TopUpStatusPending, KyrenSnapshot: originalPayment, EntitlementSnapshot: entitlementSnapshot,
	}
	require.NoError(t, db.Create(predecessor).Error)

	claim, err := SupersedeKyrenSubscriptionPaymentOrder(KyrenSubscriptionPaymentOrderRetryRequest{
		PredecessorTradeNo: predecessor.TradeNo,
		Successor: &model.SubscriptionOrder{
			UserId: userID, PlanId: planID, TradeNo: "kyren-retry-identity-successor",
			PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
			Status: common.TopUpStatusPending, KyrenSnapshot: differentPayment, EntitlementSnapshot: entitlementSnapshot,
		},
		PurchaseMode: model.SubscriptionPurchaseModeTimed,
	})

	require.ErrorIs(t, err, ErrKyrenSubscriptionPaymentIdentityConflict)
	assert.Nil(t, claim)
	var persisted model.SubscriptionOrder
	require.NoError(t, db.First(&persisted, predecessor.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, persisted.Status)
	assert.Zero(t, persisted.SupersededByOrderID)
	var count int64
	require.NoError(t, db.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", userID, planID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func setupKyrenPaymentServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "kyren-payment.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.TopUp{}, &model.Log{},
		&model.SubscriptionOrder{}, &model.SubscriptionPlan{},
		&model.PaymentProviderOrder{}, &model.PaymentProviderCreationLock{}, &model.PaymentProviderEvent{},
	))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		_ = sqlDB.Close()
	})
	return db
}
