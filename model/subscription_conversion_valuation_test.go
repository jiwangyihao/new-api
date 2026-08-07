package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConfirmTimedSubscriptionConversionFreezesSameCurrencyValuation(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, migrateCreditValuationSchema(DB))

	const (
		userID               = 26_101
		sourcePlanID         = 26_102
		sourceSubscriptionID = 26_103
		sourcePriceMicros    = int64(40_000_000)
		creditBasis          = int64(100)
		currentRemaining     = int64(25)
		grossCredit          = int64(125)
		grossCostMicros      = int64(50_000_000)
	)
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
		Id: userID, Username: "conversion-same-currency", Status: common.UserStatusEnabled,
	}).Error)

	plan := seedConversionQuoteTimedPlan(t, sourcePlanID, creditBasis)
	plan.PriceAmount = 40
	plan.PriceAmountMicros = pointerToInt64(sourcePriceMicros)
	plan.Currency = valuationCurrency
	require.NoError(t, DB.Save(plan).Error)

	require.NoError(t, DB.Create(&UserSubscription{
		Id:                      sourceSubscriptionID,
		UserId:                  userID,
		PlanId:                  sourcePlanID,
		EntitlementType:         SubscriptionEntitlementTimed,
		TokenLimit:              75,
		TokenUsed:               50,
		GrantReason:             SubscriptionGrantOrder,
		Source:                  SubscriptionGrantOrder,
		StartTime:               now - 40*24*60*60,
		EndTime:                 now + TimedSubscriptionConversionBlockSeconds + 60,
		Status:                  SubscriptionStatusActive,
		LastGrantedAt:           now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(creditBasis),
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)

	result, err := ConfirmTimedSubscriptionConversion(userID, sourceSubscriptionID, "same-currency-valuation")

	require.NoError(t, err)
	require.False(t, result.Replayed)
	conversion := result.Conversion
	require.Equal(t, int64(1), conversion.Full31DayBlocks)
	require.Equal(t, creditBasis, conversion.CreditBasis)
	require.Equal(t, currentRemaining, conversion.CurrentRemainingCredit)
	require.Equal(t, grossCredit, conversion.GrossCredit)
	require.Equal(t, valuationCurrency, conversion.ValuationCurrency)
	require.Equal(t, sourcePriceMicros, conversion.ValuationSourcePriceMicros)
	require.Equal(t, creditBasis, conversion.ValuationCreditBasis)
	require.Equal(t, grossCostMicros, conversion.ValuationGrossCostMicros)
	require.Equal(t, grossCostMicros, conversion.ValuationNetCostMicros)
	require.Equal(t, CreditValuationConfidenceExact, conversion.ValuationConfidence)
	require.Equal(t, CreditValuationRuleVersion, conversion.ValuationRuleVersion)
	require.Equal(t, valuationCurrency, conversion.FxSourceCurrency)
	require.Equal(t, int64(1), conversion.FxRateNumerator)
	require.Equal(t, int64(1), conversion.FxRateDenominator)
	require.Positive(t, conversion.FxCapturedAt)
	require.NotEmpty(t, conversion.ParameterFingerprint)
	require.Equal(t, int64(75), conversion.SourceTokenLimit)
	require.Equal(t, int64(50), conversion.SourceTokenUsed)
	require.Equal(t, SubscriptionDurationMonth, conversion.SourceDurationUnit)
	require.Equal(t, 1, conversion.SourceDurationValue)
	require.Equal(t, SubscriptionResetMonthly, conversion.SourceQuotaResetPeriod)
	require.Equal(t, int64(400_000), conversion.ValuationUnitValueNumeratorMicros)
	require.Equal(t, int64(1), conversion.ValuationUnitValueDenominator)

	var ledger CreditBalanceLedger
	require.NoError(t, DB.First(&ledger, conversion.LedgerId).Error)
	require.Equal(t, grossCredit, ledger.GrossCredit)
	require.Equal(t, valuationCurrency, ledger.ValuationCurrency)
	require.Equal(t, grossCostMicros, ledger.ValuationGrossCostMicros)
	require.Equal(t, grossCostMicros, ledger.ValuationNetCostMicros)
	require.Equal(t, int64(1), ledger.FxRateNumerator)
	require.Equal(t, int64(1), ledger.FxRateDenominator)
	require.Equal(t, conversion.FxCapturedAt, ledger.FxCapturedAt)
	require.Equal(t, conversion.ParameterFingerprint, ledger.ParameterFingerprint)
	require.Equal(t, sourcePlanID, ledger.SourcePlanId)
	require.Equal(t, conversion.TargetPlanId, ledger.TargetPlanId)
	require.Equal(t, int64(75), ledger.SourceTokenLimit)
	require.Equal(t, int64(50), ledger.SourceTokenUsed)
	require.Equal(t, SubscriptionStatusActive, ledger.SourceStatus)
	require.Equal(t, SubscriptionDurationMonth, ledger.SourceDurationUnit)
	require.Equal(t, 1, ledger.SourceDurationValue)
	require.Equal(t, SubscriptionResetMonthly, ledger.SourceQuotaResetPeriod)
	require.Equal(t, sourcePriceMicros, ledger.ValuationSourcePriceMicros)
	require.Equal(t, creditBasis, ledger.ValuationCreditBasis)
	require.Equal(t, int64(400_000), ledger.ValuationUnitValueNumeratorMicros)
	require.Equal(t, int64(1), ledger.ValuationUnitValueDenominator)
	require.Equal(t, grossCredit, ledger.NetGrantedCredit)

	var source UserSubscription
	require.NoError(t, DB.First(&source, sourceSubscriptionID).Error)
	require.Equal(t, SubscriptionStatusConverted, source.Status)
	require.Equal(t, conversion.Id, source.ConversionId)

	var valuation CreditValuationState
	require.NoError(t, DB.Where("user_subscription_id = ?", conversion.TargetSubscriptionId).First(&valuation).Error)
	require.Equal(t, grossCredit, valuation.AvailableCredit)
	require.Equal(t, grossCostMicros, valuation.ExactCostMicros)
}

func pointerToInt64(value int64) *int64 {
	return &value
}

func TestConfirmTimedSubscriptionConversionFreezesCrossCurrencyValuationAndReplay(t *testing.T) {
	tests := []struct {
		name                  string
		sourceCurrency        string
		valuationCurrency     string
		sourcePriceMicros     int64
		wantGrossCostMicros   int64
		wantFXRateNumerator   int64
		wantFXRateDenominator int64
	}{
		{
			name:                  "CNY to USD floors reciprocal conversion",
			sourceCurrency:        "CNY",
			valuationCurrency:     "USD",
			sourcePriceMicros:     40_000_000,
			wantGrossCostMicros:   6_849_315,
			wantFXRateNumerator:   10,
			wantFXRateDenominator: 73,
		},
		{
			name:                  "USD to CNY floors forward conversion",
			sourceCurrency:        "USD",
			valuationCurrency:     "CNY",
			sourcePriceMicros:     4_000_000,
			wantGrossCostMicros:   36_500_000,
			wantFXRateNumerator:   73,
			wantFXRateDenominator: 10,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupSubscriptionConversionQuoteTestDB(t)
			require.NoError(t, DB.AutoMigrate(&User{}))
			require.NoError(t, migrateCreditValuationSchema(DB))
			updateFXOption := seedCreditFXRateOptionForTest(t, "7.3")

			userID := 26_201 + index*10
			sourcePlanID := userID + 1
			sourceSubscriptionID := userID + 2
			const creditBasis = int64(100)
			const grossCredit = int64(125)
			now := GetDBTimestamp()

			require.NoError(t, DB.Model(&SubscriptionPlan{}).
				Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).
				UpdateColumn("valuation_currency", test.valuationCurrency).Error)
			require.NoError(t, DB.Create(&CreditValuationMigration{
				Version:           CreditValuationRuleVersion,
				Status:            CreditValuationMigrationReady,
				ValuationCurrency: test.valuationCurrency,
			}).Error)
			require.NoError(t, DB.Create(&User{
				Id: userID, Username: "conversion-cross-currency", Status: common.UserStatusEnabled,
			}).Error)

			plan := seedConversionQuoteTimedPlan(t, sourcePlanID, creditBasis)
			plan.PriceAmountMicros = pointerToInt64(test.sourcePriceMicros)
			plan.Currency = test.sourceCurrency
			require.NoError(t, DB.Save(plan).Error)
			require.NoError(t, DB.Create(&UserSubscription{
				Id:                      sourceSubscriptionID,
				UserId:                  userID,
				PlanId:                  sourcePlanID,
				EntitlementType:         SubscriptionEntitlementTimed,
				TokenLimit:              75,
				TokenUsed:               50,
				GrantReason:             SubscriptionGrantOrder,
				Source:                  SubscriptionGrantOrder,
				StartTime:               now - 40*24*60*60,
				EndTime:                 now + TimedSubscriptionConversionBlockSeconds + 60,
				Status:                  SubscriptionStatusActive,
				LastGrantedAt:           now - TimedSubscriptionConversionCooldownSeconds - 60,
				LastGrantCreditSnapshot: pointerToInt64(creditBasis),
				LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
				LastGrantSource:         SubscriptionGrantOrder,
			}).Error)

			first, err := ConfirmTimedSubscriptionConversion(userID, sourceSubscriptionID, "cross-currency-valuation")
			require.NoError(t, err)
			require.False(t, first.Replayed)
			require.Equal(t, grossCredit, first.Conversion.GrossCredit)
			require.Equal(t, test.valuationCurrency, first.Conversion.ValuationCurrency)
			require.Equal(t, test.sourcePriceMicros, first.Conversion.ValuationSourcePriceMicros)
			require.Equal(t, test.wantGrossCostMicros, first.Conversion.ValuationGrossCostMicros)
			require.Equal(t, test.sourceCurrency, first.Conversion.FxSourceCurrency)
			require.Equal(t, test.wantFXRateNumerator, first.Conversion.FxRateNumerator)
			require.Equal(t, test.wantFXRateDenominator, first.Conversion.FxRateDenominator)
			require.Positive(t, first.Conversion.FxCapturedAt)
			require.NotEmpty(t, first.Conversion.ParameterFingerprint)
			wantUnitNumerator, wantUnitDenominator, ratioErr := creditValuationUnitValueRatio(
				test.sourcePriceMicros,
				creditBasis,
				test.wantFXRateNumerator,
				test.wantFXRateDenominator,
			)
			require.NoError(t, ratioErr)
			require.Equal(t, wantUnitNumerator, first.Conversion.ValuationUnitValueNumeratorMicros)
			require.Equal(t, wantUnitDenominator, first.Conversion.ValuationUnitValueDenominator)

			var ledger CreditBalanceLedger
			require.NoError(t, DB.First(&ledger, first.Conversion.LedgerId).Error)
			require.Equal(t, test.wantGrossCostMicros, ledger.ValuationGrossCostMicros)
			require.Equal(t, test.wantFXRateNumerator, ledger.FxRateNumerator)
			require.Equal(t, test.wantFXRateDenominator, ledger.FxRateDenominator)
			require.Equal(t, first.Conversion.FxCapturedAt, ledger.FxCapturedAt)
			require.Equal(t, first.Conversion.ParameterFingerprint, ledger.ParameterFingerprint)
			require.Equal(t, first.Conversion.ValuationUnitValueNumeratorMicros, ledger.ValuationUnitValueNumeratorMicros)
			require.Equal(t, first.Conversion.ValuationUnitValueDenominator, ledger.ValuationUnitValueDenominator)

			updateFXOption("8.1")
			replayed, err := ConfirmTimedSubscriptionConversion(userID, sourceSubscriptionID, "cross-currency-valuation")
			require.NoError(t, err)
			require.True(t, replayed.Replayed)
			require.Equal(t, first.Conversion, replayed.Conversion, "committed snapshot must not follow later Option changes")

			before := captureConversionValuationWriteCounts(t)
			_, err = ConfirmTimedSubscriptionConversion(userID, sourceSubscriptionID+99, "cross-currency-valuation")
			require.Error(t, err)
			require.Equal(t, before, captureConversionValuationWriteCounts(t), "conflicting replay must produce zero writes")
		})
	}
}

