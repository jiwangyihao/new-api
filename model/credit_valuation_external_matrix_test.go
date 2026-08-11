package model

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type creditValuationExternalMatrixDatabase struct {
	name            string
	envName         string
	versionContains string
	open            func(string) (*gorm.DB, error)
}

func TestCreditValuationExternalMatrix(t *testing.T) {
	tests := []creditValuationExternalMatrixDatabase{
		{
			name:            common.DatabaseTypeSQLite,
			versionContains: "3.",
			open: func(_ string) (*gorm.DB, error) {
				dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "credit-valuation-external-matrix.sqlite")) + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)"
				return gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			},
		},
		{
			name:            common.DatabaseTypeMySQL,
			envName:         "TEST_MYSQL_DSN",
			versionContains: "5.7.44",
			open: func(dsn string) (*gorm.DB, error) {
				return gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			},
		},
		{
			name:            common.DatabaseTypePostgreSQL,
			envName:         "TEST_POSTGRES_DSN",
			versionContains: "9.6.24",
			open: func(dsn string) (*gorm.DB, error) {
				return gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCreditValuationExternalMatrix(t, test)
		})
	}
}

func runCreditValuationExternalMatrix(t *testing.T, test creditValuationExternalMatrixDatabase) {
	t.Helper()
	dsn := ""
	if test.envName != "" {
		dsn = strings.TrimSpace(os.Getenv(test.envName))
		if dsn == "" {
			t.Fatalf("%s is required; Gate F forbids skipped databases", test.envName)
		}
	}
	db, err := test.open(dsn)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
	sqlDB.SetMaxOpenConns(16)
	sqlDB.SetMaxIdleConns(16)

	resetCreditValuationExternalMatrixDatabase(t, db, test.name)
	installCreditValuationExternalMatrixGlobals(t, db, test.name)

	versionQuery := "SELECT VERSION()"
	if test.name == common.DatabaseTypeSQLite {
		versionQuery = "SELECT sqlite_version()"
	}
	var serverVersion string
	require.NoError(t, db.Raw(versionQuery).Scan(&serverVersion).Error)
	require.Contains(t, serverVersion, test.versionContains)
	t.Logf("database_version=%s", serverVersion)

	require.NoError(t, migrateDB())
	t.Run("schema", func(t *testing.T) {
		assertCreditValuationExternalSchema(t, db, test.name)
	})
	t.Run("row_lock", func(t *testing.T) {
		assertCreditValuationExternalRowLock(t, db, test.name)
	})
	t.Run("price_migration", func(t *testing.T) {
		assertCreditValuationExternalPriceMigration(t, db)
	})
	t.Run("migration_engine", func(t *testing.T) {
		prepareCreditValuationExternalBusinessStage(t, db, test.name)
		assertCreditValuationExternalMigrationEngine(t, db, test.name)
	})
	t.Run("lifecycle", func(t *testing.T) {
		prepareCreditValuationExternalBusinessStage(t, db, test.name)
		assertCreditValuationExternalLifecycle(t, db, test.name)
	})
	t.Run("conversion", func(t *testing.T) {
		prepareCreditValuationExternalBusinessStage(t, db, test.name)
		assertCreditValuationExternalConversion(t, db, test.name)
	})
	t.Run("recovery", func(t *testing.T) {
		prepareCreditValuationExternalBusinessStage(t, db, test.name)
		assertCreditValuationExternalRecovery(t, db, test.name)
	})
	t.Run("concurrent_grant_grant", func(t *testing.T) {
		prepareCreditValuationExternalBusinessStage(t, db, test.name)
		assertCreditValuationExternalConcurrentGrantGrant(t, db, test.name)
	})
	t.Run("concurrent_grant_consume", func(t *testing.T) {
		prepareCreditValuationExternalBusinessStage(t, db, test.name)
		assertCreditValuationExternalConcurrentGrantConsume(t, db, test.name)
	})
	t.Run("concurrent_consume_restore", func(t *testing.T) {
		prepareCreditValuationExternalBusinessStage(t, db, test.name)
		assertCreditValuationExternalConcurrentConsumeRestore(t, db, test.name)
	})
	t.Run("concurrent_conversion_settlement", func(t *testing.T) {
		prepareCreditValuationExternalBusinessStage(t, db, test.name)
		assertCreditValuationExternalConcurrentConversionSettlement(t, db, test.name)
	})
	t.Run("concurrent_refund_admin_decrease", func(t *testing.T) {
		prepareCreditValuationExternalBusinessStage(t, db, test.name)
		assertCreditValuationExternalConcurrentRefundAdminDecrease(t, db, test.name)
	})
}

