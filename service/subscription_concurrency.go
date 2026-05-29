package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
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

type SubscriptionConcurrencyCounters struct {
	AcquiredTotal              int64
	QueuedTotal                int64
	QueueFullRejectionsTotal   int64
	UnavailableRejectionsTotal int64
	RedisErrorsTotal           int64
}

var (
	subscriptionConcurrencyAcquiredTotal              atomic.Int64
	subscriptionConcurrencyQueuedTotal                atomic.Int64
	subscriptionConcurrencyQueueFullRejectionsTotal   atomic.Int64
	subscriptionConcurrencyUnavailableRejectionsTotal atomic.Int64
	subscriptionConcurrencyRedisErrorsTotal           atomic.Int64
)

func SubscriptionConcurrencyCountersSnapshot() SubscriptionConcurrencyCounters {
	return SubscriptionConcurrencyCounters{
		AcquiredTotal:              subscriptionConcurrencyAcquiredTotal.Load(),
		QueuedTotal:                subscriptionConcurrencyQueuedTotal.Load(),
		QueueFullRejectionsTotal:   subscriptionConcurrencyQueueFullRejectionsTotal.Load(),
		UnavailableRejectionsTotal: subscriptionConcurrencyUnavailableRejectionsTotal.Load(),
		RedisErrorsTotal:           subscriptionConcurrencyRedisErrorsTotal.Load(),
	}
}

func resetSubscriptionConcurrencyStatsForTest() {
	subscriptionConcurrencyAcquiredTotal.Store(0)
	subscriptionConcurrencyQueuedTotal.Store(0)
	subscriptionConcurrencyQueueFullRejectionsTotal.Store(0)
	subscriptionConcurrencyUnavailableRejectionsTotal.Store(0)
	subscriptionConcurrencyRedisErrorsTotal.Store(0)
}

func recordSubscriptionConcurrencyAcquired() {
	subscriptionConcurrencyAcquiredTotal.Add(1)
}

func recordSubscriptionConcurrencyQueued() {
	subscriptionConcurrencyQueuedTotal.Add(1)
}

func recordSubscriptionConcurrencyQueueFullRejection() {
	subscriptionConcurrencyQueueFullRejectionsTotal.Add(1)
}

func recordSubscriptionConcurrencyUnavailableRejection() {
	subscriptionConcurrencyUnavailableRejectionsTotal.Add(1)
}

func recordSubscriptionConcurrencyRedisError() {
	subscriptionConcurrencyRedisErrorsTotal.Add(1)
}

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
	queuedAt  int64
}

type SubscriptionConcurrencyUserRuntime struct {
	UserID              int
	Active              int64
	Queued              int64
	OldestQueuedSeconds int64
}

type SubscriptionConcurrencySnapshotQuery struct {
	Now               time.Time
	Limit             int
	MinActiveOrQueued int64
}

func newMemorySubscriptionConcurrencyLimiter() *memorySubscriptionConcurrencyLimiter {
	return &memorySubscriptionConcurrencyLimiter{
		requests: make(map[int]map[string]struct{}),
		waiting:  make(map[int][]memorySubscriptionConcurrencyWaiter),
	}
}