type conversionValuationWriteCounts struct {
	conversions int64
	ledgers     int64
	states      int64
}

func captureConversionValuationWriteCounts(t *testing.T) conversionValuationWriteCounts {
	t.Helper()
	var counts conversionValuationWriteCounts
	require.NoError(t, DB.Model(&SubscriptionConversion{}).Count(&counts.conversions).Error)
	require.NoError(t, DB.Model(&CreditBalanceLedger{}).Count(&counts.ledgers).Error)
	require.NoError(t, DB.Model(&CreditValuationState{}).Count(&counts.states).Error)
	return counts
}

func seedCreditFXRateOptionForTest(t *testing.T, value string) func(string) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Option{}))
	var previous Option
	previousQuery := DB.Where("key = ?", "USDExchangeRate").Limit(1).Find(&previous)
	require.NoError(t, previousQuery.Error)
	existed := previousQuery.RowsAffected == 1
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	require.NoError(t, UpdateOption("USDExchangeRate", value))
	requirePersistedCreditFXRateOption(t, value)
	t.Cleanup(func() {
		if existed {
			require.NoError(t, UpdateOption("USDExchangeRate", previous.Value))
			return
		}
		require.NoError(t, DB.Where("key = ?", "USDExchangeRate").Delete(&Option{}).Error)
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, "USDExchangeRate")
		common.OptionMapRWMutex.Unlock()
	})
	return func(next string) {
		require.NoError(t, UpdateOption("USDExchangeRate", next))
		requirePersistedCreditFXRateOption(t, next)
	}
}

