package model

import (
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestKyrenPaymentSnapshotRoundTrip(t *testing.T) {
	snapshot := KyrenPaymentSnapshot{ProductID: "prod_sub", Amount: "40.00", Currency: "CNY"}

	payload, err := MarshalKyrenPaymentSnapshot(snapshot)
	require.NoError(t, err)

	got, err := UnmarshalKyrenPaymentSnapshot(payload)
	require.NoError(t, err)

	assert.Equal(t, snapshot.ProductID, got.ProductID)
	assert.Equal(t, snapshot.Amount, got.Amount)
	assert.Equal(t, snapshot.Currency, got.Currency)
}

func TestKyrenSubscriptionEntitlementSnapshotRoundTrip(t *testing.T) {
	businessCode := "kyren_monthly"
	plan := SubscriptionPlan{
		Id:                      1001,
		TotalAmount:             100000,
		MonthlyTokenLimit:       2000,
		ConcurrencyLimit:        3,
		QueueCapacity:           9,
		DurationUnit:            SubscriptionDurationMonth,
		DurationValue:           1,
		QuotaResetPeriod:        SubscriptionResetMonthly,
		MaxPurchasePerUser:      2,
		BusinessCode:            &businessCode,
		IsTrial:                 true,
		InviteTrial:             true,
		RewardEligible:          true,
		CustomSeconds:           0,
		QuotaResetCustomSeconds: 0,
	}

	snapshot := NewSubscriptionEntitlementSnapshotFromPlan(&plan)
	payload, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)

	got, err := UnmarshalSubscriptionEntitlementSnapshot(payload)
	require.NoError(t, err)

	assert.Equal(t, plan.Id, got.PlanID)
	assert.Equal(t, plan.TotalAmount, got.TotalAmount)
	assert.Equal(t, plan.MonthlyTokenLimit, got.MonthlyTokenLimit)
	assert.Equal(t, plan.ConcurrencyLimit, got.ConcurrencyLimit)
	assert.Equal(t, plan.QueueCapacity, got.QueueCapacity)
	assert.Equal(t, plan.DurationUnit, got.DurationUnit)
	assert.Equal(t, plan.DurationValue, got.DurationValue)
	assert.Equal(t, plan.CustomSeconds, got.CustomSeconds)
	assert.Equal(t, plan.QuotaResetPeriod, got.QuotaResetPeriod)
	assert.Equal(t, plan.QuotaResetCustomSeconds, got.QuotaResetCustomSeconds)
	assert.Equal(t, plan.MaxPurchasePerUser, got.MaxPurchasePerUser)
	assert.Equal(t, businessCode, got.BusinessCode)
	assert.Equal(t, plan.IsTrial, got.IsTrial)
	assert.Equal(t, plan.InviteTrial, got.InviteTrial)
	assert.Equal(t, plan.RewardEligible, got.RewardEligible)
}

func TestKyrenPaymentConstants(t *testing.T) {
	assert.Equal(t, "kyren", PaymentProviderKyren)
	assert.Equal(t, "kyren", PaymentMethodKyren)
}

func TestClaimPaymentProviderEventOnlyOneFreshClaim(t *testing.T) {
	truncateTables(t)
	request := PaymentProviderEventClaimRequest{
		Provider: PaymentProviderKyren, EventID: "evt_single_fresh_claim", EventType: "order.paid",
		PayloadHash: strings.Repeat("a", 64), TradeNo: "trade_single_fresh_claim",
		OrderKind: PaymentOrderKindSubscription, ProviderOrderID: "order_single_fresh_claim",
		StaleBefore: common.GetTimestamp() - 300,
	}
	var firstOutcome PaymentProviderEventClaimOutcome
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		_, firstOutcome, _ = ClaimPaymentProviderEventTx(tx, request)
		return nil
	}))
	assert.Equal(t, PaymentProviderEventClaimed, firstOutcome)
	var secondOutcome PaymentProviderEventClaimOutcome
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		_, secondOutcome, _ = ClaimPaymentProviderEventTx(tx, request)
		return nil
	}))
	assert.Equal(t, PaymentProviderEventInProgress, secondOutcome)
}

