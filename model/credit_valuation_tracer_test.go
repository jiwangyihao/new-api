package model

import (
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func setupCreditValuationTracerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	oldRedis := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	initCol()
	resetDBTimestampCacheForTest()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	ClearSubscriptionPlanCacheForTest()
	ClearPrimaryBillableSubscriptionCacheForTest()
	require.NoError(t, db.AutoMigrate(
		&User{},
		&SubscriptionPlan{},
		&SubscriptionOrder{},
		&UserSubscription{},
	))
	require.NoError(t, migrateCreditValuationSchema(db))
	require.NoError(t, db.Create(&CreditValuationMigration{Version: 1, Status: CreditValuationMigrationReady, ValuationCurrency: "CNY"}).Error)

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		common.RedisEnabled = oldRedis
		resetDBTimestampCacheForTest()
		initCol()
		ClearSubscriptionPlanCacheForTest()
		ClearPrimaryBillableSubscriptionCacheForTest()
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedCreditValuationOrder(t *testing.T, db *gorm.DB, provider string) (User, SubscriptionPlan, SubscriptionPlan, SubscriptionOrder) {
	t.Helper()
	priceMicros := int64(40_000_000)
	valuationCurrency := "CNY"
	user := User{Id: 91_001, Username: "credit-valuation-tracer", Status: common.UserStatusEnabled}
	optionCode := "credit-valuation-option"
	option := SubscriptionPlan{
		Id:                       91_002,
		Title:                    "40 CNY / 1,000 Credit",
		PriceAmount:              40,
		PriceAmountMicros:        &priceMicros,
		Currency:                 "CNY",
		DurationUnit:             SubscriptionDurationMonth,
		DurationValue:            1,
		Enabled:                  true,
		PublicVisible:            true,
		MonthlyTokenLimit:        1_000,
		QuotaResetPeriod:         SubscriptionResetMonthly,
		UnlimitedPurchaseEnabled: true,
		BusinessCode:             &optionCode,
		EntitlementType:          SubscriptionEntitlementTimed,
	}
	creditCode := "credit-balance-pool"
	creditPlan := SubscriptionPlan{
		Id:                           91_003,
		Title:                        "Credit balance",
		Currency:                     "CNY",
		ValuationCurrency:            &valuationCurrency,
		Enabled:                      true,
		CreditBalanceConfigured:      true,
		CreditBalancePurchaseEnabled: true,
		EntitlementType:              SubscriptionEntitlementCreditBalance,
		BusinessCode:                 &creditCode,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&option).Error)
	require.NoError(t, db.Create(&creditPlan).Error)

	method := PaymentMethodAccountBalance
	if provider != PaymentProviderBalance {
		method = provider
	}
	snapshot := NewSubscriptionEntitlementSnapshot(&option, SubscriptionPurchaseModeCreditBalance, creditPlan.Id)
	snapshot.SetTargetCreditBalancePlanSnapshot(&creditPlan)
	snapshot.SetPaymentSnapshot(provider, "controlled-product", method, 4_000, "CNY")
	snapshotPayload, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := SubscriptionOrder{
		Id:                  91_004,
		UserId:              user.Id,
		PlanId:              option.Id,
		Money:               40,
		AmountCents:         4_000,
		Currency:            "CNY",
		CreditGrantAmount:   1_000,
		CreditTargetPlanID:  creditPlan.Id,
		TradeNo:             "credit-valuation-order-" + provider,
		PaymentMethod:       method,
		PaymentProvider:     provider,
		Status:              common.TopUpStatusPending,
		EntitlementSnapshot: snapshotPayload,
	}
	require.NoError(t, db.Create(&order).Error)
	return user, option, creditPlan, order
}

func completeCreditValuationOrder(t *testing.T, db *gorm.DB, order *SubscriptionOrder) *SubscriptionOrderCompletionResult {
	t.Helper()
	var result *SubscriptionOrderCompletionResult
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var locked SubscriptionOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, order.Id).Error; err != nil {
			return err
		}
		var err error
		result, err = CompleteSubscriptionOrderTx(tx, &locked, `{}`, order.PaymentMethod)
		return err
	}))
	require.NotNil(t, result)
	require.NotNil(t, result.CreditBalance)
	return result
}