func (m *memorySubscriptionConcurrencyLimiter) Snapshot(now time.Time) []SubscriptionConcurrencyUserRuntime {
	if m == nil {
		return nil
	}
	nowUnix := now.Unix()
	if now.IsZero() {
		nowUnix = time.Now().Unix()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	byUser := make(map[int]SubscriptionConcurrencyUserRuntime, len(m.requests)+len(m.waiting))
	for userID, active := range m.requests {
		if len(active) == 0 {
			continue
		}
		row := byUser[userID]
		row.UserID = userID
		row.Active = int64(len(active))
		byUser[userID] = row
	}
	for userID, waiting := range m.waiting {
		if len(waiting) == 0 {
			continue
		}
		row := byUser[userID]
		row.UserID = userID
		row.Queued = int64(len(waiting))
		oldestQueuedSeconds := nowUnix - waiting[0].queuedAt
		if oldestQueuedSeconds < 0 {
			oldestQueuedSeconds = 0
		}
		row.OldestQueuedSeconds = oldestQueuedSeconds
		byUser[userID] = row
	}

	rows := make([]SubscriptionConcurrencyUserRuntime, 0, len(byUser))
	for _, row := range byUser {
		if row.Active == 0 && row.Queued == 0 {
			continue
		}
		rows = append(rows, row)
	}
	sortSubscriptionConcurrencyRuntimeRows(rows)
	return rows
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
		recordSubscriptionConcurrencyAcquired()
		return &memorySubscriptionConcurrencyLease{limiter: m, userId: userId, requestId: requestId}, nil
	}
	if queueCapacity <= 0 || len(m.waiting[userId]) >= queueCapacity {
		m.mu.Unlock()
		recordSubscriptionConcurrencyQueueFullRejection()
		return nil, ErrSubscriptionConcurrencyExceeded
	}
	waiter := memorySubscriptionConcurrencyWaiter{requestId: requestId, ready: make(chan struct{}), queuedAt: time.Now().Unix()}
	m.waiting[userId] = append(m.waiting[userId], waiter)
	m.mu.Unlock()
	recordSubscriptionConcurrencyQueued()

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
	recordSubscriptionConcurrencyAcquired()
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
  return 3
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
	return acquireUserConcurrency(ctx, userId, requestId, limit, common.SubscriptionConcurrencyQueueCapacity)
}

func AcquireUserConcurrencyWithQueueCapacity(ctx context.Context, userId int, requestId string, limit int, queueCapacity int) (ConcurrencyLease, error) {
	return acquireUserConcurrency(ctx, userId, requestId, limit, queueCapacity)
}

func acquireUserConcurrency(ctx context.Context, userId int, requestId string, limit int, queueCapacity int) (ConcurrencyLease, error) {
	if limit <= 0 || userId <= 0 || requestId == "" {
		return noopConcurrencyLease{}, nil
	}
	queueCapacity = effectiveSubscriptionConcurrencyQueueCapacity(queueCapacity)
	if common.RedisEnabled {
		return acquireRedisUserConcurrency(ctx, userId, requestId, limit, queueCapacity)
	}
	if common.SubscriptionConcurrencyRequireRedis {
		if common.SubscriptionConcurrencyFailOpen {
			common.SysLog("subscription concurrency redis required but disabled; fail-open")
			return noopConcurrencyLease{}, nil
		}
		recordSubscriptionConcurrencyUnavailableRejection()
		return nil, ErrSubscriptionConcurrencyUnavailable
	}
	return subscriptionConcurrencyMemory.Acquire(ctx, userId, requestId, limit, queueCapacity)
}

func effectiveSubscriptionConcurrencyQueueCapacity(queueCapacity int) int {
	if queueCapacity > 0 {
		return queueCapacity
	}
	return common.SubscriptionConcurrencyQueueCapacity
}

func acquireRedisUserConcurrency(ctx context.Context, userId int, requestId string, limit int, queueCapacity int) (ConcurrencyLease, error) {
	evaler := subscriptionConcurrencyRedis
	if evaler == nil && common.RDB != nil {
		evaler = redisClientEvaler{client: common.RDB}
	}
	return acquireRedisUserConcurrencyWithEvaler(ctx, evaler, userId, requestId, limit, queueCapacity)
}

func acquireRedisUserConcurrencyWithEvaler(ctx context.Context, evaler redisEvaler, userId int, requestId string, limit int, queueCapacity int) (ConcurrencyLease, error) {
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
			recordSubscriptionConcurrencyAcquired()
			recordRedisSubscriptionConcurrencyUser(ctx, evaler, userId, ttl, now)
			return &redisSubscriptionConcurrencyLease{evaler: evaler, key: activeKey, requestId: requestId}, nil
		case redisAcquireQueuedNew:
			recordSubscriptionConcurrencyQueued()
			recordRedisSubscriptionConcurrencyUser(ctx, evaler, userId, ttl, now)
			if err := waitSubscriptionConcurrencyQueuePoll(ctx); err != nil {
				removeRedisSubscriptionConcurrencyQueueEntry(context.Background(), evaler, queueKey, requestId)
				return nil, err
			}
		case redisAcquireQueuedExisting:
			if err := waitSubscriptionConcurrencyQueuePoll(ctx); err != nil {
				removeRedisSubscriptionConcurrencyQueueEntry(context.Background(), evaler, queueKey, requestId)
				return nil, err
			}
		default:
			recordSubscriptionConcurrencyQueueFullRejection()
			return nil, ErrSubscriptionConcurrencyExceeded
		}
	}
}