func TestClaimPaymentProviderEventRecoversOnlyStaleToken(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	event := PaymentProviderEvent{
		Provider: PaymentProviderKyren, EventID: "evt_stale_token_owner", EventType: "order.paid",
		PayloadHash: strings.Repeat("a", 64), TradeNo: "trade_stale_token_owner",
		OrderKind: PaymentOrderKindTopUp, ProviderOrderID: "order_stale_token_owner",
		Status: PaymentProviderEventProcessing, ProcessingToken: "old-event-token",
		ProcessingStartedAt: now - 600, CreatedAt: now - 600, UpdatedAt: now - 600,
	}
	require.NoError(t, DB.Create(&event).Error)
	request := PaymentProviderEventClaimRequest{
		Provider: event.Provider, EventID: event.EventID, EventType: event.EventType,
		PayloadHash: event.PayloadHash, TradeNo: event.TradeNo, OrderKind: event.OrderKind,
		ProviderOrderID: event.ProviderOrderID, StaleBefore: now - 300,
	}
	var claimed *PaymentProviderEvent
	var outcome PaymentProviderEventClaimOutcome
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		claimed, outcome, err = ClaimPaymentProviderEventTx(tx, request)
		return err
	}))
	assert.Equal(t, PaymentProviderEventClaimed, outcome)
	require.NotNil(t, claimed)
	assert.NotEmpty(t, claimed.ProcessingToken)
	assert.NotEqual(t, "old-event-token", claimed.ProcessingToken)
}

func TestClaimPaymentProviderEventConflictPreservesRefundProcessingLease(t *testing.T) {
	truncateTables(t)
	request := PaymentProviderEventClaimRequest{
		Provider:        PaymentProviderKyren,
		EventID:         "evt_refund_processing_conflict",
		EventType:       "order.refunded",
		PayloadHash:     strings.Repeat("a", 64),
		TradeNo:         "trade_refund_processing",
		OrderKind:       PaymentOrderKindSubscription,
		ProviderOrderID: "order_refund_processing",
		StaleBefore:     common.GetTimestamp() - 300,
	}

	var claimed *PaymentProviderEvent
	var outcome PaymentProviderEventClaimOutcome
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		claimed, outcome, err = ClaimPaymentProviderEventTx(tx, request)
		return err
	}))
	require.Equal(t, PaymentProviderEventClaimed, outcome)
	require.NotNil(t, claimed)
	originalToken := claimed.ProcessingToken
	originalStartedAt := claimed.ProcessingStartedAt
	require.NotEmpty(t, originalToken)

	conflicting := request
	conflicting.PayloadHash = strings.Repeat("b", 64)
	conflicting.TradeNo = "trade_refund_conflicting"
	var conflictEvent *PaymentProviderEvent
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		conflictEvent, outcome, err = ClaimPaymentProviderEventTx(tx, conflicting)
		return err
	}))
	require.Equal(t, PaymentProviderEventConflicted, outcome)
	require.NotNil(t, conflictEvent)
	assert.Equal(t, PaymentProviderEventProcessing, conflictEvent.Status)
	assert.Equal(t, originalToken, conflictEvent.ProcessingToken)
	assert.Equal(t, originalStartedAt, conflictEvent.ProcessingStartedAt)
	assert.Equal(t, 1, conflictEvent.ConflictCount)
	assert.Equal(t, conflicting.PayloadHash, conflictEvent.ConflictPayloadHash)
	assert.Equal(t, conflicting.TradeNo, conflictEvent.ConflictTradeNo)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return FinishPaymentProviderEventTx(tx, claimed, PaymentProviderEventApplied, "subscription refund recovered", "")
	}))
	var persisted PaymentProviderEvent
	require.NoError(t, DB.Where("provider = ? AND event_id = ?", request.Provider, request.EventID).First(&persisted).Error)
	assert.Equal(t, PaymentProviderEventApplied, persisted.Status)
	assert.Empty(t, persisted.ProcessingToken)
	assert.Zero(t, persisted.ProcessingStartedAt)
	assert.Positive(t, persisted.ProcessedAt)
	assert.Equal(t, "subscription refund recovered", persisted.OutcomeReason)
	assert.Equal(t, 1, persisted.ConflictCount)
}