func requirePersistedCreditFXRateOption(t *testing.T, want string) {
	t.Helper()
	var stored Option
	require.NoError(t, DB.Where("key = ?", "USDExchangeRate").First(&stored).Error)
	require.Equal(t, want, stored.Value)
}

func TestConfirmTimedSubscriptionConversionRejectsChangedAuthoritativeFactsOnReplay(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, migrateCreditValuationSchema(DB))

	const (
		userID               = 26_301
		sourcePlanID         = 26_302
		sourceSubscriptionID = 26_303
		creditBasis          = int64(100)
	)
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
		Id: userID, Username: "conversion-fingerprint-conflict", Status: common.UserStatusEnabled,
	}).Error)

	plan := seedConversionQuoteTimedPlan(t, sourcePlanID, creditBasis)
	plan.PriceAmountMicros = pointerToInt64(40_000_000)
	plan.Currency = valuationCurrency
	require.NoError(t, DB.Save(plan).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:                      sourceSubscriptionID,
		UserId:                  userID,
		PlanId:                  sourcePlanID,
		EntitlementType:         SubscriptionEntitlementTimed,
		TokenLimit:              75,
		TokenUsed:               50,
		GrantReason:             SubscriptionGrantOrder,
		Source:                  SubscriptionGrantOrder,
		StartTime:               now - 40*24*60*60,
		EndTime:                 now + TimedSubscriptionConversionBlockSeconds + 60,
		Status:                  SubscriptionStatusActive,
		LastGrantedAt:           now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(creditBasis),
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)

	first, err := ConfirmTimedSubscriptionConversion(userID, sourceSubscriptionID, "authoritative-facts-conflict")
	require.NoError(t, err)
	require.False(t, first.Replayed)
	before := captureConversionValuationWriteCounts(t)

	require.NoError(t, DB.Model(&SubscriptionPlan{}).
		Where("id = ?", sourcePlanID).
		UpdateColumn("price_amount_micros", int64(41_000_000)).Error)

	replayed, err := ConfirmTimedSubscriptionConversion(userID, sourceSubscriptionID, "authoritative-facts-conflict")

	require.ErrorIs(t, err, ErrConversionIdempotencyConflict)
	require.Nil(t, replayed)
	require.Equal(t, before, captureConversionValuationWriteCounts(t), "conflicting replay must produce zero writes")
}

