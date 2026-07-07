package service

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestComposeTieredTextQuotaClampsTieredSurchargeAtMaxInt32(t *testing.T) {
	summary := textQuotaSummary{
		ToolCallSurchargeQuota: decimal.NewFromInt(10),
	}
	nearMaxQuota := math.MaxInt32 - 5
	relayInfo := &relaycommon.RelayInfo{}

	withTieredResult := composeTieredTextQuota(relayInfo, summary, nearMaxQuota, &billingexpr.TieredResult{
		ActualQuotaBeforeRatio: float64(nearMaxQuota),
		ActualQuota:            nearMaxQuota,
	})
	require.Equal(t, math.MaxInt32, withTieredResult)

	fallback := composeTieredTextQuota(relayInfo, summary, nearMaxQuota, nil)
	require.Equal(t, math.MaxInt32, fallback)
}