func TestClaimPaymentProviderEventConflictPreservesAppliedTerminalState(t *testing.T) {
	truncateTables(t)
	request := PaymentProviderEventClaimRequest{
		Provider:        PaymentProviderKyren,
		EventID:         "evt_applied_conflict",
		EventType:       "order.paid",
		PayloadHash:     strings.Repeat("a", 64),
		TradeNo:         "trade_applied",
		OrderKind:       PaymentOrderKindTopUp,
		ProviderOrderID: "order_applied",
		StaleBefore:     common.GetTimestamp() - 300,
	}
	var event *PaymentProviderEvent
	var outcome PaymentProviderEventClaimOutcome
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		event, outcome, err = ClaimPaymentProviderEventTx(tx, request)
		return err
	}))
	require.Equal(t, PaymentProviderEventClaimed, outcome)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return FinishPaymentProviderEventTx(tx, event, PaymentProviderEventApplied, "top-up payment applied", "")
	}))

	conflicting := request
	conflicting.PayloadHash = strings.Repeat("b", 64)
	conflicting.TradeNo = "trade_conflicting"
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		event, outcome, err = ClaimPaymentProviderEventTx(tx, conflicting)
		return err
	}))
	require.Equal(t, PaymentProviderEventConflicted, outcome)
	require.NotNil(t, event)
	assert.Equal(t, PaymentProviderEventApplied, event.Status)
	assert.Empty(t, event.ProcessingToken)
	assert.Equal(t, "top-up payment applied", event.OutcomeReason)
	assert.Equal(t, 1, event.ConflictCount)

	var persisted PaymentProviderEvent
	require.NoError(t, DB.Where("provider = ? AND event_id = ?", request.Provider, request.EventID).First(&persisted).Error)
	assert.Equal(t, PaymentProviderEventApplied, persisted.Status)
	assert.Equal(t, "top-up payment applied", persisted.OutcomeReason)
	assert.Equal(t, 1, persisted.ConflictCount)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		event, outcome, err = ClaimPaymentProviderEventTx(tx, request)
		return err
	}))
	assert.Equal(t, PaymentProviderEventDuplicate, outcome)
	assert.Equal(t, PaymentProviderEventApplied, event.Status)
}

func TestPaymentProviderSchemaEnforcesIdentityIndexesSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PaymentProviderOrder{}, &PaymentProviderEvent{}))

	for table, indexes := range map[any][]string{
		&PaymentProviderOrder{}: {"idx_provider_trade", "idx_provider_order_id", "idx_provider_checkout_id", "idx_provider_local_order"},
		&PaymentProviderEvent{}: {"idx_provider_event"},
	} {
		for _, index := range indexes {
			require.True(t, db.Migrator().HasIndex(table, index), "missing index %s", index)
		}
	}

	providerOrderID := "order_schema_identity"
	checkoutID := "checkout_schema_identity"
	base := PaymentProviderOrder{
		Provider: modelPaymentProviderTestName, OrderKind: PaymentOrderKindTopUp,
		LocalOrderID: 1, TradeNo: "trade_schema_identity", UserID: 1,
		ProviderOrderID: &providerOrderID, ProviderCheckoutID: &checkoutID,
		CreatedAt: 1, UpdatedAt: 1,
	}
	require.NoError(t, db.Create(&base).Error)

	otherOrderID := "order_schema_other"
	otherCheckoutID := "checkout_schema_other"
	duplicates := []PaymentProviderOrder{
		{Provider: modelPaymentProviderTestName, OrderKind: PaymentOrderKindTopUp, LocalOrderID: 2, TradeNo: base.TradeNo, UserID: 2, ProviderOrderID: &otherOrderID, ProviderCheckoutID: &otherCheckoutID, CreatedAt: 1, UpdatedAt: 1},
		{Provider: modelPaymentProviderTestName, OrderKind: PaymentOrderKindTopUp, LocalOrderID: base.LocalOrderID, TradeNo: "trade_schema_local", UserID: 2, CreatedAt: 1, UpdatedAt: 1},
		{Provider: modelPaymentProviderTestName, OrderKind: PaymentOrderKindTopUp, LocalOrderID: 3, TradeNo: "trade_schema_order", UserID: 3, ProviderOrderID: &providerOrderID, CreatedAt: 1, UpdatedAt: 1},
		{Provider: modelPaymentProviderTestName, OrderKind: PaymentOrderKindTopUp, LocalOrderID: 4, TradeNo: "trade_schema_checkout", UserID: 4, ProviderCheckoutID: &checkoutID, CreatedAt: 1, UpdatedAt: 1},
	}
	for i := range duplicates {
		require.Error(t, db.Create(&duplicates[i]).Error)
	}
	require.NoError(t, db.Create(&PaymentProviderOrder{Provider: modelPaymentProviderTestName, OrderKind: PaymentOrderKindTopUp, LocalOrderID: 5, TradeNo: "trade_schema_null_a", UserID: 5, CreatedAt: 1, UpdatedAt: 1}).Error)
	require.NoError(t, db.Create(&PaymentProviderOrder{Provider: modelPaymentProviderTestName, OrderKind: PaymentOrderKindTopUp, LocalOrderID: 6, TradeNo: "trade_schema_null_b", UserID: 6, CreatedAt: 1, UpdatedAt: 1}).Error)

	event := PaymentProviderEvent{Provider: modelPaymentProviderTestName, EventID: "event_schema_identity", EventType: "order.failed", PayloadHash: strings.Repeat("a", 64), Status: PaymentProviderEventProcessing, CreatedAt: 1, UpdatedAt: 1}
	require.NoError(t, db.Create(&event).Error)
	event.ID = 0
	require.Error(t, db.Create(&event).Error)
}

