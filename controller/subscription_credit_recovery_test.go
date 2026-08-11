package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecoverDirectCreditOrderUsesSnapshotCreatesDebtAndReplaysOnce(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	const userID = 9701
	const optionPlanID = 9702
	const creditPlanID = 9703
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "credit-recovery", Status: common.UserStatusEnabled}).Error)
	optionCode := "credit-recovery-option"
	creditCode := "credit-recovery-balance"
	optionPlan := &model.SubscriptionPlan{Id: optionPlanID, Title: "Recovery option", EntitlementType: model.SubscriptionEntitlementTimed, PriceAmount: 40, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 1000, Enabled: true, PublicVisible: true, BusinessCode: &optionCode}
	creditPlan := &model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, BusinessCode: &creditCode}
	require.NoError(t, model.DB.Create(optionPlan).Error)
	require.NoError(t, model.DB.Create(creditPlan).Error)

	snapshot := model.NewSubscriptionEntitlementSnapshot(optionPlan, model.SubscriptionPurchaseModeCreditBalance, creditPlanID)
	snapshot.SetTargetCreditBalancePlanSnapshot(creditPlan)
	snapshot.SetPaymentSnapshot(model.PaymentProviderStripe, "price_recovery", model.PaymentMethodStripe, 4000, "CNY")
	snapshotJSON, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := &model.SubscriptionOrder{UserId: userID, PlanId: optionPlanID, Money: 40, AmountCents: 4000, Currency: "CNY", CreditGrantAmount: 1000, CreditTargetPlanID: creditPlanID, TradeNo: "credit-recovery-order", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusSuccess, CompleteTime: common.GetTimestamp(), EntitlementSnapshot: snapshotJSON}
	require.NoError(t, model.DB.Create(order).Error)

	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		_, grantErr := model.GrantCreditBalanceTx(tx, model.CreditBalanceGrantRequest{UserId: userID, GrossCredit: 1000, IdempotencyKey: order.TradeNo, SourceType: model.CreditBalanceLedgerSourceSubscriptionOrder, SourceId: order.Id, Type: model.CreditBalanceLedgerTypePurchase, TargetPlanId: creditPlanID, TargetPlanSnapshot: creditPlan, Reason: "purchase"})
		return grantErr
	}))
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", balance.Id).Update("token_used", int64(800)).Error)

	first, err := model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderStripe, RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "provider refund"})
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.False(t, first.Replayed)
	assert.Equal(t, int64(1000), first.GrossCredit)
	assert.Equal(t, int64(800), first.SettlementDebt)

	require.NoError(t, model.DB.First(&balance, balance.Id).Error)
	assert.Equal(t, int64(1000), balance.TokenLimit)
	assert.Equal(t, int64(1800), balance.TokenUsed)
	require.NoError(t, model.DB.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusRefunded, order.Status)

	var recoveryLedger model.CreditBalanceLedger
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).First(&recoveryLedger).Error)
	assert.Equal(t, model.CreditBalanceLedgerTypeRefund, recoveryLedger.Type)
	assert.Equal(t, int64(-1000), recoveryLedger.GrossCredit)
	assert.Equal(t, int64(800), recoveryLedger.DebtFormed)
	assert.Equal(t, int64(200), recoveryLedger.BalanceBefore)
	assert.Equal(t, int64(-800), recoveryLedger.BalanceAfter)

	second, err := model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderStripe, RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "provider refund"})
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.True(t, second.Replayed)
	require.NoError(t, model.DB.First(&balance, balance.Id).Error)
	assert.Equal(t, int64(1800), balance.TokenUsed)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
}

