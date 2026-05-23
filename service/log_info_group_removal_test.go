package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestGenerateTextOtherInfoOmitsBusinessGroupRatios(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{}
	testRelayInfoStartTimes(relayInfo)

	other := GenerateTextOtherInfo(testBillingInfoContext(t), relayInfo, 2, 9, 3, 4, 5, 6, 7)

	assert.Equal(t, float64(2), other["model_ratio"])
	assert.Equal(t, float64(3), other["completion_ratio"])
	assert.Equal(t, 4, other["cache_tokens"])
	assert.Equal(t, float64(5), other["cache_ratio"])
	assert.Equal(t, float64(6), other["model_price"])
	assert.NotContains(t, other, "group_ratio")
	assert.NotContains(t, other, "user_group_ratio")
	assert.NotContains(t, other, "group_group_ratio")
}
