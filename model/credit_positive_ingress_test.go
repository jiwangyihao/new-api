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
	require.NoError(t, err)
	require.NotNil(t, changedPrice)
	require.True(t, changedPrice.Replayed)
	require.Equal(t, first.CreditBalance.GrossAmountMicros, changedPrice.CreditBalance.GrossAmountMicros)
	require.Equal(t, first.CreditBalance.ValuationStateVersionAfter, changedPrice.CreditBalance.ValuationStateVersionAfter)

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

func TestAdminCreditBalanceIncreaseUsesFrozenFXSnapshotBothDirections(t *testing.T) {
	tests := []struct {
		name              string
		sourceCurrency    string
		valuationCurrency string
		wantNumerator     int64
		wantDenominator   int64
		wantDirection     string
		wantGrossMicros   int64
	}{
		{name: "CNY to USD", sourceCurrency: "CNY", valuationCurrency: "USD", wantNumerator: 10, wantDenominator: 73, wantDirection: CreditFXDirectionCNYtoUSD, wantGrossMicros: 4_383_561},
		{name: "USD to CNY", sourceCurrency: "USD", valuationCurrency: "CNY", wantNumerator: 73, wantDenominator: 10, wantDirection: CreditFXDirectionUSDtoCNY, wantGrossMicros: 233_600_000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalFX := runtimeCreditFXRateSnapshot.Load()
			t.Cleanup(func() { runtimeCreditFXRateSnapshot.Store(originalFX) })
			frozenFX, err := prepareCreditFXRateSnapshot("7.3", 1_800_000_300)
			require.NoError(t, err)
			publishCreditFXRateSnapshot(frozenFX)

			db, userID, optionPlanID := seedAdminCreditPositiveIngress(t)
			require.NoError(t, db.Model(&SubscriptionPlan{}).Where("id = ?", optionPlanID).Update("currency", test.sourceCurrency).Error)
			require.NoError(t, db.Model(&SubscriptionPlan{}).Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Updates(map[string]any{
				"currency": test.valuationCurrency, "valuation_currency": test.valuationCurrency,
			}).Error)

			result, err := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
				UserId: userID, Operation: CreditBalanceAdjustmentIncrease, Amount: 800,
				PlanId: optionPlanID, IdempotencyKey: "admin-credit-cross-" + test.name,
				OperatorUserId: 92_599, Reason: "跨币种售后补偿",
			})
			require.NoError(t, err)
			require.False(t, result.Replayed)
			require.Equal(t, test.sourceCurrency, result.CreditBalance.SourceCurrency)
			require.Equal(t, test.valuationCurrency, result.CreditBalance.ValuationCurrency)
			require.Equal(t, test.wantNumerator, result.CreditBalance.FxRateNumerator)
			require.Equal(t, test.wantDenominator, result.CreditBalance.FxRateDenominator)
			require.Equal(t, int64(1_800_000_300), result.CreditBalance.FxCapturedAt)
			require.Equal(t, test.wantDirection, result.CreditBalance.FxDirection)
			require.Equal(t, test.wantGrossMicros, result.CreditBalance.GrossAmountMicros)
			require.Equal(t, test.wantGrossMicros, result.CreditBalance.NetAmountMicros)

			var ledger CreditBalanceLedger
			require.NoError(t, db.First(&ledger, result.CreditBalance.LedgerId).Error)
			require.Equal(t, optionPlanID, ledger.PlanId)
			require.Equal(t, int64(800), ledger.GrossCredit)
			require.Equal(t, int64(800), ledger.NetCredit)
			require.Equal(t, int64(40_000_000), ledger.SourcePriceMicros)
			require.Equal(t, int64(1_000), ledger.SourcePlanCredit)
			require.Equal(t, test.sourceCurrency, ledger.FxSourceCurrency)
			require.Equal(t, test.valuationCurrency, ledger.ValuationCurrency)
			require.Equal(t, test.wantNumerator, ledger.FxRateNumerator)
			require.Equal(t, test.wantDenominator, ledger.FxRateDenominator)
			require.Equal(t, int64(1_800_000_300), ledger.FxCapturedAt)
			require.Equal(t, test.wantDirection, ledger.FxDirection)
		})
	}
}