func TestRecoverDirectCreditOrderRejectsProviderAndTerminalMismatch(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	const userID = 9704
	const optionPlanID = 9705
	const creditPlanID = 9706
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "credit-recovery-mismatch", Status: common.UserStatusEnabled}).Error)
	optionCode := "credit-recovery-mismatch-option"
	creditCode := "credit-recovery-mismatch-balance"
	optionPlan := &model.SubscriptionPlan{Id: optionPlanID, Title: "Recovery mismatch option", EntitlementType: model.SubscriptionEntitlementTimed, MonthlyTokenLimit: 500, Enabled: true, BusinessCode: &optionCode}
	creditPlan := &model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, BusinessCode: &creditCode}
	require.NoError(t, model.DB.Create(optionPlan).Error)
	require.NoError(t, model.DB.Create(creditPlan).Error)
	snapshot := model.NewSubscriptionEntitlementSnapshot(optionPlan, model.SubscriptionPurchaseModeCreditBalance, creditPlanID)
	snapshot.SetTargetCreditBalancePlanSnapshot(creditPlan)
	snapshot.SetPaymentSnapshot(model.PaymentProviderStripe, "price_mismatch", model.PaymentMethodStripe, 1000, "CNY")
	snapshotJSON, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := &model.SubscriptionOrder{UserId: userID, PlanId: optionPlanID, AmountCents: 1000, Currency: "CNY", CreditGrantAmount: 500, CreditTargetPlanID: creditPlanID, TradeNo: "credit-recovery-mismatch-order", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusSuccess, EntitlementSnapshot: snapshotJSON}
	require.NoError(t, model.DB.Create(order).Error)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		_, grantErr := model.GrantCreditBalanceTx(tx, model.CreditBalanceGrantRequest{UserId: userID, GrossCredit: 500, IdempotencyKey: order.TradeNo, SourceType: model.CreditBalanceLedgerSourceSubscriptionOrder, SourceId: order.Id, Type: model.CreditBalanceLedgerTypePurchase, TargetPlanId: creditPlanID, TargetPlanSnapshot: creditPlan, Reason: "purchase"})
		return grantErr
	}))

	_, err = model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderCreem, RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "provider refund"})
	require.ErrorIs(t, err, model.ErrPaymentMethodMismatch)
	first, err := model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderStripe, RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "provider refund"})
	require.NoError(t, err)
	assert.False(t, first.Replayed)
	chargeback, err := model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderStripe, RecoveryType: model.SubscriptionOrderRecoveryChargeback, Reason: "provider chargeback"})
	require.NoError(t, err)
	assert.True(t, chargeback.Replayed)
	assert.Equal(t, common.TopUpStatusChargeback, chargeback.Status)
	require.NoError(t, model.DB.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusChargeback, order.Status)
	assert.Equal(t, "provider chargeback", order.RecoveryReason)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(500), balance.TokenUsed)
}

func TestRecoverDirectCreditOrderRollsBackStatusAndBalanceOnLedgerFailure(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	const userID = 9707
	const optionPlanID = 9708
	const creditPlanID = 9709
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "credit-recovery-rollback", Status: common.UserStatusEnabled}).Error)
	optionCode := "credit-recovery-rollback-option"
	creditCode := "credit-recovery-rollback-balance"
	optionPlan := &model.SubscriptionPlan{Id: optionPlanID, Title: "Recovery rollback", EntitlementType: model.SubscriptionEntitlementTimed, MonthlyTokenLimit: 400, Enabled: true, BusinessCode: &optionCode}
	creditPlan := &model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, BusinessCode: &creditCode}
	require.NoError(t, model.DB.Create(optionPlan).Error)
	require.NoError(t, model.DB.Create(creditPlan).Error)
	snapshot := model.NewSubscriptionEntitlementSnapshot(optionPlan, model.SubscriptionPurchaseModeCreditBalance, creditPlanID)
	snapshot.SetTargetCreditBalancePlanSnapshot(creditPlan)
	snapshot.SetPaymentSnapshot(model.PaymentProviderKyren, "product_rollback", model.PaymentMethodKyren, 1000, "CNY")
	snapshotJSON, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := &model.SubscriptionOrder{UserId: userID, PlanId: optionPlanID, AmountCents: 1000, Currency: "CNY", CreditGrantAmount: 400, CreditTargetPlanID: creditPlanID, TradeNo: "credit-recovery-rollback-order", PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren, Status: common.TopUpStatusSuccess, EntitlementSnapshot: snapshotJSON}
	require.NoError(t, model.DB.Create(order).Error)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		_, grantErr := model.GrantCreditBalanceTx(tx, model.CreditBalanceGrantRequest{UserId: userID, GrossCredit: 400, IdempotencyKey: order.TradeNo, SourceType: model.CreditBalanceLedgerSourceSubscriptionOrder, SourceId: order.Id, Type: model.CreditBalanceLedgerTypePurchase, TargetPlanId: creditPlanID, TargetPlanSnapshot: creditPlan, Reason: "purchase"})
		return grantErr
	}))
	require.NoError(t, model.DB.Exec(`CREATE TRIGGER reject_credit_recovery_ledger BEFORE INSERT ON credit_balance_ledgers WHEN NEW.source_type = 'subscription_order_recovery' BEGIN SELECT RAISE(FAIL, 'injected recovery ledger failure'); END`).Error)
	t.Cleanup(func() { _ = model.DB.Exec(`DROP TRIGGER IF EXISTS reject_credit_recovery_ledger`).Error })

	_, err = model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderKyren, RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "provider refund"})
	require.Error(t, err)
	require.NoError(t, model.DB.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Zero(t, order.RecoveryLedgerID)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(0), balance.TokenUsed)
	var recoveryCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Count(&recoveryCount).Error)
	assert.Zero(t, recoveryCount)
}

