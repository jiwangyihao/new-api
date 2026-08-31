package cachex

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLocalFrontCacheExpiryInvalidationAndCapacity(t *testing.T) {
	now := time.Unix(100, 0)
	cache := newLocalFrontCacheWithClock[int](5*time.Second, 2, func() time.Time { return now })

	cache.SetWithTTL("plan", 7, time.Minute)
	value, found := cache.Get("plan")
	require.True(t, found)
	require.Equal(t, 7, value)

	now = now.Add(5 * time.Second)
	_, found = cache.Get("plan")
	require.False(t, found)

	cache.SetWithTTL("plan", 8, time.Minute)
	cache.Delete("plan")
	_, found = cache.Get("plan")
	require.False(t, found)

	cache.SetWithTTL("plan:1", 1, time.Minute)
	cache.SetWithTTL("plan:2", 2, time.Minute)
	cache.DeleteByPrefix("plan:")
	_, found = cache.Get("plan:1")
	require.False(t, found)
	_, found = cache.Get("plan:2")
	require.False(t, found)

	cache.SetWithTTL("a", 1, time.Minute)
	now = now.Add(time.Second)
	cache.SetWithTTL("b", 2, time.Minute)
	now = now.Add(time.Second)
	cache.SetWithTTL("c", 3, time.Minute)
	_, found = cache.Get("a")
	require.False(t, found, "oldest entry must be evicted at capacity")
	_, found = cache.Get("b")
	require.True(t, found)
	_, found = cache.Get("c")
	require.True(t, found)

	cache.Purge()
	_, found = cache.Get("b")
	require.False(t, found)
	_, found = cache.Get("c")
	require.False(t, found)
}

func TestLocalFrontCacheDoesNotOutliveShorterSharedTTL(t *testing.T) {
	now := time.Unix(200, 0)
	cache := newLocalFrontCacheWithClock[int](time.Second, 2, func() time.Time { return now })
	cache.SetWithTTL("plan", 7, 100*time.Millisecond)

	now = now.Add(100 * time.Millisecond)
	_, found := cache.Get("plan")
	require.False(t, found)
}

func TestLocalFrontCacheSingleflightsSameKey(t *testing.T) {
	cache := NewLocalFrontCache[int](time.Second, 8)
	var calls atomic.Int64
	start := make(chan struct{})
	results := make(chan error, 32)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			value, found, err := cache.GetOrLoad("plan", time.Minute, func() (int, bool, error) {
				calls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return 7, true, nil
			})
			if err != nil || !found || value != 7 {
				results <- errors.New("unexpected singleflight result")
			}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, calls.Load())
}

func TestLocalFrontCacheInvalidationPreventsOldLoadBackfill(t *testing.T) {
	cache := NewLocalFrontCache[int](time.Second, 8)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = cache.GetOrLoad("plan", time.Minute, func() (int, bool, error) {
			close(started)
			<-release
			return 7, true, nil
		})
	}()
	<-started
	cache.Delete("plan")
	close(release)
	<-done
	_, found := cache.Get("plan")
	require.False(t, found)
}

func TestLocalFrontCacheLoaderErrorIsNotCached(t *testing.T) {
	cache := NewLocalFrontCache[int](time.Second, 8)
	var calls int
	loader := func() (int, bool, error) {
		calls++
		if calls == 1 {
			return 0, false, errors.New("temporary")
		}
		return 9, true, nil
	}
	_, _, err := cache.GetOrLoad("plan", time.Minute, loader)
	require.Error(t, err)
	value, found, err := cache.GetOrLoad("plan", time.Minute, loader)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 9, value)
	require.Equal(t, 2, calls)
}
