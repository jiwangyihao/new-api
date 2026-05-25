package model

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type countingSQLLogger struct {
	logger.Interface
	selects     atomic.Int64
	selectDelay time.Duration
}

func (l *countingSQLLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
		l.selects.Add(1)
		if l.selectDelay > 0 {
			time.Sleep(l.selectDelay)
		}
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

func setupCacheStampedeTestDB(t *testing.T, tables ...any) *countingSQLLogger {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldRedisEnabled := common.RedisEnabled
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL

	counter := &countingSQLLogger{Interface: logger.Default.LogMode(logger.Silent), selectDelay: 10 * time.Millisecond}
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: counter})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	oldTokenLookupGroup := tokenLookupGroup
	oldUserCacheLookupGroup := userCacheLookupGroup
	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, db.AutoMigrate(tables...))
	counter.selects.Store(0)

	tokenLookupGroup = singleflight.Group{}
	userCacheLookupGroup = singleflight.Group{}
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		tokenLookupGroup = oldTokenLookupGroup
		userCacheLookupGroup = oldUserCacheLookupGroup
		_ = sqlDB.Close()
	})
	return counter
}

func TestConcurrentTokenValidationCoalescesColdDBLookup(t *testing.T) {
	counter := setupCacheStampedeTestDB(t, &Token{})
	require.NoError(t, DB.Create(&Token{Id: 1, UserId: 10, Key: "loadtestsub", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100}).Error)
	counter.selects.Store(0)

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			token, err := ValidateUserToken("loadtestsub")
			require.NoError(t, err)
			require.NotNil(t, token)
			require.Equal(t, 1, token.Id)
		}()
	}
	close(start)
	wg.Wait()

	require.Equal(t, int64(1), counter.selects.Load(), "cold concurrent token validation should share the same DB lookup")
}

func TestConcurrentUserCacheCoalescesColdDBLookup(t *testing.T) {
	counter := setupCacheStampedeTestDB(t, &User{})
	require.NoError(t, DB.Create(&User{Id: 10, Username: "loadtest", Status: common.UserStatusEnabled, Quota: 100, AffCode: "loadtest"}).Error)
	counter.selects.Store(0)

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			user, err := GetUserCache(10)
			require.NoError(t, err)
			require.NotNil(t, user)
			require.Equal(t, common.UserStatusEnabled, user.Status)
		}()
	}
	close(start)
	wg.Wait()

	require.Equal(t, int64(1), counter.selects.Load(), "cold concurrent user cache loads should share the same DB lookup")
}