func installCreditValuationExternalMatrixGlobals(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	oldDB, oldLogDB := DB, LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	oldLogSQLType, oldRedis := common.LogSqlType, common.RedisEnabled
	DB, LOG_DB = db, db
	common.UsingSQLite = dialect == common.DatabaseTypeSQLite
	common.UsingMySQL = dialect == common.DatabaseTypeMySQL
	common.UsingPostgreSQL = dialect == common.DatabaseTypePostgreSQL
	common.LogSqlType = dialect
	common.RedisEnabled = false
	initCol()
	resetDBTimestampCacheForTest()
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	t.Setenv("LOG_SQL_DSN", "")
	t.Cleanup(func() {
		resetCreditValuationExternalMatrixDatabase(t, db, dialect)
		DB, LOG_DB = oldDB, oldLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = oldSQLite, oldMySQL, oldPostgres
		common.LogSqlType, common.RedisEnabled = oldLogSQLType, oldRedis
		initCol()
		resetDBTimestampCacheForTest()
		ClearPrimaryBillableSubscriptionCacheForTest()
		ClearSubscriptionPlanCacheForTest()
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func resetCreditValuationExternalMatrixDatabase(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	tables, err := db.Migrator().GetTables()
	require.NoError(t, err)
	if len(tables) == 0 {
		return
	}
	quoted := make([]string, 0, len(tables))
	for _, table := range tables {
		switch dialect {
		case common.DatabaseTypePostgreSQL:
			quoted = append(quoted, `"`+strings.ReplaceAll(table, `"`, `""`)+`"`)
		default:
			quoted = append(quoted, "`"+strings.ReplaceAll(table, "`", "``")+"`")
		}
	}
	switch dialect {
	case common.DatabaseTypeMySQL:
		require.NoError(t, db.Exec("SET FOREIGN_KEY_CHECKS = 0").Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS "+strings.Join(quoted, ", ")).Error)
		require.NoError(t, db.Exec("SET FOREIGN_KEY_CHECKS = 1").Error)
	case common.DatabaseTypePostgreSQL:
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS "+strings.Join(quoted, ", ")+" CASCADE").Error)
	default:
		for i := len(tables) - 1; i >= 0; i-- {
			require.NoError(t, db.Migrator().DropTable(tables[i]), tables[i])
		}
	}
}

func prepareCreditValuationExternalBusinessStage(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	tables, err := db.Migrator().GetTables()
	require.NoError(t, err)
	preserved := map[string]bool{
		"credit_valuation_migrations": true,
		"sqlite_sequence":             true,
	}
	quoted := make([]string, 0, len(tables))
	for _, table := range tables {
		if preserved[table] {
			continue
		}
		switch dialect {
		case common.DatabaseTypePostgreSQL:
			quoted = append(quoted, `"`+strings.ReplaceAll(table, `"`, `""`)+`"`)
		default:
			quoted = append(quoted, "`"+strings.ReplaceAll(table, "`", "``")+"`")
		}
	}
	switch dialect {
	case common.DatabaseTypeMySQL:
		require.NoError(t, db.Exec("SET FOREIGN_KEY_CHECKS = 0").Error)
		defer func() { require.NoError(t, db.Exec("SET FOREIGN_KEY_CHECKS = 1").Error) }()
		for _, table := range quoted {
			require.NoError(t, db.Exec("DELETE FROM "+table).Error, table)
		}
	case common.DatabaseTypePostgreSQL:
		if len(quoted) > 0 {
			require.NoError(t, db.Exec("TRUNCATE TABLE "+strings.Join(quoted, ", ")+" RESTART IDENTITY CASCADE").Error)
		}
	default:
		require.NoError(t, db.Exec("PRAGMA foreign_keys = OFF").Error)
		defer func() { require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error) }()
		for _, table := range quoted {
			require.NoError(t, db.Exec("DELETE FROM "+table).Error, table)
		}
	}
	var marker CreditValuationMigration
	require.NoError(t, db.First(&marker, "version = ?", CreditValuationRuleVersion).Error)
	require.Equal(t, CreditValuationMigrationReady, marker.Status)
	resetDBTimestampCacheForTest()
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
}

func assertCreditValuationExternalSchema(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	for _, model := range []any{&CreditValuationState{}, &CreditValuationMigration{}, &TimedSubscriptionValuationGrant{}} {
		require.True(t, db.Migrator().HasTable(model))
	}
	for _, index := range []struct {
		model any
		name  string
	}{
		{&CreditValuationState{}, "uidx_credit_valuation_states_user_id"},
		{&TimedSubscriptionValuationGrant{}, "uidx_timed_valuation_grants_idempotency_key"},
		{&TimedSubscriptionValuationGrant{}, "uidx_timed_valuation_grants_source"},
	} {
		require.True(t, db.Migrator().HasIndex(index.model, index.name), index.name)
	}
	var marker CreditValuationMigration
	require.NoError(t, db.First(&marker, "version = ?", CreditValuationRuleVersion).Error)
	require.Equal(t, CreditValuationMigrationReady, marker.Status)
	require.NotEmpty(t, marker.Checksum)

	state := CreditValuationState{UserSubscriptionId: 990001, UserId: 990001, Currency: "CNY", RuleVersion: 1, StateVersion: 1}
	require.NoError(t, db.Create(&state).Error)
	duplicate := state
	duplicate.UserSubscriptionId++
	require.Error(t, db.Create(&duplicate).Error)
	require.NoError(t, db.Delete(&state).Error)

	grant := TimedSubscriptionValuationGrant{
		IdempotencyKey: "external-schema-grant", UserSubscriptionId: 990002, UserId: 990002, PlanId: 990002,
		SourceType: TimedSubscriptionGrantSourceAdmin, SourceKey: "admin:external-schema-grant", SourceId: 990002,
		EventStartTime: 1, EventEndTime: 2, GrantCredit: 1, SourcePriceMicros: 1, SourceCurrency: "CNY",
		ValuationAmountMicros: 1, ValuationCurrency: "CNY", Confidence: TimedSubscriptionValuationConfidenceExact,
		RuleVersion: 1, FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: 1, SourceSnapshot: `{}`, CreatedAt: 1,
	}
	require.NoError(t, db.Create(&grant).Error)
	duplicateGrant := grant
	duplicateGrant.Id = 0
	duplicateGrant.IdempotencyKey = "external-schema-grant-2"
	require.Error(t, db.Create(&duplicateGrant).Error)
	require.NoError(t, db.Exec("DELETE FROM timed_subscription_valuation_grants WHERE source_type = ? AND source_key = ?", grant.SourceType, grant.SourceKey).Error)
	t.Logf("schema_unique_constraints=%s", dialect)
}

func assertCreditValuationExternalRowLock(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	if dialect == common.DatabaseTypeSQLite {
		t.Log("row_lock=sqlite_serialized_writer")
		return
	}
	const version = 990010
	require.NoError(t, db.Create(&CreditValuationMigration{Version: version, Status: CreditValuationMigrationPending}).Error)
	locked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- db.Transaction(func(tx *gorm.DB) error {
			var marker CreditValuationMigration
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&marker, "version = ?", version).Error; err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- db.Transaction(func(tx *gorm.DB) error {
			var marker CreditValuationMigration
			return tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&marker, "version = ?", version).Error
		})
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("FOR UPDATE did not block: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
	require.NoError(t, db.Delete(&CreditValuationMigration{}, "version = ?", version).Error)
	t.Log("row_lock=for_update_blocked")
}

func assertCreditValuationExternalPriceMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	businessCode := "external-price-plan"
	require.NoError(t, db.Exec(`INSERT INTO subscription_plans (title, price_amount, price_amount_micros, business_code, entitlement_type) VALUES (?, ?, NULL, ?, ?)`, "External 12.345678", "12.345678", businessCode, SubscriptionEntitlementTimed).Error)
	var plan SubscriptionPlan
	require.NoError(t, db.Where("business_code = ?", businessCode).First(&plan).Error)

	first, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{BatchSize: 1})
	require.NoError(t, err)
	second, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{BatchSize: 1})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, int64(1), first.RowsTotal-first.RowsAlreadyExact)
	require.Zero(t, first.RowsInvalid)

	applied, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{Apply: true, BatchSize: 1})
	require.NoError(t, err)
	require.Equal(t, int64(1), applied.RowsBackfilled)
	require.NoError(t, db.First(&plan, plan.Id).Error)
	require.NotNil(t, plan.PriceAmountMicros)
	require.Equal(t, int64(12_345_678), *plan.PriceAmountMicros)

	verified, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{BatchSize: 1})
	require.NoError(t, err)
	require.Equal(t, verified.RowsTotal, verified.RowsAlreadyExact)
	require.Zero(t, verified.RowsInvalid)

	mismatchCode := "external-price-mismatch"
	require.NoError(t, db.Exec(`INSERT INTO subscription_plans (title, price_amount, price_amount_micros, business_code, entitlement_type) VALUES (?, ?, ?, ?, ?)`, "External mismatch", "7.25", int64(7_000_000), mismatchCode, SubscriptionEntitlementTimed).Error)
	mismatch, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{BatchSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), mismatch.RowsInvalid)
	require.Equal(t, SubscriptionPlanPriceDiagnosticRoundtripMismatch, mismatch.Diagnostics[0].Reason)
	t.Log("price_dry_run_apply_verify=pass")
}

