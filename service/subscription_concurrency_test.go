package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
	oldRDB := common.RDB
	oldMemory := subscriptionConcurrencyMemory
	common.RedisEnabled = false
	common.SubscriptionConcurrencyRequireRedis = false
	common.SubscriptionConcurrencyFailOpen = false
	common.SubscriptionConcurrencyTTLSeconds = 600
	common.SubscriptionConcurrencyQueueCapacity = 10
	subscriptionConcurrencyRedis = nil
	common.RDB = nil
	subscriptionConcurrencyMemory = newMemorySubscriptionConcurrencyLimiter()
	resetSubscriptionConcurrencyStatsForTest()
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.SubscriptionConcurrencyRequireRedis = oldRequireRedis
		common.SubscriptionConcurrencyFailOpen = oldFailOpen
		common.SubscriptionConcurrencyTTLSeconds = oldTTL
		common.SubscriptionConcurrencyQueueCapacity = oldQueueCapacity
		subscriptionConcurrencyRedis = oldRedis
		common.RDB = oldRDB
		subscriptionConcurrencyMemory = oldMemory
		resetSubscriptionConcurrencyStatsForTest()
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

func TestSubscriptionConcurrencyFailClosedLogsRedisErrorClass(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	common.RedisEnabled = true
	common.SubscriptionConcurrencyFailOpen = false
	subscriptionConcurrencyRedis = brokenRedisEvaler{}

	oldWriter := gin.DefaultWriter
	var buf bytes.Buffer
	common.LogWriterMu.Lock()
	gin.DefaultWriter = &buf
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	_, err := AcquireUserConcurrency(context.Background(), 7506, "req", 1)
	require.ErrorIs(t, err, ErrSubscriptionConcurrencyUnavailable)
	logLine := buf.String()
	if !strings.Contains(logLine, "subscription concurrency redis error; fail-closed") || !strings.Contains(logLine, "class=redis down") {
		t.Fatalf("missing redis diagnostic log: %q", logLine)
	}
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

func TestMemorySubscriptionConcurrencySnapshotReportsActiveAndQueued(t *testing.T) {
	limiter := newMemorySubscriptionConcurrencyLimiter()
	ctx := context.Background()
	lease, err := limiter.Acquire(ctx, 1001, "req-active", 1, 1)
	require.NoError(t, err)

	queuedCtx, cancelQueued := context.WithCancel(ctx)
	queuedLease := make(chan ConcurrencyLease, 1)
	queuedErr := make(chan error, 1)
	go func() {
		lease, err := limiter.Acquire(queuedCtx, 1001, "req-queued", 1, 1)
		if err != nil {
			queuedErr <- err
			return
		}
		queuedLease <- lease
	}()

	require.Eventually(t, func() bool {
		rows := limiter.Snapshot(time.Now())
		return len(rows) == 1 && rows[0].UserID == 1001 && rows[0].Active == 1 && rows[0].Queued == 1
	}, time.Second, 10*time.Millisecond)

	rows := limiter.Snapshot(time.Now())
	require.Len(t, rows, 1)
	assert.Equal(t, 1001, rows[0].UserID)
	assert.EqualValues(t, 1, rows[0].Active)
	assert.EqualValues(t, 1, rows[0].Queued)
	assert.GreaterOrEqual(t, rows[0].OldestQueuedSeconds, int64(0))

	cancelQueued()
	select {
	case lease := <-queuedLease:
		require.NoError(t, lease.Release(ctx))
	case <-queuedErr:
	case <-time.After(time.Second):
		t.Fatal("queued request did not exit after cancellation")
	}
	require.NoError(t, lease.Release(ctx))
}

func TestSubscriptionConcurrencyCountersTrackQueueRejection(t *testing.T) {
	resetSubscriptionConcurrencyStatsForTest()
	oldRedisEnabled := common.RedisEnabled
	oldRequireRedis := common.SubscriptionConcurrencyRequireRedis
	oldMemory := subscriptionConcurrencyMemory
	common.RedisEnabled = false
	common.SubscriptionConcurrencyRequireRedis = false
	subscriptionConcurrencyMemory = newMemorySubscriptionConcurrencyLimiter()
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.SubscriptionConcurrencyRequireRedis = oldRequireRedis
		subscriptionConcurrencyMemory = oldMemory
		resetSubscriptionConcurrencyStatsForTest()
	})
	ctx := context.Background()
	lease, err := AcquireUserConcurrencyWithQueueCapacity(ctx, 1002, "req-active", 1, 1)
	require.NoError(t, err)

	queuedCtx, cancelQueued := context.WithCancel(ctx)
	queuedDone := make(chan error, 1)
	go func() {
		queuedLease, err := AcquireUserConcurrencyWithQueueCapacity(queuedCtx, 1002, "req-queued", 1, 1)
		if queuedLease != nil {
			_ = queuedLease.Release(ctx)
		}
		queuedDone <- err
	}()

	require.Eventually(t, func() bool {
		snapshot := subscriptionConcurrencyMemory.Snapshot(time.Now())
		return len(snapshot) == 1 && snapshot[0].Queued == 1
	}, time.Second, 10*time.Millisecond)

	_, err = AcquireUserConcurrencyWithQueueCapacity(ctx, 1002, "req-rejected", 1, 1)
	assert.ErrorIs(t, err, ErrSubscriptionConcurrencyExceeded)

	counters := SubscriptionConcurrencyCountersSnapshot()
	assert.EqualValues(t, 1, counters.AcquiredTotal)
	assert.EqualValues(t, 1, counters.QueuedTotal)
	assert.EqualValues(t, 1, counters.QueueFullRejectionsTotal)

	cancelQueued()
	select {
	case <-queuedDone:
	case <-time.After(time.Second):
		t.Fatal("queued request did not exit after cancellation")
	}
	require.NoError(t, lease.Release(ctx))
}