func TestRecoverConvertedTimedOrderUsesOrderMonthlyCreditSnapshot(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionConversion{}))
	const userID = 9711
	const timedPlanID = 9712
	const creditPlanID = 9713
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "converted-refund", Status: common.UserStatusEnabled}).Error)
	timedCode := "converted-refund-timed"
	creditCode := "converted-refund-credit"
	timedPlan := &model.SubscriptionPlan{Id: timedPlanID, Title: "Converted plan", EntitlementType: model.SubscriptionEntitlementTimed, MonthlyTokenLimit: 1500, Enabled: true, BusinessCode: &timedCode}
	creditPlan := &model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, BusinessCode: &creditCode}
	require.NoError(t, model.DB.Create(timedPlan).Error)
	require.NoError(t, model.DB.Create(creditPlan).Error)
	balance := &model.UserSubscription{UserId: userID, PlanId: creditPlanID, EntitlementType: model.SubscriptionEntitlementCreditBalance, Status: model.SubscriptionStatusActive, TokenLimit: 3000}
	require.NoError(t, model.DB.Create(balance).Error)
	source := &model.UserSubscription{UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusConverted, TokenLimit: 1000, ConvertedToSubscriptionId: balance.Id, ConvertedAt: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(source).Error)
	conversion := &model.SubscriptionConversion{UserId: userID, IdempotencyKey: "converted-refund", SourceSubscriptionId: source.Id, SourcePlanId: timedPlanID, SourcePlanTitle: timedPlan.Title, TargetSubscriptionId: balance.Id, TargetPlanId: creditPlanID, LedgerId: 9791, GrossCredit: 1000, SourceStatus: model.SubscriptionStatusActive, GrantSource: model.SubscriptionGrantOrder, ConvertedAt: common.GetTimestamp(), CreatedAt: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(conversion).Error)
	require.NoError(t, model.DB.Model(source).Updates(map[string]any{"conversion_id": conversion.Id, "converted_to_subscription_id": balance.Id}).Error)
	snapshot := model.NewSubscriptionEntitlementSnapshot(&model.SubscriptionPlan{Id: timedPlanID, Title: "Converted plan", EntitlementType: model.SubscriptionEntitlementTimed, MonthlyTokenLimit: 1000}, model.SubscriptionPurchaseModeTimed, 0)
	snapshotJSON, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := &model.SubscriptionOrder{UserId: userID, PlanId: timedPlanID, TradeNo: "converted-order-snapshot", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusSuccess, FulfilledSubscriptionID: source.Id, EntitlementSnapshot: snapshotJSON}
	require.NoError(t, model.DB.Create(order).Error)

	result, err := model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderStripe, RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "converted refund"})
	require.NoError(t, err)
	assert.Equal(t, int64(1000), result.GrossCredit)
	require.NoError(t, model.DB.First(balance, balance.Id).Error)
	assert.Equal(t, int64(1000), balance.TokenUsed)
}

