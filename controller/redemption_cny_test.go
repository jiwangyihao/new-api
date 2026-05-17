package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRedemptionCNYTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Redemption{}, &model.Log{}))

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})
}

func TestRedemptionCNYAmountToQuotaUsesOneToOneWalletBalance(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	quota, err := redemptionCNYAmountToQuota(40)

	require.NoError(t, err)
	assert.Equal(t, int(common.QuotaPerUnit*40), quota)
}

func TestAddRedemptionStoresCNYAmountAsWalletQuota(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	redemption := model.Redemption{Name: "forty-cny", Quota: 40, Count: 1}
	created, err := buildRedemptionsForCreate(1, redemption, func() string { return "fixed-redemption-key" })

	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, int(common.QuotaPerUnit*40), created[0].Quota)
}

func TestUpdateRedemptionStoresCNYAmountAsWalletQuota(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	existing := &model.Redemption{Name: "old", Quota: 1, Count: 1}
	update := model.Redemption{Name: "new", Quota: 40, ExpiredTime: 0}
	err := applyRedemptionUpdate(existing, update)

	require.NoError(t, err)
	assert.Equal(t, int(common.QuotaPerUnit*40), existing.Quota)
}
