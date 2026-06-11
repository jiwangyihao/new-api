package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormlogger "gorm.io/gorm/logger"
)

type logStatSelectCounter struct {
	gormlogger.Interface
	selects atomic.Int64
	delay   time.Duration
}

func (l *logStatSelectCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	normalized := strings.ToLower(strings.Join(strings.Fields(sql), " "))
	selectsLogs := strings.HasPrefix(normalized, "select") && (strings.Contains(normalized, "from `logs`") || strings.Contains(normalized, "from logs") || strings.Contains(normalized, "from \"logs\""))
	if selectsLogs {
		l.selects.Add(1)
		if l.delay > 0 {
			time.Sleep(l.delay)
		}
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

func resetLogStatsCacheTestData(t *testing.T) {
	t.Helper()
	resetLogStatTokenTestData(t)
	ResetLogStatCacheForTest()
	t.Cleanup(ResetLogStatCacheForTest)
}

func createLogStatCacheTestLog(t *testing.T, log Log) {
	t.Helper()
	require.NoError(t, LOG_DB.Create(&log).Error)
}

func assertLogStatTotalTokens(t *testing.T, filter LogFilter, want int) {
	t.Helper()
	stat, err := SumUsedQuotaWithFilter(filter)
	require.NoError(t, err)
	assert.Equal(t, want, stat.TotalTokens)
}

func TestSumUsedQuotaCacheKeyIncludesFullFilter(t *testing.T) {
	resetLogStatsCacheTestData(t)

	now := time.Now().Unix()
	trueValue := true
	falseValue := false
	createLogStatCacheTestLog(t, Log{UserId: 101, Username: "cache-user-101", CreatedAt: now - 50, Type: LogTypeConsume, ModelName: "gpt-alpha", TokenName: "token-alpha", TokenId: 11, ChannelId: 21, RequestId: "req-alpha", UpstreamRequestId: "up-alpha", IsStream: true, Quota: 10, MeteredTokens: intPtrForLogStatTokenTest(10)})
	createLogStatCacheTestLog(t, Log{UserId: 202, Username: "cache-user-202", CreatedAt: now - 40, Type: LogTypeConsume, ModelName: "gpt-beta", TokenName: "token-beta", TokenId: 12, ChannelId: 22, RequestId: "req-beta", UpstreamRequestId: "up-beta", IsStream: false, Quota: 20, MeteredTokens: intPtrForLogStatTokenTest(20)})
	createLogStatCacheTestLog(t, Log{UserId: 101, Username: "cache-user-101", CreatedAt: now - 30, Type: LogTypeError, ModelName: "gpt-alpha", TokenName: "token-alpha", TokenId: 11, ChannelId: 21, RequestId: "req-error", UpstreamRequestId: "up-error", IsStream: false, Quota: 30, MeteredTokens: intPtrForLogStatTokenTest(30)})
	createLogStatCacheTestLog(t, Log{UserId: 101, Username: "cache-user-101", CreatedAt: now - 10, Type: LogTypeConsume, ModelName: "gpt-alpha", TokenName: "token-alpha", TokenId: 11, ChannelId: 21, RequestId: "req-recent", UpstreamRequestId: "up-recent", IsStream: true, Quota: 40, MeteredTokens: intPtrForLogStatTokenTest(40)})
	createLogStatCacheTestLog(t, Log{UserId: 303, Username: "cache-user-303", CreatedAt: now - 20, Type: LogTypeConsume, ModelName: "gpt-gamma", TokenName: "token-gamma", TokenId: 13, ChannelId: 23, RequestId: "req-gamma", UpstreamRequestId: "up-gamma", IsStream: false, Quota: 25, MeteredTokens: intPtrForLogStatTokenTest(25)})

	user101 := 101
	user202 := 202
	token11 := 11
	token12 := 12
	zeroToken := 0
	zeroUser := 0

	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, SelfUserId: &user101}, 50)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, SelfUserId: &user202}, 20)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, UserId: &user101}, 50)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, UserId: &user202}, 20)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, ModelName: "gpt-alpha"}, 50)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, ModelName: "gpt-beta"}, 20)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, TokenName: "token-alpha"}, 50)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, TokenName: "token-beta"}, 20)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, RequestId: "req-alpha"}, 10)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, RequestId: "req-recent"}, 40)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, UpstreamRequestId: "up-alpha"}, 10)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, UpstreamRequestId: "up-recent"}, 40)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, Channel: 21}, 50)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, Channel: 22}, 20)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, TokenId: &token11}, 50)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, TokenId: &token12}, 20)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, IsStream: &trueValue}, 50)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, IsStream: &falseValue}, 45)
	assertLogStatTotalTokens(t, LogFilter{Status: UsageAnalyticsStatusSuccess}, 95)
	assertLogStatTotalTokens(t, LogFilter{Status: UsageAnalyticsStatusError}, 30)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, StartTimestamp: now - 35}, 65)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, EndTimestamp: now - 35}, 30)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume}, 95)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeError}, 30)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, Username: "cache-user-101"}, 50)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, Username: "cache-user-303"}, 25)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, TokenId: &zeroToken}, 0)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, UserId: &zeroUser}, 0)
	assertLogStatTotalTokens(t, LogFilter{LogType: LogTypeConsume, SelfUserId: &zeroUser}, 0)
}

