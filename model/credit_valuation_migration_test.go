package model

import (
	"github.com/QuantumNous/new-api/common"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateCreditValuationMigrationRequestRequiresVersion(t *testing.T) {
	err := ValidateCreditValuationMigrationRequest(CreditValuationMigrationRequest{
		Mode:      CreditValuationMigrationModeDryRun,
		BatchSize: 100,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrCreditValuationMigrationVersionRequired)
}

func TestFreshCreditValuationDatabaseAutoReadiesThroughStartupMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := DB, LOG_DB
	previousSQLite, previousMySQL, previousPostgreSQL := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	DB, LOG_DB = db, db
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = true, false, false
	t.Setenv("LOG_SQL_DSN", "")
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = previousSQLite, previousMySQL, previousPostgreSQL
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, migrateDB())
	empty, err := creditValuationMigrationBusinessDatabaseEmpty(db)
	require.NoError(t, err)
	require.True(t, empty, "the built-in Credit container is not historical business data")

	var marker CreditValuationMigration
	require.NoError(t, db.First(&marker, "version = ?", 1).Error)
	require.Equal(t, CreditValuationMigrationReady, marker.Status)
	require.Positive(t, marker.FxCapturedAt)
	require.Equal(t, int64(1), marker.FxRateNumerator)
	require.Equal(t, int64(1), marker.FxRateDenominator)
	require.NotEmpty(t, marker.Checksum)

	report, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeVerify, Version: 1, BatchSize: 100,
	})
	require.NoError(t, err)
	require.True(t, report.Ready)
	require.Equal(t, marker.Checksum, report.Checksum)
}

func setupCreditValuationMigrationLifecycleTestDB(t *testing.T, tables ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := DB, LOG_DB
	previousSQLite, previousMySQL, previousPostgreSQL := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	DB, LOG_DB = db, db
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = true, false, false
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	require.NoError(t, db.AutoMigrate(append([]any{&CreditValuationMigration{}}, tables...)...))
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = previousSQLite, previousMySQL, previousPostgreSQL
		ClearPrimaryBillableSubscriptionCacheForTest()
		ClearSubscriptionPlanCacheForTest()
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestVerifyCreditValuationMigrationRejectsEveryNonReadyStatus(t *testing.T) {
	for _, status := range []string{
		CreditValuationMigrationPending,
		CreditValuationMigrationRunning,
		CreditValuationMigrationFailed,
		CreditValuationMigrationSuspended,
	} {
		t.Run(status, func(t *testing.T) {
			db := setupCreditValuationMigrationLifecycleTestDB(t)
			require.NoError(t, db.Create(&CreditValuationMigration{Version: 7, Status: status}).Error)
			report, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
				Mode: CreditValuationMigrationModeVerify, Version: 7, BatchSize: 100,
			})
			require.ErrorIs(t, err, ErrCreditValuationMigrationNotReady)
			require.Equal(t, status, report.Status)
			require.False(t, report.Ready)
		})
	}
}

func TestHigherFailedMarkerBlocksLowerReadyApplyReplay(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t)
	require.NoError(t, db.Create(&CreditValuationMigration{Version: 1, Status: CreditValuationMigrationReady, Checksum: "ready"}).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{Version: 2, Status: CreditValuationMigrationFailed, Checksum: "failed"}).Error)
	report, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeApply, Version: 1, BatchSize: 100,
	})
	require.ErrorIs(t, err, ErrCreditValuationMigrationConflict)
	require.False(t, report.Changed)
}

func TestRunningMigrationLeaseActiveRejectsClaim(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t)
	now, err := getDBTimestampStrictTx(db)
	require.NoError(t, err)
	fx := CreditValuationMigrationFXSnapshot{SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1, CapturedAt: now}
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: 3, Status: CreditValuationMigrationRunning, ValuationCurrency: "CNY",
		FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: now,
		PreflightChecksum: "same", RunLeaseExpiresAt: now + 60,
	}).Error)
	_, err = claimCreditValuationMigration(db, CreditValuationMigrationRequest{Version: 3}, "CNY", fx, "same", false)
	require.ErrorIs(t, err, ErrCreditValuationMigrationConflict)
}

