package perfmetrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

type perfMetricsRedisCommandCounter struct {
	mu        sync.Mutex
	pipelines int
}

func (c *perfMetricsRedisCommandCounter) record() {
	c.mu.Lock()
	c.pipelines++
	c.mu.Unlock()
}

func (c *perfMetricsRedisCommandCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pipelines
}

func setupPerfMetricsRedisRecordTest(t *testing.T) (*miniredis.Miniredis, *perfMetricsRedisCommandCounter) {
	t.Helper()
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled, common.RDB = oldEnabled, oldRDB
		server.Close()
	})
	return server, &perfMetricsRedisCommandCounter{}
}

func TestRecordPerfMetricRedisWritesBucketAndActiveModelInOnePipeline(t *testing.T) {
	server, counter := setupPerfMetricsRedisRecordTest(t)
	_ = counter
	old := time.Now().Unix()
	Record(Sample{Model: "redis-perf-test", LatencyMs: 100, Success: true, HasTtft: true, TtftMs: 20, OutputTokens: 30, GenerationMs: 80})
	bucket := bucketStart(old)
	bucketKeyName := redisBucketKey(bucketKey{model: "redis-perf-test", bucketTs: bucket})
	values, err := common.RDB.HGetAll(context.Background(), bucketKeyName).Result()
	require.NoError(t, err)
	require.Equal(t, "1", values["req"])
	require.Equal(t, "1", values["ok"])
	require.Equal(t, "100", values["lat"])
	require.Equal(t, "20", values["ttft"])
	require.Equal(t, "1", values["ttft_n"])
	require.Equal(t, "30", values["out"])
	require.Equal(t, "80", values["gen_ms"])
	activeModels, err := server.Members(perfMetricsActiveModelsKey())
	require.NoError(t, err)
	require.Contains(t, activeModels, "redis-perf-test")
	require.Greater(t, server.TTL(bucketKeyName), time.Duration(0))
	require.Greater(t, server.TTL(perfMetricsActiveModelsKey()), time.Duration(0))
}
