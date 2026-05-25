package model

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormlogger "gorm.io/gorm/logger"
)

func resetLogStatTokenTestData(t *testing.T) {
	t.Helper()

	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
	require.NoError(t, DB.AutoMigrate(&QuotaData{}))
	clearQuotaDataCacheForLogStatTokenTest()
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	require.NoError(t, DB.Exec("DELETE FROM quota_data").Error)

	t.Cleanup(func() {
		clearQuotaDataCacheForLogStatTokenTest()
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
		require.NoError(t, DB.Exec("DELETE FROM quota_data").Error)
	})
}

func clearQuotaDataCacheForLogStatTokenTest() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	CacheQuotaData = make(map[string]*QuotaData)
}

func intPtrForLogStatTokenTest(value int) *int {
	return &value
}

func TestSumUsedQuotaUsesMeteredTokensForSubscriptionLogs(t *testing.T) {
	resetLogStatTokenTestData(t)

	now := time.Now()
	start := now.Add(-5 * time.Minute).Unix()
	end := now.Add(time.Minute).Unix()
	legacyLog := &Log{
		UserId:           1002,
		Username:         "legacy-token-stat-user",
		CreatedAt:        now.Add(-10 * time.Second).Unix(),
		Type:             LogTypeConsume,
		ModelName:        "legacy-model",
		Quota:            40,
		PromptTokens:     7,
		CompletionTokens: 3,
	}
	require.Nil(t, legacyLog.MeteredTokens, "legacy fixture must leave metered_tokens NULL so SumUsedQuota falls back to prompt+completion")

	recentSubscriptionLog := &Log{
		UserId:           1001,
		Username:         "subscription-token-stat-user",
		CreatedAt:        now.Add(-10 * time.Second).Unix(),
		Type:             LogTypeConsume,
		ModelName:        "subscription-model",
		Quota:            100,
		PromptTokens:     10,
		CompletionTokens: 5,
		MeteredTokens:    intPtrForLogStatTokenTest(80),
	}
	oldSubscriptionLog := &Log{
		UserId:           1003,
		Username:         "old-subscription-token-stat-user",
		CreatedAt:        now.Add(-120 * time.Second).Unix(),
		Type:             LogTypeConsume,
		ModelName:        "old-subscription-model",
		Quota:            9,
		PromptTokens:     1,
		CompletionTokens: 1,
		MeteredTokens:    intPtrForLogStatTokenTest(50),
	}
	require.NoError(t, LOG_DB.Create(recentSubscriptionLog).Error)
	require.NoError(t, LOG_DB.Create(legacyLog).Error)
	require.NoError(t, LOG_DB.Create(oldSubscriptionLog).Error)

	stat, err := SumUsedQuota(LogTypeConsume, start, end, "", "", "", 0, "")
	require.NoError(t, err)

	assert.Equal(t, 149, stat.Quota)
	assert.Equal(t, 140, stat.TotalTokens, "total_tokens must use metered_tokens when present and prompt+completion only when metered_tokens is NULL")
	assert.Equal(t, 2, stat.Rpm)
	assert.Equal(t, 90, stat.Tpm, "tpm must use the same normalized token contract but only for logs in the most recent 60 seconds")
}

func TestSumUsedQuotaPreservesAuthoritativeZeroMeteredTokens(t *testing.T) {
	resetLogStatTokenTestData(t)

	now := time.Now()
	start := now.Add(-5 * time.Minute).Unix()
	end := now.Add(time.Minute).Unix()
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           2001,
		Username:         "zero-metered-token-stat-user",
		CreatedAt:        now.Add(-10 * time.Second).Unix(),
		Type:             LogTypeConsume,
		ModelName:        "zero-metered-model",
		Quota:            100,
		PromptTokens:     10,
		CompletionTokens: 5,
		MeteredTokens:    intPtrForLogStatTokenTest(0),
	}).Error)

	stat, err := SumUsedQuota(LogTypeConsume, start, end, "", "", "", 0, "")
	require.NoError(t, err)

	assert.Equal(t, 100, stat.Quota)
	assert.Equal(t, 0, stat.TotalTokens, "explicit metered_tokens=0 is authoritative and must not fall back to prompt+completion")
	assert.Equal(t, 0, stat.Tpm, "recent tpm must preserve authoritative metered_tokens=0")
}