func TestExpiredRunningMigrationSameChecksumCanResume(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t)
	now, err := getDBTimestampStrictTx(db)
	require.NoError(t, err)
	fx := CreditValuationMigrationFXSnapshot{SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1, CapturedAt: now}
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: 4, Status: CreditValuationMigrationRunning, ValuationCurrency: "CNY",
		FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: now,
		PreflightChecksum: "same", RunLeaseExpiresAt: now - 1,
	}).Error)
	claimed, err := claimCreditValuationMigration(db, CreditValuationMigrationRequest{Version: 4}, "CNY", fx, "same", false)
	require.NoError(t, err)
	require.Equal(t, CreditValuationMigrationRunning, claimed.Status)
	require.Greater(t, claimed.RunLeaseExpiresAt, now)
}

func TestExpiredRunningMigrationChecksumDriftRejectsResume(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t)
	now, err := getDBTimestampStrictTx(db)
	require.NoError(t, err)
	fx := CreditValuationMigrationFXSnapshot{SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1, CapturedAt: now}
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: 5, Status: CreditValuationMigrationRunning, ValuationCurrency: "CNY",
		FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: now,
		PreflightChecksum: "original", RunLeaseExpiresAt: now - 1,
	}).Error)
	_, err = claimCreditValuationMigration(db, CreditValuationMigrationRequest{Version: 5}, "CNY", fx, "drifted", false)
	require.ErrorIs(t, err, ErrCreditValuationMigrationChecksumMismatch)
}

func TestFailedMigrationRetryPreservesFrozenFXAndChecksum(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t)
	now, err := getDBTimestampStrictTx(db)
	require.NoError(t, err)
	fx := CreditValuationMigrationFXSnapshot{SourceCurrency: "USD", ValuationCurrency: "CNY", Numerator: 73, Denominator: 10, CapturedAt: now}
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: 6, Status: CreditValuationMigrationFailed, ValuationCurrency: "CNY",
		FxRateNumerator: 73, FxRateDenominator: 10, FxCapturedAt: now,
		PreflightChecksum: "frozen",
	}).Error)
	claimed, err := claimCreditValuationMigration(db, CreditValuationMigrationRequest{Version: 6}, "CNY", fx, "frozen", false)
	require.NoError(t, err)
	require.Equal(t, int64(73), claimed.FxRateNumerator)
	require.Equal(t, int64(10), claimed.FxRateDenominator)
	require.Equal(t, now, claimed.FxCapturedAt)
	require.Equal(t, "frozen", claimed.PreflightChecksum)
}

func TestSuspendedMigrationRejectsGrantAndPreconsumeWithoutSideEffects(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t,
		&User{}, &SubscriptionPlan{}, &UserSubscription{}, &CreditBalanceLedger{}, &SubscriptionPreConsumeRecord{},
	)
	const userID = 27_001
	const planID = 27_002
	const subscriptionID = 27_003
	valuationCurrency := "CNY"
	require.NoError(t, db.Create(&CreditValuationMigration{Version: 1, Status: CreditValuationMigrationSuspended}).Error)
	require.NoError(t, db.Create(&User{Id: userID, Username: "suspended-writer", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: planID, Title: "Suspended Credit", Enabled: true, EntitlementType: SubscriptionEntitlementCreditBalance,
		MonthlyTokenLimit: 100, ValuationCurrency: &valuationCurrency,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: subscriptionID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementCreditBalance,
		Status: SubscriptionStatusActive, TokenLimit: 100, TokenUsed: 0,
	}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, grantErr := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
			UserId: userID, GrossCredit: 10, IdempotencyKey: "suspended-grant",
			SourceType: CreditBalanceLedgerSourceAdminAdjustment, SourceId: 27_004,
			Type: CreditBalanceLedgerTypeAdminIncrease, TargetPlanId: planID,
		})
		return grantErr
	})
	require.ErrorIs(t, err, ErrCreditValuationMigrationSuspended)
	_, err = PreConsumeUserSubscription("suspended-preconsume", userID, "gpt-4o", 0, 5)
	require.ErrorIs(t, err, ErrCreditValuationMigrationSuspended)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, subscriptionID).Error)
	require.Equal(t, int64(100), subscription.TokenLimit)
	require.Zero(t, subscription.TokenUsed)
	var ledgerCount, preconsumeCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Count(&ledgerCount).Error)
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).Count(&preconsumeCount).Error)
	require.Zero(t, ledgerCount)
	require.Zero(t, preconsumeCount)
}

