package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCreditHistoricalBackfillDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&UserSubscription{}, &CreditValuationState{}, &CreditBalanceLedger{}, &CreditValuationMigration{}))
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func creditHistoricalRequest(apply bool) CreditValuationHistoricalBackfillRequest {
	return CreditValuationHistoricalBackfillRequest{
		Apply: apply, MigrationVersion: 1, BatchSize: 10, ValuationCurrency: "CNY",
		FX: CreditValuationMigrationFXSnapshot{SourceCurrency: "USD", ValuationCurrency: "CNY", Numerator: 7, Denominator: 1, CapturedAt: 1_800_000_000},
	}
}

func TestCreditHistoricalBackfillEstimatesRemainingValueAndDryRunDoesNotWrite(t *testing.T) {
	db := setupCreditHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 101, UserId: 201, PlanId: 301, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 1000, TokenUsed: 200}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Create(&CreditBalanceLedger{
		Id: 401, UserId: sub.UserId, UserSubscriptionId: sub.Id, Type: CreditBalanceLedgerTypePurchase,
		IdempotencyKey: "order-401", SourceType: CreditBalanceLedgerSourceSubscriptionOrder, SourceId: 401,
		GrossCredit: 1000, NetCredit: 1000, SourcePriceMicros: 40_000_000, SourcePlanCredit: 1000,
		FxSourceCurrency: "CNY", FxRateNumerator: 1, FxRateDenominator: 1,
	}).Error)

	report, err := RunCreditValuationHistoricalBackfill(db, creditHistoricalRequest(false))
	require.NoError(t, err)
	require.Equal(t, int64(1), report.RowsEstimated)
	require.Equal(t, int64(32_000_000), report.EstimatedCostMicros)
	require.Zero(t, report.UnknownCredit)
	var count int64
	require.NoError(t, db.Model(&CreditValuationState{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCreditHistoricalBackfillMissingSourceAndApplyPreservesExisting(t *testing.T) {
	db := setupCreditHistoricalBackfillDB(t)
	missing := UserSubscription{Id: 102, UserId: 202, PlanId: 302, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 1000, TokenUsed: 200}
	existingSub := UserSubscription{Id: 103, UserId: 203, PlanId: 302, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 500, TokenUsed: 100}
	require.NoError(t, db.Create(&missing).Error)
	require.NoError(t, db.Create(&existingSub).Error)
	existing := CreditValuationState{UserSubscriptionId: existingSub.Id, UserId: existingSub.UserId, AvailableCredit: 400, ExactCostMicros: 9, Currency: "CNY", RuleVersion: CreditValuationRuleVersion, StateVersion: 7, CreatedAt: 1, UpdatedAt: 1}
	require.NoError(t, db.Create(&existing).Error)

	report, err := RunCreditValuationHistoricalBackfill(db, creditHistoricalRequest(true))
	require.NoError(t, err)
	require.Equal(t, int64(800), report.UnknownCredit)
	require.Equal(t, int64(1), report.RowsSkippedExisting)
	var gotMissing CreditValuationState
	require.NoError(t, db.First(&gotMissing, "user_subscription_id = ?", missing.Id).Error)
	require.Equal(t, int64(800), gotMissing.UnknownCredit)
	require.Zero(t, gotMissing.ExactCostMicros)
	var gotExisting CreditValuationState
	require.NoError(t, db.First(&gotExisting, "user_subscription_id = ?", existingSub.Id).Error)
	require.Equal(t, int64(7), gotExisting.StateVersion)
	require.Equal(t, int64(9), gotExisting.ExactCostMicros)
}

func TestCreditHistoricalBackfillRepairRequiresApplyAndHigherVersion(t *testing.T) {
	db := setupCreditHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 104, UserId: 204, PlanId: 304, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 900, TokenUsed: 100}
	require.NoError(t, db.Create(&sub).Error)
	request := creditHistoricalRequest(false)
	request.RepairMissingAsUnknown = true
	_, err := RunCreditValuationHistoricalBackfill(db, request)
	require.ErrorIs(t, err, ErrCreditValuationMigrationRepairInvalid)

	require.NoError(t, db.Create(&CreditValuationMigration{Version: 1, Status: CreditValuationMigrationReady}).Error)
	request.Apply = true
	request.MigrationVersion = 2
	report, err := RunCreditValuationHistoricalBackfill(db, request)
	require.NoError(t, err)
	require.Equal(t, int64(800), report.UnknownCredit)
	var state CreditValuationState
	require.NoError(t, db.First(&state, "user_subscription_id = ?", sub.Id).Error)
	require.Equal(t, 2, state.MigrationVersion)
	require.Equal(t, int64(800), state.UnknownCredit)
	require.Equal(t, "repair_missing_as_unknown", state.LastMutationType)
}

func TestCreditHistoricalBackfillDeduplicatesStableSourceIdentity(t *testing.T) {
	db := setupCreditHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 105, UserId: 205, PlanId: 305, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 1000, TokenUsed: 200}
	require.NoError(t, db.Create(&sub).Error)
	for id := 1; id <= 2; id++ {
		require.NoError(t, db.Create(&CreditBalanceLedger{Id: 500 + id, UserId: sub.UserId, UserSubscriptionId: sub.Id, Type: CreditBalanceLedgerTypePurchase, IdempotencyKey: "dup-" + string(rune('0'+id)), SourceType: CreditBalanceLedgerSourceSubscriptionOrder, SourceId: 501 + id, SourceKey: "order:501", GrossCredit: 1000, NetCredit: 1000, SourcePriceMicros: 40_000_000, SourcePlanCredit: 1000, FxSourceCurrency: "CNY", FxRateNumerator: 1, FxRateDenominator: 1}).Error)
	}
	report, err := RunCreditValuationHistoricalBackfill(db, creditHistoricalRequest(false))
	require.NoError(t, err)
	require.Equal(t, int64(32_000_000), report.EstimatedCostMicros)
	require.Zero(t, report.AmbiguousRows)
}

func TestCreditHistoricalBackfillAllocatesKnownAndUnknownSourcesAndIgnoresOutflows(t *testing.T) {
	db := setupCreditHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 106, UserId: 206, PlanId: 306, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 1000, TokenUsed: 200}
	require.NoError(t, db.Create(&sub).Error)
	ledgers := []CreditBalanceLedger{
		{Id: 601, UserId: sub.UserId, UserSubscriptionId: sub.Id, Type: CreditBalanceLedgerTypePurchase, IdempotencyKey: "known-601", SourceType: CreditBalanceLedgerSourceSubscriptionOrder, SourceId: 601, SourceKey: "order:601", GrossCredit: 1000, NetCredit: 1000, SourcePriceMicros: 40_000_000, SourcePlanCredit: 1000, FxSourceCurrency: "CNY"},
		{Id: 602, UserId: sub.UserId, UserSubscriptionId: sub.Id, Type: CreditBalanceLedgerTypeRedemption, IdempotencyKey: "unknown-602", SourceType: CreditBalanceLedgerSourceRedemption, SourceId: 602, SourceKey: "redemption:602", GrossCredit: 1000, NetCredit: 1000, SourcePlanCredit: 1000, FxSourceCurrency: "EUR"},
		{Id: 603, UserId: sub.UserId, UserSubscriptionId: sub.Id, Type: CreditBalanceLedgerTypeAdminDecrease, IdempotencyKey: "outflow-603", SourceType: CreditBalanceLedgerSourceAdminAdjustment, SourceId: 603, SourceKey: "admin_adjustment:603", GrossCredit: -200, NetCredit: -200},
	}
	require.NoError(t, db.Create(&ledgers).Error)

	report, err := RunCreditValuationHistoricalBackfill(db, creditHistoricalRequest(false))
	require.NoError(t, err)
	require.Equal(t, int64(16_000_000), report.EstimatedCostMicros)
	require.Equal(t, int64(400), report.UnknownCredit)
	require.Equal(t, int64(1), report.RowsEstimated)
	require.Equal(t, int64(1), report.RowsUnknown)
}

