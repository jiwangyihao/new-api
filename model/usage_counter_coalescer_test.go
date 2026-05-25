package model

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	gormlogger "gorm.io/gorm/logger"
)

type usageCounterUpdateLogger struct {
	gormlogger.Interface
	userUpdates    atomic.Int64
	channelUpdates atomic.Int64
}

func (l *usageCounterUpdateLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	if isUsageCounterUpdateSQL(sql, "users") {
		l.userUpdates.Add(1)
	}
	if isUsageCounterUpdateSQL(sql, "channels") {
		l.channelUpdates.Add(1)
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

func isUsageCounterUpdateSQL(sql string, table string) bool {
	lower := strings.ToLower(sql)
	return strings.Contains(lower, "update") &&
		(strings.Contains(lower, "`"+table+"`") || strings.Contains(lower, `"`+table+`"`)) &&
		strings.Contains(lower, "used_quota")
}

func TestUsageCounterUpdatesCoalesceConcurrentHotWrites(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &Channel{}))

	const (
		userID    = 95001
		channelID = 95002
		workers   = 32
		quota     = 7
	)
	require.NoError(t, DB.Create(&User{Id: userID, Username: "usage_counter_user", Status: common.UserStatusEnabled, AffCode: "aff95001"}).Error)
	require.NoError(t, DB.Create(&Channel{Id: channelID, Name: "usage_counter_channel", Key: "sk-test", Status: common.ChannelStatusEnabled}).Error)

	oldBatchUpdate := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatchUpdate })
	oldUserCoalescer := userUsageCounterCoalescer
	oldChannelCoalescer := channelUsageCounterCoalescer
	userUsageCounterCoalescer = newUsageCounterCoalescer(usageCounterCoalesceDelay, updateUserUsedQuotaAndRequestCount)
	channelUsageCounterCoalescer = newUsageCounterCoalescer(usageCounterCoalesceDelay, func(id int, quota int, _ int) {
		updateChannelUsedQuota(id, quota)
	})
	t.Cleanup(func() {
		userUsageCounterCoalescer = oldUserCoalescer
		channelUsageCounterCoalescer = oldChannelCoalescer
	})

	counter := &usageCounterUpdateLogger{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	restoreLogger := DB.Config.Logger
	DB.Config.Logger = counter
	t.Cleanup(func() { DB.Config.Logger = restoreLogger })

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			UpdateUserUsedQuotaAndRequestCount(userID, quota)
			UpdateChannelUsedQuota(channelID, quota)
		}()
	}
	close(start)
	wg.Wait()

	var user User
	require.NoError(t, DB.Select("used_quota", "request_count").Where("id = ?", userID).First(&user).Error)
	require.Equal(t, workers*quota, user.UsedQuota)
	require.Equal(t, workers, user.RequestCount)

	var channel Channel
	require.NoError(t, DB.Select("used_quota").Where("id = ?", channelID).First(&channel).Error)
	require.Equal(t, int64(workers*quota), channel.UsedQuota)

	require.LessOrEqual(t, counter.userUpdates.Load(), int64(4), "user usage counters should be coalesced under concurrent hot writes")
	require.LessOrEqual(t, counter.channelUpdates.Load(), int64(4), "channel usage counters should be coalesced under concurrent hot writes")
}