func TestSumUsedQuotaSingleflightCoalescesConcurrentExactFilter(t *testing.T) {
	resetLogStatsCacheTestData(t)

	now := time.Now().Unix()
	createLogStatCacheTestLog(t, Log{UserId: 301, CreatedAt: now - 10, Type: LogTypeConsume, ModelName: "singleflight-a", MeteredTokens: intPtrForLogStatTokenTest(10)})
	createLogStatCacheTestLog(t, Log{UserId: 302, CreatedAt: now - 10, Type: LogTypeConsume, ModelName: "singleflight-b", MeteredTokens: intPtrForLogStatTokenTest(20)})

	counter := &logStatSelectCounter{Interface: gormlogger.Default.LogMode(gormlogger.Silent), delay: 25 * time.Millisecond}
	restoreLogger := LOG_DB.Config.Logger
	LOG_DB.Config.Logger = counter
	t.Cleanup(func() { LOG_DB.Config.Logger = restoreLogger })

	const workers = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]Stat, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			stat, err := SumUsedQuotaWithFilter(LogFilter{LogType: LogTypeConsume, ModelName: "singleflight-a"})
			require.NoError(t, err)
			results[i] = stat
		}(i)
	}
	close(start)
	wg.Wait()

	for _, stat := range results {
		assert.Equal(t, 10, stat.TotalTokens)
	}
	assert.Equal(t, int64(2), counter.selects.Load(), "concurrent exact filter stats should share the quota and rpm/tpm queries")

	counter.selects.Store(0)
	ResetLogStatCacheForTest()
	start = make(chan struct{})
	wg = sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		stat, err := SumUsedQuotaWithFilter(LogFilter{LogType: LogTypeConsume, ModelName: "singleflight-a"})
		require.NoError(t, err)
		assert.Equal(t, 10, stat.TotalTokens)
	}()
	go func() {
		defer wg.Done()
		<-start
		stat, err := SumUsedQuotaWithFilter(LogFilter{LogType: LogTypeConsume, ModelName: "singleflight-b"})
		require.NoError(t, err)
		assert.Equal(t, 20, stat.TotalTokens)
	}()
	close(start)
	wg.Wait()
	assert.Equal(t, int64(4), counter.selects.Load(), "different filters must not share singleflight work")
}

