package model

import (
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