func assertCreditValuationExternalMigrationEngine(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	const (
		migrationVersion     = CreditValuationRuleVersion + 1
		creditUserID         = 91_001
		creditPlanID         = 91_002
		creditSubscriptionID = 91_003
		creditLedgerID       = 91_004
		timedUserID          = 91_011
		timedPlanID          = 91_012
		timedSubscriptionID  = 91_013
		timedOrderID         = 91_014
		timedEventID         = 91_015
	)
	now := GetDBTimestamp()
	valuationCurrency := "CNY"
	zeroMicros := int64(0)
	timedPriceMicros := int64(40_000_000)
	creditCode := "external-migration-credit"
	timedCode := "external-migration-timed"
	require.NoError(t, db.Create(&Option{Key: "USDExchangeRate", Value: "7.3"}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: creditPlanID, Title: "External migration Credit", EntitlementType: SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true,
		PriceAmountMicros: &zeroMicros, Currency: "CNY", ValuationCurrency: &valuationCurrency,
		MonthlyTokenLimit: 1_000,
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: timedPlanID, Title: "External migration timed", EntitlementType: SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, PriceAmount: 40, PriceAmountMicros: &timedPriceMicros,
		Currency: "CNY", MonthlyTokenLimit: 1_000, DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever,
	}).Error)
	require.NoError(t, db.Create(&[]User{
		{Id: creditUserID, Username: "external-migration-credit", AffCode: "external-migration-credit", Status: common.UserStatusEnabled},
		{Id: timedUserID, Username: "external-migration-timed", AffCode: "external-migration-timed", Status: common.UserStatusEnabled},
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: creditSubscriptionID, UserId: creditUserID, PlanId: creditPlanID,
		EntitlementType: SubscriptionEntitlementCreditBalance, Status: SubscriptionStatusActive,
		TokenLimit: 1_000, TokenUsed: 200, StartTime: now - 300,
	}).Error)
	require.NoError(t, db.Create(&CreditBalanceLedger{
		Id: creditLedgerID, UserId: creditUserID, UserSubscriptionId: creditSubscriptionID,
		Type: CreditBalanceLedgerTypePurchase, IdempotencyKey: "external-migration-credit-order",
		SourceType: CreditBalanceLedgerSourceSubscriptionOrder, SourceId: creditLedgerID,
		SourceKey: "subscription_order:external-migration-credit-order", GrossCredit: 1_000, NetCredit: 1_000,
		SourcePriceMicros: 40_000_000, SourcePlanCredit: 1_000, FxSourceCurrency: "CNY",
		FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: now, CreatedAt: now,
	}).Error)
	timedStart, timedEnd := now-200, now-100
	require.NoError(t, db.Create(&UserSubscription{
		Id: timedSubscriptionID, UserId: timedUserID, PlanId: timedPlanID,
		EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive,
		TokenLimit: 1_000, StartTime: timedStart, EndTime: timedEnd,
	}).Error)
	snapshotPayload, err := common.Marshal(SubscriptionEntitlementSnapshot{
		PurchaseMode: SubscriptionPurchaseModeTimed, PlanID: timedPlanID,
		PlanEntitlementType: SubscriptionEntitlementTimed, ListPriceMicros: &timedPriceMicros,
		ListPriceCurrency: "CNY", MonthlyTokenLimit: 1_000,
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&SubscriptionOrder{
		Id: timedOrderID, UserId: timedUserID, PlanId: timedPlanID,
		TradeNo: "external-migration-timed-order", Status: common.TopUpStatusSuccess,
		FulfilledSubscriptionID: timedSubscriptionID, EntitlementSnapshot: string(snapshotPayload),
	}).Error)
	require.NoError(t, db.Create(&InvitationRewardEvent{
		Id: timedEventID, InviteeId: timedUserID,
		SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: timedOrderID,
		SourceOrderId: timedOrderID, SourceSubscriptionId: timedSubscriptionID,
		EventStartTime: timedStart, EventEndTime: timedEnd,
		Status: InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now,
	}).Error)

	request := CreditValuationMigrationRequest{
		Mode: CreditValuationMigrationModeDryRun, Version: migrationVersion, BatchSize: 1,
	}
	dryRun, err := RunCreditValuationMigration(db, request)
	require.NoError(t, err, "dry_run_report=%+v", dryRun)
	require.Positive(t, dryRun.FX.CapturedAt)
	require.True(t, dryRun.ReadOnly)
	require.False(t, dryRun.Changed)
	require.Equal(t, int64(1), dryRun.Credit.RowsEstimated)
	require.Equal(t, int64(32_000_000), dryRun.Credit.EstimatedCostMicros)
	require.Equal(t, int64(1), dryRun.Timed.RowsEstimated)
	require.Equal(t, int64(40_000_000), dryRun.Timed.EstimatedCostMicros)
	secondDryRun, err := RunCreditValuationMigration(db, request)
	require.NoError(t, err, "second_dry_run_report=%+v", secondDryRun)
	require.Equal(t, dryRun.Checksum, secondDryRun.Checksum)
	require.Equal(t, dryRun.Price, secondDryRun.Price)
	require.Equal(t, dryRun.Credit, secondDryRun.Credit)
	require.Equal(t, dryRun.Timed, secondDryRun.Timed)
	require.Equal(t, dryRun.Reasons, secondDryRun.Reasons)
	require.Equal(t, dryRun.Blockers, secondDryRun.Blockers)
	require.Equal(t, dryRun.Batches, secondDryRun.Batches)
	require.Positive(t, secondDryRun.FX.CapturedAt)
	require.False(t, secondDryRun.Changed)
	require.True(t, secondDryRun.ReadOnly)
	var stateCount, grantCount, markerCount int64
	require.NoError(t, db.Model(&CreditValuationState{}).Count(&stateCount).Error)
	require.NoError(t, db.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
	require.NoError(t, db.Model(&CreditValuationMigration{}).Where("version = ?", migrationVersion).Count(&markerCount).Error)
	require.Zero(t, stateCount)
	require.Zero(t, grantCount)
	require.Zero(t, markerCount)

	request.Mode = CreditValuationMigrationModeApply
	applied, err := RunCreditValuationMigration(db, request)
	require.NoError(t, err, "apply_report=%+v", applied)
	require.True(t, applied.Ready)
	require.True(t, applied.Changed)
	require.Equal(t, CreditValuationMigrationReady, applied.Status)
	require.Equal(t, int64(1), applied.Credit.RowsEstimated)
	require.Equal(t, int64(32_000_000), applied.Credit.EstimatedCostMicros)
	require.Zero(t, applied.Timed.RowsEstimated)
	require.Equal(t, int64(1), applied.Timed.RowsSkippedExisting)
	require.Zero(t, applied.Timed.EstimatedCostMicros)
	require.NotEmpty(t, applied.Checksum)

	var state CreditValuationState
	require.NoError(t, db.First(&state, "user_subscription_id = ?", creditSubscriptionID).Error)
	require.Equal(t, int64(800), state.AvailableCredit)
	require.Zero(t, state.ExactCostMicros)
	require.Equal(t, int64(32_000_000), state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, migrationVersion, state.MigrationVersion)
	var grant TimedSubscriptionValuationGrant
	require.NoError(t, db.First(&grant, "user_subscription_id = ?", timedSubscriptionID).Error)
	require.Equal(t, CreditValuationConfidenceEstimated, grant.Confidence)
	require.Equal(t, int64(40_000_000), grant.ValuationAmountMicros)
	require.True(t, validTimedSubscriptionValuationGrant(grant))

	request.Mode = CreditValuationMigrationModeVerify
	verified, err := RunCreditValuationMigration(db, request)
	require.NoError(t, err)
	require.True(t, verified.Ready)
	require.True(t, verified.ReadOnly)
	require.False(t, verified.Changed)
	require.Equal(t, applied.Checksum, verified.Checksum)
	request.Mode = CreditValuationMigrationModeApply
	replayed, err := RunCreditValuationMigration(db, request)
	require.NoError(t, err)
	require.True(t, replayed.Ready)
	require.False(t, replayed.Changed)
	require.Equal(t, applied.Checksum, replayed.Checksum)
	require.NoError(t, db.Model(&CreditValuationState{}).Count(&stateCount).Error)
	require.NoError(t, db.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
	require.Equal(t, int64(1), stateCount)
	require.Equal(t, int64(1), grantCount)
	t.Logf("historical_migration_dry_run_apply_verify=%s", dialect)
}

type creditValuationExternalConcurrentOperation struct {
	name string
	run  func() error
}

func runCreditValuationExternalConcurrentPair(t *testing.T, operations ...creditValuationExternalConcurrentOperation) map[string]error {
	t.Helper()
	require.Len(t, operations, 2)
	type outcome struct {
		name string
		err  error
	}
	ready := make(chan struct{}, len(operations))
	start := make(chan struct{})
	outcomes := make(chan outcome, len(operations))
	for _, operation := range operations {
		operation := operation
		go func() {
			ready <- struct{}{}
			<-start
			outcomes <- outcome{name: operation.name, err: operation.run()}
		}()
	}
	for range operations {
		<-ready
	}
	close(start)
	errorsByName := make(map[string]error, len(operations))
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	for range operations {
		select {
		case result := <-outcomes:
			errorsByName[result.name] = result.err
		case <-timer.C:
			t.Fatal("concurrent database operations did not complete; possible deadlock or lock-order inversion")
		}
	}
	return errorsByName
}

func assertCreditValuationExternalConcurrentGrantGrant(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	user, _, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	purchase := completeCreditValuationOrder(t, db, &order)
	capturedAt := GetDBTimestamp()
	grant := func(sourceID int, amount int64, priceMicros int64) error {
		return transactionWithUserSettingCASRetry(func(tx *gorm.DB) error {
			_, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
				UserId: user.Id, GrossCredit: amount,
				IdempotencyKey: "external-concurrent-grant-" + strconv.Itoa(sourceID),
				SourceType:     CreditBalanceLedgerSourceAdminAdjustment, SourceId: sourceID,
				SourceKey: "admin_adjustment:external-concurrent-grant-" + strconv.Itoa(sourceID),
				Type:      CreditBalanceLedgerTypeAdminIncrease, TargetPlanId: creditPlan.Id,
				Reason: "external concurrent grant",
				ValuationSource: &CreditValuationSourceSnapshot{
					SourcePriceMicros: priceMicros, SourcePlanCredit: amount, GrossCredit: amount,
					SourceCurrency: "CNY", ValuationCurrency: "CNY", RuleVersion: CreditValuationRuleVersion,
					FXRateSnapshot: &CreditFXRateSnapshot{
						SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1,
						CapturedAt: capturedAt, Direction: CreditFXDirectionIdentity,
					},
				},
			})
			return err
		})
	}
	errorsByName := runCreditValuationExternalConcurrentPair(t,
		creditValuationExternalConcurrentOperation{name: "grant-100", run: func() error { return grant(94_001, 100, 4_000_000) }},
		creditValuationExternalConcurrentOperation{name: "grant-200", run: func() error { return grant(94_002, 200, 8_000_000) }},
	)
	require.NoError(t, errorsByName["grant-100"])
	require.NoError(t, errorsByName["grant-200"])

	var balance UserSubscription
	require.NoError(t, db.First(&balance, purchase.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(1_300), balance.TokenLimit)
	require.Zero(t, balance.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, balance.Id).Error)
	require.Equal(t, int64(1_300), state.AvailableCredit)
	require.Equal(t, int64(52_000_000), state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, int64(3), state.StateVersion)
	var ledgers []CreditBalanceLedger
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("id ASC").Find(&ledgers).Error)
	require.Len(t, ledgers, 3)
	var grossCredit, grossCost int64
	for _, ledger := range ledgers {
		grossCredit += ledger.GrossCredit
		grossCost += ledger.ValuationGrossCostMicros
	}
	require.Equal(t, int64(1_300), grossCredit)
	require.Equal(t, int64(52_000_000), grossCost)
	t.Logf("concurrent_grant_grant=%s", dialect)
}

