package model

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const dbTimestampCacheTTL = 900 * time.Millisecond

var dbTimestampCache atomic.Int64
var dbTimestampCacheUnixNano atomic.Int64
var dbTimestampLookupGroup singleflight.Group

// GetDBTimestamp returns a UNIX timestamp from database time.
// Falls back to application time on error.
func GetDBTimestamp() int64 {
	return getCachedDBTimestamp(DB)
}

func getCachedDBTimestamp(tx *gorm.DB) int64 {
	if tx == nil {
		return common.GetTimestamp()
	}
	now := time.Now()
	if cached := dbTimestampCache.Load(); cached > 0 && now.Sub(time.Unix(0, dbTimestampCacheUnixNano.Load())) < dbTimestampCacheTTL {
		return cached
	}
	value, err, _ := dbTimestampLookupGroup.Do("now", func() (interface{}, error) {
		queryStarted := time.Now()
		if cached := dbTimestampCache.Load(); cached > 0 && queryStarted.Sub(time.Unix(0, dbTimestampCacheUnixNano.Load())) < dbTimestampCacheTTL {
			return cached, nil
		}
		ts := getDBTimestampTx(tx)
		dbTimestampCache.Store(ts)
		dbTimestampCacheUnixNano.Store(queryStarted.UnixNano())
		return ts, nil
	})
	if err != nil {
		return common.GetTimestamp()
	}
	if ts, ok := value.(int64); ok && ts > 0 {
		return ts
	}
	return common.GetTimestamp()
}

func resetDBTimestampCacheForTest() {
	dbTimestampCache.Store(0)
	dbTimestampCacheUnixNano.Store(0)
}

// getDBTimestampStrictTx returns the database UTC time in whole UNIX seconds.
// Unlike getDBTimestampTx it never falls back to an application clock.
func getDBTimestampStrictTx(tx *gorm.DB) (int64, error) {
	if tx == nil {
		return 0, fmt.Errorf("database time query requires a transaction")
	}
	var ts int64
	var err error
	switch {
	case common.UsingPostgreSQL:
		err = tx.Raw("SELECT FLOOR(EXTRACT(EPOCH FROM CURRENT_TIMESTAMP))::bigint").Scan(&ts).Error
	case common.UsingSQLite:
		err = tx.Raw("SELECT CAST(strftime('%s','now') AS INTEGER)").Scan(&ts).Error
	default:
		err = tx.Raw("SELECT UNIX_TIMESTAMP()").Scan(&ts).Error
	}
	if err != nil {
		return 0, fmt.Errorf("query database time: %w", err)
	}
	if ts <= 0 {
		return 0, fmt.Errorf("database returned invalid timestamp %d", ts)
	}
	return ts, nil
}

func getDBTimestampTx(tx *gorm.DB) int64 {
	if tx == nil {
		return common.GetTimestamp()
	}
	var ts int64
	var err error
	switch {
	case common.UsingPostgreSQL:
		err = tx.Raw("SELECT EXTRACT(EPOCH FROM NOW())::bigint").Scan(&ts).Error
	case common.UsingSQLite:
		err = tx.Raw("SELECT strftime('%s','now')").Scan(&ts).Error
	default:
		err = tx.Raw("SELECT UNIX_TIMESTAMP()").Scan(&ts).Error
	}
	if err != nil || ts <= 0 {
		return common.GetTimestamp()
	}
	return ts
}