func TestVerifyCreditValuationMigrationSourcesUsesCanonicalCreditSourceKey(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &CreditBalanceLedger{})
	ledgers := []CreditBalanceLedger{
		{Id: 1, UserId: 1, UserSubscriptionId: 1, Type: CreditBalanceLedgerTypePurchase, IdempotencyKey: "canonical-source-a", SourceType: CreditBalanceLedgerSourceSubscriptionOrder, SourceId: 101, SourceKey: "subscription_order:shared", GrossCredit: 100, NetCredit: 100},
		{Id: 2, UserId: 2, UserSubscriptionId: 2, Type: CreditBalanceLedgerTypePurchase, IdempotencyKey: "canonical-source-b", SourceType: CreditBalanceLedgerSourceSubscriptionOrder, SourceId: 102, SourceKey: "subscription_order:shared", GrossCredit: 100, NetCredit: 100},
	}
	require.NoError(t, db.Create(&ledgers).Error)

	failures, err := verifyCreditValuationMigrationSources(db)
	require.NoError(t, err)
	require.Contains(t, failures, CreditValuationMigrationReasonCount{Reason: "credit_valuation_source_duplicate", Count: 2})
}

func TestVerifyCreditValuationMigrationSourcesAllowsLegacyMissingCreditSourceKey(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &CreditBalanceLedger{})
	require.NoError(t, db.Create(&CreditBalanceLedger{
		Id: 3, UserId: 3, UserSubscriptionId: 3, Type: CreditBalanceLedgerTypePurchase,
		IdempotencyKey: "legacy-missing-source", GrossCredit: 100, NetCredit: 100,
	}).Error)

	failures, err := verifyCreditValuationMigrationSources(db)
	require.NoError(t, err)
	require.Empty(t, failures)
}

func TestMigrationSnapshotUsesFXDirectionForUSDValuation(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &Option{}, &SubscriptionPlan{}, &UserSubscription{})
	valuationCurrency := "USD"
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: 27_101, Title: "USD Credit", Currency: "USD", ValuationCurrency: &valuationCurrency,
		EntitlementType: SubscriptionEntitlementCreditBalance, MonthlyTokenLimit: 1_000,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: 27_102, UserId: 27_103, PlanId: 27_101, EntitlementType: SubscriptionEntitlementCreditBalance,
		Status: SubscriptionStatusActive, TokenLimit: 1_000,
	}).Error)
	require.NoError(t, db.Create(&Option{Key: "USDExchangeRate", Value: "7.3"}).Error)

	fx, currency, blockers, err := migrationSnapshotInputs(db, CreditValuationMigration{}, false, true)
	require.NoError(t, err)
	require.Equal(t, "USD", currency)
	require.Empty(t, blockers)
	require.Equal(t, "CNY", fx.SourceCurrency)
	require.Equal(t, "USD", fx.ValuationCurrency)
	require.Equal(t, int64(10), fx.Numerator)
	require.Equal(t, int64(73), fx.Denominator)
	require.Positive(t, fx.CapturedAt)
}