func TestRecoverHistoricalConvertedTimedOrderFallsBackToCurrentMonthlyCredit(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionConversion{}))
	const userID = 9721
	const timedPlanID = 9722
	const creditPlanID = 9723
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "historical-converted-refund", Status: common.UserStatusEnabled}).Error)
	timedCode := "historical-converted-timed"
	creditCode := "historical-converted-credit"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: timedPlanID, Title: "Historical converted", EntitlementType: model.SubscriptionEntitlementTimed, MonthlyTokenLimit: 777, Enabled: true, BusinessCode: &timedCode}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, BusinessCode: &creditCode}).Error)
	balance := &model.UserSubscription{UserId: userID, PlanId: creditPlanID, EntitlementType: model.SubscriptionEntitlementCreditBalance, Status: model.SubscriptionStatusActive, TokenLimit: 3000}
	require.NoError(t, model.DB.Create(balance).Error)
	now := common.GetTimestamp()
	source := &model.UserSubscription{UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusConverted, TokenLimit: 1000, StartTime: now - 86400, EndTime: now + 86400, ConvertedToSubscriptionId: balance.Id, ConvertedAt: now}
	require.NoError(t, model.DB.Create(source).Error)
	conversion := &model.SubscriptionConversion{UserId: userID, IdempotencyKey: "historical-converted-refund", SourceSubscriptionId: source.Id, SourcePlanId: timedPlanID, SourcePlanTitle: "Historical converted", TargetSubscriptionId: balance.Id, TargetPlanId: creditPlanID, LedgerId: 9792, GrossCredit: 777, SourceStatus: model.SubscriptionStatusActive, GrantSource: model.SubscriptionGrantOrder, ConvertedAt: now, CreatedAt: now}
	require.NoError(t, model.DB.Create(conversion).Error)
	require.NoError(t, model.DB.Model(source).Updates(map[string]any{"conversion_id": conversion.Id, "converted_to_subscription_id": balance.Id}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusActive, StartTime: now + 2*86400, EndTime: now + 3*86400}).Error)
	order := &model.SubscriptionOrder{UserId: userID, PlanId: timedPlanID, TradeNo: "historical-converted-order", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CompleteTime: now - 60}
	require.NoError(t, model.DB.Create(order).Error)

	result, err := model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderEpay, RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "historical converted refund"})
	require.NoError(t, err)
	assert.Equal(t, int64(777), result.GrossCredit)
	require.NoError(t, model.DB.First(balance, balance.Id).Error)
	assert.Equal(t, int64(777), balance.TokenUsed)
}

