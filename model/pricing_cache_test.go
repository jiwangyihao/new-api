package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestGetModelQuotaTypesRefreshesPricingAfterInvalidation(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}, &Model{}))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
		InvalidatePricingCache()
	})

	priceBefore := ratio_setting.ModelPrice2JSONString()
	ratioBefore := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(priceBefore))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(ratioBefore))
	})

	priority := int64(1)
	channel := &Channel{
		Id:       81001,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-pricing-quota-types",
		Status:   common.ChannelStatusEnabled,
		Name:     "pricing-quota-types",
		Models:   "gpt-quota-type-cache",
		Priority: &priority,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-quota-type-cache":0.01}`))
	InvalidatePricingCache()

	require.Equal(t, []int{1}, GetModelQuotaTypes("gpt-quota-type-cache"))
}
