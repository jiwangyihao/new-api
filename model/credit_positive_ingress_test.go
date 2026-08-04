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

func TestAdminCreditBalanceIncreaseOffsetsDebtBeforeExactValue(t *testing.T) {
	tests := []struct {
		name          string
		debt          int64
		wantNetCredit int64
		wantNetCost   int64
		wantAvailable int64
	}{
		{name: "partial debt", debt: 300, wantNetCredit: 500, wantNetCost: 20_000_000, wantAvailable: 500},
		{name: "full debt", debt: 900, wantNetCredit: 0, wantNetCost: 0, wantAvailable: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, userID, optionPlanID := seedAdminCreditPositiveIngress(t)
			var creditPlan SubscriptionPlan
			require.NoError(t, db.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).First(&creditPlan).Error)
			balance := UserSubscription{
				UserId: userID, PlanId: creditPlan.Id, EntitlementType: SubscriptionEntitlementCreditBalance,
				Status: SubscriptionStatusActive, TokenLimit: 0, TokenUsed: test.debt,
			}
			require.NoError(t, db.Create(&balance).Error)
			now := common.GetTimestamp()
			require.NoError(t, db.Create(&CreditValuationState{
				UserSubscriptionId: balance.Id, UserId: userID, Currency: "CNY",
				RuleVersion: CreditValuationRuleVersion, CreatedAt: now, UpdatedAt: now,
			}).Error)

			result, err := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
				UserId: userID, Operation: CreditBalanceAdjustmentIncrease, Amount: 800,
				PlanId: optionPlanID, IdempotencyKey: "admin-credit-debt-" + test.name,
				OperatorUserId: 92_299, Reason: "售后补偿",
			})
			require.NoError(t, err)
			require.Equal(t, minInt64(test.debt, 800), result.CreditBalance.DebtOffset)
			require.Equal(t, test.wantNetCredit, result.CreditBalance.NetCredit)
			require.Equal(t, int64(32_000_000), result.CreditBalance.GrossAmountMicros)
			require.Equal(t, test.wantNetCost, result.CreditBalance.NetAmountMicros)
			require.Equal(t, test.wantAvailable, result.CreditBalance.AvailableCredit)
			var state CreditValuationState
			require.NoError(t, db.First(&state, balance.Id).Error)
			require.Equal(t, test.wantAvailable, state.AvailableCredit)
			require.Equal(t, test.wantNetCost, state.ExactCostMicros)
		})
	}
}

func TestAdminCreditBalanceIncreaseIdempotencyBindsCompleteSnapshot(t *testing.T) {
	db, userID, optionPlanID := seedAdminCreditPositiveIngress(t)
	request := CreditBalanceAdjustmentRequest{
		UserId: userID, Operation: CreditBalanceAdjustmentIncrease, Amount: 800,
		PlanId: optionPlanID, IdempotencyKey: "admin-credit-complete-fingerprint",
		OperatorUserId: 92_399, Reason: "售后补偿",
	}
	first, err := AdjustCreditBalance(request)
	require.NoError(t, err)
	require.False(t, first.Replayed)
	replay, err := AdjustCreditBalance(request)
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.Equal(t, first.CreditBalance.LedgerId, replay.CreditBalance.LedgerId)
	require.Equal(t, first.CreditBalance.ValuationStateVersionAfter, replay.CreditBalance.ValuationStateVersionAfter)

	mutations := []struct {
		name   string
		change func(*CreditBalanceAdjustmentRequest)
	}{
		{name: "amount", change: func(input *CreditBalanceAdjustmentRequest) { input.Amount++ }},
		{name: "plan", change: func(input *CreditBalanceAdjustmentRequest) { input.PlanId++ }},
		{name: "operation", change: func(input *CreditBalanceAdjustmentRequest) {
			input.Operation = CreditBalanceAdjustmentDecrease
			input.PlanId = 0
		}},
		{name: "reason", change: func(input *CreditBalanceAdjustmentRequest) { input.Reason = "不同原因" }},
		{name: "operator", change: func(input *CreditBalanceAdjustmentRequest) { input.OperatorUserId++ }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			conflict := request
			mutation.change(&conflict)
			result, conflictErr := AdjustCreditBalance(conflict)
			require.Nil(t, result)
			require.ErrorIs(t, conflictErr, ErrCreditValuationIdempotencyMismatch)
		})
	}

	priceMicros := int64(41_000_000)
	require.NoError(t, db.Model(&SubscriptionPlan{}).Where("id = ?", optionPlanID).Update("price_amount_micros", priceMicros).Error)
	changedPrice, err := AdjustCreditBalance(request)
	require.Nil(t, changedPrice)
	require.ErrorIs(t, err, ErrCreditValuationIdempotencyMismatch)

	var balance UserSubscription
	require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", userID, SubscriptionEntitlementCreditBalance).First(&balance).Error)
	require.Equal(t, int64(800), balance.TokenLimit)
	var state CreditValuationState
	require.NoError(t, db.First(&state, balance.Id).Error)
	require.Equal(t, int64(1), state.StateVersion)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)
}

func TestAdminCreditBalanceIncreaseLedgerFailureRollsBackEverything(t *testing.T) {
	db, userID, optionPlanID := seedAdminCreditPositiveIngress(t)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_admin_positive_ledger BEFORE INSERT ON credit_balance_ledgers WHEN NEW.source_type = 'admin_adjustment' BEGIN SELECT RAISE(FAIL, 'injected admin ledger failure'); END`).Error)
	t.Cleanup(func() { _ = db.Exec(`DROP TRIGGER IF EXISTS reject_admin_positive_ledger`).Error })

	result, err := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
		UserId: userID, Operation: CreditBalanceAdjustmentIncrease, Amount: 800,
		PlanId: optionPlanID, IdempotencyKey: "admin-credit-rollback",
		OperatorUserId: 92_499, Reason: "售后补偿",
	})
	require.Nil(t, result)
	require.ErrorContains(t, err, "injected admin ledger failure")
	for _, target := range []any{&CreditBalanceAdjustment{}, &CreditBalanceLedger{}, &CreditValuationState{}, &UserSubscription{}} {
		var count int64
		require.NoError(t, db.Model(target).Count(&count).Error)
		require.Zero(t, count)
	}
}