func TestConfirmTimedSubscriptionConversionFallbackRejectsChangedAuthoritativeFacts(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, migrateCreditValuationSchema(DB))

	const (
		userID               = 26_311
		sourcePlanID         = 26_312
		sourceSubscriptionID = 26_313
		creditBasis          = int64(100)
	)
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
		Id: userID, Username: "conversion-fallback-conflict", Status: common.UserStatusEnabled,
	}).Error)

	plan := seedConversionQuoteTimedPlan(t, sourcePlanID, creditBasis)
	plan.PriceAmountMicros = pointerToInt64(40_000_000)
	plan.Currency = valuationCurrency
	require.NoError(t, DB.Save(plan).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:                      sourceSubscriptionID,
		UserId:                  userID,
		PlanId:                  sourcePlanID,
		EntitlementType:         SubscriptionEntitlementTimed,
		TokenLimit:              75,
		TokenUsed:               50,
		GrantReason:             SubscriptionGrantOrder,
		Source:                  SubscriptionGrantOrder,
		StartTime:               now - 40*24*60*60,
		EndTime:                 now + TimedSubscriptionConversionBlockSeconds + 60,
		Status:                  SubscriptionStatusActive,
		LastGrantedAt:           now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(creditBasis),
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)

	first, err := ConfirmTimedSubscriptionConversion(userID, sourceSubscriptionID, "fallback-authoritative-facts-conflict")
	require.NoError(t, err)
	require.False(t, first.Replayed)
	before := captureConversionValuationWriteCounts(t)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).
		Where("id = ?", sourcePlanID).
		UpdateColumn("price_amount_micros", int64(41_000_000)).Error)

	injectedErr := errors.New("injected conversion transaction failure")
	const callbackName = "issue26:inject_conversion_transaction_failure"
	injected := false
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "User" {
			injected = true
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callbackName) })

	replayed, err := ConfirmTimedSubscriptionConversion(userID, sourceSubscriptionID, "fallback-authoritative-facts-conflict")

	require.True(t, injected, "test must drive the transaction-failure fallback")
	require.ErrorIs(t, err, ErrConversionIdempotencyConflict)
	require.Nil(t, replayed)
	require.Equal(t, before, captureConversionValuationWriteCounts(t), "conflicting fallback replay must produce zero writes")
}

