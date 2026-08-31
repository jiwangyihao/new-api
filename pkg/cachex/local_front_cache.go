package cachex

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

type localFrontEntry[V any] struct {
	value     V
	expiresAt time.Time
}

// LocalFrontCache is a bounded-staleness, process-local cache intended to sit
// in front of a shared cache. Expiration is absolute: reads do not extend it.
type LocalFrontCache[V any] struct {
	mu         sync.RWMutex
	items      map[string]localFrontEntry[V]
	ttl        time.Duration
	capacity   int
	now        func() time.Time
	generation atomic.Uint64
	loadMu     sync.RWMutex
	loads      *singleflight.Group
}

func NewLocalFrontCache[V any](ttl time.Duration, capacity int) *LocalFrontCache[V] {
	return newLocalFrontCacheWithClock[V](ttl, capacity, time.Now)
}

func newLocalFrontCacheWithClock[V any](ttl time.Duration, capacity int, now func() time.Time) *LocalFrontCache[V] {
	if now == nil {
		now = time.Now
	}
	return &LocalFrontCache[V]{
		items:    make(map[string]localFrontEntry[V]),
		ttl:      ttl,
		capacity: capacity,
		now:      now,
		loads:    &singleflight.Group{},
	}
}

func (c *LocalFrontCache[V]) enabled() bool {
	return c != nil && c.ttl > 0 && c.capacity > 0
}

func (c *LocalFrontCache[V]) Get(key string) (V, bool) {
	var zero V
	if !c.enabled() || key == "" {
		return zero, false
	}

	now := c.now()
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return zero, false
	}
	if now.Before(entry.expiresAt) {
		return entry.value, true
	}

	c.mu.Lock()
	if current, exists := c.items[key]; exists && !now.Before(current.expiresAt) {
		delete(c.items, key)
	}
	c.mu.Unlock()
	return zero, false
}

type localFrontLoadResult[V any] struct {
	value V
	found bool
}

func (c *LocalFrontCache[V]) GetOrLoad(key string, sharedTTL time.Duration, loader func() (V, bool, error)) (V, bool, error) {
	var zero V
	if loader == nil {
		return zero, false, nil
	}
	if !c.enabled() || key == "" {
		return loader()
	}
	if value, found := c.Get(key); found {
		return value, true, nil
	}
	generation := c.generation.Load()
	c.loadMu.RLock()
	loads := c.loads
	c.loadMu.RUnlock()
	loaded, err, _ := loads.Do(key, func() (any, error) {
		if value, found := c.Get(key); found {
			return localFrontLoadResult[V]{value: value, found: true}, nil
		}
		value, found, loadErr := loader()
		if loadErr != nil {
			return nil, loadErr
		}
		if found {
			c.setWithTTLIfGeneration(key, value, sharedTTL, generation)
		}
		return localFrontLoadResult[V]{value: value, found: found}, nil
	})
	if err != nil {
		return zero, false, err
	}
	result := loaded.(localFrontLoadResult[V])
	return result.value, result.found, nil
}

func (c *LocalFrontCache[V]) forgetLoad(key string) {
	c.generation.Add(1)
	c.loadMu.RLock()
	loads := c.loads
	c.loadMu.RUnlock()
	loads.Forget(key)
}

func (c *LocalFrontCache[V]) replaceLoads() {
	c.generation.Add(1)
	c.loadMu.Lock()
	c.loads = &singleflight.Group{}
	c.loadMu.Unlock()
}

func (c *LocalFrontCache[V]) setWithTTLIfGeneration(key string, value V, sharedTTL time.Duration, generation uint64) bool {
	if c.generation.Load() != generation {
		return false
	}
	return c.setWithTTL(key, value, sharedTTL)
}

func (c *LocalFrontCache[V]) SetWithTTL(key string, value V, sharedTTL time.Duration) {
	if !c.enabled() || key == "" {
		return
	}
	c.forgetLoad(key)
	c.setWithTTL(key, value, sharedTTL)
}

func (c *LocalFrontCache[V]) setWithTTL(key string, value V, sharedTTL time.Duration) bool {
	ttl := c.ttl
	if sharedTTL > 0 && sharedTTL < ttl {
		ttl = sharedTTL
	}
	if ttl <= 0 {
		return false
	}

	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[key]; !exists && len(c.items) >= c.capacity {
		for cachedKey, entry := range c.items {
			if !now.Before(entry.expiresAt) {
				delete(c.items, cachedKey)
			}
		}
		if len(c.items) >= c.capacity {
			var oldestKey string
			var oldestExpiry time.Time
			for cachedKey, entry := range c.items {
				if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
					oldestKey = cachedKey
					oldestExpiry = entry.expiresAt
				}
			}
			delete(c.items, oldestKey)
		}
	}
	c.items[key] = localFrontEntry[V]{value: value, expiresAt: now.Add(ttl)}
	return true
}

func (c *LocalFrontCache[V]) Delete(key string) {
	if c == nil || key == "" {
		return
	}
	c.forgetLoad(key)
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

func (c *LocalFrontCache[V]) DeleteByPrefix(prefix string) {
	if c == nil || prefix == "" {
		return
	}
	c.replaceLoads()
	c.mu.Lock()
	for key := range c.items {
		if strings.HasPrefix(key, prefix) {
			delete(c.items, key)
		}
	}
	c.mu.Unlock()
}

func (c *LocalFrontCache[V]) Purge() {
	if c == nil {
		return
	}
	c.replaceLoads()
	c.mu.Lock()
	c.items = make(map[string]localFrontEntry[V])
	c.mu.Unlock()
}
