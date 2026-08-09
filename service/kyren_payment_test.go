package service

import (
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
		&model.PaymentProviderOrder{}, &model.PaymentProviderEvent{},
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
