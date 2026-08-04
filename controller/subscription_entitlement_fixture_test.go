package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedAuthoritativeTimedPlanFixture(t *testing.T, plan model.SubscriptionPlan, priceMicros int64) model.SubscriptionPlan {
	t.Helper()
	require.Positive(t, priceMicros)
	plan.EntitlementType = model.SubscriptionEntitlementTimed
	plan.PriceAmount = float64(priceMicros) / 1_000_000
	plan.PriceAmountMicros = &priceMicros
	plan.Currency = "CNY"
	plan.Enabled = true
	plan.IsTrial = false
	plan.InviteTrial = false
	if strings.TrimSpace(plan.DurationUnit) == "" {
		plan.DurationUnit = model.SubscriptionDurationMonth
	}
	if plan.DurationUnit != model.SubscriptionDurationCustom && plan.DurationValue <= 0 {
		plan.DurationValue = 1
	}
	if strings.TrimSpace(plan.QuotaResetPeriod) == "" {
		plan.QuotaResetPeriod = model.SubscriptionResetNever
	}
	if plan.MonthlyTokenLimit <= 0 {
		plan.MonthlyTokenLimit = 1_000
	}
	if plan.BusinessCode == nil || strings.TrimSpace(*plan.BusinessCode) == "" {
		businessCode := fmt.Sprintf("controller_timed_fixture_%d", plan.Id)
		plan.BusinessCode = &businessCode
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	require.NotNil(t, plan.PriceAmountMicros)
	assert.Equal(t, priceMicros, *plan.PriceAmountMicros)
	assert.Equal(t, model.SubscriptionEntitlementTimed, plan.EntitlementType)
	assert.Equal(t, "CNY", plan.Currency)
	assert.True(t, plan.Enabled)
	assert.False(t, plan.IsTrial)
	assert.False(t, plan.InviteTrial)
	assert.Positive(t, plan.MonthlyTokenLimit)
	assert.NotEmpty(t, plan.DurationUnit)
	assert.NotEmpty(t, plan.QuotaResetPeriod)
	assert.NotEmpty(t, strings.TrimSpace(*plan.BusinessCode))
	return plan
}

func marshalAuthorizedTimedOrderSnapshotFixture(
	t *testing.T,
	plan *model.SubscriptionPlan,
	paymentProvider string,
	providerProductID string,
	paymentMethod string,
	amountCents int64,
	paymentCurrency string,
) string {
	t.Helper()
	require.NotNil(t, plan)
	require.NotNil(t, plan.PriceAmountMicros)
	require.Positive(t, *plan.PriceAmountMicros)
	snapshot := model.NewSubscriptionEntitlementSnapshot(plan, model.SubscriptionPurchaseModeTimed, 0)
	if strings.TrimSpace(paymentProvider) != "" {
		snapshot.SetPaymentSnapshot(paymentProvider, providerProductID, paymentMethod, amountCents, paymentCurrency)
	}
	payload, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	require.NotEmpty(t, payload)
	assertAuthorizedTimedSnapshotFixture(t, payload, plan)
	return payload
}

func assertAuthorizedTimedSnapshotFixture(t *testing.T, payload string, plan *model.SubscriptionPlan) {
	t.Helper()
	require.NotNil(t, plan)
	persisted, err := model.UnmarshalSubscriptionEntitlementSnapshot(payload)
	require.NoError(t, err)
	assert.Equal(t, plan.Id, persisted.PlanID)
	assert.Equal(t, model.SubscriptionPurchaseModeTimed, persisted.PurchaseMode)
	assert.Equal(t, model.SubscriptionEntitlementTimed, persisted.PlanEntitlementType)
	require.NotNil(t, persisted.ListPriceMicros)
	require.NotNil(t, plan.PriceAmountMicros)
	assert.Equal(t, *plan.PriceAmountMicros, *persisted.ListPriceMicros)
	assert.Equal(t, plan.Currency, persisted.ListPriceCurrency)
	assert.Equal(t, plan.MonthlyTokenLimit, persisted.MonthlyTokenLimit)
	assert.Equal(t, plan.DurationUnit, persisted.DurationUnit)
	assert.Equal(t, plan.DurationValue, persisted.DurationValue)
	assert.Equal(t, plan.CustomSeconds, persisted.CustomSeconds)
	assert.Equal(t, plan.QuotaResetPeriod, persisted.QuotaResetPeriod)
	assert.Equal(t, plan.QuotaResetCustomSeconds, persisted.QuotaResetCustomSeconds)
	require.NotNil(t, plan.BusinessCode)
	assert.Equal(t, strings.TrimSpace(*plan.BusinessCode), persisted.BusinessCode)
}

func assertAuthorizedTimedOrderSnapshotFixture(t *testing.T, order *model.SubscriptionOrder, plan *model.SubscriptionPlan) {
	t.Helper()
	require.NotNil(t, order)
	require.NotEmpty(t, order.EntitlementSnapshot)
	assertAuthorizedTimedSnapshotFixture(t, order.EntitlementSnapshot, plan)
}

func seedAuthorizedTimedSubscriptionOrderFixture(
	t *testing.T,
	plan *model.SubscriptionPlan,
	order model.SubscriptionOrder,
	providerProductID string,
) model.SubscriptionOrder {
	t.Helper()
	paymentProvider := ""
	if order.AmountCents > 0 && strings.TrimSpace(order.Currency) != "" {
		paymentProvider = order.PaymentProvider
	}
	order.EntitlementSnapshot = marshalAuthorizedTimedOrderSnapshotFixture(
		t,
		plan,
		paymentProvider,
		providerProductID,
		order.PaymentMethod,
		order.AmountCents,
		order.Currency,
	)
	require.NoError(t, model.DB.Create(&order).Error)
	require.NotZero(t, order.Id)
	require.NotEmpty(t, order.EntitlementSnapshot)
	return order
}
