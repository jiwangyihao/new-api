package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/go-redis/redis/v8"
)

var (
	ErrSubscriptionConcurrencyExceeded    = errors.New("subscription concurrency exceeded")
	ErrSubscriptionConcurrencyUnavailable = errors.New("subscription concurrency unavailable")
)

type redisResult interface {
	Result() (interface{}, error)
}

type redisEvaler interface {
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) redisResult
}

type redisClientEvaler struct{ client *redis.Client }

func (r redisClientEvaler) Eval(ctx context.Context, script string, keys []string, args ...interface{}) redisResult {
	return r.client.Eval(ctx, script, keys, args...)
}

type ConcurrencyLease interface {
	Release(ctx context.Context) error
}

type SubscriptionConcurrencyLease = ConcurrencyLease

type noopConcurrencyLease struct{}

func (noopConcurrencyLease) Release(context.Context) error { return nil }

type memorySubscriptionConcurrencyLimiter struct {
	mu       sync.Mutex
	requests map[int]map[string]struct{}
	waiting  map[int][]memorySubscriptionConcurrencyWaiter
}

type memorySubscriptionConcurrencyWaiter struct {
	requestId string
	ready     chan struct{}
}

func newMemorySubscriptionConcurrencyLimiter() *memorySubscriptionConcurrencyLimiter {
	return &memorySubscriptionConcurrencyLimiter{
		requests: make(map[int]map[string]struct{}),
		waiting:  make(map[int][]memorySubscriptionConcurrencyWaiter),
	}
}

func (m *memorySubscriptionConcurrencyLimiter) Acquire(ctx context.Context, userId int, requestId string, limit int, queueCapacity int) (ConcurrencyLease, error) {
	if limit <= 0 {
		return noopConcurrencyLease{}, nil
	}
	m.mu.Lock()
	active := m.requests[userId]
	if active == nil {
		active = make(map[string]struct{})
		m.requests[userId] = active
	}
	if _, ok := active[requestId]; ok {
		m.mu.Unlock()
		return &memorySubscriptionConcurrencyLease{limiter: m, userId: userId, requestId: requestId}, nil
	}
	if len(active) < limit {
		active[requestId] = struct{}{}
		m.mu.Unlock()
		return &memorySubscriptionConcurrencyLease{limiter: m, userId: userId, requestId: requestId}, nil
	}
	if queueCapacity <= 0 || len(m.waiting[userId]) >= queueCapacity {
		m.mu.Unlock()
		return nil, ErrSubscriptionConcurrencyExceeded
	}
	waiter := memorySubscriptionConcurrencyWaiter{requestId: requestId, ready: make(chan struct{})}
	m.waiting[userId] = append(m.waiting[userId], waiter)
	m.mu.Unlock()

	select {
	case <-waiter.ready:
		return &memorySubscriptionConcurrencyLease{limiter: m, userId: userId, requestId: requestId}, nil
	case <-ctx.Done():
		if m.cancelMemoryWaiter(userId, requestId) {
			return &memorySubscriptionConcurrencyLease{limiter: m, userId: userId, requestId: requestId}, nil
		}
		return nil, ctx.Err()
	}
}

type memorySubscriptionConcurrencyLease struct {
	limiter   *memorySubscriptionConcurrencyLimiter
	userId    int
	requestId string
	released  atomic.Bool
}

func (l *memorySubscriptionConcurrencyLease) Release(_ context.Context) error {
	if l == nil || l.limiter == nil || !l.released.CompareAndSwap(false, true) {
		return nil
	}
	l.limiter.mu.Lock()
	defer l.limiter.mu.Unlock()
	active := l.limiter.requests[l.userId]
	if active == nil {
		return nil
	}
	delete(active, l.requestId)
	l.limiter.promoteMemoryWaiterLocked(l.userId, active)
	return nil
}

func (m *memorySubscriptionConcurrencyLimiter) cancelMemoryWaiter(userId int, requestId string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.requests[userId]
	if active != nil {
		if _, ok := active[requestId]; ok {
			return true
		}
	}
	waiting := m.waiting[userId]
	for i, waiter := range waiting {
		if waiter.requestId != requestId {
			continue
		}
		copy(waiting[i:], waiting[i+1:])
		waiting = waiting[:len(waiting)-1]
		if len(waiting) == 0 {
			delete(m.waiting, userId)
		} else {
			m.waiting[userId] = waiting
		}
		return false
	}
	return false
}

