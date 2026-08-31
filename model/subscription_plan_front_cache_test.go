package model

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/cachex"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
	"github.com/stretchr/testify/require"
)

type countingPlanCodec struct {
	decodes atomic.Int64
}

func (c *countingPlanCodec) Encode(value SubscriptionPlan) (string, error) {
	return cachex.JSONCodec[SubscriptionPlan]{}.Encode(value)
}

func (c *countingPlanCodec) Decode(value string) (SubscriptionPlan, error) {
	c.decodes.Add(1)
	return cachex.JSONCodec[SubscriptionPlan]{}.Decode(value)
}

func newSubscriptionPlanCacheTestHarness(t *testing.T, localTTL time.Duration) (*cachex.HybridCache[SubscriptionPlan], *cachex.LocalFrontCache[SubscriptionPlan], *miniredis.Miniredis, *redis.Client, *countingPlanCodec) {
	t.Helper()
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	codec := &countingPlanCodec{}
	shared := cachex.NewHybridCache[SubscriptionPlan](cachex.HybridCacheConfig[SubscriptionPlan]{
		Namespace:  cachex.Namespace("plan-test"),
		Redis:      client,
		RedisCodec: codec,
		RedisEnabled: func() bool {
			return true
		},
		Memory: func() *hot.HotCache[string, SubscriptionPlan] {
			return hot.NewHotCache[string, SubscriptionPlan](hot.LRU, 8).Build()
		},
	})
	front := cachex.NewLocalFrontCache[SubscriptionPlan](localTTL, 8)
	t.Cleanup(func() {
		_ = client.Close()
		server.Close()
	})
	return shared, front, server, client, codec
}

func getPlanFromTwoLevelCache(shared *cachex.HybridCache[SubscriptionPlan], front *cachex.LocalFrontCache[SubscriptionPlan], key string, ttl time.Duration) (SubscriptionPlan, bool, error) {
	if value, found := front.Get(key); found {
		return value, true, nil
	}
	value, found, err := shared.Get(key)
	if err == nil && found {
		front.SetWithTTL(key, value, ttl)
	}
	return value, found, err
}

func TestSubscriptionPlanFrontCacheAvoidsRepeatedRedisDecode(t *testing.T) {
	shared, front, server, client, codec := newSubscriptionPlanCacheTestHarness(t, time.Second)
	plan := SubscriptionPlan{Id: 11, Title: "Cached plan", Enabled: true}
	require.NoError(t, shared.SetWithTTL("11", plan, time.Minute))

	first, found, err := getPlanFromTwoLevelCache(shared, front, "11", time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, plan, first)
	require.EqualValues(t, 1, codec.decodes.Load())
	commandsAfterWarm := server.CommandCount()
	require.NoError(t, server.Set("plan-test:11", `{"id":11,"title":"Changed remotely","enabled":true}`))

	second, found, err := getPlanFromTwoLevelCache(shared, front, "11", time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Cached plan", second.Title)
	require.EqualValues(t, 1, codec.decodes.Load())
	require.Equal(t, commandsAfterWarm, server.CommandCount(), "front hit must not issue Redis commands")

	front.Delete("11")
	third, found, err := getPlanFromTwoLevelCache(shared, front, "11", time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Changed remotely", third.Title)
	require.EqualValues(t, 2, codec.decodes.Load())

	_, err = client.Get(context.Background(), "plan-test:11").Result()
	require.NoError(t, err)
}

func TestSubscriptionPlanFrontCacheBoundsCrossNodeStaleness(t *testing.T) {
	shared, front, server, _, _ := newSubscriptionPlanCacheTestHarness(t, 20*time.Millisecond)
	plan := SubscriptionPlan{Id: 12, Title: "Old", Enabled: true}
	require.NoError(t, shared.SetWithTTL("12", plan, time.Minute))
	_, found, err := getPlanFromTwoLevelCache(shared, front, "12", time.Minute)
	require.NoError(t, err)
	require.True(t, found)

	require.NoError(t, server.Set("plan-test:12", `{"id":12,"title":"New","enabled":true}`))
	require.Eventually(t, func() bool {
		loaded, found, err := getPlanFromTwoLevelCache(shared, front, "12", time.Minute)
		return err == nil && found && loaded.Title == "New"
	}, time.Second, 5*time.Millisecond)
}

func TestSubscriptionPlanFrontCacheDeletePreventsOldRedisLoadBackfill(t *testing.T) {
	front := cachex.NewLocalFrontCache[SubscriptionPlan](time.Second, 8)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = front.GetOrLoad("14", time.Minute, func() (SubscriptionPlan, bool, error) {
			close(started)
			<-release
			return SubscriptionPlan{Id: 14, Title: "Old"}, true, nil
		})
	}()
	<-started
	front.Delete("14")
	close(release)
	<-done
	_, found := front.Get("14")
	require.False(t, found)
}

func BenchmarkSubscriptionPlanFrontCacheHit(b *testing.B) {
	front := cachex.NewLocalFrontCache[SubscriptionPlan](time.Hour, 8)
	front.SetWithTTL("13", SubscriptionPlan{Id: 13, Title: "Benchmark", Enabled: true}, time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, found := front.Get("13"); !found {
			b.Fatal("front cache miss")
		}
	}
}