func TestMigrationSnapshotUsesPlanCurrencyAndIdentityFXWithoutExchangeRate(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &SubscriptionPlan{}, &UserSubscription{})
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: 27_151, Title: "Legacy CNY Credit", Currency: "CNY",
		EntitlementType: SubscriptionEntitlementCreditBalance, MonthlyTokenLimit: 1_000,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: 27_152, UserId: 27_153, PlanId: 27_151,
		EntitlementType: SubscriptionEntitlementCreditBalance,
		Status:          SubscriptionStatusActive, TokenLimit: 1_000, TokenUsed: 200,
	}).Error)

	fx, currency, blockers, err := migrationSnapshotInputs(db, CreditValuationMigration{}, false, true)
	require.NoError(t, err)
	require.Equal(t, "CNY", currency)
	require.Empty(t, blockers)
	require.Equal(t, CreditValuationMigrationFXSnapshot{
		SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1,
		CapturedAt: fx.CapturedAt,
	}, fx)
	require.Positive(t, fx.CapturedAt)
}
func TestMigrationSnapshotNormalizesLegacyCNYMarkerFX(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &SubscriptionPlan{}, &UserSubscription{})
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: 27_161, Title: "Legacy CNY Marker", Currency: "CNY",
		EntitlementType: SubscriptionEntitlementCreditBalance, MonthlyTokenLimit: 1_000,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: 27_162, UserId: 27_163, PlanId: 27_161,
		EntitlementType: SubscriptionEntitlementCreditBalance,
		Status:          SubscriptionStatusActive, TokenLimit: 1_000, TokenUsed: 200,
	}).Error)

	fx, currency, blockers, err := migrationSnapshotInputs(db, CreditValuationMigration{
		Version: 1, Status: CreditValuationMigrationReady, ValuationCurrency: "CNY",
		FxRateNumerator: 73, FxRateDenominator: 10, FxCapturedAt: 123,
	}, true, true)
	require.NoError(t, err)
	require.Equal(t, "CNY", currency)
	require.Empty(t, blockers)
	require.Equal(t, CreditValuationMigrationFXSnapshot{
		SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1, CapturedAt: 123,
	}, fx)
}

func TestEmptyMigrationSnapshotNormalizesLegacyCNYMarkerFX(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t)

	fx, currency, blockers, err := migrationSnapshotInputs(db, CreditValuationMigration{
		Version: 1, Status: CreditValuationMigrationReady, ValuationCurrency: "CNY",
		FxRateNumerator: 73, FxRateDenominator: 10, FxCapturedAt: 123,
	}, true, true)
	require.NoError(t, err)
	require.Equal(t, "CNY", currency)
	require.Empty(t, blockers)
	require.Equal(t, CreditValuationMigrationFXSnapshot{
		SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1, CapturedAt: 123,
	}, fx)
}
func TestCreditValuationMigrationPersistsCreditPlanCurrencyFallbackBeforeReady(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &Option{}, &SubscriptionPlan{}, &UserSubscription{}, &CreditValuationState{}, &CreditBalanceLedger{}, &TimedSubscriptionValuationGrant{})

	zeroMicros := int64(0)
	priceMicros := int64(40_000_000)

	whitespaceCurrency := "   "
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: 27_150, Title: "Legacy CNY Credit", PriceAmountMicros: &zeroMicros,
		Currency: "CNY", ValuationCurrency: &whitespaceCurrency,
		EntitlementType: SubscriptionEntitlementCreditBalance, MonthlyTokenLimit: 1_000,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: 27_151, UserId: 27_152, PlanId: 27_150,
		EntitlementType: SubscriptionEntitlementCreditBalance,
		Status:          SubscriptionStatusActive, TokenLimit: 1_000, TokenUsed: 200,
	}).Error)
	now := GetDBTimestamp()
	require.NoError(t, db.Create(&CreditBalanceLedger{
		Id: 27_154, UserId: 27_152, UserSubscriptionId: 27_151,
		Type: CreditBalanceLedgerTypePurchase, IdempotencyKey: "fallback-currency-order",
		SourceType: CreditBalanceLedgerSourceSubscriptionOrder, SourceId: 27_154,
		SourceKey:   "subscription_order:fallback-currency-order",
		GrossCredit: 1_000, NetCredit: 1_000,
		SourcePriceMicros: 40_000_000, SourcePlanCredit: 1_000,
		FxSourceCurrency: "CNY", FxRateNumerator: 1, FxRateDenominator: 1,
		FxCapturedAt: now, CreatedAt: now,
	}).Error)

	report, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeApply, Version: 1, BatchSize: 100,
	})
	require.NoError(t, err, "report=%+v", report)
	require.True(t, report.Ready)
	require.Equal(t, "CNY", report.ValuationCurrency)

	var plan SubscriptionPlan
	require.NoError(t, db.First(&plan, 27_150).Error)
	require.NotNil(t, plan.ValuationCurrency)
	require.Equal(t, "CNY", *plan.ValuationCurrency)

	snapshot := NewSubscriptionEntitlementSnapshot(&SubscriptionPlan{
		Id: 27_153, Title: "40 CNY / 1,000 Credit", PriceAmountMicros: &priceMicros,
		Currency: "CNY", EntitlementType: SubscriptionEntitlementTimed,
		MonthlyTokenLimit: 1_000, DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, QuotaResetPeriod: SubscriptionResetMonthly,
	}, SubscriptionPurchaseModeCreditBalance, plan.Id)
	snapshot.SetTargetCreditBalancePlanSnapshot(&plan)
	require.Equal(t, "CNY", snapshot.TargetCreditBalanceValuationCurrency)
}