func (m *memorySubscriptionConcurrencyLimiter) promoteMemoryWaiterLocked(userId int, active map[string]struct{}) {
	if len(active) == 0 {
		delete(m.requests, userId)
	}
	waiting := m.waiting[userId]
	if len(waiting) == 0 {
		return
	}
	waiter := waiting[0]
	copy(waiting[0:], waiting[1:])
	waiting = waiting[:len(waiting)-1]
	if len(waiting) == 0 {
		delete(m.waiting, userId)
	} else {
		m.waiting[userId] = waiting
	}
	if active == nil {
		active = make(map[string]struct{})
		m.requests[userId] = active
	}
	active[waiter.requestId] = struct{}{}
	close(waiter.ready)
}

var (
	subscriptionConcurrencyRedis  redisEvaler
	subscriptionConcurrencyMemory = newMemorySubscriptionConcurrencyLimiter()
)

const subscriptionConcurrencyAcquireScript = `
local active_key = KEYS[1]
local queue_key = KEYS[2]
local request_id = ARGV[1]
local limit = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local expired_before = tonumber(ARGV[4])
local now = tonumber(ARGV[5])
local queue_capacity = tonumber(ARGV[6])
if limit <= 0 then
  return 1
end
redis.call('ZREMRANGEBYSCORE', active_key, '-inf', expired_before)
if redis.call('ZSCORE', active_key, request_id) then
  redis.call('EXPIRE', active_key, ttl)
  return 1
end
if redis.call('ZCARD', active_key) < limit then
  redis.call('ZADD', active_key, now, request_id)
  redis.call('ZREM', queue_key, request_id)
  redis.call('EXPIRE', active_key, ttl)
  redis.call('EXPIRE', queue_key, ttl)
  return 1
end
if queue_capacity <= 0 then
  return 0
end
if redis.call('ZSCORE', queue_key, request_id) then
  redis.call('EXPIRE', active_key, ttl)
  redis.call('EXPIRE', queue_key, ttl)
  return 2
end
if redis.call('ZCARD', queue_key) >= queue_capacity then
  return 0
end
redis.call('ZADD', queue_key, now, request_id)
redis.call('EXPIRE', active_key, ttl)
redis.call('EXPIRE', queue_key, ttl)
return 2
`

const subscriptionConcurrencyReleaseScript = `
redis.call('ZREM', KEYS[1], ARGV[1])
return 1
`

func AcquireUserConcurrency(ctx context.Context, userId int, requestId string, limit int) (ConcurrencyLease, error) {
	if limit <= 0 || userId <= 0 || requestId == "" {
		return noopConcurrencyLease{}, nil
	}
	queueCapacity := common.SubscriptionConcurrencyQueueCapacity
	if common.RedisEnabled {
		return acquireRedisUserConcurrency(ctx, userId, requestId, limit, queueCapacity)
	}
	if common.SubscriptionConcurrencyRequireRedis {
		if common.SubscriptionConcurrencyFailOpen {
			common.SysLog("subscription concurrency redis required but disabled; fail-open")
			return noopConcurrencyLease{}, nil
		}
		return nil, ErrSubscriptionConcurrencyUnavailable
	}
	return subscriptionConcurrencyMemory.Acquire(ctx, userId, requestId, limit, queueCapacity)
}

func acquireRedisUserConcurrency(ctx context.Context, userId int, requestId string, limit int, queueCapacity int) (ConcurrencyLease, error) {
	evaler := subscriptionConcurrencyRedis
	if evaler == nil && common.RDB != nil {
		evaler = redisClientEvaler{client: common.RDB}
	}
	if evaler == nil {
		return handleSubscriptionConcurrencyRedisError(errors.New("redis client is nil"))
	}
	ttl := common.SubscriptionConcurrencyTTLSeconds
	if ttl <= 0 {
		ttl = 600
	}
	activeKey := subscriptionConcurrencyKey(userId)
	queueKey := subscriptionConcurrencyQueueKey(userId)
	for {
		now := time.Now().Unix()
		result, err := evaler.Eval(ctx, subscriptionConcurrencyAcquireScript, []string{activeKey, queueKey}, requestId, limit, ttl, now-int64(ttl), now, queueCapacity).Result()
		if err != nil {
			return handleSubscriptionConcurrencyRedisError(err)
		}
		switch redisAcquireState(result) {
		case redisAcquireAllowed:
			return &redisSubscriptionConcurrencyLease{evaler: evaler, key: activeKey, requestId: requestId}, nil
		case redisAcquireQueued:
			if err := waitSubscriptionConcurrencyQueuePoll(ctx); err != nil {
				removeRedisSubscriptionConcurrencyQueueEntry(context.Background(), evaler, queueKey, requestId)
				return nil, err
			}
		default:
			return nil, ErrSubscriptionConcurrencyExceeded
		}
	}
}