func TestLogQuotaDataStoresMeteredTokens(t *testing.T) {
	resetLogStatTokenTestData(t)

	createdAt := time.Now().Unix()
	roundedCreatedAt := createdAt - createdAt%3600
	LogQuotaData(3001, "quota-token-stat-user", "quota-token-stat-model", 123, createdAt, 80)
	SaveQuotaDataCache()

	var quotaData QuotaData
	require.NoError(t, DB.Table("quota_data").Where(
		"user_id = ? AND username = ? AND model_name = ? AND created_at = ?",
		3001,
		"quota-token-stat-user",
		"quota-token-stat-model",
		roundedCreatedAt,
	).First(&quotaData).Error)

	assert.Equal(t, 123, quotaData.Quota)
	assert.Equal(t, 80, quotaData.TokenUsed)
}

func testRecordConsumeLogContext(t *testing.T, username string) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("username", username)
	ctx.Set(common.RequestIdKey, username+"-request")
	ctx.Set(common.UpstreamRequestIdKey, username+"-upstream")
	return ctx
}

func waitForQuotaDataCacheForLogStatTokenTest(t *testing.T, username string, tokenUsed int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		CacheQuotaDataLock.Lock()
		matched := false
		for _, quotaData := range CacheQuotaData {
			if quotaData.Username == username && quotaData.TokenUsed == tokenUsed {
				matched = true
				break
			}
		}
		CacheQuotaDataLock.Unlock()
		if matched {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("quota data cache did not receive token_used=%d for %s", tokenUsed, username)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertRecordConsumeLogTokenAccounting(t *testing.T, username string, other map[string]interface{}, expectedTokens int) {
	t.Helper()
	resetLogStatTokenTestData(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldDataExportEnabled := common.DataExportEnabled
	common.LogConsumeEnabled = true
	common.DataExportEnabled = true
	t.Cleanup(func() {
		common.LogConsumeEnabled = oldLogConsumeEnabled
		common.DataExportEnabled = oldDataExportEnabled
	})

	RecordConsumeLog(testRecordConsumeLogContext(t, username), 4001, RecordConsumeLogParams{
		ChannelId:        1,
		PromptTokens:     10,
		CompletionTokens: 5,
		ModelName:        "record-token-model",
		TokenName:        "record-token-key",
		Quota:            100,
		Content:          "record token accounting",
		TokenId:          2,
		UseTimeSeconds:   3,
		Group:            "default",
		Other:            other,
	})
	waitForQuotaDataCacheForLogStatTokenTest(t, username, expectedTokens)
	SaveQuotaDataCache()

	var log Log
	require.NoError(t, LOG_DB.Where("username = ?", username).First(&log).Error)
	require.NotNil(t, log.MeteredTokens)
	assert.Equal(t, expectedTokens, *log.MeteredTokens)

	var quotaData QuotaData
	require.NoError(t, DB.Table("quota_data").Where("username = ?", username).First(&quotaData).Error)
	assert.Equal(t, 100, quotaData.Quota)
	assert.Equal(t, expectedTokens, quotaData.TokenUsed)
}

func TestRecordConsumeLogUsesSubscriptionConsumedForMeteredTokensAndQuotaData(t *testing.T) {
	assertRecordConsumeLogTokenAccounting(t, "record-subscription-token-user", map[string]interface{}{
		"billing_source":               "subscription",
		"subscription_tokens_consumed": int64(80),
	}, 80)
}

func TestRecordConsumeLogTreatsZeroSubscriptionConsumedAsAuthoritative(t *testing.T) {
	assertRecordConsumeLogTokenAccounting(t, "record-zero-token-user", map[string]interface{}{
		"billing_source":               "subscription",
		"subscription_tokens_consumed": int64(0),
	}, 0)
}

func TestRecordConsumeLogFallsBackWhenSubscriptionConsumedMissing(t *testing.T) {
	assertRecordConsumeLogTokenAccounting(t, "record-fallback-token-user", map[string]interface{}{
		"billing_source": "subscription",
	}, 15)
}

func TestRecordConsumeLogStdoutSummaryDoesNotSerializeFullParams(t *testing.T) {
	resetLogStatTokenTestData(t)

	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldDataExportEnabled := common.DataExportEnabled
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	t.Cleanup(func() {
		common.LogConsumeEnabled = oldLogConsumeEnabled
		common.DataExportEnabled = oldDataExportEnabled
	})

	var buf bytes.Buffer
	oldWriter := gin.DefaultWriter
	gin.DefaultWriter = &buf
	t.Cleanup(func() { gin.DefaultWriter = oldWriter })

	ctx := testRecordConsumeLogContext(t, "perf-user")
	RecordConsumeLog(ctx, 4001, RecordConsumeLogParams{
		ChannelId:        7,
		PromptTokens:     123,
		CompletionTokens: 45,
		ModelName:        "gpt-5.5",
		TokenName:        "perf-token",
		Quota:            999,
		TokenId:          9,
		UseTimeSeconds:   3,
		IsStream:         true,
		Group:            "default",
		Other: map[string]interface{}{
			"large_payload": strings.Repeat("x", 8192),
		},
	})

	out := buf.String()
	if strings.Contains(out, "params=") {
		t.Fatalf("consume log stdout should not include full params JSON: %s", out)
	}
	if strings.Contains(out, "large_payload") || strings.Contains(out, strings.Repeat("x", 128)) {
		t.Fatalf("consume log stdout should not include large Other payload: %s", out)
	}
	for _, want := range []string{"record consume log", "userId=4001", "model=gpt-5.5", "quota=999", "prompt=123", "completion=45"} {
		if !strings.Contains(out, want) {
			t.Fatalf("consume log stdout missing %q in %s", want, out)
		}
	}
}

type consumeLogInsertCounter struct {
	gormlogger.Interface
	inserts atomic.Int64
}

func (l *consumeLogInsertCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	lower := strings.ToLower(sql)
	if strings.Contains(lower, "insert") && strings.Contains(lower, "logs") {
		l.inserts.Add(1)
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

func TestRecordConsumeLogCoalescesConcurrentInserts(t *testing.T) {
	resetLogStatTokenTestData(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldDataExportEnabled := common.DataExportEnabled
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	t.Cleanup(func() {
		common.LogConsumeEnabled = oldLogConsumeEnabled
		common.DataExportEnabled = oldDataExportEnabled
	})

	counter := &consumeLogInsertCounter{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	restoreLogger := LOG_DB.Config.Logger
	LOG_DB.Config.Logger = counter
	t.Cleanup(func() { LOG_DB.Config.Logger = restoreLogger })

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			ctx := testRecordConsumeLogContext(t, "coalesced-log-user")
			ctx.Set(common.RequestIdKey, "coalesced-log-request-"+strconv.Itoa(i))
			RecordConsumeLog(ctx, 910001, RecordConsumeLogParams{
				ChannelId:        910020,
				PromptTokens:     11,
				CompletionTokens: 17,
				ModelName:        "gpt-5.5",
				TokenName:        "loadtest subscription",
				Quota:            113,
				TokenId:          1,
				UseTimeSeconds:   2,
				IsStream:         true,
				Other: map[string]interface{}{
					"billing_source":               "subscription",
					"subscription_tokens_consumed": int64(28),
				},
			})
		}(i)
	}
	close(start)
	wg.Wait()

	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ?", 910001).Count(&count).Error)
	assert.Equal(t, int64(workers), count)
	require.LessOrEqual(t, counter.inserts.Load(), int64(4), "consume logs should be batch-inserted under concurrent hot writes")
}
