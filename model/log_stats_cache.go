package model

import (
	"sync"
	"time"
)

const (
	logStatCacheTTL        = 5 * time.Second
	logStatCacheMaxEntries = 1024
)

type LogStatOptions struct {
	Refresh bool
}

type logStatCacheKey struct {
	LogType           int
	StartTimestamp    int64
	EndTimestamp      int64
	ModelName         string
	Username          string
	TokenName         string
	Channel           int
	RequestId         string
	UpstreamRequestId string
	TokenIdSet        bool
	TokenId           int
	IsStreamSet       bool
	IsStream          bool
	Status            string
	SelfUserIdSet     bool
	SelfUserId        int
	UserIdSet         bool
	UserId            int
}

type logStatInflightKey struct {
	cacheKey logStatCacheKey
	refresh  bool
}

type logStatCacheEntry struct {
	stat      Stat
	expiresAt time.Time
	updatedAt time.Time
}

type logStatInflightCall struct {
	wg   sync.WaitGroup
	stat Stat
	err  error
}

var logStatCacheMu sync.Mutex
var logStatCache = make(map[logStatCacheKey]logStatCacheEntry)
var logStatInflight = make(map[logStatInflightKey]*logStatInflightCall)

func makeLogStatCacheKey(filter LogFilter) logStatCacheKey {
	key := logStatCacheKey{
		LogType:           filter.LogType,
		StartTimestamp:    filter.StartTimestamp,
		EndTimestamp:      filter.EndTimestamp,
		ModelName:         filter.ModelName,
		Username:          filter.Username,
		TokenName:         filter.TokenName,
		Channel:           filter.Channel,
		RequestId:         filter.RequestId,
		UpstreamRequestId: filter.UpstreamRequestId,
		Status:            filter.Status,
	}
	if filter.TokenId != nil {
		key.TokenIdSet = true
		key.TokenId = *filter.TokenId
	}
	if filter.IsStream != nil {
		key.IsStreamSet = true
		key.IsStream = *filter.IsStream
	}
	if filter.SelfUserId != nil {
		key.SelfUserIdSet = true
		key.SelfUserId = *filter.SelfUserId
	}
	if filter.UserId != nil {
		key.UserIdSet = true
		key.UserId = *filter.UserId
	}
	return key
}

func getLogStatCache(key logStatCacheKey, now time.Time) (Stat, bool) {
	logStatCacheMu.Lock()
	defer logStatCacheMu.Unlock()
	entry, ok := logStatCache[key]
	if !ok {
		return Stat{}, false
	}
	if !now.Before(entry.expiresAt) {
		delete(logStatCache, key)
		return Stat{}, false
	}
	return entry.stat, true
}

func storeLogStatCache(key logStatCacheKey, stat Stat, now time.Time) {
	pruneExpiredLogStatCacheLocked(now)
	if _, exists := logStatCache[key]; !exists {
		for len(logStatCache) >= logStatCacheMaxEntries {
			pruneOldestLogStatCacheLocked()
		}
	}
	logStatCache[key] = logStatCacheEntry{stat: stat, expiresAt: now.Add(logStatCacheTTL), updatedAt: now}
}

func pruneExpiredLogStatCacheLocked(now time.Time) {
	for key, entry := range logStatCache {
		if !now.Before(entry.expiresAt) {
			delete(logStatCache, key)
		}
	}
}

func pruneOldestLogStatCacheLocked() {
	var oldestKey logStatCacheKey
	var oldestTime time.Time
	found := false
	for key, entry := range logStatCache {
		if !found || entry.updatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.updatedAt
			found = true
		}
	}
	if found {
		delete(logStatCache, oldestKey)
	}
}

func updateLogStatCache(key logStatCacheKey, stat Stat) {
	now := time.Now()
	logStatCacheMu.Lock()
	storeLogStatCache(key, stat, now)
	logStatCacheMu.Unlock()
}

func updateLogStatCacheIfNotNewer(key logStatCacheKey, stat Stat, startedAt time.Time) {
	now := time.Now()
	logStatCacheMu.Lock()
	entry, ok := logStatCache[key]
	if !ok || !entry.updatedAt.After(startedAt) {
		storeLogStatCache(key, stat, now)
	}
	logStatCacheMu.Unlock()
}

func beginLogStatInflight(key logStatInflightKey) (*logStatInflightCall, bool) {
	logStatCacheMu.Lock()
	defer logStatCacheMu.Unlock()
	if call := logStatInflight[key]; call != nil {
		return call, false
	}
	call := &logStatInflightCall{}
	call.wg.Add(1)
	logStatInflight[key] = call
	return call, true
}

func finishLogStatInflight(key logStatInflightKey, call *logStatInflightCall, stat Stat, err error) {
	call.stat = stat
	call.err = err
	call.wg.Done()
	logStatCacheMu.Lock()
	if logStatInflight[key] == call {
		delete(logStatInflight, key)
	}
	logStatCacheMu.Unlock()
}

func SumUsedQuotaWithFilterOptions(filter LogFilter, options LogStatOptions) (Stat, error) {
	cacheKey := makeLogStatCacheKey(filter)
	if !options.Refresh {
		if stat, ok := getLogStatCache(cacheKey, time.Now()); ok {
			return stat, nil
		}
	}

	inflightKey := logStatInflightKey{cacheKey: cacheKey, refresh: options.Refresh}
	call, leader := beginLogStatInflight(inflightKey)
	if !leader {
		call.wg.Wait()
		return call.stat, call.err
	}
	startedAt := time.Now()
	stat, usedAggregate, err := sumUsedQuotaWithFilterAggregated(filter)
	if !usedAggregate {
		stat, err = sumUsedQuotaWithFilterDirect(filter)
	}
	if err == nil {
		if options.Refresh {
			updateLogStatCache(cacheKey, stat)
		} else {
			updateLogStatCacheIfNotNewer(cacheKey, stat, startedAt)
		}
	}
	finishLogStatInflight(inflightKey, call, stat, err)
	return stat, err
}

func ResetLogStatCacheForTest() {
	logStatCacheMu.Lock()
	logStatCache = make(map[logStatCacheKey]logStatCacheEntry)
	logStatInflight = make(map[logStatInflightKey]*logStatInflightCall)
	logStatCacheMu.Unlock()
}