func TestRecoverHistoricalTimedOrderWithoutReliableMappingDoesNotWithdrawCredit(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionConversion{}))
	const userID = 9724
	const timedPlanID = 9725
	const creditPlanID = 9726
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "historical-unconverted-refund", Status: common.UserStatusEnabled}).Error)
	timedCode := "historical-unconverted-timed"
	creditCode := "historical-unconverted-credit"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: timedPlanID, Title: "Historical unconverted", EntitlementType: model.SubscriptionEntitlementTimed, MonthlyTokenLimit: 500, Enabled: true, BusinessCode: &timedCode}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, BusinessCode: &creditCode}).Error)
	balance := &model.UserSubscription{UserId: userID, PlanId: creditPlanID, EntitlementType: model.SubscriptionEntitlementCreditBalance, Status: model.SubscriptionStatusActive, TokenLimit: 1000}
	require.NoError(t, model.DB.Create(balance).Error)
	now := common.GetTimestamp()
	converted := &model.UserSubscription{UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusConverted, StartTime: now - 4*86400, EndTime: now - 3*86400, ConvertedToSubscriptionId: balance.Id, ConvertedAt: now - 2*86400}
	require.NoError(t, model.DB.Create(converted).Error)
	conversion := &model.SubscriptionConversion{UserId: userID, IdempotencyKey: "historical-other-conversion", SourceSubscriptionId: converted.Id, SourcePlanId: timedPlanID, SourcePlanTitle: "Historical unconverted", TargetSubscriptionId: balance.Id, TargetPlanId: creditPlanID, LedgerId: 9793, SourceStatus: model.SubscriptionStatusActive, GrantSource: model.SubscriptionGrantOrder, ConvertedAt: now - 2*86400, CreatedAt: now - 2*86400}
	require.NoError(t, model.DB.Create(conversion).Error)
	require.NoError(t, model.DB.Model(converted).Updates(map[string]any{"conversion_id": conversion.Id, "converted_to_subscription_id": balance.Id}).Error)
	unconverted := &model.UserSubscription{UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusActive, StartTime: now - 86400, EndTime: now + 86400}
	require.NoError(t, model.DB.Create(unconverted).Error)
	order := &model.SubscriptionOrder{UserId: userID, PlanId: timedPlanID, TradeNo: "historical-unconverted-order", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusSuccess, CompleteTime: now - 60}
	require.NoError(t, model.DB.Create(order).Error)

	result, err := model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderStripe, RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "provider refund", CreditRecoveryOnly: true})
	require.Nil(t, result)
	require.ErrorIs(t, err, model.ErrSubscriptionOrderCreditRecoveryNotApplicable)
	require.NoError(t, model.DB.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	require.NoError(t, model.DB.First(balance, balance.Id).Error)
	assert.Zero(t, balance.TokenUsed)
	var recoveryLedgers int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Count(&recoveryLedgers).Error)
	assert.Zero(t, recoveryLedgers)
}

