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
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type dbTimeCountingLogger struct {
	logger.Interface
	timestampSelects atomic.Int64
}

func (l *dbTimeCountingLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	if strings.Contains(sql, "strftime('%s','now')") || strings.Contains(sql, "EXTRACT(EPOCH FROM NOW())") || strings.Contains(sql, "UNIX_TIMESTAMP()") {
		l.timestampSelects.Add(1)
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

func TestGetDBTimestampCoalescesConcurrentTopLevelReads(t *testing.T) {
	oldDB := DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	logger := &dbTimeCountingLogger{Interface: logger.Default.LogMode(logger.Silent)}
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	resetDBTimestampCacheForTest()
	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		resetDBTimestampCacheForTest()
		_ = sqlDB.Close()
	})

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			require.Positive(t, GetDBTimestamp())
		}()
	}
	close(start)
	wg.Wait()

	require.Equal(t, int64(1), logger.timestampSelects.Load(), "concurrent top-level timestamp reads should share one DB query")
}