func assertCreditValuationExternalConcurrentGrantConsume(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	user, _, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	purchase := completeCreditValuationOrder(t, db, &order)
	capturedAt := GetDBTimestamp()
	grant := func() error {
		return transactionWithUserSettingCASRetry(func(tx *gorm.DB) error {
			_, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
				UserId: user.Id, GrossCredit: 500, IdempotencyKey: "external-concurrent-grant-consume",
				SourceType: CreditBalanceLedgerSourceAdminAdjustment, SourceId: 94_101,
				SourceKey: "admin_adjustment:external-concurrent-grant-consume",
				Type:      CreditBalanceLedgerTypeAdminIncrease, TargetPlanId: creditPlan.Id,
				Reason: "external concurrent grant consume",
				ValuationSource: &CreditValuationSourceSnapshot{
					SourcePriceMicros: 40_000_000, SourcePlanCredit: 500, GrossCredit: 500,
					SourceCurrency: "CNY", ValuationCurrency: "CNY", RuleVersion: CreditValuationRuleVersion,
					FXRateSnapshot: &CreditFXRateSnapshot{
						SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1,
						CapturedAt: capturedAt, Direction: CreditFXDirectionIdentity,
					},
				},
			})
			return err
		})
	}
	consume := func() error {
		_, err := PreConsumeUserSubscriptionByUnits("external-concurrent-grant-consume-request", user.Id, "gpt-4o", 0, 0, 200)
		return err
	}
	errorsByName := runCreditValuationExternalConcurrentPair(t,
		creditValuationExternalConcurrentOperation{name: "grant", run: grant},
		creditValuationExternalConcurrentOperation{name: "consume", run: consume},
	)
	require.NoError(t, errorsByName["grant"])
	require.NoError(t, errorsByName["consume"])

	var balance UserSubscription
	require.NoError(t, db.First(&balance, purchase.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(1_500), balance.TokenLimit)
	require.Equal(t, int64(200), balance.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, balance.Id).Error)
	require.Equal(t, int64(1_300), state.AvailableCredit)
	require.Contains(t, []int64{69_333_334, 72_000_000}, state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, int64(3), state.StateVersion)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", "external-concurrent-grant-consume-request").First(&record).Error)
	require.Equal(t, int64(200), record.AppliedCredit)
	require.Equal(t, int64(200), record.DeductedAvailableCredit)
	require.Contains(t, []int64{8_000_000, 10_666_666}, record.DeductedExactCostMicros)
	require.Equal(t, int64(80_000_000), state.ExactCostMicros+record.DeductedExactCostMicros)
	var ledgerCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Where("user_id = ?", user.Id).Count(&ledgerCount).Error)
	require.Equal(t, int64(2), ledgerCount)
	t.Logf("concurrent_grant_consume=%s", dialect)
}