func TestSumUsedQuotaRefreshCoalescesButDoesNotShareNormalInflight(t *testing.T) {
	resetLogStatsCacheTestData(t)
	now := time.Now().Unix()
	filter := LogFilter{LogType: LogTypeConsume, ModelName: "refresh-singleflight"}
	createLogStatCacheTestLog(t, Log{UserId: 401, CreatedAt: now - 10, Type: LogTypeConsume, ModelName: "refresh-singleflight", MeteredTokens: intPtrForLogStatTokenTest(10)})

	counter := &logStatSelectCounter{Interface: gormlogger.Default.LogMode(gormlogger.Silent), delay: 50 * time.Millisecond}
	restoreLogger := LOG_DB.Config.Logger
	LOG_DB.Config.Logger = counter
	t.Cleanup(func() { LOG_DB.Config.Logger = restoreLogger })

	normalStarted := make(chan struct{})
	normalDone := make(chan Stat, 1)
	go func() {
		close(normalStarted)
		stat, err := SumUsedQuotaWithFilterOptions(filter, LogStatOptions{})
		require.NoError(t, err)
		normalDone <- stat
	}()
	<-normalStarted
	time.Sleep(10 * time.Millisecond)
	createLogStatCacheTestLog(t, Log{UserId: 401, CreatedAt: now - 9, Type: LogTypeConsume, ModelName: "refresh-singleflight", MeteredTokens: intPtrForLogStatTokenTest(20)})

	const refreshWorkers = 8
	startRefresh := make(chan struct{})
	var wg sync.WaitGroup
	refreshResults := make([]Stat, refreshWorkers)
	wg.Add(refreshWorkers)
	for i := 0; i < refreshWorkers; i++ {
		go func(i int) {
			defer wg.Done()
			<-startRefresh
			stat, err := SumUsedQuotaWithFilterOptions(filter, LogStatOptions{Refresh: true})
			require.NoError(t, err)
			refreshResults[i] = stat
		}(i)
	}
	close(startRefresh)
	wg.Wait()
	normalStat := <-normalDone

	assert.Equal(t, 10, normalStat.TotalTokens)
	for _, stat := range refreshResults {
		assert.Equal(t, 30, stat.TotalTokens)
	}
	cached, err := SumUsedQuotaWithFilter(filter)
	require.NoError(t, err)
	assert.Equal(t, 30, cached.TotalTokens)
	assert.LessOrEqual(t, counter.selects.Load(), int64(4), "normal and refresh should each run at most one quota and one rpm/tpm query group")
}

func TestSumUsedQuotaCacheExpiresAfterTTL(t *testing.T) {
	resetLogStatsCacheTestData(t)
	assert.Equal(t, 5*time.Second, logStatCacheTTL)
	now := time.Now().Unix()
	filter := LogFilter{LogType: LogTypeConsume, ModelName: "ttl-stat"}
	createLogStatCacheTestLog(t, Log{UserId: 501, CreatedAt: now - 10, Type: LogTypeConsume, ModelName: "ttl-stat", MeteredTokens: intPtrForLogStatTokenTest(10)})
	assertLogStatTotalTokens(t, filter, 10)

	createLogStatCacheTestLog(t, Log{UserId: 501, CreatedAt: now - 9, Type: LogTypeConsume, ModelName: "ttl-stat", MeteredTokens: intPtrForLogStatTokenTest(20)})
	assertLogStatTotalTokens(t, filter, 10)

	key := makeLogStatCacheKey(filter)
	logStatCacheMu.Lock()
	entry, ok := logStatCache[key]
	require.True(t, ok)
	entry.expiresAt = time.Now().Add(-time.Second)
	logStatCache[key] = entry
	logStatCacheMu.Unlock()

	assertLogStatTotalTokens(t, filter, 30)
}

func TestLogStatCachePrunesExpiredEntries(t *testing.T) {
	ResetLogStatCacheForTest()
	now := time.Now()
	logStatCacheMu.Lock()
	defer logStatCacheMu.Unlock()
	for i := 0; i < 3; i++ {
		logStatCache[logStatCacheKey{ModelName: string(rune('a' + i))}] = logStatCacheEntry{expiresAt: now.Add(-time.Second), updatedAt: now.Add(-time.Second)}
	}
	storeLogStatCache(logStatCacheKey{ModelName: "live"}, Stat{TotalTokens: 1}, now)
	assert.Len(t, logStatCache, 1)
}

