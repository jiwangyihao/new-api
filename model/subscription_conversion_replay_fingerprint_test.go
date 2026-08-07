package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type conversionReplayFingerprintFixture struct {
	userID               int
	sourcePlanID         int
	sourceSubscriptionID int
	targetSubscriptionID int
	ledgerID             int
	conversionID         int
	idempotencyKey       string
}

type conversionReplayPersistentSnapshot struct {
	counts     conversionValuationWriteCounts
	source     UserSubscription
	target     UserSubscription
	ledger     CreditBalanceLedger
	state      CreditValuationState
	conversion SubscriptionConversion
}

func seedConversionReplayFingerprintFixture(t *testing.T, offset int) conversionReplayFingerprintFixture {
	t.Helper()
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, migrateCreditValuationSchema(DB))

	fixture := conversionReplayFingerprintFixture{
		userID:               27_000 + offset*10,
		sourcePlanID:         27_001 + offset*10,
		sourceSubscriptionID: 27_002 + offset*10,
		idempotencyKey:       "structured-replay-fingerprint",
	}
	now := GetDBTimestamp()
	valuationCurrency := "CNY"
	require.NoError(t, DB.Model(&SubscriptionPlan{}).
		Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).
		UpdateColumn("valuation_currency", valuationCurrency).Error)
	require.NoError(t, DB.Create(&CreditValuationMigration{
		Version:           CreditValuationRuleVersion,
		Status:            CreditValuationMigrationReady,
		ValuationCurrency: valuationCurrency,
		FxRateNumerator:   1,
		FxRateDenominator: 1,
		FxCapturedAt:      now,
	}).Error)
	require.NoError(t, DB.Create(&User{
		Id: fixture.userID, Username: "structured-replay-fingerprint", Status: common.UserStatusEnabled,
	}).Error)
	plan := seedConversionQuoteTimedPlan(t, fixture.sourcePlanID, 100)
	plan.PriceAmountMicros = pointerToInt64(40_000_000)
	plan.Currency = valuationCurrency
	require.NoError(t, DB.Save(plan).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:                      fixture.sourceSubscriptionID,
		UserId:                  fixture.userID,
		PlanId:                  fixture.sourcePlanID,
		EntitlementType:         SubscriptionEntitlementTimed,
		TokenLimit:              75,
		TokenUsed:               50,
		GrantReason:             SubscriptionGrantOrder,
		Source:                  SubscriptionGrantOrder,
		StartTime:               now - 40*24*60*60,
		EndTime:                 now + TimedSubscriptionConversionBlockSeconds + 60,
		Status:                  SubscriptionStatusActive,
		LastGrantedAt:           now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(100),
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)

	result, err := ConfirmTimedSubscriptionConversion(
		fixture.userID,
		fixture.sourceSubscriptionID,
		fixture.idempotencyKey,
	)
	require.NoError(t, err)
	require.False(t, result.Replayed)
	fixture.targetSubscriptionID = result.Conversion.TargetSubscriptionId
	fixture.ledgerID = result.Conversion.LedgerId
	fixture.conversionID = result.Conversion.Id
	return fixture
}

func captureConversionReplayPersistentSnapshot(t *testing.T, fixture conversionReplayFingerprintFixture) conversionReplayPersistentSnapshot {
	t.Helper()
	snapshot := conversionReplayPersistentSnapshot{counts: captureConversionValuationWriteCounts(t)}
	require.NoError(t, DB.First(&snapshot.source, fixture.sourceSubscriptionID).Error)
	require.NoError(t, DB.First(&snapshot.target, fixture.targetSubscriptionID).Error)
	require.NoError(t, DB.First(&snapshot.ledger, fixture.ledgerID).Error)
	require.NoError(t, DB.First(&snapshot.state, fixture.targetSubscriptionID).Error)
	require.NoError(t, DB.First(&snapshot.conversion, fixture.conversionID).Error)
	return snapshot
}

func TestConfirmTimedSubscriptionConversionRejectsEveryChangedStructuredReplayFactWithoutWrites(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture conversionReplayFingerprintFixture)
	}{
		{
			name: "source quantity changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Model(&UserSubscription{}).
					Where("id = ?", fixture.sourceSubscriptionID).
					UpdateColumn("token_limit", gorm.Expr("token_limit + ?", 1)).Error)
			},
		},
		{
			name: "source conversion mapping changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Model(&UserSubscription{}).
					Where("id = ?", fixture.sourceSubscriptionID).
					UpdateColumn("converted_to_subscription_id", 0).Error)
			},
		},
		{
			name: "source plan duration changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Model(&SubscriptionPlan{}).
					Where("id = ?", fixture.sourcePlanID).
					UpdateColumn("duration_value", 2).Error)
			},
		},
		{
			name: "target subscription mapping changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Model(&UserSubscription{}).
					Where("id = ?", fixture.targetSubscriptionID).
					UpdateColumn("plan_id", fixture.sourcePlanID).Error)
			},
		},
		{
			name: "ledger gross changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Exec(
					"UPDATE credit_balance_ledgers SET gross_credit = gross_credit + 1 WHERE id = ?",
					fixture.ledgerID,
				).Error)
			},
		},
		{
			name: "ledger FX changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Exec(
					"UPDATE credit_balance_ledgers SET fx_rate_numerator = fx_rate_numerator + 1 WHERE id = ?",
					fixture.ledgerID,
				).Error)
			},
		},
		{
			name: "ledger rule changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Exec(
					"UPDATE credit_balance_ledgers SET valuation_rule_version = valuation_rule_version + 1 WHERE id = ?",
					fixture.ledgerID,
				).Error)
			},
		},
		{
			name: "ledger target changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Exec(
					"UPDATE credit_balance_ledgers SET target_plan_id = target_plan_id + 1 WHERE id = ?",
					fixture.ledgerID,
				).Error)
			},
		},
		{
			name: "conversion gross changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Exec(
					"UPDATE subscription_conversions SET gross_credit = gross_credit + 1 WHERE id = ?",
					fixture.conversionID,
				).Error)
			},
		},
		{
			name: "conversion fingerprint changed",
			mutate: func(t *testing.T, fixture conversionReplayFingerprintFixture) {
				require.NoError(t, DB.Exec(
					"UPDATE subscription_conversions SET parameter_fingerprint = ? WHERE id = ?",
					"tampered", fixture.conversionID,
				).Error)
			},
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := seedConversionReplayFingerprintFixture(t, index)
			test.mutate(t, fixture)
			before := captureConversionReplayPersistentSnapshot(t, fixture)

			result, err := ConfirmTimedSubscriptionConversion(
				fixture.userID,
				fixture.sourceSubscriptionID,
				fixture.idempotencyKey,
			)

			require.ErrorIs(t, err, ErrConversionIdempotencyConflict)
			require.Nil(t, result)
			require.Equal(t, before, captureConversionReplayPersistentSnapshot(t, fixture), "conflicting replay must not write")
		})
	}
}