func TestRepairMissingAsUnknownCanBeAppliedWithSameVersion(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t,
		&Option{}, &SubscriptionPlan{}, &UserSubscription{}, &CreditValuationState{},
		&CreditBalanceLedger{}, &TimedSubscriptionValuationGrant{},
	)
	valuationCurrency := "CNY"
	zeroMicros := int64(0)

	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: 27_201, Title: "Repair Credit", PriceAmountMicros: &zeroMicros,
		Currency: "CNY", ValuationCurrency: &valuationCurrency,
		EntitlementType: SubscriptionEntitlementCreditBalance, MonthlyTokenLimit: 1_000,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: 27_202, UserId: 27_203, PlanId: 27_201,
		EntitlementType: SubscriptionEntitlementCreditBalance,
		Status:          SubscriptionStatusActive, TokenLimit: 1_000, TokenUsed: 200,
	}).Error)
	require.NoError(t, db.Create(&Option{Key: "USDExchangeRate", Value: "7.3"}).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: 1, Status: CreditValuationMigrationReady, ValuationCurrency: "CNY",
		FxRateNumerator: 73, FxRateDenominator: 10, FxCapturedAt: 1, Checksum: "previous",
	}).Error)
	var beforeStates int64
	require.NoError(t, db.Model(&CreditValuationState{}).Count(&beforeStates).Error)
	require.Zero(t, beforeStates)

	repair, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeRepairMissingAsUnknown, Version: 2, BatchSize: 100,
	})
	require.NoError(t, err)
	require.Equal(t, CreditValuationMigrationFailed, repair.Status)
	require.True(t, repair.Changed)
	require.Equal(t, int64(1), repair.Credit.RowsUnknown)
	require.Equal(t, int64(800), repair.Credit.UnknownCredit)

	applied, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeApply, Version: 2, BatchSize: 100,
	})
	require.NoError(t, err)
	require.Equal(t, CreditValuationMigrationReady, applied.Status)
	require.True(t, applied.Ready)
	require.Equal(t, int64(1), applied.Credit.RowsUnknown)
	require.Equal(t, int64(800), applied.Credit.UnknownCredit)

	verified, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeVerify, Version: 2, BatchSize: 100,
	})
	require.NoError(t, err)
	require.True(t, verified.Ready)
	require.Equal(t, applied.Checksum, verified.Checksum)
}

