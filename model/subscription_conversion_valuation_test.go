package model

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
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

	var ledger CreditBalanceLedger
	require.NoError(t, DB.First(&ledger, conversion.LedgerId).Error)
	require.Equal(t, grossCredit, ledger.GrossCredit)
	require.Equal(t, valuationCurrency, ledger.ValuationCurrency)
	require.Equal(t, grossCostMicros, ledger.ValuationGrossCostMicros)
	require.Equal(t, grossCostMicros, ledger.ValuationNetCostMicros)
	require.Equal(t, int64(1), ledger.FxRateNumerator)
	require.Equal(t, int64(1), ledger.FxRateDenominator)
	require.Equal(t, conversion.FxCapturedAt, ledger.FxCapturedAt)

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

			var ledger CreditBalanceLedger
			require.NoError(t, DB.First(&ledger, first.Conversion.LedgerId).Error)
			require.Equal(t, test.wantGrossCostMicros, ledger.ValuationGrossCostMicros)
			require.Equal(t, test.wantFXRateNumerator, ledger.FxRateNumerator)
			require.Equal(t, test.wantFXRateDenominator, ledger.FxRateDenominator)
			require.Equal(t, first.Conversion.FxCapturedAt, ledger.FxCapturedAt)

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