func TestAdminCreditBalanceIncreaseReplayKeepsFrozenFXAfterOptionChange(t *testing.T) {
	originalFX := runtimeCreditFXRateSnapshot.Load()
	t.Cleanup(func() { runtimeCreditFXRateSnapshot.Store(originalFX) })
	frozenFX, err := prepareCreditFXRateSnapshot("7.3", 1_800_000_301)
	require.NoError(t, err)
	publishCreditFXRateSnapshot(frozenFX)
	db, userID, optionPlanID := seedAdminCreditPositiveIngress(t)
	valuationCurrency := "USD"
	require.NoError(t, db.Model(&SubscriptionPlan{}).Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Updates(map[string]any{
		"currency": valuationCurrency, "valuation_currency": valuationCurrency,
	}).Error)
	request := CreditBalanceAdjustmentRequest{
		UserId: userID, Operation: CreditBalanceAdjustmentIncrease, Amount: 800,
		PlanId: optionPlanID, IdempotencyKey: "admin-credit-frozen-fx-replay",
		OperatorUserId: 92_699, Reason: "跨币种售后补偿",
	}

	first, err := AdjustCreditBalance(request)
	require.NoError(t, err)
	updatedFX, err := prepareCreditFXRateSnapshot("8.1", 1_800_000_302)
	require.NoError(t, err)
	publishCreditFXRateSnapshot(updatedFX)
	replay, err := AdjustCreditBalance(request)
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.True(t, replay.CreditBalance.Replayed)
	require.Equal(t, first.CreditBalance.LedgerId, replay.CreditBalance.LedgerId)
	require.Equal(t, first.CreditBalance.GrossAmountMicros, replay.CreditBalance.GrossAmountMicros)
	require.Equal(t, first.CreditBalance.FxRateNumerator, replay.CreditBalance.FxRateNumerator)
	require.Equal(t, first.CreditBalance.FxRateDenominator, replay.CreditBalance.FxRateDenominator)
	require.Equal(t, first.CreditBalance.FxCapturedAt, replay.CreditBalance.FxCapturedAt)
	require.Equal(t, first.CreditBalance.ValuationStateVersionAfter, replay.CreditBalance.ValuationStateVersionAfter)
	var ledgerCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Where("source_type = ?", CreditBalanceLedgerSourceAdminAdjustment).Count(&ledgerCount).Error)
	require.Equal(t, int64(1), ledgerCount)
}