func assertCreditValuationExternalConcurrentConsumeRestore(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	purchase := completeCreditValuationOrder(t, db, &order)
	const restoreRequestID = "external-concurrent-consume-restore"
	restored, err := PreConsumeUserSubscriptionByUnits(restoreRequestID, user.Id, "gpt-4o", 0, 0, 400)
	require.NoError(t, err)
	require.Equal(t, purchase.CreditBalance.UserSubscriptionId, restored.UserSubscriptionId)

	consume := func() error {
		_, consumeErr := PreConsumeUserSubscriptionByUnits("external-concurrent-consume-other", user.Id, "gpt-4o", 0, 0, 200)
		return consumeErr
	}
	restore := func() error {
		return SettleUserSubscriptionRequestTarget(restoreRequestID, restored.UserSubscriptionId, 0, true)
	}
	errorsByName := runCreditValuationExternalConcurrentPair(t,
		creditValuationExternalConcurrentOperation{name: "consume", run: consume},
		creditValuationExternalConcurrentOperation{name: "restore", run: restore},
	)
	require.NoError(t, errorsByName["consume"])
	require.NoError(t, errorsByName["restore"])

	var balance UserSubscription
	require.NoError(t, db.First(&balance, restored.UserSubscriptionId).Error)
	require.Equal(t, int64(200), balance.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, balance.Id).Error)
	require.Equal(t, int64(800), state.AvailableCredit)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, int64(4), state.StateVersion)
	var restoredRecord, consumedRecord SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", restoreRequestID).First(&restoredRecord).Error)
	require.NoError(t, db.Where("request_id = ?", "external-concurrent-consume-other").First(&consumedRecord).Error)
	require.Zero(t, restoredRecord.AppliedCredit)
	require.Zero(t, restoredRecord.DeductedAvailableCredit)
	require.Zero(t, restoredRecord.DeductedExactCostMicros)
	require.Equal(t, "refunded", restoredRecord.Status)
	require.Equal(t, int64(200), consumedRecord.AppliedCredit)
	require.Equal(t, int64(200), consumedRecord.DeductedAvailableCredit)
	require.Equal(t, int64(8_000_000), consumedRecord.DeductedExactCostMicros)
	require.Equal(t, "consumed", consumedRecord.Status)
	t.Logf("concurrent_consume_restore=%s", dialect)
}