func TestSubscriptionConcurrencyUserIndexKey(t *testing.T) {
	assert.Equal(t, "subscription:concurrency:users", subscriptionConcurrencyUserIndexKey())
	assert.Equal(t, "subscription:concurrency:user:42", subscriptionConcurrencyKey(42))
	assert.Equal(t, "subscription:concurrency:user:42:queue", subscriptionConcurrencyQueueKey(42))
}

func TestRedisSubscriptionConcurrencySnapshotSummarizesAllUsersBeforeLimit(t *testing.T) {
	now := time.Unix(1780000000, 0)
	fake := newFakeSubscriptionConcurrencyRedisSnapshot(now, map[int]SubscriptionConcurrencyUserRuntime{
		10: {UserID: 10, Active: 3, Queued: 0, OldestQueuedSeconds: 0},
		20: {UserID: 20, Active: 0, Queued: 4, OldestQueuedSeconds: 12},
		30: {UserID: 30, Active: 1, Queued: 0, OldestQueuedSeconds: 0},
	})

	rows, err := snapshotRedisSubscriptionConcurrency(context.Background(), fake, SubscriptionConcurrencySnapshotQuery{Now: now, Limit: 1, MinActiveOrQueued: 1})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, 20, rows[0].UserID)
	assert.Equal(t, 10, rows[1].UserID)
	assert.Equal(t, 30, rows[2].UserID)
	assert.EqualValues(t, 4, rows[0].Queued)
	assert.EqualValues(t, 12, rows[0].OldestQueuedSeconds)
}

func TestRedisSubscriptionConcurrencyQueuedCounterCountsOnlyFirstEnqueue(t *testing.T) {
	resetSubscriptionConcurrencyStatsForTest()
	fake := &fakeRedisAcquireEvaler{results: []interface{}{int64(2), int64(3)}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	_, _ = acquireRedisUserConcurrencyWithEvaler(ctx, fake, 2001, "req-queued", 1, 1)
	assert.EqualValues(t, 1, SubscriptionConcurrencyCountersSnapshot().QueuedTotal)
}

type fakeSubscriptionConcurrencyRedisSnapshot struct {
	now  time.Time
	rows map[int]SubscriptionConcurrencyUserRuntime
}

func newFakeSubscriptionConcurrencyRedisSnapshot(now time.Time, rows map[int]SubscriptionConcurrencyUserRuntime) fakeSubscriptionConcurrencyRedisSnapshot {
	return fakeSubscriptionConcurrencyRedisSnapshot{now: now, rows: rows}
}

func (f fakeSubscriptionConcurrencyRedisSnapshot) Eval(ctx context.Context, script string, keys []string, args ...interface{}) redisResult {
	values := make([]interface{}, 0, len(f.rows)*4)
	for userID, row := range f.rows {
		oldestQueuedScore := int64(0)
		if row.Queued > 0 && row.OldestQueuedSeconds > 0 {
			oldestQueuedScore = f.now.Unix() - row.OldestQueuedSeconds
		}
		values = append(values, int64(userID), row.Active, row.Queued, oldestQueuedScore)
	}
	return redisResultFunc(func() (interface{}, error) { return values, nil })
}

type fakeRedisAcquireEvaler struct {
	results []interface{}
	calls   int
}

func (f *fakeRedisAcquireEvaler) Eval(ctx context.Context, script string, keys []string, args ...interface{}) redisResult {
	if script != subscriptionConcurrencyAcquireScript {
		return redisResultFunc(func() (interface{}, error) { return int64(1), nil })
	}
	result := interface{}(int64(0))
	if f.calls < len(f.results) {
		result = f.results[f.calls]
	}
	f.calls++
	return redisResultFunc(func() (interface{}, error) { return result, nil })
}