func TestAdminCreditBalanceIncreaseRejectsInvalidFXAtomically(t *testing.T) {
	originalFX := runtimeCreditFXRateSnapshot.Load()
	t.Cleanup(func() { runtimeCreditFXRateSnapshot.Store(originalFX) })
	runtimeCreditFXRateSnapshot.Store(nil)
	db, userID, optionPlanID := seedAdminCreditPositiveIngress(t)
	valuationCurrency := "USD"
	require.NoError(t, db.Model(&SubscriptionPlan{}).Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Updates(map[string]any{
		"currency": valuationCurrency, "valuation_currency": valuationCurrency,
	}).Error)

	result, err := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
		UserId: userID, Operation: CreditBalanceAdjustmentIncrease, Amount: 800,
		PlanId: optionPlanID, IdempotencyKey: "admin-credit-invalid-fx",
		OperatorUserId: 92_799, Reason: "跨币种售后补偿",
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrCreditValuationInvalidFX)
	for _, target := range []any{&CreditBalanceAdjustment{}, &CreditBalanceLedger{}, &CreditValuationState{}, &UserSubscription{}} {
		var count int64
		require.NoError(t, db.Model(target).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestAdminCreditBalanceIncreaseCrossCurrencyLedgerFailureRollsBackEverything(t *testing.T) {
	originalFX := runtimeCreditFXRateSnapshot.Load()
	t.Cleanup(func() { runtimeCreditFXRateSnapshot.Store(originalFX) })
	frozenFX, err := prepareCreditFXRateSnapshot("7.3", 1_800_000_303)
	require.NoError(t, err)
	publishCreditFXRateSnapshot(frozenFX)
	db, userID, optionPlanID := seedAdminCreditPositiveIngress(t)
	valuationCurrency := "USD"
	require.NoError(t, db.Model(&SubscriptionPlan{}).Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Updates(map[string]any{
		"currency": valuationCurrency, "valuation_currency": valuationCurrency,
	}).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_admin_cross_currency_ledger BEFORE INSERT ON credit_balance_ledgers WHEN NEW.source_type = 'admin_adjustment' BEGIN SELECT RAISE(FAIL, 'injected admin cross-currency ledger failure'); END`).Error)
	t.Cleanup(func() { _ = db.Exec(`DROP TRIGGER IF EXISTS reject_admin_cross_currency_ledger`).Error })

	result, err := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
		UserId: userID, Operation: CreditBalanceAdjustmentIncrease, Amount: 800,
		PlanId: optionPlanID, IdempotencyKey: "admin-credit-cross-failure",
		OperatorUserId: 92_899, Reason: "跨币种售后补偿",
	})
	require.Nil(t, result)
	require.ErrorContains(t, err, "injected admin cross-currency ledger failure")
	for _, target := range []any{&CreditBalanceAdjustment{}, &CreditBalanceLedger{}, &CreditValuationState{}, &UserSubscription{}} {
		var count int64
		require.NoError(t, db.Model(target).Count(&count).Error)
		require.Zero(t, count)
	}
}

func seedRedemptionCreditPositiveIngress(t *testing.T, debt int64) (*gorm.DB, int, int, int) {
	return seedRedemptionCreditPositiveIngressCurrencies(t, debt, "CNY", "CNY")
}

func seedRedemptionCreditPositiveIngressCurrencies(t *testing.T, debt int64, sourceCurrency string, valuationCurrency string) (*gorm.DB, int, int, int) {
	t.Helper()
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Redemption{}, &InvitationRewardEvent{}, &Log{}))
	const userID = 93_001
	const optionPlanID = 93_002
	const creditPlanID = 93_003
	const redemptionID = 93_004
	priceMicros := int64(40_000_000)
	optionCode := "redemption-credit-positive-option"
	creditCode := "redemption-credit-positive-pool"
	require.NoError(t, db.Create(&User{
		Id: userID, Username: "redemption-credit-positive", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: optionPlanID, Title: "40 / 1,000 Credit", PriceAmount: 40,
		PriceAmountMicros: &priceMicros, Currency: sourceCurrency, Enabled: true,
		EntitlementType: SubscriptionEntitlementTimed, DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, QuotaResetPeriod: SubscriptionResetMonthly,
		MonthlyTokenLimit: 1_000, UnlimitedPurchaseEnabled: true, BusinessCode: &optionCode,
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: creditPlanID, Title: "Credit balance", Currency: valuationCurrency,
		ValuationCurrency: &valuationCurrency, Enabled: true,
		CreditBalanceConfigured: true, CreditBalanceRedemptionEnabled: true,
		EntitlementType: SubscriptionEntitlementCreditBalance, BusinessCode: &creditCode,
	}).Error)
	redemption := &Redemption{
		Id: redemptionID, Key: "credit-positive-redemption", Type: RedemptionTypeSubscription,
		PlanId: optionPlanID, Status: common.RedemptionCodeStatusEnabled,
	}
	require.NoError(t, redemption.Insert())
	if debt > 0 {
		balance := UserSubscription{
			UserId: userID, PlanId: creditPlanID, EntitlementType: SubscriptionEntitlementCreditBalance,
			Status: SubscriptionStatusActive, TokenLimit: 0, TokenUsed: debt,
		}
		require.NoError(t, db.Create(&balance).Error)
		now := common.GetTimestamp()
		require.NoError(t, db.Create(&CreditValuationState{
			UserSubscriptionId: balance.Id, UserId: userID, Currency: valuationCurrency,
			RuleVersion: CreditValuationRuleVersion, CreatedAt: now, UpdatedAt: now,
		}).Error)
	}
	return db, userID, optionPlanID, redemptionID
}

func TestRedemptionCreditBalanceFreezesExactTierSnapshot(t *testing.T) {
	db, userID, optionPlanID, redemptionID := seedRedemptionCreditPositiveIngress(t, 0)

	result, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Replayed)
	require.NotNil(t, result.CreditBalance)
	require.Equal(t, int64(1_000), result.CreditBalance.GrossCredit)
	require.Equal(t, int64(1_000), result.CreditBalance.NetCredit)
	require.Equal(t, int64(40_000_000), result.CreditBalance.GrossAmountMicros)
	require.Equal(t, int64(40_000_000), result.CreditBalance.NetAmountMicros)
	require.Equal(t, CreditValuationConfidenceExact, result.CreditBalance.ValuationConfidence)

	var ledger CreditBalanceLedger
	require.NoError(t, db.First(&ledger, result.CreditBalance.LedgerId).Error)
	require.Equal(t, optionPlanID, ledger.PlanId)
	require.Equal(t, int64(40_000_000), ledger.SourcePriceMicros)
	require.Equal(t, int64(1_000), ledger.SourcePlanCredit)
	require.Equal(t, "redemption:93004", ledger.SourceKey)
	require.Equal(t, "completed", ledger.SourceStatus)
	require.NotEmpty(t, ledger.ParameterFingerprint)

	newPriceMicros := int64(80_000_000)
	require.NoError(t, db.Model(&SubscriptionPlan{}).Where("id = ?", optionPlanID).Updates(map[string]any{
		"price_amount": 80, "price_amount_micros": newPriceMicros,
	}).Error)
	var saved Redemption
	require.NoError(t, db.First(&saved, redemptionID).Error)
	var fulfillment RedemptionFulfillmentSnapshot
	require.NoError(t, common.UnmarshalJsonStr(saved.FulfillmentSnapshot, &fulfillment))
	require.NotNil(t, fulfillment.Entitlement.ListPriceMicros)
	require.Equal(t, int64(40_000_000), *fulfillment.Entitlement.ListPriceMicros)
	require.NoError(t, db.First(&ledger, ledger.Id).Error)
	require.Equal(t, int64(40_000_000), ledger.SourcePriceMicros)
	var state CreditValuationState
	require.NoError(t, db.First(&state, result.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(40_000_000), state.ExactCostMicros)
}

func TestRedemptionCreditBalanceReplaysExactResultAndRejectsSourceConflict(t *testing.T) {
	db, userID, _, redemptionID := seedRedemptionCreditPositiveIngress(t, 0)

	first, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotNil(t, first.CreditBalance)

	replay, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
	require.NoError(t, err)
	require.NotNil(t, replay)
	require.True(t, replay.Replayed)
	require.Equal(t, first.CreditBalance.LedgerId, replay.CreditBalance.LedgerId)
	require.Equal(t, first.CreditBalance.ValuationStateVersionAfter, replay.CreditBalance.ValuationStateVersionAfter)

	var saved Redemption
	require.NoError(t, db.First(&saved, redemptionID).Error)
	var fulfillment RedemptionFulfillmentSnapshot
	require.NoError(t, common.UnmarshalJsonStr(saved.FulfillmentSnapshot, &fulfillment))
	conflictingPriceMicros := int64(80_000_000)
	fulfillment.Entitlement.ListPriceMicros = &conflictingPriceMicros
	conflictingPayload, err := common.Marshal(fulfillment)
	require.NoError(t, err)
	require.NoError(t, db.Model(&Redemption{}).Where("id = ?", redemptionID).Updates(map[string]any{
		"status": common.RedemptionCodeStatusEnabled, "fulfillment_snapshot": string(conflictingPayload),
	}).Error)

	conflict, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
	require.Nil(t, conflict)
	require.ErrorIs(t, err, ErrCreditValuationIdempotencyMismatch)

	var ledgerCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceRedemption, redemptionID).Count(&ledgerCount).Error)
	require.Equal(t, int64(1), ledgerCount)
	var state CreditValuationState
	require.NoError(t, db.First(&state, first.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(1), state.StateVersion)
	require.Equal(t, int64(40_000_000), state.ExactCostMicros)
	var balance UserSubscription
	require.NoError(t, db.First(&balance, first.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(1_000), balance.TokenLimit)
}

func TestRedemptionCreditBalanceOffsetsDebtBeforeExactValue(t *testing.T) {
	tests := []struct {
		name          string
		debt          int64
		wantNetCredit int64
		wantNetMicros int64
		wantDebtAfter int64
	}{
		{name: "partial debt", debt: 300, wantNetCredit: 700, wantNetMicros: 28_000_000},
		{name: "full debt", debt: 1_200, wantNetCredit: 0, wantNetMicros: 0, wantDebtAfter: 200},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, userID, _, _ := seedRedemptionCreditPositiveIngress(t, test.debt)

			result, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.CreditBalance)
			require.Equal(t, int64(1_000), result.CreditBalance.GrossCredit)
			require.Equal(t, test.wantNetCredit, result.CreditBalance.NetCredit)
			require.Equal(t, int64(40_000_000), result.CreditBalance.GrossAmountMicros)
			require.Equal(t, test.wantNetMicros, result.CreditBalance.NetAmountMicros)
			require.Equal(t, test.wantDebtAfter, result.CreditBalance.SettlementDebt)
			var state CreditValuationState
			require.NoError(t, db.First(&state, result.CreditBalance.UserSubscriptionId).Error)
			require.Equal(t, test.wantNetCredit, state.AvailableCredit)
			require.Equal(t, test.wantNetMicros, state.ExactCostMicros)
			require.Zero(t, state.EstimatedCostMicros)
			require.Zero(t, state.UnknownCredit)
			var ledger CreditBalanceLedger
			require.NoError(t, db.First(&ledger, result.CreditBalance.LedgerId).Error)
			require.Equal(t, test.debt-test.wantDebtAfter, ledger.DebtOffset)
			require.Equal(t, test.wantNetCredit, ledger.NetCredit)
			require.Equal(t, test.wantNetMicros, ledger.ValuationNetCostMicros)
		})
	}
}

func TestRedemptionCreditBalanceLedgerFailureRollsBackEverything(t *testing.T) {
	originalFX := runtimeCreditFXRateSnapshot.Load()
	t.Cleanup(func() { runtimeCreditFXRateSnapshot.Store(originalFX) })
	frozenFX, err := prepareCreditFXRateSnapshot("7.3", 1_800_000_200)
	require.NoError(t, err)
	publishCreditFXRateSnapshot(frozenFX)

	db, userID, _, redemptionID := seedRedemptionCreditPositiveIngressCurrencies(t, 0, "CNY", "USD")
	var before Redemption
	require.NoError(t, db.First(&before, redemptionID).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_redemption_positive_ledger BEFORE INSERT ON credit_balance_ledgers WHEN NEW.source_type = 'redemption' BEGIN SELECT RAISE(FAIL, 'injected redemption ledger failure'); END`).Error)
	t.Cleanup(func() { _ = db.Exec(`DROP TRIGGER IF EXISTS reject_redemption_positive_ledger`).Error })

	result, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrRedeemFailed)

	var after Redemption
	require.NoError(t, db.First(&after, redemptionID).Error)
	require.Equal(t, common.RedemptionCodeStatusEnabled, after.Status)
	require.Zero(t, after.UsedUserId)
	require.Zero(t, after.RedeemedTime)
	require.Empty(t, after.FulfillmentMode)
	require.Equal(t, before.FulfillmentSnapshot, after.FulfillmentSnapshot)
	require.NotContains(t, after.FulfillmentSnapshot, "credit_fx_rate_snapshot")
	require.Zero(t, after.FulfillmentSubscriptionId)
	for _, target := range []any{&CreditBalanceLedger{}, &CreditValuationState{}, &UserSubscription{}, &InvitationRewardEvent{}, &Log{}} {
		var count int64
		require.NoError(t, db.Model(target).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestRedemptionCreditBalanceCrossCurrencyRequiresFrozenFXSnapshot(t *testing.T) {
	originalFX := runtimeCreditFXRateSnapshot.Load()
	t.Cleanup(func() { runtimeCreditFXRateSnapshot.Store(originalFX) })
	frozenFX, err := prepareCreditFXRateSnapshot("7.3", 1_800_000_000)
	require.NoError(t, err)
	publishCreditFXRateSnapshot(frozenFX)

	db, userID, _, redemptionID := seedRedemptionCreditPositiveIngressCurrencies(t, 0, "CNY", "USD")

	result, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.CreditBalance)
	require.Equal(t, "CNY", result.CreditBalance.SourceCurrency)
	require.Equal(t, "USD", result.CreditBalance.ValuationCurrency)
	require.Equal(t, int64(10), result.CreditBalance.FxRateNumerator)
	require.Equal(t, int64(73), result.CreditBalance.FxRateDenominator)
	require.Equal(t, int64(1_800_000_000), result.CreditBalance.FxCapturedAt)
	require.Equal(t, CreditFXDirectionCNYtoUSD, result.CreditBalance.FxDirection)
	require.Equal(t, int64(5_479_452), result.CreditBalance.GrossAmountMicros)

	updatedFX, err := prepareCreditFXRateSnapshot("8.1", 1_800_000_001)
	require.NoError(t, err)
	publishCreditFXRateSnapshot(updatedFX)
	replay, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.True(t, replay.CreditBalance.Replayed)
	require.Equal(t, result.CreditBalance.FxRateNumerator, replay.CreditBalance.FxRateNumerator)
	require.Equal(t, result.CreditBalance.FxRateDenominator, replay.CreditBalance.FxRateDenominator)
	require.Equal(t, result.CreditBalance.FxCapturedAt, replay.CreditBalance.FxCapturedAt)

	var saved Redemption
	require.NoError(t, db.First(&saved, redemptionID).Error)
	var fulfillment RedemptionFulfillmentSnapshot
	require.NoError(t, common.UnmarshalJsonStr(saved.FulfillmentSnapshot, &fulfillment))
	require.NotNil(t, fulfillment.CreditFXRateSnapshot)
	require.Equal(t, int64(10), fulfillment.CreditFXRateSnapshot.Numerator)
	require.Equal(t, int64(73), fulfillment.CreditFXRateSnapshot.Denominator)
}

func TestRedemptionCreditBalanceSupportsUSDtoCNYFrozenFXSnapshot(t *testing.T) {
	originalFX := runtimeCreditFXRateSnapshot.Load()
	t.Cleanup(func() { runtimeCreditFXRateSnapshot.Store(originalFX) })
	frozenFX, err := prepareCreditFXRateSnapshot("7.3", 1_800_000_100)
	require.NoError(t, err)
	publishCreditFXRateSnapshot(frozenFX)

	db, userID, _, _ := seedRedemptionCreditPositiveIngressCurrencies(t, 0, "USD", "CNY")
	result, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
	require.NoError(t, err)
	require.Equal(t, "USD", result.CreditBalance.SourceCurrency)
	require.Equal(t, "CNY", result.CreditBalance.ValuationCurrency)
	require.Equal(t, int64(73), result.CreditBalance.FxRateNumerator)
	require.Equal(t, int64(10), result.CreditBalance.FxRateDenominator)
	require.Equal(t, CreditFXDirectionUSDtoCNY, result.CreditBalance.FxDirection)
	require.Equal(t, int64(292_000_000), result.CreditBalance.GrossAmountMicros)

	var ledger CreditBalanceLedger
	require.NoError(t, db.First(&ledger, result.CreditBalance.LedgerId).Error)
	require.Equal(t, CreditFXDirectionUSDtoCNY, ledger.FxDirection)
	require.Equal(t, int64(1_800_000_100), ledger.FxCapturedAt)
}

func TestRedemptionCreditBalanceRejectsInvalidFXAtomically(t *testing.T) {
	originalFX := runtimeCreditFXRateSnapshot.Load()
	t.Cleanup(func() { runtimeCreditFXRateSnapshot.Store(originalFX) })
	runtimeCreditFXRateSnapshot.Store(nil)
	db, userID, _, redemptionID := seedRedemptionCreditPositiveIngressCurrencies(t, 0, "CNY", "USD")
	var before Redemption
	require.NoError(t, db.First(&before, redemptionID).Error)

	result, err := Redeem("credit-positive-redemption", userID, RedemptionModeCreditBalance)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrCreditValuationInvalidFX)

	var after Redemption
	require.NoError(t, db.First(&after, redemptionID).Error)
	require.Equal(t, common.RedemptionCodeStatusEnabled, after.Status)
	require.Equal(t, before.FulfillmentSnapshot, after.FulfillmentSnapshot)
	for _, target := range []any{&CreditBalanceLedger{}, &CreditValuationState{}, &UserSubscription{}} {
		var count int64
		require.NoError(t, db.Model(target).Count(&count).Error)
		require.Zero(t, count)
	}
}
