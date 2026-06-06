package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestKyrenPaymentSnapshotRoundTrip(t *testing.T) {
	snapshot := KyrenPaymentSnapshot{ProductID: "prod_sub", Amount: "40.00", Currency: "CNY"}

	payload, err := MarshalKyrenPaymentSnapshot(snapshot)
	require.NoError(t, err)

	got, err := UnmarshalKyrenPaymentSnapshot(payload)
	require.NoError(t, err)

	assert.Equal(t, snapshot.ProductID, got.ProductID)
	assert.Equal(t, snapshot.Amount, got.Amount)
	assert.Equal(t, snapshot.Currency, got.Currency)
}

func TestKyrenSubscriptionEntitlementSnapshotRoundTrip(t *testing.T) {
	businessCode := "kyren_monthly"
	plan := SubscriptionPlan{
		Id:                      1001,
		TotalAmount:             100000,
		MonthlyTokenLimit:       2000,
		ConcurrencyLimit:        3,
		QueueCapacity:           9,
		DurationUnit:            SubscriptionDurationMonth,
		DurationValue:           1,
		QuotaResetPeriod:        SubscriptionResetMonthly,
		MaxPurchasePerUser:      2,
		BusinessCode:            &businessCode,
		IsTrial:                 true,
		InviteTrial:             true,
		RewardEligible:          true,
		CustomSeconds:           0,
		QuotaResetCustomSeconds: 0,
	}

	snapshot := NewSubscriptionEntitlementSnapshotFromPlan(&plan)
	payload, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)

	got, err := UnmarshalSubscriptionEntitlementSnapshot(payload)
	require.NoError(t, err)

	assert.Equal(t, plan.Id, got.PlanID)
	assert.Equal(t, plan.TotalAmount, got.TotalAmount)
	assert.Equal(t, plan.MonthlyTokenLimit, got.MonthlyTokenLimit)
	assert.Equal(t, plan.ConcurrencyLimit, got.ConcurrencyLimit)
	assert.Equal(t, plan.QueueCapacity, got.QueueCapacity)
	assert.Equal(t, plan.DurationUnit, got.DurationUnit)
	assert.Equal(t, plan.DurationValue, got.DurationValue)
	assert.Equal(t, plan.CustomSeconds, got.CustomSeconds)
	assert.Equal(t, plan.QuotaResetPeriod, got.QuotaResetPeriod)
	assert.Equal(t, plan.QuotaResetCustomSeconds, got.QuotaResetCustomSeconds)
	assert.Equal(t, plan.MaxPurchasePerUser, got.MaxPurchasePerUser)
	assert.Equal(t, businessCode, got.BusinessCode)
	assert.Equal(t, plan.IsTrial, got.IsTrial)
	assert.Equal(t, plan.InviteTrial, got.InviteTrial)
	assert.Equal(t, plan.RewardEligible, got.RewardEligible)
}

func TestKyrenPaymentConstants(t *testing.T) {
	assert.Equal(t, "kyren", PaymentProviderKyren)
	assert.Equal(t, "kyren", PaymentMethodKyren)
}

func TestKyrenTerminalStatusUpdateOnlyClaimsPendingOrder(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&SubscriptionOrder{TradeNo: "kyren-claim-pending", Status: common.TopUpStatusPending, PaymentProvider: PaymentProviderKyren}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{TradeNo: "kyren-claim-success", Status: common.TopUpStatusSuccess, PaymentProvider: PaymentProviderKyren}).Error)

	claimed, _, err := ClaimPendingKyrenSubscriptionOrder("kyren-claim-pending")
	require.NoError(t, err)
	assert.True(t, claimed)

	claimed, _, err = ClaimPendingKyrenSubscriptionOrder("kyren-claim-success")
	require.NoError(t, err)
	assert.False(t, claimed)
}

func TestRestoreClaimedKyrenSubscriptionOrderDoesNotOverwriteNewLease(t *testing.T) {
	truncateTables(t)
	tradeNo := "kyren-claim-lease-owner"
	recentLease := common.GetTimestamp()
	recoveredLease := recentLease + 10
	require.NoError(t, DB.Create(&SubscriptionOrder{TradeNo: tradeNo, Status: common.TopUpStatusFailed, PaymentProvider: PaymentProviderKyren, CompleteTime: recoveredLease}).Error)

	require.NoError(t, RestoreClaimedKyrenSubscriptionOrder(tradeNo, recentLease))

	order := GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
	assert.Equal(t, recoveredLease, order.CompleteTime)
}

func TestMarkClaimedKyrenSubscriptionOrderSuccessRequiresLeaseOwner(t *testing.T) {
	truncateTables(t)
	tradeNo := "kyren-success-lease-owner"
	oldLease := common.GetTimestamp() - 600
	newLease := common.GetTimestamp()
	require.NoError(t, DB.Create(&SubscriptionOrder{TradeNo: tradeNo, Status: common.TopUpStatusFailed, PaymentProvider: PaymentProviderKyren, CompleteTime: newLease}).Error)

	err := DB.Transaction(func(tx *gorm.DB) error {
		order := &SubscriptionOrder{TradeNo: tradeNo, PaymentProvider: PaymentProviderKyren, Status: common.TopUpStatusFailed, CompleteTime: common.GetTimestamp(), ProviderPayload: "{}"}
		return MarkClaimedKyrenSubscriptionOrderSuccessTx(tx, order, oldLease)
	})

	require.ErrorIs(t, err, ErrKyrenSubscriptionOrderLeaseMismatch)
	order := GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
	assert.Equal(t, newLease, order.CompleteTime)
}

func TestKyrenTerminalStatusUpdateOnlyClaimsPendingTopUp(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&TopUp{TradeNo: "kyren-topup-claim-pending", Status: common.TopUpStatusPending, PaymentProvider: PaymentProviderKyren}).Error)
	require.NoError(t, DB.Create(&TopUp{TradeNo: "kyren-topup-claim-success", Status: common.TopUpStatusSuccess, PaymentProvider: PaymentProviderKyren}).Error)

	claimed, err := ClaimPendingKyrenTopUp("kyren-topup-claim-pending")
	require.NoError(t, err)
	assert.True(t, claimed)

	claimed, err = ClaimPendingKyrenTopUp("kyren-topup-claim-success")
	require.NoError(t, err)
	assert.False(t, claimed)
}
