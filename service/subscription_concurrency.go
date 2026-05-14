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
}

func newMemorySubscriptionConcurrencyLimiter() *memorySubscriptionConcurrencyLimiter {
	return &memorySubscriptionConcurrencyLimiter{requests: make(map[int]map[string]struct{})}
}

func (m *memorySubscriptionConcurrencyLimiter) Acquire(_ context.Context, userId int, requestId string, limit int) (ConcurrencyLease, error) {
	if limit <= 0 {
		return noopConcurrencyLease{}, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.requests[userId]
	if active == nil {
		active = make(map[string]struct{})
		m.requests[userId] = active
	}
	if _, ok := active[requestId]; ok {
		return &memorySubscriptionConcurrencyLease{limiter: m, userId: userId, requestId: requestId}, nil
	}
	if len(active) >= limit {
		return nil, ErrSubscriptionConcurrencyExceeded
	}
	active[requestId] = struct{}{}
	return &memorySubscriptionConcurrencyLease{limiter: m, userId: userId, requestId: requestId}, nil
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
	if len(active) == 0 {
		delete(l.limiter.requests, l.userId)
	}
	return nil
}

var (
	subscriptionConcurrencyRedis  redisEvaler
	subscriptionConcurrencyMemory = newMemorySubscriptionConcurrencyLimiter()
)

const subscriptionConcurrencyAcquireScript = `
local key = KEYS[1]
local request_id = ARGV[1]
local limit = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
if limit <= 0 then
  return 1
end
redis.call('ZREMRANGEBYSCORE', key, '-inf', tonumber(ARGV[4]))
if redis.call('ZSCORE', key, request_id) then
  redis.call('EXPIRE', key, ttl)
  return 1
end
if redis.call('ZCARD', key) >= limit then
  return 0
end
redis.call('ZADD', key, tonumber(ARGV[5]), request_id)
redis.call('EXPIRE', key, ttl)
return 1
`

const subscriptionConcurrencyReleaseScript = `
redis.call('ZREM', KEYS[1], ARGV[1])
return 1
`

func AcquireUserConcurrency(ctx context.Context, userId int, requestId string, limit int) (ConcurrencyLease, error) {
	if limit <= 0 || userId <= 0 || requestId == "" {
		return noopConcurrencyLease{}, nil
	}
	if common.RedisEnabled {
		return acquireRedisUserConcurrency(ctx, userId, requestId, limit)
	}
	if common.SubscriptionConcurrencyRequireRedis {
		if common.SubscriptionConcurrencyFailOpen {
			common.SysLog("subscription concurrency redis required but disabled; fail-open")
			return noopConcurrencyLease{}, nil
		}
		return nil, ErrSubscriptionConcurrencyUnavailable
	}
	return subscriptionConcurrencyMemory.Acquire(ctx, userId, requestId, limit)
}

func acquireRedisUserConcurrency(ctx context.Context, userId int, requestId string, limit int) (ConcurrencyLease, error) {
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
	now := time.Now().Unix()
	key := subscriptionConcurrencyKey(userId)
	result, err := evaler.Eval(ctx, subscriptionConcurrencyAcquireScript, []string{key}, requestId, limit, ttl, now-int64(ttl), now).Result()
	if err != nil {
		return handleSubscriptionConcurrencyRedisError(err)
	}
	if !redisResultAllowed(result) {
		return nil, ErrSubscriptionConcurrencyExceeded
	}
	return &redisSubscriptionConcurrencyLease{evaler: evaler, key: key, requestId: requestId}, nil
}

func handleSubscriptionConcurrencyRedisError(err error) (ConcurrencyLease, error) {
	if common.SubscriptionConcurrencyFailOpen {
		common.SysLog("subscription concurrency redis error; fail-open: " + err.Error())
		return noopConcurrencyLease{}, nil
	}
	return nil, ErrSubscriptionConcurrencyUnavailable
}

func redisResultAllowed(result interface{}) bool {
	switch v := result.(type) {
	case int64:
		return v == 1
	case int:
		return v == 1
	case string:
		return v == "1"
	default:
		return false
	}
}

func subscriptionConcurrencyKey(userId int) string {
	return fmt.Sprintf("subscription:concurrency:user:%d", userId)
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
	return types.NewErrorWithStatusCode(
		fmt.Errorf("subscription concurrency exceeded, limit=%d", limit),
		"subscription_concurrency_exceeded",
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
	return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeUpdateDataError, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
}
