package service

import (
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateOtherInfoHelpersOmitLegacyBusinessGroupRatios(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{}
	testRelayInfoStartTimes(relayInfo)
	ctx := testBillingInfoContext(t)

	cases := []struct {
		name  string
		other map[string]interface{}
	}{
		{
			name:  "text",
			other: GenerateTextOtherInfo(ctx, relayInfo, 2, 3, 4, 5, 6),
		},
		{
			name:  "wss",
			other: GenerateWssOtherInfo(ctx, relayInfo, &dto.RealtimeUsage{}, 2, 3, 4, 5, 6),
		},
		{
			name:  "audio",
			other: GenerateAudioOtherInfo(ctx, relayInfo, &dto.Usage{}, 2, 3, 4, 5, 6),
		},
		{
			name:  "claude",
			other: GenerateClaudeOtherInfo(ctx, relayInfo, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12),
		},
		{
			name:  "mj",
			other: GenerateMjOtherInfo(relayInfo, types.PriceData{ModelPrice: 6}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for key := range tc.other {
				assert.NotContains(t, key, "group", key)
			}
			assert.NotContains(t, tc.other, "group_ratio")
			assert.NotContains(t, tc.other, "user_group_ratio")
			assert.NotContains(t, tc.other, "group_group_ratio")
		})
	}
}

func TestToolCallQuotaIgnoresLegacyBusinessGroupMultiplier(t *testing.T) {
	usage := ToolCallUsage{ModelName: "gpt-4.1", WebSearchCalls: 2, WebSearchToolName: "web_search_preview"}

	base := ComputeToolCallQuota(usage, 1)
	legacyGrouped := ComputeToolCallQuota(usage, 9)

	require.NotZero(t, base.TotalQuota)
	assert.Equal(t, base.TotalQuota, legacyGrouped.TotalQuota)
	require.Len(t, legacyGrouped.Items, 1)
	assert.Equal(t, base.Items[0].Quota, legacyGrouped.Items[0].Quota)
}

func TestCalculateTextQuotaSummaryIgnoresLegacyQuotaMultiplier(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "group-ignored-model",
		PriceData: types.PriceData{
			ModelRatio:          2,
			CompletionRatio:     3,
			QuotaMultiplierInfo: types.QuotaMultiplierInfo{Ratio: 9, SpecialRatio: 7},
		},
		StartTime: time.Now(),
	}
	usage := &dto.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}

	summary := calculateTextQuotaSummary(testBillingInfoContext(t), relayInfo, usage)

	assert.Equal(t, 1.0, summary.QuotaMultiplier)
	assert.Equal(t, 50, summary.Quota)
}

func TestCalculateAudioQuotaUsesModelRatioOnly(t *testing.T) {
	quota := calculateAudioQuota(QuotaInfo{InputDetails: TokenDetails{TextTokens: 10}, OutputDetails: TokenDetails{TextTokens: 5}, ModelName: "audio-group-ignored", ModelRatio: 2})

	assert.Equal(t, 30, quota)
}

func TestCalcViolationFeeQuotaIgnoresLegacyBusinessGroupMultiplier(t *testing.T) {
	base := calcViolationFeeQuota(0.05, 1)
	legacyGrouped := calcViolationFeeQuota(0.05, 9)

	require.Equal(t, int(math.Round(0.05*common.QuotaPerUnit)), base)
	assert.Equal(t, base, legacyGrouped)
}
