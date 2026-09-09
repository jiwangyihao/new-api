package perfmetrics

import (
	"strconv"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/stretchr/testify/require"
)

func TestRecordConcurrentFirstBucketKeepsAllSamples(t *testing.T) {
	originalSetting := perf_metrics_setting.GetSetting()
	originalRedisEnabled, originalRDB := common.RedisEnabled, common.RDB
	modelName := t.Name()
	clearModel := func() {
		hotBuckets.Range(func(key, _ any) bool {
			if key.(bucketKey).model == modelName {
				hotBuckets.Delete(key)
			}
			return true
		})
	}
	clearModel()
	t.Cleanup(func() {
		clearModel()
		common.RedisEnabled, common.RDB = originalRedisEnabled, originalRDB
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"perf_metrics_setting.enabled":     strconv.FormatBool(originalSetting.Enabled),
			"perf_metrics_setting.bucket_time": originalSetting.BucketTime,
		}))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"perf_metrics_setting.enabled":     "true",
		"perf_metrics_setting.bucket_time": "hour",
	}))
	common.RedisEnabled, common.RDB = false, nil

	const workers = 64
	// The first wave races to create the bucket; the second also exercises hits.
	for wave := 1; wave <= 2; wave++ {
		start := make(chan struct{})
		var ready, done sync.WaitGroup
		ready.Add(workers)
		done.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer done.Done()
				ready.Done()
				<-start
				Record(Sample{Model: modelName, LatencyMs: 10, Success: true, HasTtft: true, TtftMs: 4, OutputTokens: 8, GenerationMs: 6})
			}()
		}
		ready.Wait()
		close(start)
		done.Wait()

		var total counters
		hotBuckets.Range(func(key, value any) bool {
			if key.(bucketKey).model != modelName {
				return true
			}
			bucket := value.(*atomicBucket).snapshot()
			total.requestCount += bucket.requestCount
			total.successCount += bucket.successCount
			total.totalLatencyMs += bucket.totalLatencyMs
			total.ttftSumMs += bucket.ttftSumMs
			total.ttftCount += bucket.ttftCount
			total.outputTokens += bucket.outputTokens
			total.generationMs += bucket.generationMs
			return true
		})
		n := int64(workers * wave)
		require.Equal(t, counters{
			requestCount: n, successCount: n, totalLatencyMs: n * 10,
			ttftSumMs: n * 4, ttftCount: n, outputTokens: n * 8, generationMs: n * 6,
		}, total)
	}
}
