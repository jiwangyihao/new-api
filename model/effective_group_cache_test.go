package model

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEffectiveGroupRowsCacheCollapsesConcurrentLoads(t *testing.T) {
	cache := newEffectiveGroupRowsCache()
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	load := func(channelID int) ([]effectiveGroupForChannelRow, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []effectiveGroupForChannelRow{{ChannelGroup: ChannelGroup{Id: channelID, Name: "paid"}, SelectedMemberCount: 1}}, nil
	}
	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			rows, err := cache.get(42, load)
			if err != nil || len(rows) != 1 || rows[0].Id != 42 {
				t.Errorf("rows=%#v err=%v", rows, err)
			}
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("load did not start")
	}
	close(release)
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("load calls=%d want=1", got)
	}
}

func TestEffectiveGroupRowsCacheDifferentChannelsLoadInParallel(t *testing.T) {
	cache := newEffectiveGroupRowsCache()
	started := make(chan int, 2)
	release := make(chan struct{})
	load := func(channelID int) ([]effectiveGroupForChannelRow, error) {
		started <- channelID
		<-release
		return []effectiveGroupForChannelRow{{ChannelGroup: ChannelGroup{Id: channelID}}}, nil
	}
	var wg sync.WaitGroup
	for _, id := range []int{1, 2} {
		wg.Add(1)
		go func(channelID int) {
			defer wg.Done()
			_, _ = cache.get(channelID, load)
		}(id)
	}
	seen := map[int]bool{}
	for len(seen) < 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(time.Second):
			t.Fatal("different channel loads did not run in parallel")
		}
	}
	close(release)
	wg.Wait()
}

func TestEffectiveGroupRowsCacheDoesNotCacheErrors(t *testing.T) {
	cache := newEffectiveGroupRowsCache()
	wantErr := errors.New("load failed")
	calls := 0
	load := func(channelID int) ([]effectiveGroupForChannelRow, error) {
		calls++
		if calls == 1 {
			return nil, wantErr
		}
		return []effectiveGroupForChannelRow{{ChannelGroup: ChannelGroup{Id: channelID}}}, nil
	}
	_, err := cache.get(7, load)
	if !errors.Is(err, wantErr) {
		t.Fatalf("first err=%v", err)
	}
	rows, err := cache.get(7, load)
	if err != nil || len(rows) != 1 || calls != 2 {
		t.Fatalf("rows=%#v err=%v calls=%d", rows, err, calls)
	}
}

func TestEffectiveGroupRowsCacheFlushStartsNewGeneration(t *testing.T) {
	cache := newEffectiveGroupRowsCache()
	oldStarted := make(chan struct{})
	releaseOld := make(chan struct{})
	loadOld := func(int) ([]effectiveGroupForChannelRow, error) {
		close(oldStarted)
		<-releaseOld
		return []effectiveGroupForChannelRow{{ChannelGroup: ChannelGroup{Id: 1}}}, nil
	}
	oldDone := make(chan struct{})
	go func() {
		_, _ = cache.get(9, loadOld)
		close(oldDone)
	}()
	<-oldStarted
	cache.flush()
	rows, err := cache.get(9, func(int) ([]effectiveGroupForChannelRow, error) {
		return []effectiveGroupForChannelRow{{ChannelGroup: ChannelGroup{Id: 2}}}, nil
	})
	if err != nil || rows[0].Id != 2 {
		t.Fatalf("new generation rows=%#v err=%v", rows, err)
	}
	close(releaseOld)
	<-oldDone
	rows, err = cache.get(9, func(int) ([]effectiveGroupForChannelRow, error) {
		t.Fatal("old generation overwrote new cache")
		return nil, nil
	})
	if err != nil || rows[0].Id != 2 {
		t.Fatalf("cached rows=%#v err=%v", rows, err)
	}
}

func BenchmarkEffectiveGroupRowsCacheHit(b *testing.B) {
	cache := newEffectiveGroupRowsCache()
	_, err := cache.get(42, func(channelID int) ([]effectiveGroupForChannelRow, error) {
		return []effectiveGroupForChannelRow{{ChannelGroup: ChannelGroup{Id: channelID, Name: "paid"}, SelectedMemberCount: 1}}, nil
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rows, err := cache.get(42, func(int) ([]effectiveGroupForChannelRow, error) {
			b.Fatal("cache miss")
			return nil, nil
		})
		if err != nil || len(rows) != 1 {
			b.Fatal(err)
		}
	}
}