func TestRecoverTimedOrderRecordsRecoveredAndUnrecoverableCommission(t *testing.T) {
	for index, test := range []struct {
		name                   string
		availableCents         int64
		withdrawnCents         int64
		transferredCents       int64
		expectedRecordStatus   string
		expectedReversalStatus string
		expectedRecovered      int64
		expectedUnrecovered    int64
		expectedLedgerTypes    []string
		expectedLedgerAmounts  []int64
	}{
		{
			name: "available commission is recovered", availableCents: 400,
			expectedRecordStatus:   model.InvitationCommissionStatusCancelled,
			expectedReversalStatus: model.InvitationCommissionReversalStatusRecovered,
			expectedRecovered:      400,
			expectedLedgerTypes:    []string{model.InvitationCommissionLedgerRefundReversal},
			expectedLedgerAmounts:  []int64{-400},
		},
		{
			name: "transferred commission records unrecoverable debt", transferredCents: 400,
			expectedRecordStatus:   model.InvitationCommissionStatusUnrecoverable,
			expectedReversalStatus: model.InvitationCommissionReversalStatusUnrecoverable,
			expectedUnrecovered:    400,
			expectedLedgerTypes:    []string{model.InvitationCommissionLedgerRefundUnrecoverable},
			expectedLedgerAmounts:  []int64{400},
		},
		{
			name: "withdrawn commission records unrecoverable debt", withdrawnCents: 400,
			expectedRecordStatus:   model.InvitationCommissionStatusUnrecoverable,
			expectedReversalStatus: model.InvitationCommissionReversalStatusUnrecoverable,
			expectedUnrecovered:    400,
			expectedLedgerTypes:    []string{model.InvitationCommissionLedgerRefundUnrecoverable},
			expectedLedgerAmounts:  []int64{400},
		},
		{
			name: "partially transferred commission recovers available remainder", availableCents: 150, transferredCents: 250,
			expectedRecordStatus:   model.InvitationCommissionStatusUnrecoverable,
			expectedReversalStatus: model.InvitationCommissionReversalStatusUnrecoverable,
			expectedRecovered:      150, expectedUnrecovered: 250,
			expectedLedgerTypes:   []string{model.InvitationCommissionLedgerRefundReversal, model.InvitationCommissionLedgerRefundUnrecoverable},
			expectedLedgerAmounts: []int64{-150, 250},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupSubscriptionBalancePurchaseTestDB(t)
			require.NoError(t, model.DB.AutoMigrate(
				&model.SubscriptionConversion{}, &model.InvitationCommissionAccount{},
				&model.InvitationCommissionRecord{}, &model.InvitationCommissionLedger{},
			))
			inviterID := 9800 + index*10
			inviteeID := inviterID + 1
			planID := inviterID + 2
			require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "refund-commission-inviter", Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("refund-commission-inviter-%d", index)}).Error)
			require.NoError(t, model.DB.Create(&model.User{Id: inviteeID, Username: "refund-commission-invitee", Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("refund-commission-invitee-%d", index), InviterId: inviterID}).Error)
			code := fmt.Sprintf("refund_commission_plan_%d", index)
			require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Refund commission", EntitlementType: model.SubscriptionEntitlementTimed, Enabled: true, BusinessCode: &code}).Error)
			order := model.SubscriptionOrder{
				UserId: inviteeID, PlanId: planID, TradeNo: fmt.Sprintf("refund-commission-order-%d", index),
				PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay",
				Status: common.TopUpStatusSuccess, AmountCents: 4000, Currency: "CNY",
			}
			require.NoError(t, model.DB.Create(&order).Error)
			now := common.GetTimestamp()
			event := model.InvitationRewardEvent{
				InviterId: inviterID, InviteeId: inviteeID,
				SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: order.Id,
				SourceOrderId: order.Id, SourceAmountCents: 4000, SourceCurrency: "CNY",
				Status: model.InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now,
			}
			require.NoError(t, model.DB.Create(&event).Error)
			account := model.InvitationCommissionAccount{
				UserId: inviterID, AvailableCents: test.availableCents,
				WithdrawnCents: test.withdrawnCents, TransferredCents: test.transferredCents,
				CreatedAt: now, UpdatedAt: now,
			}
			require.NoError(t, model.DB.Create(&account).Error)
			record := model.InvitationCommissionRecord{
				EventId: event.Id, InviterId: inviterID, InviteeId: inviteeID,
				SourceType: model.InvitationCommissionSourceSubscriptionOrder, SourceId: order.Id,
				SourceTradeNo: order.TradeNo, SourceAmountCents: 4000, SourceCurrency: "CNY",
				CommissionRateBps: 1000, CommissionCents: 400,
				Status: model.InvitationCommissionStatusAvailable, CreatedAt: now, AvailableAt: now,
			}
			require.NoError(t, model.DB.Create(&record).Error)

			result, err := model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{
				TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderEpay,
				RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "verified timed refund",
			})

			require.NoError(t, err)
			assert.Equal(t, common.TopUpStatusRefunded, result.Status)
			require.NoError(t, model.DB.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusRefunded, order.Status)
			require.NoError(t, model.DB.First(&event, event.Id).Error)
			assert.Equal(t, model.InvitationRewardEventStatusCancelled, event.Status)
			require.NoError(t, model.DB.First(&record, record.Id).Error)
			assert.Equal(t, test.expectedRecordStatus, record.Status)
			assert.Equal(t, test.expectedReversalStatus, record.ReversalStatus)
			assert.Equal(t, test.expectedRecovered, record.RecoveredCents)
			assert.Equal(t, test.expectedUnrecovered, record.UnrecoveredCents)
			assert.Equal(t, "verified timed refund", record.ReversalReason)
			var ledgers []model.InvitationCommissionLedger
			require.NoError(t, model.DB.Where("reference_type = ? AND reference_id = ?", "commission_record", record.Id).Order("id asc").Find(&ledgers).Error)
			require.Len(t, ledgers, len(test.expectedLedgerTypes))
			for ledgerIndex := range ledgers {
				assert.Equal(t, test.expectedLedgerTypes[ledgerIndex], ledgers[ledgerIndex].Type)
				assert.Equal(t, test.expectedLedgerAmounts[ledgerIndex], ledgers[ledgerIndex].AmountCents)
			}
			require.NoError(t, model.DB.First(&account, account.Id).Error)
			assert.Zero(t, account.AvailableCents)
			assert.Equal(t, test.transferredCents, account.TransferredCents)
			assert.Equal(t, test.withdrawnCents, account.WithdrawnCents)
		})
	}
}