func TestCreditHistoricalBackfillReplayReportsPersistedMigrationState(t *testing.T) {
	db := setupCreditHistoricalBackfillDB(t)
	sub := UserSubscription{Id: 107, UserId: 207, PlanId: 307, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 1_000, TokenUsed: 200}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Create(&CreditValuationState{
		UserSubscriptionId: sub.Id, UserId: sub.UserId, AvailableCredit: 800,
		EstimatedCostMicros: 16_000_000, UnknownCredit: 200, Currency: "CNY",
		RuleVersion: CreditValuationRuleVersion, MigrationVersion: 2, LastMutationType: "historical_backfill",
	}).Error)

	report, err := RunCreditValuationHistoricalBackfill(db, CreditValuationHistoricalBackfillRequest{
		MigrationVersion: 2, BatchSize: 10, ValuationCurrency: "CNY",
		FX: CreditValuationMigrationFXSnapshot{SourceCurrency: "USD", ValuationCurrency: "CNY", Numerator: 7, Denominator: 1, CapturedAt: 1_800_000_000},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), report.RowsSkippedExisting)
	require.Equal(t, int64(1), report.RowsEstimated)
	require.Equal(t, int64(1), report.RowsUnknown)
	require.Equal(t, int64(16_000_000), report.EstimatedCostMicros)
	require.Equal(t, int64(200), report.UnknownCredit)
}