func TestCreditValuationOrderIngressCreatesExactState(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	_, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)

	result := completeCreditValuationOrder(t, db, &order)

	var state CreditValuationState
	require.NoError(t, db.First(&state, result.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(1_000), state.AvailableCredit)
	require.Equal(t, int64(40_000_000), state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, "CNY", state.Currency)
	require.Equal(t, CreditValuationRuleVersion, state.RuleVersion)
	require.Equal(t, int64(1), state.StateVersion)

	var ledger CreditBalanceLedger
	require.NoError(t, db.First(&ledger, result.CreditBalance.LedgerId).Error)
	require.Equal(t, int64(40_000_000), ledger.ValuationGrossCostMicros)
	require.Equal(t, int64(40_000_000), ledger.ValuationNetCostMicros)
	require.Equal(t, "exact", ledger.ValuationConfidence)
	require.Equal(t, int64(1), ledger.ValuationStateVersionAfter)
	require.NotEmpty(t, ledger.SourceSnapshot)
}

func TestCreditValuationOrderIngressPreservesLegacyPathWhenMarkerNotReady(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.Model(&CreditValuationMigration{}).Where("version = ?", 1).Update("status", CreditValuationMigrationPending).Error)
	_, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)

	snapshot, err := UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
	require.NoError(t, err)
	snapshot.ListPriceMicros = nil
	snapshot.ListPriceCurrency = ""
	snapshot.ValuationRuleVersion = 0
	snapshot.TargetCreditBalanceValuationCurrency = ""
	order.EntitlementSnapshot, err = MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	require.NoError(t, db.Model(&SubscriptionOrder{}).Where("id = ?", order.Id).Update("entitlement_snapshot", order.EntitlementSnapshot).Error)

	result := completeCreditValuationOrder(t, db, &order)
	require.Equal(t, int64(1_000), result.CreditBalance.AvailableCredit)

	var stateCount int64
	require.NoError(t, db.Model(&CreditValuationState{}).Count(&stateCount).Error)
	require.Zero(t, stateCount)
	var ledger CreditBalanceLedger
	require.NoError(t, db.First(&ledger, result.CreditBalance.LedgerId).Error)
	require.Zero(t, ledger.ValuationGrossCostMicros)
	require.Zero(t, ledger.ValuationNetCostMicros)
	require.Zero(t, ledger.ValuationStateVersionAfter)
	require.Empty(t, ledger.ValuationConfidence)
}