func TestRevalueHistoricalMigrationRequiresSuspendedReadyVersionAndPublishesNextReadyMarker(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t,
		&Option{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{},
		&SubscriptionConversion{}, &Redemption{}, &CreditBalanceAdjustment{},
		&CreditValuationState{}, &CreditBalanceLedger{}, &TimedSubscriptionValuationGrant{},
	)
	valuationCurrency := "CNY"
	zeroMicros := int64(0)
	priceMicros := int64(40_000_000)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: 27_301, Title: "Credit", PriceAmountMicros: &zeroMicros, Currency: "CNY",
		ValuationCurrency: &valuationCurrency, EntitlementType: SubscriptionEntitlementCreditBalance,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: 27_302, UserId: 27_303, PlanId: 27_301, EntitlementType: SubscriptionEntitlementCreditBalance,
		TokenLimit: 1_000, TokenUsed: 200,
	}).Error)
	require.NoError(t, db.Create(&CreditValuationState{
		UserSubscriptionId: 27_302, UserId: 27_303, AvailableCredit: 800, UnknownCredit: 800,
		Currency: "CNY", RuleVersion: 1, MigrationVersion: 1, StateVersion: 0,
		LastMutationType: "historical_backfill", CreatedAt: 1, UpdatedAt: 1,
	}).Error)
	snapshot, err := MarshalSubscriptionEntitlementSnapshot(SubscriptionEntitlementSnapshot{
		PurchaseMode: SubscriptionPurchaseModeCreditBalance, PlanID: 27_304,
		PlanEntitlementType: SubscriptionEntitlementTimed, ListPriceMicros: &priceMicros,
		ListPriceCurrency: "CNY", MonthlyTokenLimit: 1_000, TargetCreditBalancePlanID: 27_301,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&SubscriptionOrder{
		Id: 27_305, UserId: 27_303, PlanId: 27_304, Status: common.TopUpStatusSuccess,
		CreditGrantAmount: 1_000, CreditTargetPlanID: 27_301, FulfilledSubscriptionID: 27_302,
		EntitlementSnapshot: snapshot,
	}).Error)
	require.NoError(t, db.Create(&CreditBalanceLedger{
		Id: 27_306, UserId: 27_303, UserSubscriptionId: 27_302, Type: CreditBalanceLedgerTypePurchase,
		IdempotencyKey: "revalue-order", SourceType: CreditBalanceLedgerSourceSubscriptionOrder,
		SourceId: 27_305, GrossCredit: 1_000, AvailableCreditAfter: 1_000,
	}).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: 1, Status: CreditValuationMigrationReady, ValuationCurrency: "CNY",
		FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: 1, Checksum: "v1",
	}).Error)

	_, err = RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeRevalueHistorical, Version: 2, BatchSize: 100,
	})
	require.ErrorIs(t, err, ErrCreditValuationMigrationRepairInvalid)

	suspended, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeSuspend, Version: 1, BatchSize: 100, Reason: "historical revaluation",
	})
	require.NoError(t, err)
	require.Equal(t, CreditValuationMigrationSuspended, suspended.Status)

	revalued, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeRevalueHistorical, Version: 2, BatchSize: 100,
	})
	require.NoError(t, err)
	require.Equal(t, CreditValuationMigrationReady, revalued.Status)
	require.True(t, revalued.Ready)
	require.Equal(t, int64(32_000_000), revalued.Credit.EstimatedCostMicros)
	require.Zero(t, revalued.Credit.UnknownCredit)

	verified, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeVerify, Version: 2, BatchSize: 100,
	})
	require.NoError(t, err)
	require.Equal(t, revalued.Checksum, verified.Checksum)
}

func TestVerifyCreditValuationMigrationSourcesRejectsInvalidTimedGrantFacts(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &TimedSubscriptionValuationGrant{})
	grants := []TimedSubscriptionValuationGrant{
		{Id: 1, IdempotencyKey: "invalid-window", UserSubscriptionId: 1, UserId: 1, PlanId: 1, SourceType: TimedSubscriptionGrantSourceOrder, SourceKey: "subscription_order:1", SourceId: 1, EventStartTime: 200, EventEndTime: 100, GrantCredit: 1_000, SourcePriceMicros: 40_000_000, SourceCurrency: "CNY", ValuationAmountMicros: 40_000_000, ValuationCurrency: "CNY", Confidence: CreditValuationConfidenceEstimated, RuleVersion: CreditValuationRuleVersion, FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: 1, CreatedAt: 1},
		{Id: 2, IdempotencyKey: "invalid-value", UserSubscriptionId: 2, UserId: 2, PlanId: 2, SourceType: TimedSubscriptionGrantSourceOrder, SourceKey: "subscription_order:2", SourceId: 2, EventStartTime: 100, EventEndTime: 200, GrantCredit: 0, SourcePriceMicros: -1, SourceCurrency: "EUR", ValuationAmountMicros: -1, ValuationCurrency: "USD", Confidence: "invalid", RuleVersion: 0, FxRateNumerator: 0, FxRateDenominator: 0, CreatedAt: 1},
	}
	require.NoError(t, db.Create(&grants).Error)

	failures, err := verifyCreditValuationMigrationSources(db)
	require.NoError(t, err)
	require.Contains(t, failures, CreditValuationMigrationReasonCount{Reason: "timed_valuation_grant_invalid", Count: 2})
}

