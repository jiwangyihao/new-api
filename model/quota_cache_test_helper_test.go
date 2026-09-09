package model

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

type blockingQuotaDB struct {
	blocked     chan struct{}
	releaseCh   chan struct{}
	once        sync.Once
	releaseOnce sync.Once
	entered     chan struct{}
}

func newBlockingQuotaDB(t *testing.T) *blockingQuotaDB {
	t.Helper()
	return &blockingQuotaDB{blocked: make(chan struct{}), releaseCh: make(chan struct{}), entered: make(chan struct{}, 8)}
}

func (b *blockingQuotaDB) waitUntilBlocked(t *testing.T) {
	t.Helper()
	select {
	case <-b.blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("quota save did not reach database I/O")
	}
}

func (b *blockingQuotaDB) release() {
	b.releaseOnce.Do(func() { close(b.releaseCh) })
}

func (b *blockingQuotaDB) markBlocked() {
	b.once.Do(func() { close(b.blocked) })
}

func quotaBlockingLogger(base gormlogger.Interface, owner *blockingQuotaDB) gormlogger.Interface {
	return &quotaBlockingLoggerImpl{Interface: base, owner: owner}
}

type quotaBlockingLoggerImpl struct {
	gormlogger.Interface
	owner *blockingQuotaDB
}

func (l *quotaBlockingLoggerImpl) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	if strings.Contains(strings.ToLower(sql), "quota_data") {
		l.owner.markBlocked()
		select {
		case l.owner.entered <- struct{}{}:
		default:
		}
		<-l.owner.releaseCh
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}