func TestLogStatCacheEvictsOldestEntryAtCapacity(t *testing.T) {
	ResetLogStatCacheForTest()
	now := time.Now()
	logStatCacheMu.Lock()
	defer logStatCacheMu.Unlock()
	oldestKey := logStatCacheKey{ModelName: "oldest"}
	refreshedKey := logStatCacheKey{ModelName: "refreshed"}
	for i := 0; i < logStatCacheMaxEntries; i++ {
		key := logStatCacheKey{ModelName: "entry", RequestId: strconv.Itoa(i)}
		updatedAt := now.Add(time.Duration(i) * time.Millisecond)
		if i == 0 {
			key = oldestKey
			updatedAt = now.Add(-time.Hour)
		}
		if i == 1 {
			key = refreshedKey
		}
		logStatCache[key] = logStatCacheEntry{expiresAt: now.Add(time.Hour), updatedAt: updatedAt}
	}
	storeLogStatCache(refreshedKey, Stat{TotalTokens: 2}, now.Add(time.Millisecond))
	assert.Len(t, logStatCache, logStatCacheMaxEntries)
	_, refreshedStillPresent := logStatCache[refreshedKey]
	assert.True(t, refreshedStillPresent)
	storeLogStatCache(logStatCacheKey{ModelName: "new"}, Stat{TotalTokens: 3}, now.Add(2*time.Millisecond))
	assert.Len(t, logStatCache, logStatCacheMaxEntries)
	_, oldestStillPresent := logStatCache[oldestKey]
	assert.False(t, oldestStillPresent)
}

func TestGetUserLogsWithFilterDoesNotUseLogStatCache(t *testing.T) {
	resetLogStatsCacheTestData(t)
	now := time.Now().Unix()
	filter := LogFilter{LogType: LogTypeConsume, ModelName: "list-no-cache"}
	createLogStatCacheTestLog(t, Log{UserId: 601, CreatedAt: now - 10, Type: LogTypeConsume, ModelName: "list-no-cache", MeteredTokens: intPtrForLogStatTokenTest(10)})

	logs, total, err := GetUserLogsWithFilter(filter, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)

	createLogStatCacheTestLog(t, Log{UserId: 601, CreatedAt: now - 9, Type: LogTypeConsume, ModelName: "list-no-cache", MeteredTokens: intPtrForLogStatTokenTest(20)})
	logs, total, err = GetUserLogsWithFilter(filter, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, logs, 2)
}

func TestSumUsedQuotaReadsLogUsageHourlyAggregationWithUnappliedFallback(t *testing.T) {
	setupLogAggregationTestDBs(t)
	ResetLogStatCacheForTest()
	base := int64(1778716800)
	filter := LogFilter{LogType: LogTypeConsume, StartTimestamp: base, EndTimestamp: base + 7200, UserId: intPtrForLogStatTokenTest(8101), ModelName: "gpt-aggregate", Channel: 31, TokenId: intPtrForLogStatTokenTest(41)}
	modelHash := fmt.Sprintf("%x", sha256.Sum256([]byte("gpt-aggregate")))
	require.NoError(t, LOG_DB.Create(&LogUsageHourly{BucketStart: base, UserID: 8101, TokenID: 41, ChannelID: 31, Status: UsageAnalyticsStatusSuccess, ModelKeyHash: modelHash, ModelName: "gpt-aggregate", RequestCount: 2, QuotaSum: 100, MeteredTokensSum: 100}).Error)
	require.NoError(t, LOG_DB.Create(&Log{Id: 8102, UserId: 8101, CreatedAt: base + 1800, Type: LogTypeConsume, ModelName: "gpt-aggregate", TokenId: 41, ChannelId: 31, Quota: 5, MeteredTokens: intPtrForLogStatTokenTest(5)}).Error)

	stat, err := SumUsedQuotaWithFilterOptions(filter, LogStatOptions{Refresh: true})
	require.NoError(t, err)
	assert.Equal(t, 105, stat.Quota)
	assert.Equal(t, 105, stat.TotalTokens)
}

func TestStrongConsistencyPathsDoNotReferenceLogStatCache(t *testing.T) {
	for _, path := range []string{
		"model/subscription.go",
		"service/billing_session.go",
		"service/funding_source.go",
		"service/token_limit_session.go",
		"model/token_limit_preconsume.go",
	} {
		t.Run(path, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Clean(filepath.Join("..", path)))
			require.NoError(t, err)
			text := string(content)
			assert.NotContains(t, text, "SumUsedQuotaWithFilterOptions")
			assert.NotContains(t, text, "SumUsedQuotaWithFilter(")
			assert.NotContains(t, text, "logStatCache")
			assert.NotContains(t, text, "ResetLogStatCacheForTest")
		})
	}
}