func TestCreditValuationRequestPreConsumeRemovesMovingAverageCost(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	result := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-valuation-request-200"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.Equal(t, result.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)
	require.Equal(t, int64(200), preConsumed.PreConsumed)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, result.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(1_000), subscription.TokenLimit)
	require.Equal(t, int64(200), subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(800), state.AvailableCredit)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, int64(2), state.StateVersion)
	require.Equal(t, CreditValuationMutationConsume, state.LastMutationType)

	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(200), record.AppliedCredit)
	require.Equal(t, int64(200), record.DeductedAvailableCredit)
	require.Zero(t, record.DebtFormedCredit)
	require.Equal(t, subscription.Id, record.ValuationSubscriptionId)
	require.Equal(t, int64(8_000_000), record.DeductedExactCostMicros)

	require.Zero(t, record.DeductedEstimatedCostMicros)
	require.Zero(t, record.DeductedUnknownCredit)
	require.Equal(t, CreditValuationRuleVersion, record.ValuationRuleVersion)
	require.Equal(t, int64(1), record.SettlementVersion)
}
func TestCreditOrderRecoveryUsesImmutablePurchaseFactsAndTerminalReplay(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&TopUp{}, &InvitationRewardEvent{}, &InvitationCommissionRecord{}))
	user, _, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderStripe)
	purchase := completeCreditValuationOrder(t, db, &order)

	capturedAt := common.GetTimestamp()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
			UserId: user.Id, GrossCredit: 1_000, IdempotencyKey: "credit-recovery-mixed-ingress",
			SourceType: CreditBalanceLedgerSourceAdminAdjustment, SourceId: 91_099,
			Type: CreditBalanceLedgerTypeAdminIncrease, TargetPlanId: creditPlan.Id,
			TargetPlanSnapshot: &creditPlan, Reason: "second exact ingress",
			ValuationSource: &CreditValuationSourceSnapshot{
				SourcePriceMicros: 80_000_000, SourcePlanCredit: 1_000, GrossCredit: 1_000,
				SourceCurrency: "CNY", ValuationCurrency: "CNY", RuleVersion: CreditValuationRuleVersion,
				FXRateSnapshot: &CreditFXRateSnapshot{
					SourceCurrency: "CNY", ValuationCurrency: "CNY", Numerator: 1, Denominator: 1,
					CapturedAt: capturedAt, Direction: CreditFXDirectionIdentity,
				},
			},
		})
		return err
	}))

	var purchaseLedger CreditBalanceLedger
	require.NoError(t, db.First(&purchaseLedger, purchase.CreditBalance.LedgerId).Error)
	originalSnapshot := order.EntitlementSnapshot
	require.NoError(t, db.Model(&SubscriptionOrder{}).Where("id = ?", order.Id).Updates(map[string]any{
		"credit_grant_amount": int64(1), "credit_target_plan_id": creditPlan.Id + 99,
		"entitlement_snapshot": `{"corrupted":true}`,
	}).Error)

	request := SubscriptionOrderRecoveryRequest{
		TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
		RecoveryType: SubscriptionOrderRecoveryRefund, ProviderPayload: `{"event":"refund-v1"}`,
		OperatorUserId: 91_070, Reason: "provider refund immutable facts",
	}
	first, err := RecoverSubscriptionOrder(request)
	require.NoError(t, err)
	require.False(t, first.Replayed)
	require.Equal(t, int64(1_000), first.GrossCredit)
	require.Equal(t, int64(1_000), first.AvailableCredit)

	var state CreditValuationState
	require.NoError(t, db.Where("user_subscription_id = ?", purchase.CreditBalance.UserSubscriptionId).First(&state).Error)
	require.Equal(t, int64(1_000), state.AvailableCredit)
	require.Equal(t, int64(60_000_000), state.ExactCostMicros)
	require.Equal(t, int64(3), state.StateVersion)

	var recoveryLedger CreditBalanceLedger
	require.NoError(t, db.First(&recoveryLedger, first.LedgerId).Error)
	require.Equal(t, originalSnapshot, recoveryLedger.SourceSnapshot)
	require.Equal(t, purchaseLedger.SourcePriceMicros, recoveryLedger.SourcePriceMicros)
	require.Equal(t, purchaseLedger.SourcePlanCredit, recoveryLedger.SourcePlanCredit)
	require.Equal(t, purchaseLedger.FxSourceCurrency, recoveryLedger.FxSourceCurrency)
	require.Equal(t, purchaseLedger.FxRateNumerator, recoveryLedger.FxRateNumerator)
	require.Equal(t, purchaseLedger.FxRateDenominator, recoveryLedger.FxRateDenominator)
	require.Equal(t, purchaseLedger.FxCapturedAt, recoveryLedger.FxCapturedAt)
	require.Equal(t, purchaseLedger.FxDirection, recoveryLedger.FxDirection)
	require.Equal(t, int64(60_000_000), recoveryLedger.ValuationGrossCostMicros)
	require.NotEmpty(t, recoveryLedger.ParameterFingerprint)

	replay, err := RecoverSubscriptionOrder(request)
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.Equal(t, first.LedgerId, replay.LedgerId)
	var replayState CreditValuationState
	require.NoError(t, db.Where("user_subscription_id = ?", purchase.CreditBalance.UserSubscriptionId).First(&replayState).Error)
	var replayLedgerCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Count(&replayLedgerCount).Error)
	payloadConflict := request
	payloadConflict.ProviderPayload = `{"event":"refund-v2"}`
	conflicted, err := RecoverSubscriptionOrder(payloadConflict)
	require.Nil(t, conflicted)
	require.ErrorIs(t, err, ErrSubscriptionOrderRecoveryConflict)
	var conflictState CreditValuationState
	require.NoError(t, db.Where("user_subscription_id = ?", purchase.CreditBalance.UserSubscriptionId).First(&conflictState).Error)
	require.Equal(t, replayState, conflictState)
	var conflictLedgerCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Count(&conflictLedgerCount).Error)
	require.Equal(t, replayLedgerCount, conflictLedgerCount)

	reasonConflict := request
	reasonConflict.Reason = "different recovery facts"
	conflicted, err = RecoverSubscriptionOrder(reasonConflict)
	require.Nil(t, conflicted)
	require.ErrorIs(t, err, ErrSubscriptionOrderRecoveryConflict)

	chargebackRequest := request
	chargebackRequest.RecoveryType = SubscriptionOrderRecoveryChargeback
	chargebackRequest.Reason = "provider chargeback terminal"
	chargeback, err := RecoverSubscriptionOrder(chargebackRequest)
	require.NoError(t, err)
	require.True(t, chargeback.Replayed)
	require.Equal(t, common.TopUpStatusChargeback, chargeback.Status)
	chargebackReplay, err := RecoverSubscriptionOrder(chargebackRequest)
	require.NoError(t, err)
	require.True(t, chargebackReplay.Replayed)
	require.Equal(t, first.LedgerId, chargebackReplay.LedgerId)

	var recoveryCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).
		Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).
		Count(&recoveryCount).Error)
	require.Equal(t, int64(1), recoveryCount)
	require.NoError(t, db.Where("user_subscription_id = ?", purchase.CreditBalance.UserSubscriptionId).First(&state).Error)
	require.Equal(t, int64(3), state.StateVersion)
}

