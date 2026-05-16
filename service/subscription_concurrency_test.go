package service

import (
	"context"
	"errors"
	"testing"
	"time"

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
	oldQueueCapacity := common.SubscriptionConcurrencyQueueCapacity
	oldRedis := subscriptionConcurrencyRedis
	oldMemory := subscriptionConcurrencyMemory
	common.RedisEnabled = false
	common.SubscriptionConcurrencyRequireRedis = false
	common.SubscriptionConcurrencyFailOpen = false
	common.SubscriptionConcurrencyTTLSeconds = 600
	common.SubscriptionConcurrencyQueueCapacity = 10
	subscriptionConcurrencyRedis = nil
	subscriptionConcurrencyMemory = newMemorySubscriptionConcurrencyLimiter()
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.SubscriptionConcurrencyRequireRedis = oldRequireRedis
		common.SubscriptionConcurrencyFailOpen = oldFailOpen
		common.SubscriptionConcurrencyTTLSeconds = oldTTL
		common.SubscriptionConcurrencyQueueCapacity = oldQueueCapacity
		subscriptionConcurrencyRedis = oldRedis
		subscriptionConcurrencyMemory = oldMemory
	})
}

func TestMemoryConcurrencyLimiter_QueuesUntilRelease(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	limiter := newMemorySubscriptionConcurrencyLimiter()
	ctx := context.Background()
	lease, err := limiter.Acquire(ctx, 7501, "req-1", 1, 1)
	require.NoError(t, err)

	queuedLease := make(chan ConcurrencyLease, 1)
	queuedErr := make(chan error, 1)
	go func() {
		lease, err := limiter.Acquire(ctx, 7501, "req-2", 1, 1)
		if err != nil {
			queuedErr <- err
			return
		}
		queuedLease <- lease
	}()

	select {
	case lease := <-queuedLease:
		require.NoError(t, lease.Release(ctx))
		t.Fatal("queued request acquired before the active lease was released")
	case err := <-queuedErr:
		require.NoError(t, err)
	case <-time.After(25 * time.Millisecond):
	}

	require.NoError(t, lease.Release(ctx))
	select {
	case lease := <-queuedLease:
		require.NoError(t, lease.Release(ctx))
	case err := <-queuedErr:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("queued request was not acquired after the active lease was released")
	}
}

func TestMemoryConcurrencyLimiter_ReturnsErrorOnlyWhenQueueFull(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	limiter := newMemorySubscriptionConcurrencyLimiter()
	ctx := context.Background()
	lease, err := limiter.Acquire(ctx, 7501, "req-1", 1, 1)
	require.NoError(t, err)

	queuedLease := make(chan ConcurrencyLease, 1)
	queuedErr := make(chan error, 1)
	go func() {
		lease, err := limiter.Acquire(ctx, 7501, "req-2", 1, 1)
		if err != nil {
			queuedErr <- err
			return
		}
		queuedLease <- lease
	}()

	select {
	case lease := <-queuedLease:
		require.NoError(t, lease.Release(ctx))
		t.Fatal("queued request acquired before the active lease was released")
	case err := <-queuedErr:
		require.NoError(t, err)
	case <-time.After(25 * time.Millisecond):
	}

	_, err = limiter.Acquire(ctx, 7501, "req-3", 1, 1)
	require.ErrorIs(t, err, ErrSubscriptionConcurrencyExceeded)

	require.NoError(t, lease.Release(ctx))
	select {
	case lease := <-queuedLease:
		require.NoError(t, lease.Release(ctx))
	case err := <-queuedErr:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("queued request was not acquired after the active lease was released")
	}
}

func TestMemoryConcurrencyLimiter_RejectsWhenQueueCapacityZero(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	limiter := newMemorySubscriptionConcurrencyLimiter()
	ctx := context.Background()
	lease, err := limiter.Acquire(ctx, 7501, "req-1", 1, 0)
	require.NoError(t, err)
	defer lease.Release(ctx)

	_, err = limiter.Acquire(ctx, 7501, "req-2", 1, 0)
	require.ErrorIs(t, err, ErrSubscriptionConcurrencyExceeded)
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
