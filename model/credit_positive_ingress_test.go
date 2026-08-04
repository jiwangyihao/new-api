package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestAdminCreditBalanceIncreaseUsesSelectedPlanExactIngress(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&CreditBalanceAdjustment{}))

	const userID = 92_001
	const optionPlanID = 92_002
	const creditPlanID = 92_003
	priceMicros := int64(40_000_000)
	valuationCurrency := "CNY"
	optionCode := "admin-credit-positive-option"
	creditCode := "admin-credit-positive-pool"
	require.NoError(t, db.Create(&User{
		Id: userID, Username: "admin-credit-positive", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: optionPlanID, Title: "40 CNY / 1,000 Credit", PriceAmount: 40,
		PriceAmountMicros: &priceMicros, Currency: "CNY", Enabled: true,
		EntitlementType: SubscriptionEntitlementTimed, DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, QuotaResetPeriod: SubscriptionResetMonthly,
		MonthlyTokenLimit: 1_000, UnlimitedPurchaseEnabled: true, BusinessCode: &optionCode,
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: creditPlanID, Title: "Credit balance", Currency: "CNY",
		ValuationCurrency: &valuationCurrency, Enabled: true,
		CreditBalanceConfigured: true, EntitlementType: SubscriptionEntitlementCreditBalance,
		BusinessCode: &creditCode,
	}).Error)

	result, err := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
		UserId: userID, Operation: CreditBalanceAdjustmentIncrease, Amount: 800,
		PlanId: optionPlanID, IdempotencyKey: "admin-credit-positive-800",
		OperatorUserId: 92_099, Reason: "售后补偿",
	})
	require.NoError(t, err)
	require.False(t, result.Replayed)
	require.Equal(t, optionPlanID, result.Adjustment.PlanId)
	require.Equal(t, int64(800), result.CreditBalance.GrossCredit)
	require.Equal(t, int64(800), result.CreditBalance.NetCredit)
	require.Equal(t, int64(32_000_000), result.CreditBalance.GrossAmountMicros)
	require.Equal(t, int64(32_000_000), result.CreditBalance.NetAmountMicros)
	require.Equal(t, "CNY", result.CreditBalance.ValuationCurrency)
	require.Equal(t, "CNY", result.CreditBalance.SourceCurrency)
	require.Equal(t, CreditValuationConfidenceExact, result.CreditBalance.ValuationConfidence)
	require.Equal(t, int64(1), result.CreditBalance.FxRateNumerator)
	require.Equal(t, int64(1), result.CreditBalance.FxRateDenominator)
	require.Equal(t, CreditValuationRuleVersion, result.CreditBalance.ValuationRuleVersion)
	require.Equal(t, int64(1), result.CreditBalance.ValuationStateVersionAfter)

	var state CreditValuationState
	require.NoError(t, db.First(&state, result.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(800), state.AvailableCredit)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)

	var ledger CreditBalanceLedger
	require.NoError(t, db.First(&ledger, result.CreditBalance.LedgerId).Error)
	require.Equal(t, optionPlanID, ledger.PlanId)
	require.Equal(t, int64(800), ledger.NetCredit)
	require.Equal(t, int64(40_000_000), ledger.SourcePriceMicros)
	require.Equal(t, int64(1_000), ledger.SourcePlanCredit)
	require.Equal(t, "admin_adjustment", ledger.SourceType)
	require.Equal(t, "admin_adjustment:"+result.Adjustment.IdempotencyKey, ledger.SourceKey)
	require.Equal(t, "completed", ledger.SourceStatus)
	require.Equal(t, result.Adjustment.ParameterFingerprint, ledger.ParameterFingerprint)
	require.Equal(t, result.Adjustment.CreatedAt, ledger.FxCapturedAt)
}