func TestCreditOrderRecoveryCancelsOnlyMatchingInvitationReward(t *testing.T) {
	tests := []struct {
		name         string
		recoveryType string
		wantStatus   string
	}{
		{name: "refund", recoveryType: SubscriptionOrderRecoveryRefund, wantStatus: common.TopUpStatusRefunded},
		{name: "chargeback", recoveryType: SubscriptionOrderRecoveryChargeback, wantStatus: common.TopUpStatusChargeback},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupCreditValuationTracerTestDB(t)
			require.NoError(t, db.AutoMigrate(&TopUp{}, &InvitationRewardEvent{}, &InvitationCommissionRecord{}))
			user, _, _, targetOrder := seedCreditValuationOrder(t, db, PaymentProviderStripe)
			inviter := User{Id: 91_090, Username: "credit-recovery-inviter", Status: common.UserStatusEnabled, AffCode: "credit-recovery-inviter-aff"}
			require.NoError(t, db.Create(&inviter).Error)
			require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("inviter_id", inviter.Id).Error)

			targetPurchase := completeCreditValuationOrder(t, db, &targetOrder)
			otherOrder := targetOrder
			otherOrder.Id++
			otherOrder.TradeNo += "-other"
			require.NoError(t, db.Create(&otherOrder).Error)
			otherPurchase := completeCreditValuationOrder(t, db, &otherOrder)

			var generatedRewardCount int64
			require.NoError(t, db.Model(&InvitationRewardEvent{}).Count(&generatedRewardCount).Error)
			require.Zero(t, generatedRewardCount, "Credit orders must not create invitation income")

			now := common.GetTimestamp()
			require.NoError(t, db.Create(&[]InvitationRewardEvent{
				{
					Id: 91_092, InviterId: inviter.Id, InviteeId: user.Id,
					SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: targetOrder.Id,
					SourceOrderId: targetOrder.Id, SourceSubscriptionId: targetPurchase.CreditBalance.UserSubscriptionId,
					SourceAmountCents: 4_000, SourceCurrency: "CNY", EventStartTime: now, EventEndTime: now + 3_600,
					Status: InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now,
				},
				{
					Id: 91_093, InviterId: inviter.Id, InviteeId: user.Id,
					SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: otherOrder.Id,
					SourceOrderId: otherOrder.Id, SourceSubscriptionId: otherPurchase.CreditBalance.UserSubscriptionId,
					SourceAmountCents: 4_000, SourceCurrency: "CNY", EventStartTime: now, EventEndTime: now + 3_600,
					Status: InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now,
				},
			}).Error)
			var targetReward InvitationRewardEvent
			require.NoError(t, db.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, targetOrder.Id).First(&targetReward).Error)
			var otherRewardBefore InvitationRewardEvent
			require.NoError(t, db.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, otherOrder.Id).First(&otherRewardBefore).Error)
			var rewardCountBefore int64
			require.NoError(t, db.Model(&InvitationRewardEvent{}).Count(&rewardCountBefore).Error)
			require.Equal(t, int64(2), rewardCountBefore)
			var commissionCountBefore int64
			require.NoError(t, db.Model(&InvitationCommissionRecord{}).Count(&commissionCountBefore).Error)
			require.Zero(t, commissionCountBefore)
			paidSummary, err := GetAdminInvitationPaidSubscriptionsSummary(AdminAnalyticsQuery{
				SnapshotAt: now + 1, EndTimestamp: now + 1, RangeMode: AdminAnalyticsRangeModeAllHistory,
				UserIDs: []int{user.Id}, Currency: "CNY", Limit: 20,
			})
			require.NoError(t, err)
			require.Zero(t, paidSummary.Data.Summary.PaidInviteeCount)
			require.Empty(t, paidSummary.Data.Summary.RecognizedInvitationPaidAmountByCurrency)

			request := SubscriptionOrderRecoveryRequest{
				TradeNo: targetOrder.TradeNo, ExpectedUserId: user.Id,
				ExpectedPaymentProvider: PaymentProviderStripe, RecoveryType: test.recoveryType,
				ProviderPayload: `{"event":"isolated-credit-recovery"}`,
				OperatorUserId:  91_091, Reason: "isolated Credit order recovery",
			}
			first, err := RecoverSubscriptionOrder(request)
			require.NoError(t, err)
			require.False(t, first.Replayed)
			require.Equal(t, test.wantStatus, first.Status)

			require.NoError(t, db.First(&targetReward, targetReward.Id).Error)
			require.Equal(t, InvitationRewardEventStatusCancelled, targetReward.Status)
			require.Equal(t, request.Reason, targetReward.Reason)
			var otherRewardAfter InvitationRewardEvent
			require.NoError(t, db.First(&otherRewardAfter, otherRewardBefore.Id).Error)
			require.Equal(t, otherRewardBefore, otherRewardAfter)

			type recoverySnapshot struct {
				Order               SubscriptionOrder
				TargetReward        InvitationRewardEvent
				OtherReward         InvitationRewardEvent
				Subscription        UserSubscription
				Valuation           CreditValuationState
				LedgerCount         int64
				RecoveryLedgerCount int64
				RewardCount         int64
				CommissionCount     int64
			}
			capture := func() recoverySnapshot {
				var snapshot recoverySnapshot
				require.NoError(t, db.First(&snapshot.Order, targetOrder.Id).Error)
				require.NoError(t, db.First(&snapshot.TargetReward, targetReward.Id).Error)
				require.NoError(t, db.First(&snapshot.OtherReward, otherRewardBefore.Id).Error)
				require.NoError(t, db.First(&snapshot.Subscription, targetPurchase.CreditBalance.UserSubscriptionId).Error)
				require.NoError(t, db.First(&snapshot.Valuation, targetPurchase.CreditBalance.UserSubscriptionId).Error)
				require.NoError(t, db.Model(&CreditBalanceLedger{}).Count(&snapshot.LedgerCount).Error)
				require.NoError(t, db.Model(&CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, targetOrder.Id).Count(&snapshot.RecoveryLedgerCount).Error)
				require.NoError(t, db.Model(&InvitationRewardEvent{}).Count(&snapshot.RewardCount).Error)
				require.NoError(t, db.Model(&InvitationCommissionRecord{}).Count(&snapshot.CommissionCount).Error)
				return snapshot
			}

			beforeReplay := capture()
			require.Equal(t, int64(1), beforeReplay.RecoveryLedgerCount)
			require.Equal(t, rewardCountBefore, beforeReplay.RewardCount)
			require.Equal(t, commissionCountBefore, beforeReplay.CommissionCount)
			replayed, err := RecoverSubscriptionOrder(request)
			require.NoError(t, err)
			require.True(t, replayed.Replayed)
			require.Equal(t, first.LedgerId, replayed.LedgerId)
			require.Equal(t, beforeReplay, capture())
		})
	}
}