func handleSubscriptionConcurrencyRedisError(err error) (ConcurrencyLease, error) {
	if common.SubscriptionConcurrencyFailOpen {
		common.SysLog("subscription concurrency redis error; fail-open: " + err.Error())
		return noopConcurrencyLease{}, nil
	}
	return nil, ErrSubscriptionConcurrencyUnavailable
}

type redisAcquireStatus int

const (
	redisAcquireRejected redisAcquireStatus = iota
	redisAcquireAllowed
	redisAcquireQueued
)

func redisAcquireState(result interface{}) redisAcquireStatus {
	switch v := result.(type) {
	case int64:
		return redisAcquireStatus(v)
	case int:
		return redisAcquireStatus(v)
	case string:
		switch v {
		case "1":
			return redisAcquireAllowed
		case "2":
			return redisAcquireQueued
		default:
			return redisAcquireRejected
		}
	default:
		return redisAcquireRejected
	}
}

func waitSubscriptionConcurrencyQueuePoll(ctx context.Context) error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func removeRedisSubscriptionConcurrencyQueueEntry(ctx context.Context, evaler redisEvaler, queueKey string, requestId string) {
	if evaler == nil || queueKey == "" || requestId == "" {
		return
	}
	_, _ = evaler.Eval(ctx, subscriptionConcurrencyReleaseScript, []string{queueKey}, requestId).Result()
}

func subscriptionConcurrencyKey(userId int) string {
	return fmt.Sprintf("subscription:concurrency:user:%d", userId)
}

func subscriptionConcurrencyQueueKey(userId int) string {
	return fmt.Sprintf("subscription:concurrency:user:%d:queue", userId)
}

type redisSubscriptionConcurrencyLease struct {
	evaler    redisEvaler
	key       string
	requestId string
	released  atomic.Bool
}

func (l *redisSubscriptionConcurrencyLease) Release(ctx context.Context) error {
	if l == nil || l.evaler == nil || !l.released.CompareAndSwap(false, true) {
		return nil
	}
	_, err := l.evaler.Eval(ctx, subscriptionConcurrencyReleaseScript, []string{l.key}, l.requestId).Result()
	return err
}

func SubscriptionConcurrencyAPIError(limit int) *types.NewAPIError {
	return types.NewOpenAIError(
		fmt.Errorf("subscription concurrency exceeded, limit=%d", limit),
		types.ErrorCodeSubscriptionConcurrencyExceeded,
		http.StatusTooManyRequests,
		types.ErrOptionWithSkipRetry(),
		types.ErrOptionWithNoRecordErrorLog(),
	)
}

func AcquireSubscriptionConcurrency(ctx context.Context, relayInfo *relaycommon.RelayInfo) (SubscriptionConcurrencyLease, *types.NewAPIError) {
	if relayInfo == nil || relayInfo.BillingSource != BillingSourceSubscription {
		return noopConcurrencyLease{}, nil
	}
	session, ok := relayInfo.Billing.(*BillingSession)
	if !ok || !session.IsDistributorTokenBilling() || !distributorSubscriptionEligibleForBilling(relayInfo) {
		return noopConcurrencyLease{}, nil
	}
	limit := session.SubscriptionConcurrencyLimit()
	if limit <= 0 {
		return noopConcurrencyLease{}, nil
	}
	lease, err := AcquireUserConcurrency(ctx, relayInfo.UserId, relayInfo.RequestId, limit)
	if err == nil {
		return lease, nil
	}
	if errors.Is(err, ErrSubscriptionConcurrencyExceeded) {
		return nil, SubscriptionConcurrencyAPIError(limit)
	}
	return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeUpdateDataError, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
}
