package perfmetrics

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerfMetricsAggregateAcrossLegacyGroupsAndHideGroupsResponse(t *testing.T) {
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"perf_metrics_setting.enabled":     "true",
		"perf_metrics_setting.bucket_time": "hour",
	}))
	hotBuckets = syncMapForPerfGroupRemovalTest()

	Record(Sample{Model: "gpt-perf", LatencyMs: 100, Success: true, OutputTokens: 10, GenerationMs: 1000})
	Record(Sample{Model: "gpt-perf", LatencyMs: 300, Success: false, OutputTokens: 30, GenerationMs: 1000})

	result := buildQueryResult("gpt-perf", snapshotHotBucketsForPerfGroupRemovalTest())
	assert.Equal(t, "gpt-perf", result.ModelName)
	assert.Len(t, result.Series, 1)
	assert.Equal(t, int64(200), result.Series[0].AvgLatencyMs)
	assert.Equal(t, float64(50), result.Series[0].SuccessRate)

	payload, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "groups")
	assert.NotContains(t, string(payload), "vip")
	assert.NotContains(t, string(payload), "default")
}

func syncMapForPerfGroupRemovalTest() sync.Map {
	return sync.Map{}
}

func snapshotHotBucketsForPerfGroupRemovalTest() map[bucketKey]counters {
	merged := map[bucketKey]counters{}
	hotBuckets.Range(func(key, value any) bool {
		mergeCounters(merged, key.(bucketKey), value.(*atomicBucket).snapshot())
		return true
	})
	return merged
}
