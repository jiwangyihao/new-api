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

func TestLogFiltersIgnoreLegacyBusinessGroup(t *testing.T) {
	resetLogStatTokenTestData(t)
	now := time.Now().Unix()
	require.NoError(t, LOG_DB.Create(&Log{UserId: 6101, Username: "legacy-log-group", CreatedAt: now - 10, Type: LogTypeConsume, ModelName: "gpt-a", Group: "vip", Quota: 10, MeteredTokens: intPtrForLogStatTokenTest(10)}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 6102, Username: "legacy-log-group", CreatedAt: now - 9, Type: LogTypeConsume, ModelName: "gpt-b", Group: "default", Quota: 20, MeteredTokens: intPtrForLogStatTokenTest(20)}).Error)

	logs, total, err := GetAllLogsWithFilter(LogFilter{LogType: LogTypeConsume}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, logs, 2)

	stat, err := SumUsedQuotaWithFilter(LogFilter{LogType: LogTypeConsume})
	require.NoError(t, err)
	assert.Equal(t, 30, stat.Quota)
	assert.Equal(t, 30, stat.TotalTokens)
}

func TestRecordConsumeAndErrorLogsPersistEmptyBusinessGroup(t *testing.T) {
	resetLogStatTokenTestData(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	ctx := testRecordConsumeLogContext(t, "legacy-group-writer")
	RecordConsumeLog(ctx, 6201, RecordConsumeLogParams{ModelName: "gpt-a", Quota: 10, Group: "vip", Other: map[string]interface{}{"ok": true}})

	var consume Log
	require.NoError(t, LOG_DB.Where("username = ?", "legacy-group-writer").First(&consume).Error)
	assert.Empty(t, consume.Group)

	recorder := httptest.NewRecorder()
	errCtx, _ := gin.CreateTestContext(recorder)
	errCtx.Set("username", "legacy-error-writer")
	RecordErrorLog(errCtx, 6202, 0, "gpt-a", "token", "failed", 0, 0, false, "vip", nil)

	var errLog Log
	require.NoError(t, LOG_DB.Where("username = ?", "legacy-error-writer").First(&errLog).Error)
	assert.Empty(t, errLog.Group)
}
