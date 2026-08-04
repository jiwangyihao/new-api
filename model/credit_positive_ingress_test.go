package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func seedAdminCreditPositiveIngress(t *testing.T) (*gorm.DB, int, int) {
	t.Helper()
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&CreditBalanceAdjustment{}, &InvitationRewardEvent{}))
	const userID = 92_101
	const optionPlanID = 92_102
	const creditPlanID = 92_103
	priceMicros := int64(40_000_000)
	valuationCurrency := "CNY"
	optionCode := "admin-credit-eligibility-option"
	creditCode := "admin-credit-eligibility-pool"
	require.NoError(t, db.Create(&User{Id: userID, Username: "admin-credit-eligibility", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: optionPlanID, Title: "40 CNY / 1,000 Credit", PriceAmount: 40,
		PriceAmountMicros: &priceMicros, Currency: "CNY", Enabled: true,
		EntitlementType: SubscriptionEntitlementTimed, MonthlyTokenLimit: 1_000,
		UnlimitedPurchaseEnabled: true, BusinessCode: &optionCode,
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: creditPlanID, Title: "Credit balance", Currency: "CNY",
		ValuationCurrency: &valuationCurrency, Enabled: true,
		CreditBalanceConfigured: true, EntitlementType: SubscriptionEntitlementCreditBalance,
		BusinessCode: &creditCode,
	}).Error)
	return db, userID, optionPlanID
}

func TestAdminCreditBalanceIncreaseRejectsIneligiblePlansAtomically(t *testing.T) {
	tests := []struct {
		name    string
		planId  func(int) int
		mutate  map[string]any
		wantErr error
	}{
		{name: "missing plan", planId: func(int) int { return 0 }, wantErr: ErrCreditValuationPlanRequired},
		{name: "disabled", mutate: map[string]any{"enabled": false}, wantErr: ErrCreditValuationPlanIneligible},
		{name: "trial", mutate: map[string]any{"is_trial": true}, wantErr: ErrCreditValuationPlanIneligible},
		{name: "invite trial", mutate: map[string]any{"invite_trial": true}, wantErr: ErrCreditValuationPlanIneligible},
		{name: "zero exact price", mutate: map[string]any{"price_amount_micros": 0}, wantErr: ErrCreditValuationPlanIneligible},
		{name: "missing exact price", mutate: map[string]any{"price_amount_micros": nil}, wantErr: ErrCreditValuationPlanIneligible},
		{name: "zero Credit denominator", mutate: map[string]any{"monthly_token_limit": 0}, wantErr: ErrCreditValuationPlanIneligible},
		{name: "purchase disabled", mutate: map[string]any{"unlimited_purchase_enabled": false}, wantErr: ErrCreditValuationPlanIneligible},
		{name: "not timed", mutate: map[string]any{"entitlement_type": SubscriptionEntitlementCreditBalance}, wantErr: ErrCreditValuationPlanIneligible},
		{name: "unsupported currency", mutate: map[string]any{"currency": "EUR"}, wantErr: ErrCreditValuationUnsupportedCurrency},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, userID, optionPlanID := seedAdminCreditPositiveIngress(t)
			if len(test.mutate) > 0 {
				require.NoError(t, db.Model(&SubscriptionPlan{}).Where("id = ?", optionPlanID).Updates(test.mutate).Error)
			}
			planId := optionPlanID
			if test.planId != nil {
				planId = test.planId(optionPlanID)
			}

			result, err := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
				UserId: userID, Operation: CreditBalanceAdjustmentIncrease, Amount: 800,
				PlanId: planId, IdempotencyKey: "admin-credit-ineligible-" + test.name,
				OperatorUserId: 92_199, Reason: "售后补偿",
			})

			require.Nil(t, result)
			require.ErrorIs(t, err, test.wantErr)
			for _, target := range []any{&CreditBalanceAdjustment{}, &CreditBalanceLedger{}, &CreditValuationState{}, &UserSubscription{}, &InvitationRewardEvent{}} {
				var count int64
				require.NoError(t, db.Model(target).Count(&count).Error)
				require.Zero(t, count)
			}
		})
	}
}