func TestCreditValuationRequestFinalizesSameTargetIdempotently(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-valuation-final-200"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 200, true))

	var firstRecord SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&firstRecord).Error)
	require.Equal(t, "settled", firstRecord.Status)
	require.NotZero(t, firstRecord.FinalizedAt)
	require.Equal(t, int64(200), firstRecord.AppliedCredit)
	require.Equal(t, int64(1), firstRecord.SettlementVersion)

	var firstState CreditValuationState
	require.NoError(t, db.First(&firstState, firstRecord.ValuationSubscriptionId).Error)
	require.Equal(t, int64(800), firstState.AvailableCredit)
	require.Equal(t, int64(32_000_000), firstState.ExactCostMicros)
	require.Equal(t, int64(2), firstState.StateVersion)

	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 200, true))
	var replayedRecord SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&replayedRecord).Error)
	require.Equal(t, firstRecord.FinalizedAt, replayedRecord.FinalizedAt)
	require.Equal(t, firstRecord.SettlementVersion, replayedRecord.SettlementVersion)
	var replayedState CreditValuationState
	require.NoError(t, db.First(&replayedState, firstRecord.ValuationSubscriptionId).Error)
	require.Equal(t, firstState.StateVersion, replayedState.StateVersion)
}

func TestCreditValuationFiveAnalyticsViewsAgreeOnThirtyTwoCNY(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completeCreditValuationOrder(t, db, &order)
	const requestID = "credit-valuation-five-views"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 200, true))

	query := AdminAnalyticsQuery{
		SnapshotAt:   GetDBTimestamp(),
		EndTimestamp: GetDBTimestamp(),
		RangeMode:    AdminAnalyticsRangeModeSnapshot,
		Currency:     "CNY",
		Limit:        20,
	}
	summary, err := GetAdminPaidSubscriptionValueSummary(query)
	require.NoError(t, err)
	users, err := GetAdminPaidSubscriptionValueUsers(query)
	require.NoError(t, err)
	subscriptions, err := GetAdminPaidSubscriptionValueSubscriptions(query)
	require.NoError(t, err)
	plans, err := GetAdminPaidSubscriptionValuePlanBreakdown(query)
	require.NoError(t, err)
	sources, err := GetAdminPaidSubscriptionValueSourceBreakdown(query)
	require.NoError(t, err)

	require.Equal(t, 1, summary.Data.Summary.ActivePaidSubscriptionCount)
	require.Equal(t, int64(32_000_000), moneyBreakdownMicros(summary.Data.Summary.RecognizedRemainingValueByCurrency, "CNY"))
	require.Equal(t, int64(32_000_000), moneyBreakdownMicros(summary.Data.Summary.ExactRemainingValueByCurrency, "CNY"))
	require.Zero(t, moneyBreakdownMicros(summary.Data.Summary.EstimatedRemainingValueByCurrency, "CNY"))
	require.Zero(t, summary.Data.Summary.UnknownCostCredit)

	require.Len(t, users.Data.Users.Items, 1)
	require.Equal(t, int64(32_000_000), moneyBreakdownMicros(users.Data.Users.Items[0].RecognizedRemainingValueByCurrency, "CNY"))
	require.Len(t, subscriptions.Data.Subscriptions.Items, 1)
	item := subscriptions.Data.Subscriptions.Items[0]
	require.Equal(t, SubscriptionEntitlementCreditBalance, item.EntitlementType)
	require.Equal(t, int64(800), item.AvailableCredit)
	require.Equal(t, "32000000", item.RecognizedRemainingValue.AmountMicros)
	require.Equal(t, "32000000", item.ExactRemainingValue.AmountMicros)
	require.Nil(t, item.TimeBasedValue)
	require.Equal(t, "credit_moving_weighted_average", item.ValuationBasis)
	require.Equal(t, "credit_balance_pool", string(item.Source))
	require.Equal(t, "moving_weighted_pool", item.SourceAttribution)

	require.Len(t, plans.Data.Plans.Items, 1)
	require.Equal(t, creditPlan.Id, plans.Data.Plans.Items[0].PlanID)
	require.Equal(t, int64(32_000_000), moneyBreakdownMicros(plans.Data.Plans.Items[0].RecognizedRemainingValueByCurrency, "CNY"))
	require.Len(t, sources.Data.Sources.Items, 1)
	require.Equal(t, "credit_balance_pool", string(sources.Data.Sources.Items[0].Source))
	require.Equal(t, "moving_weighted_pool", sources.Data.Sources.Items[0].SourceAttribution)
	require.Equal(t, int64(32_000_000), moneyBreakdownMicros(sources.Data.Sources.Items[0].RecognizedRemainingValueByCurrency, "CNY"))
}

