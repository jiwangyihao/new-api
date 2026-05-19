package model

import (
	"net/http/httptest"
	"testing"
	"time"


	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		"billing_source":                 "subscription",
		"subscription_tokens_consumed":   int64(80),
	}, 80)
}

func TestRecordConsumeLogTreatsZeroSubscriptionConsumedAsAuthoritative(t *testing.T) {
	assertRecordConsumeLogTokenAccounting(t, "record-zero-token-user", map[string]interface{}{
		"billing_source":                 "subscription",
		"subscription_tokens_consumed":   int64(0),
	}, 0)
}

func TestRecordConsumeLogFallsBackWhenSubscriptionConsumedMissing(t *testing.T) {
	assertRecordConsumeLogTokenAccounting(t, "record-fallback-token-user", map[string]interface{}{
		"billing_source": "subscription",
	}, 15)
}