func TestPaymentProviderIdentityIndexesGenerateForMySQLAndPostgres(t *testing.T) {
	baseDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	baseStatement := baseDB.Session(&gorm.Session{DryRun: true}).Find(&[]PaymentProviderOrder{}).Statement
	tests := []struct {
		name      string
		dialector gorm.Dialector
	}{
		{name: "mysql", dialector: mysql.New(mysql.Config{Conn: baseStatement.ConnPool, SkipInitializeWithVersion: true})},
		{name: "postgres", dialector: postgres.New(postgres.Config{DSN: "host=localhost user=test dbname=test sslmode=disable", PreferSimpleProtocol: true, Conn: baseStatement.ConnPool})},
	}
	expected := map[string][]string{
		"idx_provider_trade":       {"provider", "trade_no"},
		"idx_provider_order_id":    {"provider", "provider_order_id"},
		"idx_provider_checkout_id": {"provider", "provider_checkout_id"},
		"idx_provider_local_order": {"provider", "order_kind", "local_order_id"},
		"idx_provider_event":       {"provider", "event_id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := &sqlCaptureLogger{Interface: logger.Default.LogMode(logger.Silent)}
			db, openErr := gorm.Open(test.dialector, &gorm.Config{DryRun: true, Logger: capture, DisableForeignKeyConstraintWhenMigrating: true})
			require.NoError(t, openErr)
			require.NoError(t, db.Migrator().CreateTable(&PaymentProviderOrder{}, &PaymentProviderEvent{}))
			for indexName, columns := range expected {
				pattern := regexp.MustCompile(`(?i)unique\s+(?:index\s+)?(?:if\s+not\s+exists\s+)?["` + "`" + `]?` + regexp.QuoteMeta(indexName) + `["` + "`" + `]?\s+(?:on\s+["` + "`" + `]?[^"` + "`" + `\s]+["` + "`" + `]?\s*)?\(([^)]*)\)`)
				matched := false
				for _, statement := range capture.statements {
					parts := pattern.FindStringSubmatch(statement)
					if len(parts) != 2 {
						continue
					}
					definition := strings.ToLower(parts[1])
					matched = true
					for _, column := range columns {
						matched = matched && strings.Contains(definition, column)
					}
					if matched {
						break
					}
				}
				assert.True(t, matched, "expected UNIQUE index %s on %v; statements=%#v", indexName, columns, capture.statements)
			}
		})
	}
}

const modelPaymentProviderTestName = "kyren"
