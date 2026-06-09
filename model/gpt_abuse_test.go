package model

import (
	"strings"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGPTAbuseTestDB(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldLogSQLType := common.LogSqlType
	oldRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite
	common.RedisEnabled = false
	initCol()

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"_gpt_abuse?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&GPTAbuseSignalLog{}, &GPTAbuseUserSuspension{}, &GPTAbuseWarningReset{}, &GPTAbuseRepeatBlockLog{}))

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.LogSqlType = oldLogSQLType
		common.RedisEnabled = oldRedisEnabled
		initCol()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestRecordGPTAbuseSignalLogDedupesByKey(t *testing.T) {
	setupGPTAbuseTestDB(t)
	log := &GPTAbuseSignalLog{
		CreatedAt:         1700000000,
		UserId:            1001,
		TokenId:           2001,
		ChannelId:         3001,
		RequestId:         "req-local",
		UpstreamRequestId: "req-upstream",
		Source:            "http_error",
		Kind:              "cyber_policy",
		Severity:          "high",
		CountEligible:     true,
		DedupeKey:         "1001:2001:3001:req-local:req-upstream:http_error:cyber_policy",
	}

	inserted, err := RecordGPTAbuseSignalLog(log)
	require.NoError(t, err)
	assert.True(t, inserted)

	duplicate := *log
	duplicate.Id = 0
	inserted, err = RecordGPTAbuseSignalLog(&duplicate)
	require.NoError(t, err)
	assert.False(t, inserted)

	var count int64
	require.NoError(t, DB.Model(&GPTAbuseSignalLog{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestCountGPTAbuseSignalsForUserCountsEligibleWindow(t *testing.T) {
	setupGPTAbuseTestDB(t)
	records := []GPTAbuseSignalLog{
		{CreatedAt: 100, UserId: 1, Kind: "cyber_policy", CountEligible: true, DedupeKey: "a"},
		{CreatedAt: 150, UserId: 1, Kind: "content_policy_violation", CountEligible: true, DedupeKey: "b"},
		{CreatedAt: 160, UserId: 1, Kind: "rate_limit", CountEligible: false, DedupeKey: "c"},
		{CreatedAt: 170, UserId: 2, Kind: "cyber_policy", CountEligible: true, DedupeKey: "d"},
		{CreatedAt: 99, UserId: 1, Kind: "cyber_policy", CountEligible: true, DedupeKey: "e"},
	}
	require.NoError(t, DB.Create(&records).Error)

	count, err := CountGPTAbuseSignalsForUser(1, 100, 200)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestGPTAbuseEffectiveCountUsesLatestResetCutoffLogID(t *testing.T) {
	setupGPTAbuseTestDB(t)
	const userID = 11
	const windowStart = int64(1000)
	const windowEnd = int64(2000)
	const resetAt = int64(1500)
	records := []GPTAbuseSignalLog{
		{CreatedAt: 1100, UserId: userID, Kind: GPTAbuseKindCyberPolicy, CountEligible: true, DedupeKey: "effective-count-1"},
		{CreatedAt: 1200, UserId: userID, Kind: GPTAbuseKindCyberPolicy, CountEligible: true, DedupeKey: "effective-count-2"},
		{CreatedAt: resetAt, UserId: userID, Kind: GPTAbuseKindCyberPolicy, CountEligible: true, DedupeKey: "effective-count-3"},
	}
	require.NoError(t, DB.Create(&records).Error)
	require.NoError(t, CreateGPTAbuseWarningReset(&GPTAbuseWarningReset{UserId: userID, WindowStart: windowStart, WindowEnd: windowEnd, ResetAt: resetAt, PreviousRawCount: 2, PreviousCount: 2, CutoffSignalLogID: records[1].Id, Reason: "test reset"}))

	rawCount, err := CountGPTAbuseSignalsForUserRaw(userID, windowStart, windowEnd)
	require.NoError(t, err)
	effectiveCount, reset, err := CountEffectiveGPTAbuseSignalsForUser(userID, windowStart, windowEnd)
	require.NoError(t, err)

	require.NotNil(t, reset)
	assert.Equal(t, records[1].Id, reset.CutoffSignalLogID)
	assert.Equal(t, 3, rawCount)
	assert.Equal(t, 1, effectiveCount)
}

func TestLatestGPTAbuseWarningResetOrdersByResetAtThenID(t *testing.T) {
	setupGPTAbuseTestDB(t)
	const userID = 12
	const windowStart = int64(2000)
	resetAt := int64(2500)
	older := GPTAbuseWarningReset{UserId: userID, WindowStart: windowStart, WindowEnd: 3000, ResetAt: resetAt, CutoffSignalLogID: 7, Reason: "older"}
	newer := GPTAbuseWarningReset{UserId: userID, WindowStart: windowStart, WindowEnd: 3000, ResetAt: resetAt, CutoffSignalLogID: 9, Reason: "newer"}
	require.NoError(t, CreateGPTAbuseWarningReset(&older))
	require.NoError(t, CreateGPTAbuseWarningReset(&newer))

	latest, err := LatestGPTAbuseWarningReset(userID, windowStart)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Greater(t, latest.Id, older.Id)
	assert.Equal(t, newer.Id, latest.Id)
	assert.Equal(t, newer.CutoffSignalLogID, latest.CutoffSignalLogID)
}

func TestGPTAbuseRepeatBlockLogStoresWarningAttributionWithoutBody(t *testing.T) {
	setupGPTAbuseTestDB(t)
	fingerprint := strings.Repeat("a", 64)
	repeatLog := &GPTAbuseRepeatBlockLog{
		CreatedAt:                     3000,
		UserId:                        13,
		Username:                      "abuse-user",
		TokenId:                       14,
		TokenName:                     "safe-token",
		RequestId:                     "req-repeat",
		Endpoint:                      "/v1/chat/completions",
		RelayMode:                     1,
		RequestedModel:                "gpt-4o",
		BodyFingerprint:               fingerprint,
		FirstWarningLogId:             15,
		FirstWarningAt:                2999,
		FirstWarningRequestId:         "req-warning",
		FirstWarningUpstreamRequestId: "req-upstream-warning",
		FirstWarningSource:            GPTAbuseSignalSourceHTTPError,
		FirstWarningKind:              GPTAbuseKindCyberPolicy,
		FirstWarningSeverity:          GPTAbuseSeverityHigh,
		ChannelId:                     16,
		ChannelName:                   "OpenAI Primary",
		ChannelType:                   17,
	}
	require.NoError(t, RecordGPTAbuseRepeatBlockLog(repeatLog))

	var got GPTAbuseRepeatBlockLog
	require.NoError(t, DB.First(&got, repeatLog.Id).Error)
	assert.Equal(t, fingerprint, got.BodyFingerprint)
	assert.Equal(t, repeatLog.FirstWarningLogId, got.FirstWarningLogId)
	assert.Equal(t, repeatLog.FirstWarningRequestId, got.FirstWarningRequestId)
	assert.Equal(t, repeatLog.FirstWarningUpstreamRequestId, got.FirstWarningUpstreamRequestId)
	assert.Equal(t, repeatLog.FirstWarningSource, got.FirstWarningSource)
	logType := reflect.TypeOf(GPTAbuseRepeatBlockLog{})
	field, ok := logType.FieldByName("BodyFingerprint")
	require.True(t, ok)
	assert.Equal(t, "-", field.Tag.Get("json"))
	_, ok = logType.FieldByName("Body")
	assert.False(t, ok)
	_, ok = logType.FieldByName("Prompt")
	assert.False(t, ok)
}

func TestRecordGPTAbuseRepeatBlockLogRejectsInvalidAttribution(t *testing.T) {
	setupGPTAbuseTestDB(t)

	require.NoError(t, RecordGPTAbuseRepeatBlockLog(nil))

	validLog := func() *GPTAbuseRepeatBlockLog {
		return &GPTAbuseRepeatBlockLog{
			CreatedAt:          4000,
			UserId:             13,
			BodyFingerprint:    strings.Repeat("b", 64),
			FirstWarningLogId:  15,
			FirstWarningAt:     3999,
			FirstWarningSource: GPTAbuseSignalSourceHTTPError,
			FirstWarningKind:   GPTAbuseKindCyberPolicy,
		}
	}

	invalidCases := []struct {
		name   string
		mutate func(*GPTAbuseRepeatBlockLog)
	}{
		{
			name: "zero user id",
			mutate: func(log *GPTAbuseRepeatBlockLog) {
				log.UserId = 0
			},
		},
		{
			name: "negative user id",
			mutate: func(log *GPTAbuseRepeatBlockLog) {
				log.UserId = -1
			},
		},
		{
			name: "empty body fingerprint",
			mutate: func(log *GPTAbuseRepeatBlockLog) {
				log.BodyFingerprint = ""
			},
		},
		{
			name: "blank body fingerprint",
			mutate: func(log *GPTAbuseRepeatBlockLog) {
				log.BodyFingerprint = " \t\n"
			},
		},
		{
			name: "zero first warning log id",
			mutate: func(log *GPTAbuseRepeatBlockLog) {
				log.FirstWarningLogId = 0
			},
		},
		{
			name: "negative first warning log id",
			mutate: func(log *GPTAbuseRepeatBlockLog) {
				log.FirstWarningLogId = -1
			},
		},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			repeatLog := validLog()
			tc.mutate(repeatLog)

			require.Error(t, RecordGPTAbuseRepeatBlockLog(repeatLog))

			var count int64
			require.NoError(t, DB.Model(&GPTAbuseRepeatBlockLog{}).Count(&count).Error)
			assert.Equal(t, int64(0), count)
		})
	}
}

func TestGPTAbuseSuspensionExpires(t *testing.T) {
	setupGPTAbuseTestDB(t)
	now := time.Now().Unix()
	require.NoError(t, UpsertGPTAbuseSuspension(42, 7, 5, 5, now-1))

	susp, err := GetActiveGPTAbuseSuspension(42, now)
	require.NoError(t, err)
	assert.Nil(t, susp)

	var stored GPTAbuseUserSuspension
	require.NoError(t, DB.Where("user_id = ?", 42).First(&stored).Error)
	assert.Nil(t, stored.ActiveUserId)
	assert.Equal(t, GPTAbuseSuspensionStatusExpired, stored.Status)
}

func TestGPTAbuseSuspensionActive(t *testing.T) {
	setupGPTAbuseTestDB(t)
	now := time.Now().Unix()
	require.NoError(t, UpsertGPTAbuseSuspension(42, 7, 5, 5, now+3600))

	susp, err := GetActiveGPTAbuseSuspension(42, now)
	require.NoError(t, err)
	require.NotNil(t, susp)
	assert.Equal(t, int64(now+3600), susp.SuspendedUntil)
	assert.Equal(t, GPTAbuseSuspensionStatusActive, susp.Status)
	require.NotNil(t, susp.ActiveUserId)
	assert.Equal(t, 42, *susp.ActiveUserId)
}

func TestGPTAbuseDayWindowUsesLocalCalendarDayAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	oldLocal := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = oldLocal })
	ts := time.Date(2026, 3, 8, 12, 0, 0, 0, loc).Unix()

	start, end := GPTAbuseDayWindow(ts)

	assert.Equal(t, time.Date(2026, 3, 8, 0, 0, 0, 0, loc).Unix(), start)
	assert.Equal(t, time.Date(2026, 3, 9, 0, 0, 0, 0, loc).Unix(), end)
	assert.NotEqual(t, int64(24*60*60), end-start)
}

func TestUpsertGPTAbuseSuspensionDedupesActiveUser(t *testing.T) {
	setupGPTAbuseTestDB(t)
	now := time.Now().Unix()
	require.NoError(t, UpsertGPTAbuseSuspension(42, 7, 5, 5, now+3600))
	require.NoError(t, UpsertGPTAbuseSuspension(42, 8, 6, 5, now+7200))

	var activeCount int64
	require.NoError(t, DB.Model(&GPTAbuseUserSuspension{}).Where("user_id = ? AND status = ?", 42, GPTAbuseSuspensionStatusActive).Count(&activeCount).Error)
	assert.Equal(t, int64(1), activeCount)

	var susp GPTAbuseUserSuspension
	require.NoError(t, DB.Where("user_id = ? AND status = ?", 42, GPTAbuseSuspensionStatusActive).First(&susp).Error)
	assert.Equal(t, 8, susp.TriggerLogId)
	assert.Equal(t, 6, susp.DailyCount)
	assert.Equal(t, int64(now+7200), susp.SuspendedUntil)
}

func TestUpsertGPTAbuseSuspensionConcurrentDedupesActiveUser(t *testing.T) {
	setupGPTAbuseTestDB(t)
	now := time.Now().Unix()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			require.NoError(t, UpsertGPTAbuseSuspension(42, i+1, 5+i, 5, now+int64(i+1)*3600))
		}(i)
	}
	wg.Wait()

	var activeCount int64
	require.NoError(t, DB.Model(&GPTAbuseUserSuspension{}).Where("user_id = ? AND status = ?", 42, GPTAbuseSuspensionStatusActive).Count(&activeCount).Error)
	assert.Equal(t, int64(1), activeCount)
}
