package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func rankingCandidateSubscriptionUsageMap(other string) (int, int64, bool) {
	if strings.TrimSpace(other) == "" {
		return 0, 0, false
	}
	var values map[string]interface{}
	if err := common.UnmarshalJsonStr(other, &values); err != nil {
		return 0, 0, false
	}
	id, ok := intFromOtherMapValue(values["subscription_id"])
	if !ok {
		return 0, 0, false
	}
	consumed, ok := int64FromOtherMapValue(values["subscription_tokens_consumed"])
	if !ok {
		return 0, 0, false
	}
	return id, consumed, true
}

func TestRankingCandidateSubscriptionUsageMatchesMapSemantics(t *testing.T) {
	cases := []string{
		`{"subscription_id":12,"subscription_tokens_consumed":34}`,
		`{"subscription_id":"12","subscription_tokens_consumed":"34"}`,
		`{"subscription_id":" 12 ","subscription_tokens_consumed":" 34 "}`,
		`{"subscription_id":12}`,
		`{"subscription_id":`,
		`{"subscription_id":1,"subscription_id":2,"subscription_tokens_consumed":3}`,
		`{"subscription_id":null,"subscription_tokens_consumed":3}`,
		``,
	}
	for _, other := range cases {
		wantID, wantConsumed, wantOK := rankingCandidateSubscriptionUsageMap(other)
		gotID, gotConsumed, gotOK := rankingCandidateSubscriptionUsage(model.RankingFreeUserLogCandidate{Other: other})
		require.Equal(t, wantID, gotID, other)
		require.Equal(t, wantConsumed, gotConsumed, other)
		require.Equal(t, wantOK, gotOK, other)
	}
}

func BenchmarkRankingCandidateSubscriptionUsage(b *testing.B) {
	other := `{"subscription_id":123,"subscription_tokens_consumed":456,"billing_source":"subscription","endpoint":"/v1/responses","large":"` + strings.Repeat("x", 1<<20) + `"}`
	b.Run("map", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, _ = rankingCandidateSubscriptionUsageMap(other)
		}
	})
	b.Run("gjson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, _ = rankingCandidateSubscriptionUsage(model.RankingFreeUserLogCandidate{Other: other})
		}
	})
}