func TestCreditValuationFiveAnalyticsPanelsReturnCurrentOnlyWarning(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	result := completeCreditValuationOrder(t, db, &order)
	const requestID = "credit-valuation-current-only-warning"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 200, true))

	snapshotAt := GetDBTimestamp()
	updatedAt := snapshotAt + 10
	require.NoError(t, db.Model(&CreditValuationState{}).Where("user_subscription_id = ?", result.CreditBalance.UserSubscriptionId).Update("updated_at", updatedAt).Error)
	query := AdminAnalyticsQuery{SnapshotAt: snapshotAt, EndTimestamp: snapshotAt, RangeMode: AdminAnalyticsRangeModeSnapshot, Currency: "CNY", Limit: 20}
	expected := []dto.AdminAnalyticsAvailabilityWarning{{Section: "credit_valuation", Reason: "current_only", Message: "credit valuation state is newer than snapshot"}}
	panels := []struct {
		name string
		load func(AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminPaidSubscriptionValueResponse], error)
	}{
		{name: "summary", load: GetAdminPaidSubscriptionValueSummary},
		{name: "users", load: GetAdminPaidSubscriptionValueUsers},
		{name: "subscriptions", load: GetAdminPaidSubscriptionValueSubscriptions},
		{name: "plans", load: GetAdminPaidSubscriptionValuePlanBreakdown},
		{name: "sources", load: GetAdminPaidSubscriptionValueSourceBreakdown},
	}
	for _, panel := range panels {
		t.Run(panel.name, func(t *testing.T) {
			response, err := panel.load(query)
			require.NoError(t, err)
			require.Equal(t, expected, response.Warnings)
		})
	}

	subscriptions, err := GetAdminPaidSubscriptionValueSubscriptions(query)
	require.NoError(t, err)
	require.Len(t, subscriptions.Data.Subscriptions.Items, 1)
	item := subscriptions.Data.Subscriptions.Items[0]
	require.Equal(t, adminPaidSubscriptionSnapshotSemanticsCurrentOnly, item.SnapshotSemantics)
	require.Equal(t, int64(2), item.ValuationStateVersion)
	require.Equal(t, updatedAt, item.ValuationUpdatedAt)

	query.SnapshotAt = updatedAt
	query.EndTimestamp = updatedAt
	for _, panel := range panels {
		t.Run(panel.name+"_without_current_only", func(t *testing.T) {
			response, err := panel.load(query)
			require.NoError(t, err)
			require.Empty(t, response.Warnings)
		})
	}
}

