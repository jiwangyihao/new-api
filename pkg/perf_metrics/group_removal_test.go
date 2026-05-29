package perfmetrics

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestQuerySummaryAllIncludesAvgTtftMs(t *testing.T) {
	resetPerfMetricsTestState(t)

	Record(Sample{Model: "ops-ttft", LatencyMs: 200, TtftMs: 40, HasTtft: true, Success: true})
	Record(Sample{Model: "ops-ttft", LatencyMs: 300, TtftMs: 80, HasTtft: true, Success: true})

	result, err := QuerySummaryAll(24)
	require.NoError(t, err)
	assert.EqualValues(t, 60, findModelSummary(t, result.Models, "ops-ttft").AvgTtftMs)
}

func TestQuerySummaryAllIncludesRedisActiveBucketTtft(t *testing.T) {
	resetPerfMetricsTestState(t)
	fake := newPerfMetricsRedisFake()
	seedPerfMetricsRedisActiveBucket(fake, "ops-redis-ttft", 300, 3)

	result, err := querySummaryAllWithRedisReader(24, fake)
	require.NoError(t, err)
	assert.EqualValues(t, 100, findModelSummary(t, result.Models, "ops-redis-ttft").AvgTtftMs)
}

func TestQuerySummaryAllPrefersLocalActiveBucketOverRedisActiveBucket(t *testing.T) {
	resetPerfMetricsTestState(t)
	fake := newPerfMetricsRedisFake()
	seedPerfMetricsRedisActiveBucket(fake, "ops-active-dedupe", 300, 3)
	Record(Sample{Model: "ops-active-dedupe", LatencyMs: 200, TtftMs: 40, HasTtft: true, Success: true})

	result, err := querySummaryAllWithRedisReader(24, fake)
	require.NoError(t, err)
	summary := findModelSummary(t, result.Models, "ops-active-dedupe")
	assert.EqualValues(t, 1, summary.RequestCount)
	assert.EqualValues(t, 40, summary.AvgTtftMs)
}

func TestQuerySummaryAllMergesHistoricalDBAndRedisActiveBucket(t *testing.T) {
	resetPerfMetricsTestState(t)
	fake := newPerfMetricsRedisFake()
	seedPerfMetricsRedisActiveBucket(fake, "ops-history-plus-redis", 300, 3)
	active := time.Now().Add(-time.Minute).Unix()
	require.NoError(t, model.UpsertPerfMetric(&model.PerfMetric{
		ModelName:      "ops-history-plus-redis",
		BucketTs:       active,
		RequestCount:   2,
		SuccessCount:   2,
		TotalLatencyMs: 400,
		TtftSumMs:      80,
		TtftCount:      2,
	}))
	var rows []model.PerfMetricSummary
	require.NoError(t, model.DB.Model(&model.PerfMetric{}).Select("model_name, SUM(request_count) as request_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(ttft_sum_ms) as ttft_sum_ms, SUM(ttft_count) as ttft_count, SUM(output_tokens) as output_tokens, SUM(generation_ms) as generation_ms").Where("bucket_ts >= ? AND bucket_ts <= ?", time.Now().Add(-24*time.Hour).Unix(), time.Now().Unix()).Group("model_name").Having("SUM(request_count) > 0").Find(&rows).Error)
	require.NotEmpty(t, rows)

	result, err := querySummaryAllWithRedisReader(24, fake)
	require.NoError(t, err)
	summary := findModelSummary(t, result.Models, "ops-history-plus-redis")
	assert.EqualValues(t, 5, summary.RequestCount)
	assert.EqualValues(t, 76, summary.AvgTtftMs)
}
func resetPerfMetricsTestState(t *testing.T) {
	t.Helper()
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"perf_metrics_setting.enabled":     "true",
		"perf_metrics_setting.bucket_time": "hour",
	}))
	hotBuckets = sync.Map{}
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	model.DB = db
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		hotBuckets = sync.Map{}
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
		model.DB = oldDB
		require.NoError(t, sqlDB.Close())
	})
}

func findModelSummary(t *testing.T, models []ModelSummary, modelName string) ModelSummary {
	t.Helper()
	for _, model := range models {
		if model.ModelName == modelName {
			return model
		}
	}
	t.Fatalf("missing model summary for %s: %+v", modelName, models)
	return ModelSummary{}
}

type perfMetricsRedisFake struct {
	values        map[string]map[string]string
	valuesByModel map[string]map[string]string
	activeModels  []string
}

func newPerfMetricsRedisFake() *perfMetricsRedisFake {
	return &perfMetricsRedisFake{values: map[string]map[string]string{}, valuesByModel: map[string]map[string]string{}}
}

func seedPerfMetricsRedisActiveBucket(fake *perfMetricsRedisFake, model string, ttftSum int64, ttftCount int64) {
	active := bucketStart(time.Now().Unix())
	values := map[string]string{
		"req":    "3",
		"ok":     "3",
		"ttft":   strconv.FormatInt(ttftSum, 10),
		"ttft_n": strconv.FormatInt(ttftCount, 10),
	}
	fake.values[redisBucketKey(bucketKey{model: model, bucketTs: active})] = values
	fake.valuesByModel[model] = values
	fake.activeModels = append(fake.activeModels, model)
}

func (f *perfMetricsRedisFake) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	values := f.values[key]
	if values == nil {
		for model, candidate := range f.valuesByModel {
			if strings.HasPrefix(key, "perf:"+model+":") {
				values = candidate
				break
			}
		}
	}
	if values == nil {
		return map[string]string{}, nil
	}
	return copyPerfMetricsRedisHash(values), nil
}

func (f *perfMetricsRedisFake) SMembers(ctx context.Context, key string) ([]string, error) {
	return append([]string(nil), f.activeModels...), nil
}

func (f *perfMetricsRedisFake) ZRange(ctx context.Context, key string, start int64, stop int64) ([]string, error) {
	return append([]string(nil), f.activeModels...), nil
}

func copyPerfMetricsRedisHash(values map[string]string) map[string]string {
	copyValues := make(map[string]string, len(values))
	for name, value := range values {
		copyValues[name] = value
	}
	return copyValues
}