func assertCreditValuationExternalConcurrentConversionSettlement(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	const (
		userID        = 95_001
		timedPlanID   = 95_002
		creditPlanID  = 95_003
		sourceID      = 95_004
		creditBasis   = int64(100)
		reserveCredit = int64(10)
		requestID     = "external-concurrent-conversion-settlement"
	)
	now := GetDBTimestamp()
	valuationCurrency := "CNY"
	priceMicros := int64(40_000_000)
	timedCode := "external-concurrent-conversion-timed"
	creditCode := "external-concurrent-conversion-credit"
	user := User{Id: userID, Username: "external-concurrent-conversion", AffCode: "external-concurrent-conversion", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = sourceID
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	user.SetSetting(setting)
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: timedPlanID, Title: "External concurrent timed", EntitlementType: SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: SubscriptionResetMonthly, MonthlyTokenLimit: creditBasis, TimedConversionEnabled: true,
		PriceAmountMicros: &priceMicros, PriceAmount: 40, Currency: "CNY",
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: creditPlanID, Title: "External concurrent Credit", EntitlementType: SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true,
		CreditBalanceConversionEnabled: true, ValuationCurrency: &valuationCurrency,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: sourceID, UserId: userID, PlanId: timedPlanID, EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit: creditBasis, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 40*24*60*60, EndTime: now + TimedSubscriptionConversionBlockSeconds + 60,
		Status: SubscriptionStatusActive, LastGrantedAt: now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(creditBasis), LastGrantTimeSource: SubscriptionGrantTimeSourceLive,
		LastGrantSource: SubscriptionGrantOrder,
	}).Error)
	reserved, err := PreConsumeUserSubscriptionByUnits(requestID, userID, "gpt-4o", 0, 0, reserveCredit)
	require.NoError(t, err)
	require.Equal(t, sourceID, reserved.UserSubscriptionId)

	conversionResult := make(chan *SubscriptionConversionResult, 1)
	conversion := func() error {
		result, conversionErr := ConfirmTimedSubscriptionConversion(userID, sourceID, "external-concurrent-conversion-settlement")
		conversionResult <- result
		return conversionErr
	}
	settlement := func() error {
		return settleUserSubscriptionRequestTargetDirect(requestID, sourceID, reserveCredit, true)
	}
	errorsByName := runCreditValuationExternalConcurrentPair(t,
		creditValuationExternalConcurrentOperation{name: "conversion", run: conversion},
		creditValuationExternalConcurrentOperation{name: "settlement", run: settlement},
	)
	require.NoError(t, errorsByName["conversion"])
	settlementErr := errorsByName["settlement"]
	if settlementErr != nil {
		require.ErrorIs(t, settlementErr, ErrCreditValuationStateMismatch)
		require.NoError(t, settleUserSubscriptionRequestTargetDirect(requestID, sourceID, reserveCredit, true))
	}
	converted := <-conversionResult
	require.NotNil(t, converted)
	require.False(t, converted.Replayed)
	targetID := converted.Conversion.TargetSubscriptionId
	require.Positive(t, targetID)

	var source, target UserSubscription
	require.NoError(t, db.First(&source, sourceID).Error)
	require.NoError(t, db.First(&target, targetID).Error)
	require.Equal(t, SubscriptionStatusConverted, source.Status)
	require.Equal(t, reserveCredit, source.TokenUsed)
	require.Equal(t, targetID, source.ConvertedToSubscriptionId)
	require.Equal(t, int64(190), target.TokenLimit)
	require.Zero(t, target.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, targetID).Error)
	require.Equal(t, int64(190), state.AvailableCredit)
	require.Equal(t, int64(76_000_000), state.ExactCostMicros)
	var route SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&route).Error)
	require.Equal(t, sourceID, route.UserSubscriptionId)
	require.Equal(t, targetID, route.ValuationSubscriptionId)
	require.Equal(t, reserveCredit, route.AppliedCredit)
	require.Equal(t, "settled", route.Status)
	require.Positive(t, route.FinalizedAt)
	var conversions, ledgers, states int64
	require.NoError(t, db.Model(&SubscriptionConversion{}).Where("source_subscription_id = ?", sourceID).Count(&conversions).Error)
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionConversion, sourceID).Count(&ledgers).Error)
	require.NoError(t, db.Model(&CreditValuationState{}).Where("user_id = ?", userID).Count(&states).Error)
	require.Equal(t, int64(1), conversions)
	require.Equal(t, int64(1), ledgers)
	require.Equal(t, int64(1), states)
	t.Logf("concurrent_conversion_settlement=%s", dialect)
}

