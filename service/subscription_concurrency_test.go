package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

type brokenRedisEvaler struct{}

func (brokenRedisEvaler) Eval(ctx context.Context, script string, keys []string, args ...interface{}) redisResult {
	return redisResultFunc(func() (interface{}, error) { return nil, errors.New("redis down") })
}

type redisResultFunc func() (interface{}, error)

func (f redisResultFunc) Result() (interface{}, error) { return f() }

func resetSubscriptionConcurrencyForTest(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	oldRequireRedis := common.SubscriptionConcurrencyRequireRedis
	oldFailOpen := common.SubscriptionConcurrencyFailOpen
	oldTTL := common.SubscriptionConcurrencyTTLSeconds
	oldRedis := subscriptionConcurrencyRedis
	oldMemory := subscriptionConcurrencyMemory
	common.RedisEnabled = false
	common.SubscriptionConcurrencyRequireRedis = false
	common.SubscriptionConcurrencyFailOpen = false
	common.SubscriptionConcurrencyTTLSeconds = 600
	subscriptionConcurrencyRedis = nil
	subscriptionConcurrencyMemory = newMemorySubscriptionConcurrencyLimiter()
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.SubscriptionConcurrencyRequireRedis = oldRequireRedis
		common.SubscriptionConcurrencyFailOpen = oldFailOpen
		common.SubscriptionConcurrencyTTLSeconds = oldTTL
		subscriptionConcurrencyRedis = oldRedis
		subscriptionConcurrencyMemory = oldMemory
	})
}

func TestMemoryConcurrencyLimiter_AcquireRelease(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	limiter := newMemorySubscriptionConcurrencyLimiter()
	ctx := context.Background()
	lease, err := limiter.Acquire(ctx, 7501, "req-1", 1)
	require.NoError(t, err)
	_, err = limiter.Acquire(ctx, 7501, "req-2", 1)
	require.ErrorIs(t, err, ErrSubscriptionConcurrencyExceeded)
	require.NoError(t, lease.Release(ctx))
	require.NoError(t, lease.Release(ctx))
	_, err = limiter.Acquire(ctx, 7501, "req-3", 1)
	require.NoError(t, err)
}

func TestSubscriptionConcurrencyRequiresRedisWhenConfigured(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	common.RedisEnabled = false
	common.SubscriptionConcurrencyRequireRedis = true
	common.SubscriptionConcurrencyFailOpen = false
	_, err := AcquireUserConcurrency(context.Background(), 7502, "req", 1)
	require.ErrorIs(t, err, ErrSubscriptionConcurrencyUnavailable)
}

func TestSubscriptionConcurrencyFailOpenWhenRedisRequired(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	common.RedisEnabled = false
	common.SubscriptionConcurrencyRequireRedis = true
	common.SubscriptionConcurrencyFailOpen = true
	lease, err := AcquireUserConcurrency(context.Background(), 7503, "req", 1)
	require.NoError(t, err)
	require.NoError(t, lease.Release(context.Background()))
}

func TestSubscriptionConcurrencyFailClosedWhenRedisCommandFails(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	common.RedisEnabled = true
	common.SubscriptionConcurrencyFailOpen = false
	subscriptionConcurrencyRedis = brokenRedisEvaler{}
	_, err := AcquireUserConcurrency(context.Background(), 7504, "req", 1)
	require.ErrorIs(t, err, ErrSubscriptionConcurrencyUnavailable)
}

func TestSubscriptionConcurrencyFailOpenWhenRedisCommandFails(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	common.RedisEnabled = true
	common.SubscriptionConcurrencyFailOpen = true
	subscriptionConcurrencyRedis = brokenRedisEvaler{}
	lease, err := AcquireUserConcurrency(context.Background(), 7505, "req", 1)
	require.NoError(t, err)
	require.NoError(t, lease.Release(context.Background()))
}