func TestConfirmTimedSubscriptionConversionConcurrentSameFactsWritesOnce(t *testing.T) {
	db, _ := setupSubscriptionConversionEligibilityConcurrencyTestDB(t)
	require.NoError(t, migrateCreditValuationSchema(db))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)

	const (
		userID               = 26_401
		sourcePlanID         = 26_402
		sourceSubscriptionID = 26_403
		creditBasis          = int64(100)
	)
	now := GetDBTimestamp()
	valuationCurrency := "CNY"
	require.NoError(t, db.Model(&SubscriptionPlan{}).
		Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).
		UpdateColumn("valuation_currency", valuationCurrency).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version:           CreditValuationRuleVersion,
		Status:            CreditValuationMigrationReady,
		ValuationCurrency: valuationCurrency,
		FxRateNumerator:   1,
		FxRateDenominator: 1,
		FxCapturedAt:      now,
	}).Error)
	require.NoError(t, db.Create(&User{
		Id: userID, Username: "conversion-concurrent-replay", Status: common.UserStatusEnabled,
	}).Error)

	plan := seedConversionQuoteTimedPlan(t, sourcePlanID, creditBasis)
	plan.PriceAmountMicros = pointerToInt64(40_000_000)
	plan.Currency = valuationCurrency
	require.NoError(t, db.Save(plan).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id:                      sourceSubscriptionID,
		UserId:                  userID,
		PlanId:                  sourcePlanID,
		EntitlementType:         SubscriptionEntitlementTimed,
		TokenLimit:              75,
		TokenUsed:               50,
		GrantReason:             SubscriptionGrantOrder,
		Source:                  SubscriptionGrantOrder,
		StartTime:               now - 40*24*60*60,
		EndTime:                 now + TimedSubscriptionConversionBlockSeconds + 60,
		Status:                  SubscriptionStatusActive,
		LastGrantedAt:           now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(creditBasis),
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)

	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make([]*SubscriptionConversionResult, 2)
	errorsByWorker := make([]error, 2)
	var workers sync.WaitGroup
	workers.Add(len(results))
	for index := range results {
		go func() {
			defer workers.Done()
			var signalOnce sync.Once
			hooks := &subscriptionConversionHooks{at: func(phase subscriptionConversionHookPhase) error {
				if phase == subscriptionConversionAfterQuotePhase {
					signalOnce.Do(func() { arrived <- struct{}{} })
					<-release
				}
				return nil
			}}
			results[index], errorsByWorker[index] = confirmTimedSubscriptionConversion(
				userID,
				sourceSubscriptionID,
				"concurrent-same-facts",
				hooks,
			)
		}()
	}
	<-arrived
	<-arrived
	close(release)
	workers.Wait()

	replayCount := 0
	for index := range results {
		require.NoError(t, errorsByWorker[index])
		require.NotNil(t, results[index])
		if results[index].Replayed {
			replayCount++
		}
	}
	require.Equal(t, 1, replayCount)
	require.Equal(t, results[0].Conversion, results[1].Conversion)
	require.Equal(t, conversionValuationWriteCounts{conversions: 1, ledgers: 1, states: 1}, captureConversionValuationWriteCounts(t))

	var sourceCount int64
	require.NoError(t, db.Model(&UserSubscription{}).Where("id = ?", sourceSubscriptionID).Count(&sourceCount).Error)
	require.Equal(t, int64(1), sourceCount)
	var source UserSubscription
	require.NoError(t, db.First(&source, sourceSubscriptionID).Error)
	require.Equal(t, SubscriptionStatusConverted, source.Status)
	require.Equal(t, results[0].Conversion.Id, source.ConversionId)
}

