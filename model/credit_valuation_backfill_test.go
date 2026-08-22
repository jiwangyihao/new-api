package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCreditHistoricalBackfillDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &SubscriptionConversion{},
		&Redemption{}, &CreditBalanceAdjustment{}, &CreditValuationState{}, &CreditBalanceLedger{}, &CreditValuationMigration{},
	))
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

func TestCreditHistoricalBackfillRecoversOrderAndConversionSourcesWithoutCurrentPlanPrice(t *testing.T) {
	db := setupCreditHistoricalBackfillDB(t)
	const (
		optionPlanID       = 301
		creditPlanID       = 302
		targetID           = 303
		sourceID           = 304
		orderID            = 305
		purchaseLedgerID   = 306
		conversionLedgerID = 307
		conversionID       = 308
	)
	snapshotPrice := int64(40_000_000)
	currentPrice := int64(99_000_000)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: optionPlanID, Title: "Later repriced tier", PriceAmount: 99, PriceAmountMicros: &currentPrice,
		Currency: "CNY", EntitlementType: SubscriptionEntitlementTimed, MonthlyTokenLimit: 1_000,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: targetID, UserId: 201, PlanId: creditPlanID, EntitlementType: SubscriptionEntitlementCreditBalance,
		TokenLimit: 2_000, TokenUsed: 400,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: sourceID, UserId: 201, PlanId: optionPlanID, EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit: 1_000, TokenUsed: 0, Status: SubscriptionStatusConverted,
	}).Error)
	snapshot := SubscriptionEntitlementSnapshot{
		PurchaseMode: SubscriptionPurchaseModeCreditBalance, PlanID: optionPlanID,
		PlanEntitlementType: SubscriptionEntitlementTimed, ListPriceMicros: &snapshotPrice,
		ListPriceCurrency: "CNY", MonthlyTokenLimit: 1_000, TargetCreditBalancePlanID: creditPlanID,
	}
	snapshotPayload, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	require.NoError(t, db.Create(&SubscriptionOrder{
		Id: orderID, UserId: 201, PlanId: optionPlanID, Status: common.TopUpStatusSuccess,
		AmountCents: 4_000, Currency: "CNY", CreditGrantAmount: 1_000,
		CreditTargetPlanID: creditPlanID, FulfilledSubscriptionID: targetID,
		EntitlementSnapshot: snapshotPayload,
	}).Error)
	ledgers := []CreditBalanceLedger{
		{
			Id: purchaseLedgerID, UserId: 201, UserSubscriptionId: targetID,
			Type: CreditBalanceLedgerTypePurchase, IdempotencyKey: "legacy-order",
			SourceType: CreditBalanceLedgerSourceSubscriptionOrder, SourceId: orderID,
			GrossCredit: 1_000, AvailableCreditBefore: 0, AvailableCreditAfter: 1_000,
		},
		{
			Id: conversionLedgerID, UserId: 201, UserSubscriptionId: targetID,
			Type: CreditBalanceLedgerTypeSubscriptionConversion, IdempotencyKey: "legacy-conversion",
			SourceType: CreditBalanceLedgerSourceSubscriptionConversion, SourceId: sourceID,
			GrossCredit: 1_000, AvailableCreditBefore: 1_000, AvailableCreditAfter: 2_000,
		},
	}
	require.NoError(t, db.Create(&ledgers).Error)
	require.NoError(t, db.Create(&SubscriptionConversion{
		Id: conversionID, UserId: 201, IdempotencyKey: "legacy-conversion",
		SourceSubscriptionId: sourceID, SourcePlanId: optionPlanID, TargetSubscriptionId: targetID,
		TargetPlanId: creditPlanID, LedgerId: conversionLedgerID, CreditBasis: 1_000,
		GrossCredit: 1_000, NetAvailableCredit: 1_000,
	}).Error)

	report, err := RunCreditValuationHistoricalBackfill(db, creditHistoricalRequest(false))
	require.NoError(t, err)
	require.Equal(t, int64(1), report.RowsEstimated)
	require.Equal(t, int64(64_000_000), report.EstimatedCostMicros)
	require.Zero(t, report.UnknownCredit)
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

func TestCreditHistoricalBackfillRevaluesMigrationUnknownStateButPreservesForwardExact(t *testing.T) {
	db := setupCreditHistoricalBackfillDB(t)
	legacy := UserSubscription{Id: 109, UserId: 209, PlanId: 309, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 1_000, TokenUsed: 200}
	forward := UserSubscription{Id: 110, UserId: 210, PlanId: 309, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 1_000, TokenUsed: 200}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.Create(&forward).Error)
	require.NoError(t, db.Create(&CreditValuationState{
		UserSubscriptionId: legacy.Id, UserId: legacy.UserId, AvailableCredit: 800,
		UnknownCredit: 800, Currency: "CNY", RuleVersion: 1, MigrationVersion: 1,
		StateVersion: 7, LastMutationType: CreditValuationMutationConsume, CreatedAt: 1, UpdatedAt: 1,
	}).Error)
	require.NoError(t, db.Create(&CreditValuationState{
		UserSubscriptionId: forward.Id, UserId: forward.UserId, AvailableCredit: 800,
		ExactCostMicros: 32_000_000, Currency: "CNY", RuleVersion: 1, MigrationVersion: 1,
		StateVersion: 3, LastMutationType: CreditValuationMutationGrant, CreatedAt: 1, UpdatedAt: 1,
	}).Error)
	for _, sub := range []UserSubscription{legacy, forward} {
		require.NoError(t, db.Create(&CreditBalanceLedger{
			UserId: sub.UserId, UserSubscriptionId: sub.Id, Type: CreditBalanceLedgerTypePurchase,
			IdempotencyKey: "revalue-" + strconv.Itoa(sub.Id), SourceType: CreditBalanceLedgerSourceSubscriptionOrder,
			SourceId: 800 + sub.Id, SourceKey: "order:" + strconv.Itoa(800+sub.Id),
			GrossCredit: 1_000, NetCredit: 1_000, SourcePriceMicros: 40_000_000,
			SourcePlanCredit: 1_000, FxSourceCurrency: "CNY",
		}).Error)
	}
	require.NoError(t, db.Create(&CreditValuationMigration{Version: 1, Status: CreditValuationMigrationReady}).Error)

	request := creditHistoricalRequest(true)
	request.RevalueHistorical = true
	request.MigrationVersion = 2
	report, err := RunCreditValuationHistoricalBackfill(db, request)
	require.NoError(t, err)
	require.Equal(t, int64(1), report.RowsEstimated)
	require.Equal(t, int64(1), report.RowsSkippedExisting)

	var gotLegacy, gotForward CreditValuationState
	require.NoError(t, db.First(&gotLegacy, "user_subscription_id = ?", legacy.Id).Error)
	require.Equal(t, int64(32_000_000), gotLegacy.EstimatedCostMicros)
	require.Zero(t, gotLegacy.UnknownCredit)
	require.Equal(t, 2, gotLegacy.MigrationVersion)
	require.Equal(t, int64(8), gotLegacy.StateVersion)
	require.Equal(t, "historical_revaluation", gotLegacy.LastMutationType)
	require.NoError(t, db.First(&gotForward, "user_subscription_id = ?", forward.Id).Error)
	require.Equal(t, int64(32_000_000), gotForward.ExactCostMicros)
	require.Equal(t, int64(3), gotForward.StateVersion)
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