func assertCreditValuationExternalConcurrentRefundAdminDecrease(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderStripe)
	purchase := completeCreditValuationOrder(t, db, &order)

	recovery := func() error {
		_, err := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
			TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
			RecoveryType: SubscriptionOrderRecoveryRefund, Reason: "external concurrent refund",
		})
		return err
	}
	decrease := func() error {
		_, err := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
			UserId: user.Id, Operation: CreditBalanceAdjustmentDecrease, Amount: 100,
			IdempotencyKey: "external-concurrent-refund-admin-decrease", OperatorUserId: 95_199,
			Reason: "external concurrent admin decrease",
		})
		return err
	}
	errorsByName := runCreditValuationExternalConcurrentPair(t,
		creditValuationExternalConcurrentOperation{name: "refund", run: recovery},
		creditValuationExternalConcurrentOperation{name: "admin-decrease", run: decrease},
	)
	require.NoError(t, errorsByName["refund"])
	require.NoError(t, errorsByName["admin-decrease"])

	require.NoError(t, db.First(&order, order.Id).Error)
	require.Equal(t, common.TopUpStatusRefunded, order.Status)
	require.Equal(t, SubscriptionOrderRecoveryRefund, order.RecoveryType)
	var balance UserSubscription
	require.NoError(t, db.First(&balance, purchase.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(1_000), balance.TokenLimit)
	require.Equal(t, int64(1_100), balance.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, balance.Id).Error)
	require.Zero(t, state.AvailableCredit)
	require.Zero(t, state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, int64(3), state.StateVersion)

	var recoveryLedgers, adminLedgers []CreditBalanceLedger
	require.NoError(t, db.Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Find(&recoveryLedgers).Error)
	require.NoError(t, db.Where("source_type = ? AND type = ?", CreditBalanceLedgerSourceAdminAdjustment, CreditBalanceLedgerTypeAdminDecrease).Find(&adminLedgers).Error)
	require.Len(t, recoveryLedgers, 1)
	require.Len(t, adminLedgers, 1)
	require.Equal(t, int64(-1_000), recoveryLedgers[0].GrossCredit)
	require.Equal(t, int64(-100), adminLedgers[0].GrossCredit)
	require.Equal(t, int64(40_000_000), recoveryLedgers[0].ValuationGrossCostMicros+adminLedgers[0].ValuationGrossCostMicros)
	require.Contains(t, []int64{0, 4_000_000}, adminLedgers[0].ValuationGrossCostMicros)
	require.Equal(t, int64(3), maxInt64(recoveryLedgers[0].ValuationStateVersionAfter, adminLedgers[0].ValuationStateVersionAfter))
	var adjustmentCount int64
	require.NoError(t, db.Model(&CreditBalanceAdjustment{}).Where("idempotency_key = ?", "external-concurrent-refund-admin-decrease").Count(&adjustmentCount).Error)
	require.Equal(t, int64(1), adjustmentCount)
	t.Logf("concurrent_refund_admin_decrease=%s", dialect)
}