func TestTimedConversionRealPathFeedsFiveAnalyticsWithoutNewPaymentAttribution(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	const (
		userID       = 92_001
		timedPlanID  = 92_002
		creditPlanID = 92_003
		sourceID     = 92_004
		creditBasis  = int64(100)
	)
	require.NoError(t, db.AutoMigrate(&InvitationRewardEvent{}))
	now := GetDBTimestamp()
	valuationCurrency := "CNY"
	priceMicros := int64(40_000_000)
	timedCode := "conversion-five-analytics-timed"
	creditCode := "conversion-five-analytics-credit"
	require.NoError(t, db.Create(&User{Id: userID, Username: "conversion-five-analytics", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: timedPlanID, Title: "40 CNY / 100 timed Credit", EntitlementType: SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: SubscriptionResetMonthly, MonthlyTokenLimit: creditBasis, TimedConversionEnabled: true,
		PriceAmountMicros: &priceMicros, PriceAmount: 40, Currency: "CNY",
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: creditPlanID, Title: "Credit balance", EntitlementType: SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true,
		CreditBalanceConversionEnabled: true, ValuationCurrency: &valuationCurrency,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id: sourceID, UserId: userID, PlanId: timedPlanID, EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit: creditBasis, TokenUsed: 20, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 40*24*60*60, EndTime: now + TimedSubscriptionConversionBlockSeconds + 60,
		Status: SubscriptionStatusActive, LastGrantedAt: now - TimedSubscriptionConversionCooldownSeconds - 60,
		LastGrantCreditSnapshot: pointerToInt64(creditBasis), LastGrantTimeSource: SubscriptionGrantTimeSourceLive,
		LastGrantSource: SubscriptionGrantOrder,
	}).Error)

	quote, err := ListTimedSubscriptionConversionQuotes(userID)
	require.NoError(t, err)
	require.Len(t, quote.Quotes, 1)
	require.Equal(t, int64(180), quote.Quotes[0].GrossCredit)
	conversion, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "conversion-five-analytics")
	require.NoError(t, err)
	require.False(t, conversion.Replayed)
	targetID := conversion.Conversion.TargetSubscriptionId

	query := AdminAnalyticsQuery{SnapshotAt: GetDBTimestamp(), EndTimestamp: GetDBTimestamp(), RangeMode: AdminAnalyticsRangeModeSnapshot, Currency: "CNY", Limit: 20}
	summary, err := GetAdminPaidSubscriptionValueSummary(query)
	require.NoError(t, err)
	users, err := GetAdminPaidSubscriptionValueUsers(query)
	require.NoError(t, err)
	subscriptions, err := GetAdminPaidSubscriptionValueSubscriptions(query)
	require.NoError(t, err)
	plans, err := GetAdminPaidSubscriptionValuePlanBreakdown(query)
	require.NoError(t, err)
	sources, err := GetAdminPaidSubscriptionValueSourceBreakdown(query)
	require.NoError(t, err)

	require.Equal(t, 1, summary.Data.Summary.ActivePaidSubscriptionCount)
	require.Equal(t, int64(72_000_000), moneyBreakdownMicros(summary.Data.Summary.RecognizedRemainingValueByCurrency, "CNY"))
	require.Len(t, users.Data.Users.Items, 1)
	require.Equal(t, int64(72_000_000), moneyBreakdownMicros(users.Data.Users.Items[0].RecognizedRemainingValueByCurrency, "CNY"))
	require.Len(t, subscriptions.Data.Subscriptions.Items, 1)
	item := subscriptions.Data.Subscriptions.Items[0]
	require.Equal(t, targetID, item.SubscriptionID)
	require.Equal(t, SubscriptionEntitlementCreditBalance, item.EntitlementType)
	require.Equal(t, int64(180), item.AvailableCredit)
	require.Equal(t, "72000000", item.RecognizedRemainingValue.AmountMicros)
	require.Equal(t, "credit_moving_weighted_average", item.ValuationBasis)
	require.Equal(t, "credit_balance_pool", string(item.Source))
	require.Equal(t, "moving_weighted_pool", item.SourceAttribution)
	require.Len(t, plans.Data.Plans.Items, 1)
	require.Equal(t, creditPlanID, plans.Data.Plans.Items[0].PlanID)
	require.Len(t, sources.Data.Sources.Items, 1)
	require.Equal(t, "credit_balance_pool", string(sources.Data.Sources.Items[0].Source))
	require.Equal(t, int64(72_000_000), moneyBreakdownMicros(sources.Data.Sources.Items[0].RecognizedRemainingValueByCurrency, "CNY"))

	var orderCount int64
	require.NoError(t, db.Model(&SubscriptionOrder{}).Where("user_id = ?", userID).Count(&orderCount).Error)
	require.Zero(t, orderCount, "conversion must not be represented as a new payment")
	var rewardCount int64
	require.NoError(t, db.Model(&InvitationRewardEvent{}).Where("invitee_id = ?", userID).Count(&rewardCount).Error)
	require.Zero(t, rewardCount, "conversion must not enter invitation income")

	conversionAnalytics, err := GetAdminAnalyticsSubscriptionConversion(query)
	require.NoError(t, err)
	require.Equal(t, 1, conversionAnalytics.Data.Summary.ConversionCount)
	require.Equal(t, int64(180), conversionAnalytics.Data.Summary.GrossCredit)
	require.Equal(t, int64(180), conversionAnalytics.Data.Summary.NetAvailableCredit)
	require.Equal(t, int64(72_000_000), moneyBreakdownMicros(conversionAnalytics.Data.Summary.GrossValueByCurrency, "CNY"))
	require.Equal(t, int64(72_000_000), moneyBreakdownMicros(conversionAnalytics.Data.Summary.NetValueByCurrency, "CNY"))
	require.Equal(t, 1, conversionAnalytics.Data.Summary.ExactConversionCount)
}

func moneyBreakdownMicros(items []dto.AdminAnalyticsMoneyBreakdown, currency string) int64 {
	for _, item := range items {
		if item.Currency == currency {
			value, _ := strconv.ParseInt(item.AmountMicros, 10, 64)
			return value
		}
	}
	return 0
}
