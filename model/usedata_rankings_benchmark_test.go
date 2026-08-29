package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkRankingFreeUserLogCandidates(b *testing.B) {
	setupUsageAnalyticsModelTestDBs(b)
	const (
		userID = 31001
		start  = int64(1900000000)
		count  = 1000
	)
	metered := 12
	logs := make([]Log, 0, count)
	for i := 0; i < count; i++ {
		subscriptionID := 32000 + i
		consumed := int64(i + 1)
		logs = append(logs, Log{
			Id:                         33000 + i,
			UserId:                     userID,
			CreatedAt:                  start + int64(i),
			Type:                       LogTypeConsume,
			MeteredTokens:              &metered,
			SubscriptionID:             &subscriptionID,
			SubscriptionTokensConsumed: &consumed,
			Other:                      fmt.Sprintf(`{"index":%d}`, i),
		})
	}
	require.NoError(b, LOG_DB.CreateInBatches(logs, 100).Error)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rows, err := GetRankingFreeUserLogCandidatesTx(LOG_DB, []int{userID}, start, start+count+1)
		if err != nil {
			b.Fatal(err)
		}
		if len(rows) != count {
			b.Fatalf("rows=%d", len(rows))
		}
	}
}