func TestConfirmTimedSubscriptionConversionLocksInFlightRequestsBeforeTargetIngress(t *testing.T) {
	db, observerDB := setupSubscriptionConversionEligibilityConcurrencyTestDB(t)
	require.NoError(t, migrateCreditValuationSchema(db))
	primarySQL, err := db.DB()
	require.NoError(t, err)
	observerSQL, err := observerDB.DB()
	require.NoError(t, err)
	require.NotSame(t, primarySQL, observerSQL)
	var journalMode string
	require.NoError(t, observerDB.Raw("PRAGMA journal_mode").Scan(&journalMode).Error)
	require.Equal(t, "wal", journalMode)

	const (
		userID        = 26_451
		sourcePlanID  = 26_452
		sourceID      = 26_453
		creditBasis   = int64(100)
		requestID     = "conversion-lock-order-request"
		reserveCredit = int64(10)
	)
	now := GetDBTimestamp()
	valuationCurrency := "CNY"
	require.NoError(t, db.Model(&SubscriptionPlan{}).
		Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).
		UpdateColumn("valuation_currency", valuationCurrency).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version:           CreditValuationRuleVersion,
		Status:            CreditValuationMigrationReady,
		ValuationCurrency: valuationCurrency,
		FxRateNumerator:   1,
		FxRateDenominator: 1,
		FxCapturedAt:      now,
	}).Error)
	user := User{Id: userID, Username: "conversion-lock-order", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = sourceID
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	user.SetSetting(setting)
	require.NoError(t, db.Create(&user).Error)
	plan := seedConversionQuoteTimedPlan(t, sourcePlanID, creditBasis)
	plan.PriceAmountMicros = pointerToInt64(40_000_000)
	plan.Currency = valuationCurrency
	require.NoError(t, db.Save(plan).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id:                      sourceID,
		UserId:                  userID,
		PlanId:                  sourcePlanID,
		EntitlementType:         SubscriptionEntitlementTimed,
		TokenLimit:              creditBasis,
		GrantReason:             SubscriptionGrantOrder,
		Source:                  SubscriptionGrantOrder,
		StartTime:               now - 40*24*60*60,
		EndTime:                 now + TimedSubscriptionConversionBlockSeconds + 60,
		Status:                  SubscriptionStatusActive,
		LastGrantedAt:           now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(creditBasis),
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)
	reserved, err := PreConsumeUserSubscriptionByUnits(requestID, userID, "gpt-4o", 0, 0, reserveCredit)
	require.NoError(t, err)
	require.Equal(t, sourceID, reserved.UserSubscriptionId)

	var observationMu sync.Mutex
	requestRowsSelected := false
	firstTargetMutationBeforeRequest := ""
	targetMutationAttempted := make(chan struct{})
	releaseTargetMutation := make(chan struct{})
	var targetSignalOnce sync.Once

	const requestQueryCallback = "issue26:observe_request_before_target_query"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(requestQueryCallback, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "SubscriptionPreConsumeRecord" || tx.Error != nil || tx.RowsAffected == 0 {
			return
		}
		observationMu.Lock()
		requestRowsSelected = true
		observationMu.Unlock()
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(requestQueryCallback) })

	observeTargetMutation := func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil {
			return
		}
		schemaName := tx.Statement.Schema.Name
		switch schemaName {
		case "UserSubscription", "CreditValuationState", "CreditBalanceLedger", "SubscriptionConversion":
		default:
			return
		}
		observationMu.Lock()
		beforeRequest := !requestRowsSelected
		if beforeRequest && firstTargetMutationBeforeRequest == "" {
			firstTargetMutationBeforeRequest = schemaName
		}
		observationMu.Unlock()
		if beforeRequest {
			targetSignalOnce.Do(func() { close(targetMutationAttempted) })
			<-releaseTargetMutation
		}
	}
	const targetCreateCallback = "issue26:observe_target_before_request_create"
	const targetUpdateCallback = "issue26:observe_target_before_request_update"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(targetCreateCallback, observeTargetMutation))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(targetUpdateCallback, observeTargetMutation))
	t.Cleanup(func() {
		_ = db.Callback().Create().Remove(targetCreateCallback)
		_ = db.Callback().Update().Remove(targetUpdateCallback)
	})

	type conversionOutcome struct {
		result *SubscriptionConversionResult
		err    error
	}
	done := make(chan conversionOutcome, 1)
	go func() {
		result, conversionErr := ConfirmTimedSubscriptionConversion(userID, sourceID, "conversion-lock-order")
		done <- conversionOutcome{result: result, err: conversionErr}
	}()

	var outcome conversionOutcome
	select {
	case <-targetMutationAttempted:
		var observedRequest SubscriptionPreConsumeRecord
		require.NoError(t, observerDB.Where("request_id = ?", requestID).First(&observedRequest).Error)
		require.Zero(t, observedRequest.ValuationSubscriptionId, "independent WAL reader must still observe an unfrozen request when target ingress starts early")
		require.Zero(t, observedRequest.AppliedCredit)
		close(releaseTargetMutation)
		outcome = <-done
	case outcome = <-done:
		close(releaseTargetMutation)
	}
	require.NoError(t, outcome.err)
	require.NotNil(t, outcome.result)

	observationMu.Lock()
	prematureTarget := firstTargetMutationBeforeRequest
	selected := requestRowsSelected
	observationMu.Unlock()
	require.True(t, selected, "Confirm must execute the in-flight request selection")
	require.Empty(t, prematureTarget, "target mutation %s started before Confirm selected and validated its in-flight requests", prematureTarget)
}

func TestTimedConversionConcurrentWithFinalSettleUsesLegalSerialization(t *testing.T) {
	testTimedConversionConcurrentRequestTarget(t, 10, "settled", 185, 74_000_000)
}

func TestTimedConversionConcurrentWithFullRefundUsesLegalSerialization(t *testing.T) {
	testTimedConversionConcurrentRequestTarget(t, 0, "refunded", 195, 78_000_000)
}