func assertCreditValuationExternalLifecycle(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	user, _, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	purchase := completeCreditValuationOrder(t, db, &order)
	const requestID = "external-matrix-credit-lifecycle"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err, "preconsume_error=%#v", err)
	require.Equal(t, purchase.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 200, true))

	var state CreditValuationState
	require.NoError(t, db.First(&state, purchase.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(800), state.AvailableCredit)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)

	query := AdminAnalyticsQuery{SnapshotAt: GetDBTimestamp(), EndTimestamp: GetDBTimestamp(), RangeMode: AdminAnalyticsRangeModeSnapshot, Currency: "CNY", Limit: 20}
	summary, err := GetAdminPaidSubscriptionValueSummary(query)
	require.NoError(t, err)
	users, err := GetAdminPaidSubscriptionValueUsers(query)
	require.NoError(t, err)
	subscriptions, err := GetAdminPaidSubscriptionValueSubscriptions(query)
	require.NoError(t, err)
	plans, err := GetAdminPaidSubscriptionValuePlanBreakdown(query)
	require.NoError(t, err)
	sources, err := GetAdminPaidSubscriptionValueSourceBreakdown(query)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Data.Summary.ActivePaidSubscriptionCount)
	require.Equal(t, int64(32_000_000), moneyBreakdownMicros(summary.Data.Summary.RecognizedRemainingValueByCurrency, "CNY"))
	require.Len(t, users.Data.Users.Items, 1)
	require.Equal(t, int64(32_000_000), moneyBreakdownMicros(users.Data.Users.Items[0].RecognizedRemainingValueByCurrency, "CNY"))
	require.Len(t, subscriptions.Data.Subscriptions.Items, 1)
	require.Equal(t, int64(800), subscriptions.Data.Subscriptions.Items[0].AvailableCredit)
	require.Equal(t, "32000000", subscriptions.Data.Subscriptions.Items[0].RecognizedRemainingValue.AmountMicros)
	require.Len(t, plans.Data.Plans.Items, 1)
	require.Equal(t, creditPlan.Id, plans.Data.Plans.Items[0].PlanID)
	require.Equal(t, int64(32_000_000), moneyBreakdownMicros(plans.Data.Plans.Items[0].RecognizedRemainingValueByCurrency, "CNY"))
	require.Len(t, sources.Data.Sources.Items, 1)
	require.Equal(t, int64(32_000_000), moneyBreakdownMicros(sources.Data.Sources.Items[0].RecognizedRemainingValueByCurrency, "CNY"))

	const refundRequestID = "external-matrix-credit-refund"
	refunded, err := PreConsumeUserSubscriptionByUnits(refundRequestID, user.Id, "gpt-4o", 0, 0, 100)
	require.NoError(t, err)
	require.NoError(t, SettleUserSubscriptionRequestTarget(refundRequestID, refunded.UserSubscriptionId, 0, true))
	var refundRecord SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", refundRequestID).First(&refundRecord).Error)
	require.Equal(t, "refunded", refundRecord.Status)
	require.NoError(t, db.First(&state, purchase.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(800), state.AvailableCredit)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)
	t.Logf("purchase_consume_refund_analytics=%s", dialect)
}

func assertCreditValuationExternalConversion(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	const (
		userID       = 92_001
		timedPlanID  = 92_002
		creditPlanID = 92_003
		sourceID     = 92_004
		creditBasis  = int64(100)
	)
	now := GetDBTimestamp()
	valuationCurrency := "CNY"
	priceMicros := int64(40_000_000)
	timedCode := "external-matrix-conversion-timed"
	creditCode := "external-matrix-conversion-credit"
	require.NoError(t, db.Create(&User{Id: userID, Username: "external-matrix-conversion", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: timedPlanID, Title: "External 40 CNY / 100 timed Credit", EntitlementType: SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: SubscriptionResetMonthly, MonthlyTokenLimit: creditBasis, TimedConversionEnabled: true,
		PriceAmountMicros: &priceMicros, PriceAmount: 40, Currency: "CNY",
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: creditPlanID, Title: "External conversion Credit", EntitlementType: SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true,
		CreditBalanceConversionEnabled: true, ValuationCurrency: &valuationCurrency,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: sourceID, UserId: userID, PlanId: timedPlanID, EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit: creditBasis, TokenUsed: 20, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 40*24*60*60, EndTime: now + TimedSubscriptionConversionBlockSeconds + 60,
		Status: SubscriptionStatusActive, LastGrantedAt: now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(creditBasis), LastGrantTimeSource: SubscriptionGrantTimeSourceLive,
		LastGrantSource: SubscriptionGrantOrder,
	}).Error)
	quotes, err := ListTimedSubscriptionConversionQuotes(userID)
	require.NoError(t, err)
	require.Len(t, quotes.Quotes, 1)
	require.Equal(t, int64(180), quotes.Quotes[0].GrossCredit)
	conversion, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "external-matrix-conversion")
	require.NoError(t, err)
	require.False(t, conversion.Replayed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, conversion.Conversion.TargetSubscriptionId).Error)
	require.Equal(t, int64(180), state.AvailableCredit)
	require.Equal(t, int64(72_000_000), state.ExactCostMicros)
	t.Logf("conversion=%s", dialect)
}

func assertCreditValuationExternalRecovery(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	user, _, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderStripe)
	order.Id = 93_004
	order.TradeNo = "external-matrix-recovery"
	require.NoError(t, db.Create(&order).Error)
	purchase := completeCreditValuationOrder(t, db, &order)
	capturedAt := common.GetTimestamp()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
			UserId: user.Id, GrossCredit: 1_000, IdempotencyKey: "external-matrix-recovery-ingress",
			SourceType: CreditBalanceLedgerSourceAdminAdjustment, SourceId: 93_099,
			Type: CreditBalanceLedgerTypeAdminIncrease, TargetPlanId: creditPlan.Id,
			TargetPlanSnapshot: &creditPlan, Reason: "external matrix second ingress",
			ValuationSource: &CreditValuationSourceSnapshot{
				SourcePriceMicros: 80_000_000, SourcePlanCredit: 1_000, GrossCredit: 1_000,
				SourceCurrency: "CNY", ValuationCurrency: "CNY", RuleVersion: CreditValuationRuleVersion,
				FXRateSnapshot: &CreditFXRateSnapshot{SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1, CapturedAt: capturedAt, Direction: CreditFXDirectionIdentity},
			},
		})
		return err
	}))
	recovery, err := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
		TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
		RecoveryType: SubscriptionOrderRecoveryRefund, ProviderPayload: `{"event":"external-matrix-refund"}`,
		OperatorUserId: 93_070, Reason: "external matrix recovery",
	})
	require.NoError(t, err)
	require.False(t, recovery.Replayed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, purchase.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(1_000), state.AvailableCredit)
	require.Equal(t, int64(60_000_000), state.ExactCostMicros)
	replayed, err := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
		TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
		RecoveryType: SubscriptionOrderRecoveryRefund, ProviderPayload: `{"event":"external-matrix-refund"}`,
		OperatorUserId: 93_070, Reason: "external matrix recovery",
	})
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	t.Logf("destructive_recovery=%s", dialect)
}
