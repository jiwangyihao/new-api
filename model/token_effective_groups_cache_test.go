package model

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenEffectiveGroupsCacheCollapsesConcurrentLoads(t *testing.T) {
	cache := newTokenEffectiveGroupsCache()
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	load := func(int) ([]string, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []string{"paid"}, nil
	}
	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			groups, err := cache.get(7, load)
			if err != nil || len(groups) != 1 || groups[0] != "paid" {
				t.Errorf("groups=%v err=%v", groups, err)
			}
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("loader did not start")
	}
	close(release)
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("load calls=%d want=1", got)
	}
}

func TestTokenEffectiveGroupsCacheDifferentTokensLoadInParallel(t *testing.T) {
	cache := newTokenEffectiveGroupsCache()
	started := make(chan int, 2)
	release := make(chan struct{})
	load := func(tokenID int) ([]string, error) {
		started <- tokenID
		<-release
		return []string{"paid"}, nil
	}
	var wg sync.WaitGroup
	for _, id := range []int{1, 2} {
		wg.Add(1)
		go func(tokenID int) {
			defer wg.Done()
			_, _ = cache.get(tokenID, load)
		}(id)
	}
	seen := map[int]bool{}
	for len(seen) < 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(time.Second):
			t.Fatal("different token loads did not run in parallel")
		}
	}
	close(release)
	wg.Wait()
}

func TestTokenEffectiveGroupsCacheDoesNotCacheErrors(t *testing.T) {
	cache := newTokenEffectiveGroupsCache()
	wantErr := errors.New("load failed")
	calls := 0
	load := func(int) ([]string, error) {
		calls++
		if calls == 1 {
			return nil, wantErr
		}
		return []string{"paid"}, nil
	}
	_, err := cache.get(7, load)
	if !errors.Is(err, wantErr) {
		t.Fatalf("first err=%v want=%v", err, wantErr)
	}
	groups, err := cache.get(7, load)
	if err != nil || len(groups) != 1 || calls != 2 {
		t.Fatalf("groups=%v err=%v calls=%d", groups, err, calls)
	}
}

func TestTokenEffectiveGroupsCacheFlushStartsNewGeneration(t *testing.T) {
	cache := newTokenEffectiveGroupsCache()
	oldStarted := make(chan struct{})
	releaseOld := make(chan struct{})
	oldDone := make(chan struct{})
	go func() {
		defer close(oldDone)
		_, _ = cache.get(9, func(int) ([]string, error) {
			close(oldStarted)
			<-releaseOld
			return []string{"old"}, nil
		})
	}()
	<-oldStarted
	cache.flush()
	groups, err := cache.get(9, func(int) ([]string, error) { return []string{"new"}, nil })
	if err != nil || len(groups) != 1 || groups[0] != "new" {
		t.Fatalf("new groups=%v err=%v", groups, err)
	}
	close(releaseOld)
	<-oldDone
	groups, err = cache.get(9, func(int) ([]string, error) {
		t.Fatal("old generation overwrote new cache")
		return nil, nil
	})
	if err != nil || groups[0] != "new" {
		t.Fatalf("cached groups=%v err=%v", groups, err)
	}
}

func TestTokenEffectiveGroupsCacheReturnsIndependentSlices(t *testing.T) {
	cache := newTokenEffectiveGroupsCache()
	first, err := cache.get(1, func(int) ([]string, error) {
		return []string{"paid", "trial"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	first[0] = "mutated"
	second, err := cache.get(1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second[0] != "paid" {
		t.Fatalf("cached groups were mutated: %v", second)
	}
}

func BenchmarkTokenEffectiveGroupsCacheHit(b *testing.B) {
	cache := newTokenEffectiveGroupsCache()
	_, _ = cache.get(1, func(int) ([]string, error) { return []string{"paid", "trial"}, nil })
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		groups, err := cache.get(1, nil)
		if err != nil || len(groups) != 2 {
			b.Fatal(groups, err)
		}
	}
}
