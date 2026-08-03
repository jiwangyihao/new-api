package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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

func TestCreditValuationRequestFinalizesSameTargetIdempotently(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-valuation-final-200"
	_, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.NoError(t, SettleCreditRequestTarget(requestID, 200, true))

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

	require.NoError(t, SettleCreditRequestTarget(requestID, 200, true))
	var replayedRecord SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&replayedRecord).Error)
	require.Equal(t, firstRecord.FinalizedAt, replayedRecord.FinalizedAt)
	require.Equal(t, firstRecord.SettlementVersion, replayedRecord.SettlementVersion)
	var replayedState CreditValuationState
	require.NoError(t, db.First(&replayedState, firstRecord.ValuationSubscriptionId).Error)
	require.Equal(t, firstState.StateVersion, replayedState.StateVersion)
}