func TestSuspendTransitionsHighestReadyMarkerAndKeepsReadOnlyViewsAvailable(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &SubscriptionPlan{})
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: 3, Status: CreditValuationMigrationReady, ValuationCurrency: "CNY",
		FxRateNumerator: 73, FxRateDenominator: 10, FxCapturedAt: 1, Checksum: "ready",
	}).Error)

	report, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeSuspend, Version: 3, BatchSize: 100, Reason: "maintenance window",
	})
	require.NoError(t, err)
	require.Equal(t, CreditValuationMigrationSuspended, report.Status)
	require.True(t, report.Changed)
	require.False(t, report.Ready)

	status, err := CreditValuationRuntimeStatusTx(db)
	require.NoError(t, err)
	require.Equal(t, CreditValuationRuntimeSuspended, status)
	ready, err := CreditValuationRuntimeReadyTx(db)
	require.NoError(t, err)
	require.False(t, ready)
	_, err = CreditValuationWriterReadyTx(db)
	require.ErrorIs(t, err, ErrCreditValuationMigrationSuspended)

	dryRun, err := RunCreditValuationMigration(db, CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeDryRun, Version: 3, BatchSize: 100,
	})
	require.NoError(t, err)
	require.True(t, dryRun.ReadOnly)
	require.Equal(t, CreditValuationMigrationSuspended, dryRun.Status)
}

func TestFreshDatabaseDetectionIncludesTimedHistoryWithoutCredit(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &UserSubscription{})
	require.NoError(t, db.Create(&UserSubscription{
		Id: 27_301, UserId: 27_302, PlanId: 27_303, EntitlementType: SubscriptionEntitlementTimed,
		Status: SubscriptionStatusActive, StartTime: 100, EndTime: 200,
	}).Error)

	empty, err := creditValuationMigrationBusinessDatabaseEmpty(db)
	require.NoError(t, err)
	require.False(t, empty)
	require.NoError(t, ensureCreditValuationMigration(db))
	var count int64
	require.NoError(t, db.Model(&CreditValuationMigration{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCreditValuationMigrationBlockersIgnoreLegacyConsumedWithoutActiveTask(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &SubscriptionPreConsumeRecord{}, &Task{})
	require.NoError(t, db.Create(&SubscriptionPreConsumeRecord{
		RequestId: "legacy-consumed-without-active-task", UserId: 27_401, UserSubscriptionId: 27_402,
		PreConsumed: 100, Status: "consumed",
	}).Error)

	blockers, err := creditValuationMigrationBlockers(db)
	require.NoError(t, err)
	require.Empty(t, blockers)
}

func TestCreditValuationMigrationBlockersRejectActiveTaskNonTerminalRequest(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &SubscriptionPreConsumeRecord{}, &Task{})
	const requestID = "active-task-non-terminal-request"
	require.NoError(t, db.Create(&SubscriptionPreConsumeRecord{
		RequestId: requestID, UserId: 27_411, UserSubscriptionId: 27_412,
		PreConsumed: 100, Status: "consumed",
	}).Error)
	task := Task{
		TaskID: "active-task-with-request-identity", Status: TaskStatusInProgress,
		PrivateData: TaskPrivateData{
			BillingSource: "subscription", SubscriptionId: 27_412, SubscriptionRequestId: requestID,
		},
	}
	require.NoError(t, task.Insert())

	blockers, err := creditValuationMigrationBlockers(db)
	require.NoError(t, err)
	require.Equal(t, []CreditValuationMigrationBlocker{{
		Code: creditValuationMigrationBlockerPreConsume, Count: 1,
	}}, blockers)
}

func TestCreditValuationMigrationBlockersRejectActiveSubscriptionTaskWithoutRequestIdentity(t *testing.T) {
	db := setupCreditValuationMigrationLifecycleTestDB(t, &Task{})
	task := Task{
		TaskID: "active-task-without-request-identity", Status: TaskStatusInProgress,
		PrivateData: TaskPrivateData{BillingSource: "subscription", SubscriptionId: 27_422},
	}
	require.NoError(t, task.Insert())

	blockers, err := creditValuationMigrationBlockers(db)
	require.NoError(t, err)
	require.Equal(t, []CreditValuationMigrationBlocker{{
		Code: creditValuationMigrationBlockerAsyncTaskIdentity, Count: 1,
	}}, blockers)
}