func testTimedConversionConcurrentRequestTarget(t *testing.T, targetCredit int64, wantStatus string, wantTargetLimit int64, wantExactCostMicros int64) {
	t.Helper()
	db, settlementDB := setupSubscriptionConversionEligibilityConcurrencyTestDB(t)
	require.NoError(t, migrateCreditValuationSchema(db))
	primarySQL, err := db.DB()
	require.NoError(t, err)
	settlementSQL, err := settlementDB.DB()
	require.NoError(t, err)
	require.NotSame(t, primarySQL, settlementSQL)
	var primaryJournalMode string
	require.NoError(t, db.Raw("PRAGMA journal_mode").Scan(&primaryJournalMode).Error)
	require.Equal(t, "wal", primaryJournalMode)
	var settlementJournalMode string
	require.NoError(t, settlementDB.Raw("PRAGMA journal_mode").Scan(&settlementJournalMode).Error)
	require.Equal(t, "wal", settlementJournalMode)

	const (
		userID        = 26_501
		sourcePlanID  = 26_502
		sourceID      = 26_503
		creditBasis   = int64(100)
		reserveCredit = int64(10)
		otherCredit   = int64(5)
		requestID     = "conversion-concurrent-request-target"
		otherRequest  = "conversion-concurrent-other-request"
	)
	now := GetDBTimestamp()
	valuationCurrency := "CNY"
	require.NoError(t, db.Model(&SubscriptionPlan{}).
		Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).
		UpdateColumn("valuation_currency", valuationCurrency).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version:           CreditValuationRuleVersion,
		Status:            CreditValuationMigrationReady,
		ValuationCurrency: valuationCurrency,
		FxRateNumerator:   1,
		FxRateDenominator: 1,
		FxCapturedAt:      now,
	}).Error)
	user := User{Id: userID, Username: "conversion-request-concurrency", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = sourceID
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	user.SetSetting(setting)
	require.NoError(t, db.Create(&user).Error)
	plan := seedConversionQuoteTimedPlan(t, sourcePlanID, creditBasis)
	plan.PriceAmountMicros = pointerToInt64(40_000_000)
	plan.Currency = valuationCurrency
	require.NoError(t, db.Save(plan).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id:                      sourceID,
		UserId:                  userID,
		PlanId:                  sourcePlanID,
		EntitlementType:         SubscriptionEntitlementTimed,
		TokenLimit:              creditBasis,
		GrantReason:             SubscriptionGrantOrder,
		Source:                  SubscriptionGrantOrder,
		StartTime:               now - 40*24*60*60,
		EndTime:                 now + TimedSubscriptionConversionBlockSeconds + 60,
		Status:                  SubscriptionStatusActive,
		LastGrantedAt:           now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(creditBasis),
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)
	reserved, err := PreConsumeUserSubscriptionByUnits(requestID, userID, "gpt-4o", 0, 0, reserveCredit)
	require.NoError(t, err)
	require.Equal(t, sourceID, reserved.UserSubscriptionId)
	other, err := PreConsumeUserSubscriptionByUnits(otherRequest, userID, "gpt-4o", 0, 0, otherCredit)
	require.NoError(t, err)
	require.Equal(t, sourceID, other.UserSubscriptionId)

	settleOnIndependentConnection := func(target int64) error {
		tx := settlementDB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		var route SubscriptionPreConsumeRecord
		if err := tx.Where("request_id = ?", requestID).First(&route).Error; err != nil {
			_ = tx.Rollback().Error
			return err
		}
		if route.UserSubscriptionId != sourceID {
			_ = tx.Rollback().Error
			return ErrCreditValuationMappingConflict
		}
		if err := SettleCreditRequestTargetTx(tx, &route, target, true); err != nil {
			_ = tx.Rollback().Error
			return err
		}
		return tx.Commit().Error
	}

	arrived := make(chan struct{})
	release := make(chan struct{})
	var arrivedOnce sync.Once
	hooks := &subscriptionConversionHooks{at: func(phase subscriptionConversionHookPhase) error {
		if phase == subscriptionConversionAfterEligibilityGuardPhase {
			arrivedOnce.Do(func() { close(arrived) })
			<-release
		}
		return nil
	}}
	type conversionOutcome struct {
		result *SubscriptionConversionResult
		err    error
	}
	conversionDone := make(chan conversionOutcome, 1)
	go func() {
		result, conversionErr := confirmTimedSubscriptionConversion(userID, sourceID, "conversion-request-concurrency", hooks)
		conversionDone <- conversionOutcome{result: result, err: conversionErr}
	}()
	<-arrived

	var beforeConcurrentAttempt SubscriptionPreConsumeRecord
	require.NoError(t, settlementDB.Where("request_id = ?", requestID).First(&beforeConcurrentAttempt).Error)
	concurrentErr := settleOnIndependentConnection(targetCredit)
	require.ErrorIs(t, concurrentErr, ErrCreditValuationStateMismatch)
	require.Equal(t, ErrCreditValuationStateMismatch.Error(), concurrentErr.Error(), "SQLite lock or free-text errors must not cross the domain seam")
	var afterConcurrentAttempt SubscriptionPreConsumeRecord
	require.NoError(t, settlementDB.Where("request_id = ?", requestID).First(&afterConcurrentAttempt).Error)
	require.Equal(t, beforeConcurrentAttempt, afterConcurrentAttempt, "the pre-conversion serialization must write nothing")

	close(release)
	converted := <-conversionDone
	require.NoError(t, converted.err)
	require.NotNil(t, converted.result)
	require.False(t, converted.result.Replayed)
	targetID := converted.result.Conversion.TargetSubscriptionId
	require.Positive(t, targetID)

	var otherFrozen SubscriptionPreConsumeRecord
	require.NoError(t, settlementDB.Where("request_id = ?", otherRequest).First(&otherFrozen).Error)
	require.Equal(t, sourceID, otherFrozen.UserSubscriptionId)
	require.Equal(t, targetID, otherFrozen.ValuationSubscriptionId)
	require.Equal(t, otherCredit, otherFrozen.AppliedCredit)
	require.Equal(t, otherCredit, otherFrozen.DeductedAvailableCredit)
	require.Equal(t, int64(2_000_000), otherFrozen.DeductedExactCostMicros)

	require.NoError(t, settleOnIndependentConnection(targetCredit))
	var terminal SubscriptionPreConsumeRecord
	require.NoError(t, settlementDB.Where("request_id = ?", requestID).First(&terminal).Error)
	require.Equal(t, wantStatus, terminal.Status)
	require.Equal(t, sourceID, terminal.UserSubscriptionId)
	require.Equal(t, targetID, terminal.ValuationSubscriptionId)
	require.Equal(t, targetCredit, terminal.AppliedCredit)
	require.Positive(t, terminal.FinalizedAt)

	type entityCounts struct {
		conversions int64
		sources     int64
		targets     int64
		ledgers     int64
		states      int64
		requests    int64
	}
	loadCounts := func() entityCounts {
		var counts entityCounts
		require.NoError(t, settlementDB.Model(&SubscriptionConversion{}).Where("source_subscription_id = ?", sourceID).Count(&counts.conversions).Error)
		require.NoError(t, settlementDB.Model(&UserSubscription{}).Where("id = ?", sourceID).Count(&counts.sources).Error)
		require.NoError(t, settlementDB.Model(&UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", userID, SubscriptionEntitlementCreditBalance).Count(&counts.targets).Error)
		require.NoError(t, settlementDB.Model(&CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionConversion, sourceID).Count(&counts.ledgers).Error)
		require.NoError(t, settlementDB.Model(&CreditValuationState{}).Where("user_id = ?", userID).Count(&counts.states).Error)
		require.NoError(t, settlementDB.Model(&SubscriptionPreConsumeRecord{}).Where("user_id = ?", userID).Count(&counts.requests).Error)
		return counts
	}
	loadState := func() CreditValuationState {
		var state CreditValuationState
		require.NoError(t, settlementDB.Where("user_subscription_id = ?", targetID).First(&state).Error)
		return state
	}
	countsBeforeReplay := loadCounts()
	stateVersionBeforeReplay := loadState().StateVersion
	settlementVersionBeforeReplay := terminal.SettlementVersion
	require.NoError(t, settleOnIndependentConnection(targetCredit))
	replayedConversion, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "conversion-request-concurrency")
	require.NoError(t, err)
	require.True(t, replayedConversion.Replayed)
	require.Equal(t, converted.result.Conversion, replayedConversion.Conversion)
	require.Equal(t, countsBeforeReplay, loadCounts())
	require.Equal(t, stateVersionBeforeReplay, loadState().StateVersion)
	var replayedTerminal SubscriptionPreConsumeRecord
	require.NoError(t, settlementDB.Where("request_id = ?", requestID).First(&replayedTerminal).Error)
	require.Equal(t, settlementVersionBeforeReplay, replayedTerminal.SettlementVersion)
	require.Equal(t, terminal.FinalizedAt, replayedTerminal.FinalizedAt)

	require.Equal(t, entityCounts{conversions: 1, sources: 1, targets: 1, ledgers: 1, states: 1, requests: 2}, loadCounts())
	var source UserSubscription
	require.NoError(t, settlementDB.First(&source, sourceID).Error)
	require.Equal(t, SubscriptionStatusConverted, source.Status)
	require.Equal(t, reserveCredit+otherCredit, source.TokenUsed)
	require.Equal(t, converted.result.Conversion.Id, source.ConversionId)
	require.Equal(t, targetID, source.ConvertedToSubscriptionId)
	var target UserSubscription
	require.NoError(t, settlementDB.First(&target, targetID).Error)
	require.Equal(t, wantTargetLimit, target.TokenLimit)
	require.Zero(t, target.TokenUsed)
	state := loadState()
	require.Equal(t, wantTargetLimit, state.AvailableCredit)
	require.Equal(t, wantExactCostMicros, state.ExactCostMicros)
	require.Equal(t, valuationCurrency, state.Currency)

	var otherAfter SubscriptionPreConsumeRecord
	require.NoError(t, settlementDB.Where("request_id = ?", otherRequest).First(&otherAfter).Error)
	require.Equal(t, otherFrozen.UserSubscriptionId, otherAfter.UserSubscriptionId)
	require.Equal(t, otherFrozen.ValuationSubscriptionId, otherAfter.ValuationSubscriptionId)
	require.Equal(t, otherFrozen.AppliedCredit, otherAfter.AppliedCredit)
	require.Equal(t, otherFrozen.DeductedAvailableCredit, otherAfter.DeductedAvailableCredit)
	require.Equal(t, otherFrozen.DeductedExactCostMicros, otherAfter.DeductedExactCostMicros)
	require.Equal(t, otherFrozen.SettlementVersion, otherAfter.SettlementVersion)
	require.Equal(t, "consumed", otherAfter.Status)
	require.Zero(t, otherAfter.FinalizedAt)

	conversion := converted.result.Conversion
	require.Equal(t, int64(185), conversion.GrossCredit)
	require.Equal(t, int64(74_000_000), conversion.ValuationGrossCostMicros)
	require.Equal(t, int64(40_000_000), conversion.ValuationSourcePriceMicros)
	require.Equal(t, creditBasis, conversion.ValuationCreditBasis)
	require.Equal(t, CreditValuationRuleVersion, conversion.ValuationRuleVersion)
	require.Equal(t, int64(1), conversion.FxRateNumerator)
	require.Equal(t, int64(1), conversion.FxRateDenominator)
}
