package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type rankingsCacheTestResult struct {
	data *RankingsResponse
	err  error
}

func closeRankingsTestChannel(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func TestRankingsSnapshotCacheCoalescesConcurrentBuildsForSamePeriod(t *testing.T) {
	cache := newRankingsSnapshotCache(time.Minute)
	const callers = 32
	start := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { closeRankingsTestChannel(release) })
	var ready sync.WaitGroup
	ready.Add(callers)
	var builds atomic.Int32
	want := &RankingsResponse{}
	results := make(chan rankingsCacheTestResult, callers)

	for range callers {
		go func() {
			ready.Done()
			<-start
			data, err := cache.get("week", time.Unix(100, 0), func() (*RankingsResponse, error) {
				builds.Add(1)
				<-release
				return want, nil
			})
			results <- rankingsCacheTestResult{data: data, err: err}
		}()
	}
	ready.Wait()
	close(start)
	require.Eventually(t, func() bool { return builds.Load() == 1 }, time.Second, time.Millisecond)
	closeRankingsTestChannel(release)

	for range callers {
		result := <-results
		require.NoError(t, result.err)
		require.Same(t, want, result.data)
	}
	require.EqualValues(t, 1, builds.Load())
}

func TestRankingsSnapshotCacheBuildsDifferentPeriodsInParallel(t *testing.T) {
	cache := newRankingsSnapshotCache(time.Minute)
	started := make(chan string, 2)
	release := make(chan struct{})
	t.Cleanup(func() { closeRankingsTestChannel(release) })
	results := make(chan rankingsCacheTestResult, 2)

	for _, period := range []string{"week", "month"} {
		period := period
		go func() {
			data, err := cache.get(period, time.Unix(100, 0), func() (*RankingsResponse, error) {
				started <- period
				<-release
				return &RankingsResponse{}, nil
			})
			results <- rankingsCacheTestResult{data: data, err: err}
		}()
	}

	seen := map[string]bool{}
	for range 2 {
		select {
		case period := <-started:
			seen[period] = true
		case <-time.After(time.Second):
			closeRankingsTestChannel(release)
			t.Fatal("different ranking periods did not build concurrently")
		}
	}
	require.Equal(t, map[string]bool{"week": true, "month": true}, seen)
	closeRankingsTestChannel(release)
	for range 2 {
		require.NoError(t, (<-results).err)
	}
}

func TestRankingsSnapshotCacheDoesNotCacheBuildErrors(t *testing.T) {
	cache := newRankingsSnapshotCache(time.Minute)
	buildErr := errors.New("build failed")
	var builds atomic.Int32

	data, err := cache.get("week", time.Unix(100, 0), func() (*RankingsResponse, error) {
		builds.Add(1)
		return nil, buildErr
	})
	require.Nil(t, data)
	require.ErrorIs(t, err, buildErr)

	want := &RankingsResponse{}
	data, err = cache.get("week", time.Unix(101, 0), func() (*RankingsResponse, error) {
		builds.Add(1)
		return want, nil
	})
	require.NoError(t, err)
	require.Same(t, want, data)
	require.EqualValues(t, 2, builds.Load())
}

func TestRankingsSnapshotCacheFlushStartsNewGenerationWithoutWaiting(t *testing.T) {
	cache := newRankingsSnapshotCache(time.Minute)
	oldStarted := make(chan struct{})
	oldRelease := make(chan struct{})
	newStarted := make(chan struct{})
	newRelease := make(chan struct{})
	t.Cleanup(func() {
		closeRankingsTestChannel(oldRelease)
		closeRankingsTestChannel(newRelease)
	})
	oldData := &RankingsResponse{}
	newData := &RankingsResponse{}
	oldResult := make(chan rankingsCacheTestResult, 1)
	newResult := make(chan rankingsCacheTestResult, 1)

	go func() {
		data, err := cache.get("week", time.Unix(100, 0), func() (*RankingsResponse, error) {
			close(oldStarted)
			<-oldRelease
			return oldData, nil
		})
		oldResult <- rankingsCacheTestResult{data: data, err: err}
	}()
	<-oldStarted
	cache.flush()

	go func() {
		data, err := cache.get("week", time.Unix(101, 0), func() (*RankingsResponse, error) {
			close(newStarted)
			<-newRelease
			return newData, nil
		})
		newResult <- rankingsCacheTestResult{data: data, err: err}
	}()
	select {
	case <-newStarted:
	case <-time.After(time.Second):
		closeRankingsTestChannel(oldRelease)
		closeRankingsTestChannel(newRelease)
		t.Fatal("flush did not allow a new generation to start immediately")
	}

	closeRankingsTestChannel(newRelease)
	fresh := <-newResult
	require.NoError(t, fresh.err)
	require.Same(t, newData, fresh.data)
	closeRankingsTestChannel(oldRelease)
	stale := <-oldResult
	require.NoError(t, stale.err)
	require.Same(t, oldData, stale.data)

	var unexpectedBuilds atomic.Int32
	cached, err := cache.get("week", time.Unix(102, 0), func() (*RankingsResponse, error) {
		unexpectedBuilds.Add(1)
		return &RankingsResponse{}, nil
	})
	require.NoError(t, err)
	require.Same(t, newData, cached)
	require.Zero(t, unexpectedBuilds.Load())
}

func BenchmarkRankingsSnapshotCacheConcurrentSamePeriod(b *testing.B) {
	for b.Loop() {
		cache := newRankingsSnapshotCache(time.Minute)
		var builds atomic.Int32
		var wg sync.WaitGroup
		wg.Add(32)
		start := make(chan struct{})
		for range 32 {
			go func() {
				defer wg.Done()
				<-start
				_, err := cache.get("daily", time.Unix(1, 0), func() (*RankingsResponse, error) {
					builds.Add(1)
					return &RankingsResponse{}, nil
				})
				if err != nil {
					b.Error(err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if builds.Load() != 1 {
			b.Fatalf("builds = %d, want 1", builds.Load())
		}
	}
}
