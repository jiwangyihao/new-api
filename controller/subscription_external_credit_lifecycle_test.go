package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
)

func TestExternalCreditPurchaseWebhookAndRefundLifecycle(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.InvitationMonthlyEntitlement{},
		&model.InvitationCommissionAccount{},
		&model.InvitationCommissionLedger{},
		&model.GPTAbuseSignalLog{},
		&model.GPTAbuseUserSuspension{},
		&gptAbuseWarningResetTableForSubscriptionSelfTest{},
		&model.ChannelGroup{},
		&model.ChannelGroupChannel{},
		&model.TokenGroupBinding{},
	))
	model.ClearSubscriptionPlanCacheForTest()
	model.ClearPrimaryBillableSubscriptionCacheForTest()
	t.Cleanup(func() {
		model.ClearSubscriptionPlanCacheForTest()
		model.ClearPrimaryBillableSubscriptionCacheForTest()
	})

	commissionSetting := operation_setting.GetInvitationCommissionSetting()
	originalCommissionSetting := *commissionSetting
	*commissionSetting = operation_setting.InvitationCommissionSetting{
		Enabled:              true,
		RateBps:              1000,
		MinimumWithdrawCents: 1000,
		MinimumTransferCents: 1,
	}
	t.Cleanup(func() { *commissionSetting = originalCommissionSetting })

	const (
		buyerID                              = 8801
		optionPlanID                         = 9971
		creditPlanID                         = 9972
		existingCreditSubscriptionID         = 9973
		inviterID                            = 9974
		unrelatedTimedPlanID                 = 9975
		unrelatedTimedSubscriptionID         = 9976
		unrelatedTimedOrderID                = 9977
		unrelatedInvitationEventID           = 9978
		unrelatedCommissionRecordID          = 9979
		existingCreditConcurrencyLimit       = 2
		standardOptionConcurrencyLimit       = 3
		globalCreditConcurrencyLimit         = 7
		globalCreditQueueCapacity            = 11
		initialCreditLimit             int64 = 300
		initialCreditUsed              int64 = 100
		purchasedCredit                int64 = 1000
	)

	seedExternalCreditPurchasePlans(t, optionPlanID, creditPlanID)
	standardCode := "standard_monthly_external_credit_lifecycle"
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", optionPlanID).Updates(map[string]any{
		"title":             "Standard Monthly",
		"business_code":     standardCode,
		"model_limits":      "gpt-4o-mini",
		"concurrency_limit": standardOptionConcurrencyLimit,
	}).Error)
	model.InvalidateSubscriptionPlanCache(optionPlanID)
	model.InvalidateSubscriptionPlanCache(creditPlanID)

	now := common.GetTimestamp()
	inviter := model.User{
		Id:                   inviterID,
		Username:             "external-credit-inviter",
		Status:               common.UserStatusEnabled,
		AffCode:              "external-credit-inviter",
		InvitationRewardMode: model.InvitationRewardModeCommission,
	}
	require.NoError(t, model.DB.Create(&inviter).Error)

	existingCredit := model.UserSubscription{
		Id:               existingCreditSubscriptionID,
		UserId:           buyerID,
		PlanId:           creditPlanID,
		EntitlementType:  model.SubscriptionEntitlementCreditBalance,
		TokenLimit:       initialCreditLimit,
		TokenUsed:        initialCreditUsed,
		ConcurrencyLimit: existingCreditConcurrencyLimit,
		GrantReason:      model.SubscriptionGrantOrder,
		StartTime:        now - 7200,
		EndTime:          0,
		Status:           "active",
		Source:           model.SubscriptionGrantOrder,
	}
	require.NoError(t, model.DB.Create(&existingCredit).Error)
	require.NoError(t, model.DB.Create(&model.CreditBalanceLedger{
		UserId:               buyerID,
		UserSubscriptionId:   existingCreditSubscriptionID,
		Type:                 model.CreditBalanceLedgerTypeAdminIncrease,
		IdempotencyKey:       "existing-credit-lifecycle-grant",
		SourceType:           model.CreditBalanceLedgerSourceAdminAdjustment,
		SourceId:             existingCreditSubscriptionID,
		GrossCredit:          initialCreditLimit,
		BalanceBefore:        0,
		BalanceAfter:         initialCreditLimit,
		AvailableCreditAfter: initialCreditLimit,
		Reason:               "existing Credit fixture",
		CreatedAt:            now - 7000,
	}).Error)

	var buyer model.User
	require.NoError(t, model.DB.First(&buyer, buyerID).Error)
	buyerSetting := buyer.GetSetting()
	buyerSetting.ActiveSubscriptionId = existingCreditSubscriptionID
	buyer.SetSetting(buyerSetting)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", buyerID).Updates(map[string]any{
		"inviter_id": inviterID,
		"setting":    buyer.Setting,
	}).Error)

	unrelatedTimedCode := "unrelated_timed_purchase_lifecycle"
	unrelatedTimedPlan := model.SubscriptionPlan{
		Id:                unrelatedTimedPlanID,
		Title:             "Unrelated historical timed purchase",
		PriceAmount:       40,
		Currency:          "CNY",
		DurationUnit:      model.SubscriptionDurationMonth,
		DurationValue:     1,
		Enabled:           true,
		PublicVisible:     true,
		MonthlyTokenLimit: 1000,
		ConcurrencyLimit:  4,
		RewardEligible:    true,
		BusinessCode:      &unrelatedTimedCode,
		StripePriceId:     "price_unrelated_timed",
	}
	require.NoError(t, model.DB.Create(&unrelatedTimedPlan).Error)
	unrelatedTimed := model.UserSubscription{
		Id:               unrelatedTimedSubscriptionID,
		UserId:           buyerID,
		PlanId:           unrelatedTimedPlanID,
		EntitlementType:  model.SubscriptionEntitlementTimed,
		TokenLimit:       1000,
		TokenUsed:        250,
		ConcurrencyLimit: 4,
		GrantReason:      model.SubscriptionGrantOrder,
		StartTime:        now - 7200,
		EndTime:          now - 60,
		Status:           "active",
		Source:           model.SubscriptionGrantOrder,
	}
	require.NoError(t, model.DB.Create(&unrelatedTimed).Error)
	unrelatedSnapshot := model.NewSubscriptionEntitlementSnapshot(&unrelatedTimedPlan, model.SubscriptionPurchaseModeTimed, 0)
	unrelatedSnapshot.SetPaymentSnapshot(model.PaymentProviderStripe, unrelatedTimedPlan.StripePriceId, model.PaymentMethodStripe, 4000, "CNY")
	unrelatedSnapshotJSON, err := model.MarshalSubscriptionEntitlementSnapshot(unrelatedSnapshot)
	require.NoError(t, err)
	unrelatedOrder := model.SubscriptionOrder{
		Id:                      unrelatedTimedOrderID,
		UserId:                  buyerID,
		PlanId:                  unrelatedTimedPlanID,
		Money:                   40,
		AmountCents:             4000,
		Currency:                "CNY",
		FulfilledSubscriptionID: unrelatedTimedSubscriptionID,
		TradeNo:                 "unrelated-timed-order-lifecycle",
		PaymentMethod:           model.PaymentMethodStripe,
		PaymentProvider:         model.PaymentProviderStripe,
		ProviderTransactionID:   "pi_unrelated_timed_lifecycle",
		Status:                  common.TopUpStatusSuccess,
		CreateTime:              now - 7200,
		CompleteTime:            now - 7100,
		EntitlementSnapshot:     unrelatedSnapshotJSON,
	}
	require.NoError(t, model.DB.Create(&unrelatedOrder).Error)
	unrelatedReward := model.InvitationRewardEvent{
		Id:                   unrelatedInvitationEventID,
		InviterId:            inviterID,
		InviteeId:            buyerID,
		SourceType:           model.InvitationRewardEventSourceSubscriptionOrder,
		SourceId:             unrelatedTimedOrderID,
		SourceOrderId:        unrelatedTimedOrderID,
		SourceSubscriptionId: unrelatedTimedSubscriptionID,
		SourceAmountCents:    4000,
		SourceCurrency:       "CNY",
		EventStartTime:       unrelatedTimed.StartTime,
		EventEndTime:         unrelatedTimed.EndTime,
		Status:               model.InvitationRewardEventStatusActive,
		CreatedAt:            now - 7100,
		UpdatedAt:            now - 7100,
	}
	require.NoError(t, model.DB.Create(&unrelatedReward).Error)
	unrelatedCommission := model.InvitationCommissionRecord{
		Id:                unrelatedCommissionRecordID,
		EventId:           unrelatedInvitationEventID,
		InviterId:         inviterID,
		InviteeId:         buyerID,
		SourceType:        model.InvitationCommissionSourceSubscriptionOrder,
		SourceId:          unrelatedTimedOrderID,
		SourceTradeNo:     unrelatedOrder.TradeNo,
		SourceAmountCents: 4000,
		SourceCurrency:    "CNY",
		CommissionRateBps: 1000,
		CommissionCents:   400,
		Status:            model.InvitationCommissionStatusAvailable,
		CreatedAt:         now - 7100,
		AvailableAt:       now - 7100,
	}
	require.NoError(t, model.DB.Create(&unrelatedCommission).Error)
	unrelatedCommissionAccount := model.InvitationCommissionAccount{
		UserId:         inviterID,
		AvailableCents: 400,
		CreatedAt:      now - 7100,
		UpdatedAt:      now - 7100,
	}
	require.NoError(t, model.DB.Create(&unrelatedCommissionAccount).Error)
	require.NoError(t, model.DB.Create(&model.InvitationCommissionLedger{
		UserId:              inviterID,
		Type:                model.InvitationCommissionLedgerEarned,
		AmountCents:         400,
		AvailableAfterCents: 400,
		ReferenceType:       model.InvitationCommissionSourceSubscriptionOrder,
		ReferenceId:         unrelatedTimedOrderID,
		CreatedAt:           now - 7100,
	}).Error)

	setting.StripeApiSecret = "sk_test_external_credit_lifecycle"
	setting.StripeWebhookSecret = "whsec_external_credit_lifecycle"
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceID string) (int64, string, error) {
		require.Equal(t, "price_test", priceID)
		return 4000, "CNY", nil
	})
	SetStripeSubscriptionCheckoutForTest(t, func(referenceID string, customerID string, email string, priceID string) (StripeSubscriptionCheckoutResult, error) {
		require.NotEmpty(t, referenceID)
		require.Equal(t, "price_test", priceID)
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/external-credit-lifecycle"}, nil
	})

	create := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9971,"purchase_mode":"credit_balance"}`)
	require.Equal(t, http.StatusOK, create.Code, create.Body.String())
	require.Contains(t, create.Body.String(), `"message":"success"`)

	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", buyerID, optionPlanID).First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Equal(t, optionPlanID, order.PlanId)
	assert.Equal(t, float64(40), order.Money)
	assert.Equal(t, int64(4000), order.AmountCents)
	assert.Equal(t, "CNY", order.Currency)
	assert.Equal(t, purchasedCredit, order.CreditGrantAmount)
	assert.Equal(t, creditPlanID, order.CreditTargetPlanID)

	snapshot, err := model.UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
	require.NoError(t, err)
	assert.Equal(t, model.SubscriptionPurchaseModeCreditBalance, snapshot.PurchaseMode)
	assert.Equal(t, optionPlanID, snapshot.PlanID)
	assert.Equal(t, "Standard Monthly", snapshot.PlanTitle)
	assert.Equal(t, standardCode, snapshot.BusinessCode)
	assert.Equal(t, model.SubscriptionEntitlementTimed, snapshot.PlanEntitlementType)
	assert.Equal(t, float64(40), snapshot.PriceAmount)
	assert.Equal(t, "CNY", snapshot.Currency)
	assert.Equal(t, model.SubscriptionDurationMonth, snapshot.DurationUnit)
	assert.Equal(t, 1, snapshot.DurationValue)
	assert.Equal(t, purchasedCredit, snapshot.MonthlyTokenLimit)
	assert.Equal(t, standardOptionConcurrencyLimit, snapshot.ConcurrencyLimit)
	assert.Equal(t, model.PaymentProviderStripe, snapshot.PaymentProvider)
	assert.Equal(t, model.PaymentMethodStripe, snapshot.ProviderPaymentMethod)
	assert.Equal(t, "price_test", snapshot.ProviderProductID)
	assert.Equal(t, int64(4000), snapshot.PaymentAmountCents)
	assert.Equal(t, "CNY", snapshot.PaymentCurrency)
	assert.Equal(t, creditPlanID, snapshot.TargetCreditBalancePlanID)
	assert.Equal(t, "Global Credit Balance", snapshot.TargetCreditBalanceTitle)
	assert.Equal(t, "gpt-4o", snapshot.TargetCreditBalanceModelLimits)
	assert.Equal(t, globalCreditConcurrencyLimit, snapshot.TargetCreditBalanceConcurrencyLimit)
	assert.Equal(t, globalCreditQueueCapacity, snapshot.TargetCreditBalanceQueueCapacity)
	var creditPlanCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("entitlement_type = ?", model.SubscriptionEntitlementCreditBalance).Count(&creditPlanCount).Error)
	assert.Equal(t, int64(1), creditPlanCount)

	completionPayload := stripeWebhookPayloadForSubscriptionTest(t, "evt_external_credit_lifecycle_complete", stripe.EventTypeCheckoutSessionCompleted, map[string]any{
		"id":                  "cs_external_credit_lifecycle",
		"customer":            "cus_external_credit_lifecycle",
		"client_reference_id": order.TradeNo,
		"status":              "complete",
		"payment_status":      "paid",
		"amount_total":        4000,
		"currency":            "cny",
		"payment_intent":      "pi_external_credit_lifecycle",
		"invoice":             "in_external_credit_lifecycle",
		"subscription":        "sub_external_credit_lifecycle",
		"metadata":            map[string]any{stripeSubscriptionProductMetadataKey: "price_test"},
	})
	firstCompletion := signedStripeWebhookRecorderForSubscriptionTest(t, completionPayload)
	require.Equal(t, http.StatusOK, firstCompletion.Code, firstCompletion.Body.String())

	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, existingCreditSubscriptionID, order.FulfilledSubscriptionID)
	assert.Equal(t, "pi_external_credit_lifecycle", order.ProviderTransactionID)
	assert.Equal(t, "cs_external_credit_lifecycle", order.ProviderOrderID)
	assert.Equal(t, "in_external_credit_lifecycle", order.ProviderInvoiceID)
	assert.Equal(t, "sub_external_credit_lifecycle", order.ProviderSubscriptionID)
	firstCompleteTime := order.CompleteTime

	var balance model.UserSubscription
	require.NoError(t, model.DB.First(&balance, existingCreditSubscriptionID).Error)
	assert.Equal(t, creditPlanID, balance.PlanId)
	assert.Equal(t, model.SubscriptionEntitlementCreditBalance, balance.EntitlementType)
	assert.Equal(t, initialCreditLimit+purchasedCredit, balance.TokenLimit)
	assert.Equal(t, initialCreditUsed, balance.TokenUsed)
	assert.Equal(t, existingCreditConcurrencyLimit, balance.ConcurrencyLimit, "the aggregate instance may be historical; live limits must come from the global Credit plan")

	var purchaseLedger model.CreditBalanceLedger
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrder, order.Id).First(&purchaseLedger).Error)
	assert.Equal(t, model.CreditBalanceLedgerTypePurchase, purchaseLedger.Type)
	assert.Equal(t, existingCreditSubscriptionID, purchaseLedger.UserSubscriptionId)
	assert.Equal(t, purchasedCredit, purchaseLedger.GrossCredit)
	assert.Equal(t, int64(200), purchaseLedger.BalanceBefore)
	assert.Equal(t, int64(1200), purchaseLedger.BalanceAfter)
	assert.Equal(t, int64(1200), purchaseLedger.AvailableCreditAfter)
	assert.Zero(t, purchaseLedger.SettlementDebtAfter)
	assert.Equal(t, model.PaymentProviderStripe, purchaseLedger.PaymentProvider)

	var creditSubscriptionCount, optionEntitlementCount, purchaseLedgerCount, totalCreditLedgerCount, topUpCount, orderCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", buyerID, model.SubscriptionEntitlementCreditBalance).Count(&creditSubscriptionCount).Error)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", buyerID, optionPlanID).Count(&optionEntitlementCount).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrder, order.Id).Count(&purchaseLedgerCount).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", buyerID).Count(&totalCreditLedgerCount).Error)
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", order.TradeNo).Count(&topUpCount).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("trade_no = ?", order.TradeNo).Count(&orderCount).Error)
	assert.Equal(t, int64(1), creditSubscriptionCount)
	assert.Zero(t, optionEntitlementCount, "the Standard purchase option must not become a second perpetual entitlement")
	assert.Equal(t, int64(1), purchaseLedgerCount)
	assert.Equal(t, int64(2), totalCreditLedgerCount)
	assert.Equal(t, int64(1), topUpCount)
	assert.Equal(t, int64(1), orderCount)

	var currentRewardCount, currentCommissionCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&currentRewardCount).Error)
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_type = ? AND source_id = ?", model.InvitationCommissionSourceSubscriptionOrder, order.Id).Count(&currentCommissionCount).Error)
	assert.Zero(t, currentRewardCount)
	assert.Zero(t, currentCommissionCount)
	assertExternalCreditLifecycleUnrelatedInvitationState(t, unrelatedTimedSubscriptionID, unrelatedTimedOrderID, unrelatedInvitationEventID, unrelatedCommissionRecordID, inviterID)

	afterCompletion := subscriptionSelfSummaryData(t, performGetSubscriptionSelfSummaryRequest(t, buyerID))
	completionSummary := requireSubscriptionSelfSummary(t, afterCompletion)
	assert.Equal(t, int64(existingCreditSubscriptionID), summaryInt64(t, completionSummary, "subscription_id"))
	assert.Equal(t, int64(creditPlanID), summaryInt64(t, completionSummary, "plan_id"))
	assert.Equal(t, int64(globalCreditConcurrencyLimit), summaryInt64(t, completionSummary, "concurrency_limit"))
	assert.Equal(t, int64(globalCreditQueueCapacity), summaryInt64(t, completionSummary, "queue_capacity"))
	completionCreditState, ok := afterCompletion["credit_balance"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(existingCreditSubscriptionID), completionCreditState["user_subscription_id"])
	assert.Equal(t, float64(creditPlanID), completionCreditState["plan_id"])
	assert.Equal(t, float64(1200), completionCreditState["available_credit"])
	assert.Equal(t, float64(0), completionCreditState["settlement_debt"])
	assert.Equal(t, model.CreditBalanceStatusAvailable, completionCreditState["status"])
	creditPlanState, ok := afterCompletion["credit_balance_plan"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(globalCreditConcurrencyLimit), creditPlanState["concurrency_limit"])
	assert.Equal(t, float64(globalCreditQueueCapacity), creditPlanState["queue_capacity"])
	hasSubscription, err := model.HasActiveUserSubscription(buyerID)
	require.NoError(t, err)
	assert.True(t, hasSubscription)

	completionReplay := signedStripeWebhookRecorderForSubscriptionTest(t, completionPayload)
	require.Equal(t, http.StatusOK, completionReplay.Code, completionReplay.Body.String())
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	require.NoError(t, model.DB.First(&balance, existingCreditSubscriptionID).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, firstCompleteTime, order.CompleteTime)
	assert.Equal(t, existingCreditSubscriptionID, order.FulfilledSubscriptionID)
	assert.Equal(t, initialCreditLimit+purchasedCredit, balance.TokenLimit)
	assert.Equal(t, initialCreditUsed, balance.TokenUsed)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", buyerID, model.SubscriptionEntitlementCreditBalance).Count(&creditSubscriptionCount).Error)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", buyerID, optionPlanID).Count(&optionEntitlementCount).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrder, order.Id).Count(&purchaseLedgerCount).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", buyerID).Count(&totalCreditLedgerCount).Error)
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", order.TradeNo).Count(&topUpCount).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("trade_no = ?", order.TradeNo).Count(&orderCount).Error)
	assert.Equal(t, int64(1), creditSubscriptionCount)
	assert.Zero(t, optionEntitlementCount)
	assert.Equal(t, int64(1), purchaseLedgerCount)
	assert.Equal(t, int64(2), totalCreditLedgerCount)
	assert.Equal(t, int64(1), topUpCount)
	assert.Equal(t, int64(1), orderCount)

	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPreConsumeRecord{}))
	consumed, err := model.PreConsumeUserSubscriptionByUnits("external-credit-lifecycle-consume", buyerID, "gpt-4o", 0, 0, purchasedCredit)
	require.NoError(t, err)
	require.Equal(t, existingCreditSubscriptionID, consumed.UserSubscriptionId)
	require.NoError(t, model.DB.First(&balance, existingCreditSubscriptionID).Error)
	assert.Equal(t, int64(1300), balance.TokenLimit)
	assert.Equal(t, int64(1100), balance.TokenUsed)

	refundIdentityLoads := 0
	SetStripeFinancialChargeIdentityForTest(t, func(chargeID string) (stripeFinancialChargeIdentity, error) {
		refundIdentityLoads++
		require.Equal(t, "ch_external_credit_lifecycle", chargeID)
		return stripeFinancialChargeIdentity{
			TradeNo: order.TradeNo,
			Identity: model.SubscriptionOrderProviderIdentity{
				TransactionID:  "pi_external_credit_lifecycle",
				InvoiceID:      "in_external_credit_lifecycle",
				SubscriptionID: "sub_external_credit_lifecycle",
			},
		}, nil
	})
	refundPayload := stripeWebhookPayloadForSubscriptionTest(t, "evt_external_credit_lifecycle_refund", stripe.EventTypeRefundCreated, map[string]any{
		"id":       "re_external_credit_lifecycle",
		"charge":   "ch_external_credit_lifecycle",
		"status":   "succeeded",
		"amount":   4000,
		"currency": "cny",
	})
	firstRefund := signedStripeWebhookRecorderForSubscriptionTest(t, refundPayload)
	require.Equal(t, http.StatusOK, firstRefund.Code, firstRefund.Body.String())

	require.NoError(t, model.DB.First(&order, order.Id).Error)
	require.NoError(t, model.DB.First(&balance, existingCreditSubscriptionID).Error)
	assert.Equal(t, common.TopUpStatusRefunded, order.Status)
	assert.Equal(t, model.SubscriptionOrderRecoveryRefund, order.RecoveryType)
	assert.Positive(t, order.RecoveryLedgerID)
	assert.Equal(t, existingCreditSubscriptionID, order.FulfilledSubscriptionID)
	assert.Equal(t, int64(1300), balance.TokenLimit)
	assert.Equal(t, int64(2100), balance.TokenUsed)
	firstRecoveryLedgerID := order.RecoveryLedgerID
	firstRecoveryTime := order.RecoveryTime

	var recoveryLedger model.CreditBalanceLedger
	require.NoError(t, model.DB.First(&recoveryLedger, order.RecoveryLedgerID).Error)
	assert.Equal(t, model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, recoveryLedger.SourceType)
	assert.Equal(t, order.Id, recoveryLedger.SourceId)
	assert.Equal(t, order.EntitlementSnapshot, recoveryLedger.SourceSnapshot)
	assert.Equal(t, model.CreditBalanceLedgerTypeRefund, recoveryLedger.Type)
	assert.Equal(t, existingCreditSubscriptionID, recoveryLedger.UserSubscriptionId)
	assert.Equal(t, -purchasedCredit, recoveryLedger.GrossCredit)
	assert.Equal(t, int64(0), recoveryLedger.SettlementDebtBefore)
	assert.Equal(t, int64(200), recoveryLedger.AvailableCreditBefore)
	assert.Equal(t, int64(200), recoveryLedger.BalanceBefore)
	assert.Equal(t, int64(-800), recoveryLedger.BalanceAfter)
	assert.Equal(t, int64(800), recoveryLedger.DebtFormed)
	assert.Equal(t, int64(0), recoveryLedger.AvailableCreditAfter)
	assert.Equal(t, int64(800), recoveryLedger.SettlementDebtAfter)
	assert.Equal(t, model.PaymentProviderStripe, recoveryLedger.PaymentProvider)
	assert.Equal(t, "Stripe provider refund", recoveryLedger.Reason)

	refundReplayPayload := stripeWebhookPayloadForSubscriptionTest(t, "evt_external_credit_lifecycle_refund_updated", stripe.EventTypeRefundUpdated, map[string]any{
		"id":       "re_external_credit_lifecycle",
		"charge":   "ch_external_credit_lifecycle",
		"status":   "succeeded",
		"amount":   4000,
		"currency": "cny",
	})
	refundReplay := signedStripeWebhookRecorderForSubscriptionTest(t, refundReplayPayload)
	require.Equal(t, http.StatusOK, refundReplay.Code, refundReplay.Body.String())
	assert.Equal(t, 2, refundIdentityLoads, "both signed refund deliveries must resolve through the provider charge identity seam")
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	require.NoError(t, model.DB.First(&balance, existingCreditSubscriptionID).Error)
	assert.Equal(t, common.TopUpStatusRefunded, order.Status)
	assert.Equal(t, firstRecoveryLedgerID, order.RecoveryLedgerID)
	assert.Equal(t, firstRecoveryTime, order.RecoveryTime)
	assert.Equal(t, int64(1300), balance.TokenLimit)
	assert.Equal(t, int64(2100), balance.TokenUsed)
	var recoveryLedgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Count(&recoveryLedgerCount).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", buyerID).Count(&totalCreditLedgerCount).Error)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", buyerID, model.SubscriptionEntitlementCreditBalance).Count(&creditSubscriptionCount).Error)
	assert.Equal(t, int64(1), recoveryLedgerCount)
	assert.Equal(t, int64(3), totalCreditLedgerCount)
	assert.Equal(t, int64(1), creditSubscriptionCount)

	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&currentRewardCount).Error)
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_type = ? AND source_id = ?", model.InvitationCommissionSourceSubscriptionOrder, order.Id).Count(&currentCommissionCount).Error)
	assert.Zero(t, currentRewardCount)
	assert.Zero(t, currentCommissionCount)
	assertExternalCreditLifecycleUnrelatedInvitationState(t, unrelatedTimedSubscriptionID, unrelatedTimedOrderID, unrelatedInvitationEventID, unrelatedCommissionRecordID, inviterID)

	afterRefund := subscriptionSelfSummaryData(t, performGetSubscriptionSelfSummaryRequest(t, buyerID))
	refundCreditState, ok := afterRefund["credit_balance"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(existingCreditSubscriptionID), refundCreditState["user_subscription_id"])
	assert.Equal(t, float64(creditPlanID), refundCreditState["plan_id"])
	assert.Equal(t, float64(0), refundCreditState["available_credit"])
	assert.Equal(t, float64(800), refundCreditState["settlement_debt"])
	assert.Equal(t, float64(-800), refundCreditState["balance_after"])
	assert.Equal(t, model.CreditBalanceStatusDebt, refundCreditState["status"])
	requireExternalCreditLifecycleSubscriptionID(t, afterRefund["subscriptions"], existingCreditSubscriptionID)
	requireExternalCreditLifecycleSubscriptionID(t, afterRefund["all_subscriptions"], existingCreditSubscriptionID)
	requireExternalCreditLifecycleSubscriptionID(t, afterRefund["all_subscriptions"], unrelatedTimedSubscriptionID)

	ledgerHistory, ok := afterRefund["credit_balance_ledger"].([]interface{})
	require.True(t, ok)
	require.Len(t, ledgerHistory, 3)
	latestLedger, ok := ledgerHistory[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, latestLedger["source_type"])
	assert.Equal(t, model.CreditBalanceLedgerTypeRefund, latestLedger["type"])
	assert.Equal(t, float64(-1000), latestLedger["gross_credit"])
	assert.Equal(t, float64(200), latestLedger["balance_before"])
	assert.Equal(t, float64(-800), latestLedger["balance_after"])
	assert.Equal(t, float64(800), latestLedger["settlement_debt_after"])
	assert.Equal(t, model.PaymentProviderStripe, latestLedger["payment_provider"])
	assert.Equal(t, model.SubscriptionPurchaseModeCreditBalance, latestLedger["purchase_mode"])
}

func assertExternalCreditLifecycleUnrelatedInvitationState(t *testing.T, subscriptionID int, orderID int, eventID int, recordID int, inviterID int) {
	t.Helper()
	var subscription model.UserSubscription
	require.NoError(t, model.DB.First(&subscription, subscriptionID).Error)
	assert.Equal(t, orderID-2, subscription.PlanId)
	assert.Equal(t, model.SubscriptionEntitlementTimed, subscription.EntitlementType)
	assert.Equal(t, "active", subscription.Status)
	assert.Equal(t, int64(1000), subscription.TokenLimit)
	assert.Equal(t, int64(250), subscription.TokenUsed)

	var order model.SubscriptionOrder
	require.NoError(t, model.DB.First(&order, orderID).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, subscriptionID, order.FulfilledSubscriptionID)
	assert.Zero(t, order.RecoveryLedgerID)
	assert.Empty(t, order.RecoveryType)

	var event model.InvitationRewardEvent
	require.NoError(t, model.DB.First(&event, eventID).Error)
	assert.Equal(t, model.InvitationRewardEventStatusActive, event.Status)
	assert.Equal(t, orderID, event.SourceOrderId)
	assert.Equal(t, subscriptionID, event.SourceSubscriptionId)

	var record model.InvitationCommissionRecord
	require.NoError(t, model.DB.First(&record, recordID).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
	assert.Equal(t, int64(400), record.CommissionCents)
	assert.Empty(t, record.ReversalStatus)
	assert.Zero(t, record.RecoveredCents)
	assert.Zero(t, record.UnrecoveredCents)
	assert.Zero(t, record.ReversedAt)

	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", inviterID).First(&account).Error)
	assert.Equal(t, int64(400), account.AvailableCents)
	assert.Zero(t, account.PendingCents)
	assert.Zero(t, account.WithdrawnCents)
	assert.Zero(t, account.TransferredCents)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ?", inviterID).Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
}

func requireExternalCreditLifecycleSubscriptionID(t *testing.T, value interface{}, subscriptionID int) {
	t.Helper()
	entries, ok := value.([]interface{})
	require.True(t, ok, "subscription collection must be an array")
	for _, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]interface{})
		if !ok {
			continue
		}
		rawSubscription, ok := entry["subscription"].(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := rawSubscription["id"].(float64); ok && int(id) == subscriptionID {
			return
		}
	}
	t.Fatalf("subscription %d not found in response", subscriptionID)
}
