package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
}

func TestKyrenPaymentConstants(t *testing.T) {
	assert.Equal(t, "kyren", PaymentProviderKyren)
	assert.Equal(t, "kyren", PaymentMethodKyren)
}

func TestKyrenTerminalStatusUpdateOnlyClaimsPendingOrder(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&SubscriptionOrder{TradeNo: "kyren-claim-pending", Status: common.TopUpStatusPending, PaymentProvider: PaymentProviderKyren}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{TradeNo: "kyren-claim-success", Status: common.TopUpStatusSuccess, PaymentProvider: PaymentProviderKyren}).Error)

	claimed, err := ClaimPendingKyrenSubscriptionOrder("kyren-claim-pending")
	require.NoError(t, err)
	assert.True(t, claimed)

	claimed, err = ClaimPendingKyrenSubscriptionOrder("kyren-claim-success")
	require.NoError(t, err)
	assert.False(t, claimed)
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