func handleSubscriptionConcurrencyRedisError(err error) (ConcurrencyLease, error) {
	recordSubscriptionConcurrencyRedisError()
	if common.SubscriptionConcurrencyFailOpen {
		common.SysLog("subscription concurrency redis error; fail-open: " + err.Error())
		return noopConcurrencyLease{}, nil
	}
	recordSubscriptionConcurrencyUnavailableRejection()
	common.SysLog("subscription concurrency redis error; fail-closed: class=" + subscriptionConcurrencyRedisErrorClass(err))
	return nil, ErrSubscriptionConcurrencyUnavailable
}
func subscriptionConcurrencyRedisErrorClass(err error) string {
	if err == nil {
		return "unknown"
	}
	return common.MaskSensitiveInfo(err.Error())
}

type redisAcquireStatus int

const (
	redisAcquireRejected redisAcquireStatus = iota
	redisAcquireAllowed
	redisAcquireQueuedNew
	redisAcquireQueuedExisting
)

func redisAcquireState(result interface{}) redisAcquireStatus {
	value, ok := subscriptionConcurrencyResultInt64(result)
	if !ok {
		return redisAcquireRejected
	}
	switch value {
	case 1:
		return redisAcquireAllowed
	case 2:
		return redisAcquireQueuedNew
	case 3:
		return redisAcquireQueuedExisting
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

func subscriptionConcurrencyUserIndexKey() string {
	return "subscription:concurrency:users"
}

const subscriptionConcurrencyRecordUserScript = `
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[1])
redis.call('EXPIRE', KEYS[1], ARGV[3])
return 1
`

const subscriptionConcurrencySnapshotScript = `
local user_index_key = KEYS[1]
local ttl = tonumber(ARGV[1])
local expired_before = tonumber(ARGV[2])
redis.call('ZREMRANGEBYSCORE', user_index_key, '-inf', expired_before)
local users = redis.call('ZRANGE', user_index_key, 0, -1)
local result = {}
for _, user_id in ipairs(users) do
  local active_key = 'subscription:concurrency:user:' .. user_id
  local queue_key = active_key .. ':queue'
  redis.call('ZREMRANGEBYSCORE', active_key, '-inf', expired_before)
  redis.call('ZREMRANGEBYSCORE', queue_key, '-inf', expired_before)
  local active = redis.call('ZCARD', active_key)
  local queued = redis.call('ZCARD', queue_key)
  if active == 0 and queued == 0 then
    redis.call('ZREM', user_index_key, user_id)
  else
    local oldest_queued_score = 0
    if queued > 0 then
      local oldest = redis.call('ZRANGE', queue_key, 0, 0, 'WITHSCORES')
      if oldest[2] then
        oldest_queued_score = tonumber(oldest[2]) or 0
      end
    end
    table.insert(result, user_id)
    table.insert(result, active)
    table.insert(result, queued)
    table.insert(result, oldest_queued_score)
  end
end
redis.call('EXPIRE', user_index_key, ttl)
return result
`

func recordRedisSubscriptionConcurrencyUser(ctx context.Context, evaler redisEvaler, userId int, ttl int, now int64) {
	if evaler == nil || userId <= 0 {
		return
	}
	if ttl <= 0 {
		ttl = 600
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	_, err := evaler.Eval(ctx, subscriptionConcurrencyRecordUserScript, []string{subscriptionConcurrencyUserIndexKey()}, userId, now, ttl).Result()
	if err != nil {
		common.SysLog("subscription concurrency user index update failed: " + subscriptionConcurrencyRedisErrorClass(err))
	}
}

func snapshotRedisSubscriptionConcurrency(ctx context.Context, evaler redisEvaler, query SubscriptionConcurrencySnapshotQuery) ([]SubscriptionConcurrencyUserRuntime, error) {
	if evaler == nil {
		return nil, errors.New("redis evaler is nil")
	}
	now := query.Now
	if now.IsZero() {
		now = time.Now()
	}
	ttl := common.SubscriptionConcurrencyTTLSeconds
	if ttl <= 0 {
		ttl = 600
	}
	nowUnix := now.Unix()
	result, err := evaler.Eval(ctx, subscriptionConcurrencySnapshotScript, []string{subscriptionConcurrencyUserIndexKey()}, ttl, nowUnix-int64(ttl), nowUnix).Result()
	if err != nil {
		return nil, err
	}
	values, err := subscriptionConcurrencyResultSlice(result)
	if err != nil {
		return nil, err
	}
	if len(values)%4 != 0 {
		return nil, fmt.Errorf("unexpected subscription concurrency snapshot field count: %d", len(values))
	}
	rows := make([]SubscriptionConcurrencyUserRuntime, 0, len(values)/4)
	for i := 0; i < len(values); i += 4 {
		userID64, ok := subscriptionConcurrencyResultInt64(values[i])
		if !ok {
			return nil, fmt.Errorf("unexpected subscription concurrency user id type %T", values[i])
		}
		active, ok := subscriptionConcurrencyResultInt64(values[i+1])
		if !ok {
			return nil, fmt.Errorf("unexpected subscription concurrency active type %T", values[i+1])
		}
		queued, ok := subscriptionConcurrencyResultInt64(values[i+2])
		if !ok {
			return nil, fmt.Errorf("unexpected subscription concurrency queued type %T", values[i+2])
		}
		oldestQueuedScore, ok := subscriptionConcurrencyResultInt64(values[i+3])
		if !ok {
			return nil, fmt.Errorf("unexpected subscription concurrency oldest queued type %T", values[i+3])
		}
		if active == 0 && queued == 0 {
			continue
		}
		oldestQueuedSeconds := int64(0)
		if queued > 0 && oldestQueuedScore > 0 {
			oldestQueuedSeconds = nowUnix - oldestQueuedScore
			if oldestQueuedSeconds < 0 {
				oldestQueuedSeconds = 0
			}
		}
		rows = append(rows, SubscriptionConcurrencyUserRuntime{
			UserID:              int(userID64),
			Active:              active,
			Queued:              queued,
			OldestQueuedSeconds: oldestQueuedSeconds,
		})
	}
	sortSubscriptionConcurrencyRuntimeRows(rows)
	return rows, nil
}

func subscriptionConcurrencyResultSlice(result interface{}) ([]interface{}, error) {
	switch values := result.(type) {
	case nil:
		return nil, nil
	case []interface{}:
		return values, nil
	case []string:
		out := make([]interface{}, len(values))
		for i, value := range values {
			out[i] = value
		}
		return out, nil
	case []int64:
		out := make([]interface{}, len(values))
		for i, value := range values {
			out[i] = value
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected subscription concurrency snapshot result type %T", result)
	}
}

func subscriptionConcurrencyResultInt64(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(v), true
	case uint:
		if uint64(v) > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(v), true
	case uint32:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		return parsed, err == nil
	case []byte:
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func sortSubscriptionConcurrencyRuntimeRows(rows []SubscriptionConcurrencyUserRuntime) {
	sort.Slice(rows, func(i, j int) bool {
		leftTotal := rows[i].Active + rows[i].Queued
		rightTotal := rows[j].Active + rows[j].Queued
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		if rows[i].Queued != rows[j].Queued {
			return rows[i].Queued > rows[j].Queued
		}
		if rows[i].Active != rows[j].Active {
			return rows[i].Active > rows[j].Active
		}
		return rows[i].UserID < rows[j].UserID
	})
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
	lease, err := AcquireUserConcurrencyWithQueueCapacity(ctx, relayInfo.UserId, relayInfo.RequestId, limit, session.SubscriptionQueueCapacity())
	if err == nil {
		return lease, nil
	}
	if errors.Is(err, ErrSubscriptionConcurrencyExceeded) {
		return nil, SubscriptionConcurrencyAPIError(limit)
	}
	return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeUpdateDataError, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
}
