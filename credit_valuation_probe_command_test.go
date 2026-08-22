package main

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestParseCreditValuationProbeArgsRequiresImmutableCloneIdentity(t *testing.T) {
	cloneDSN := "postgresql://probe:secret@clone-postgres:5432/probe?sslmode=disable"
	options, err := parseCreditValuationProbeArgs([]string{
		"--clone-tracer",
		"--version", "7",
		"--backup-sha256", strings.Repeat("a", 64),
		"--target-image", "registry.example/new-api@sha256:" + strings.Repeat("b", 64),
		"--clone-dsn", cloneDSN,
	})
	require.NoError(t, err)
	require.Equal(t, creditValuationProbeModeClone, options.Mode)
	require.Equal(t, 7, options.Version)
	require.Equal(t, cloneDSN, options.CloneDSN)

	validIdentity := []string{"--clone-tracer", "--version", "1", "--backup-sha256", strings.Repeat("a", 64), "--target-image", "registry.example/new-api@sha256:" + strings.Repeat("b", 64)}
	for _, args := range [][]string{
		append(append([]string{}, validIdentity...), "--clone-dsn", ""),
		append(append([]string{}, validIdentity...), "--clone-dsn", "mysql://clone"),
		{"--clone-tracer", "--version", "0", "--backup-sha256", strings.Repeat("a", 64), "--target-image", "registry.example/new-api@sha256:" + strings.Repeat("b", 64), "--clone-dsn", cloneDSN},
		{"--clone-tracer", "--version", "1", "--backup-sha256", "not-a-digest", "--target-image", "registry.example/new-api@sha256:" + strings.Repeat("b", 64), "--clone-dsn", cloneDSN},
		{"--clone-tracer", "--version", "1", "--backup-sha256", strings.Repeat("a", 64), "--target-image", "registry.example/new-api:latest", "--clone-dsn", cloneDSN},
	} {
		_, parseErr := parseCreditValuationProbeArgs(args)
		require.Error(t, parseErr)
	}
}

func TestRunCreditValuationCloneFixtureProducesThirtyTwoCNYAndDisabledPlanProof(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	oldRedis := common.RedisEnabled
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = true, false, false
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	model.ClearDBTimestampCacheForTest()
	model.ClearSubscriptionPlanCacheForTest()
	model.ClearPrimaryBillableSubscriptionCacheForTest()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = oldSQLite, oldMySQL, oldPostgres
		common.RedisEnabled = oldRedis
		model.ClearDBTimestampCacheForTest()
		model.ClearSubscriptionPlanCacheForTest()
		model.ClearPrimaryBillableSubscriptionCacheForTest()
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.Option{}))
	require.NoError(t, model.MigrateCreditValuationSchema(db))
	zeroMicros := int64(0)
	valuationCurrency := "CNY"
	businessCode := "probe-credit-pool"
	creditPlan := model.SubscriptionPlan{
		Title:                          "Probe Credit pool",
		PriceAmountMicros:              &zeroMicros,
		Currency:                       "CNY",
		ValuationCurrency:              &valuationCurrency,
		Enabled:                        true,
		CreditBalanceConfigured:        true,
		CreditBalancePurchaseEnabled:   true,
		CreditBalanceRedemptionEnabled: true,
		CreditBalanceConversionEnabled: true,
		BusinessCode:                   &businessCode,
		EntitlementType:                model.SubscriptionEntitlementCreditBalance,
	}
	require.NoError(t, db.Create(&creditPlan).Error)
	require.NoError(t, db.Create(&model.CreditValuationMigration{
		Version: 1, Status: model.CreditValuationMigrationReady, ValuationCurrency: "CNY",
		FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: model.GetDBTimestamp(),
	}).Error)

	fixture, err := runCreditValuationCloneFixture(db, strings.Repeat("a", 64))
	require.NoError(t, err)
	require.Equal(t, "40000000", fixture.PriceAmountMicros)
	require.Equal(t, int64(800), fixture.AvailableCredit)
	require.Equal(t, "32000000", fixture.ExactCostMicros)
	require.Equal(t, "0", fixture.EstimatedCostMicros)
	require.Zero(t, fixture.UnknownCredit)
	require.Equal(t, 1, fixture.ActivePaidSubscriptionCount)
	require.True(t, fixture.FiveAnalyticsEndpointsConsistent)
	require.True(t, fixture.DisabledPlanExistingConsumable)
	require.True(t, fixture.DisabledPlanNewAllocationsRejected)
	require.True(t, fixture.ModelScopeIgnored)
}

func TestCreditValuationProbeDispatchesBeforeResourceInitialization(t *testing.T) {
	content := readMainSourceForProbeTest(t)
	probeIndex := strings.Index(content, `os.Args[1] == "credit-valuation-probe"`)
	resourceIndex := strings.Index(content, "InitResources()")
	require.NotEqual(t, -1, probeIndex)
	require.NotEqual(t, -1, resourceIndex)
	require.Less(t, probeIndex, resourceIndex)
}

func readMainSourceForProbeTest(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("main.go")
	require.NoError(t, err)
	return string(data)
}
