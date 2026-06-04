package model

import (
	"strings"
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
	require.NoError(t, db.AutoMigrate(&GPTAbuseSignalLog{}, &GPTAbuseUserSuspension{}))

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
