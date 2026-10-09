package common

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
	reservations       map[string]int
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.store != nil {
		return
	}
	l.store = make(map[string]*[]int64)
	l.reservations = make(map[string]int)
	l.expirationDuration = expirationDuration
	if expirationDuration > 0 {
		go l.clearExpiredItems()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key := range l.store {
			queue := l.store[key]
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1] > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	if ok {
		if len(*queue) < maxRequestNum {
			*queue = append(*queue, now)
			return true
		} else {
			if now-(*queue)[0] >= duration {
				*queue = (*queue)[1:]
				*queue = append(*queue, now)
				return true
			} else {
				return false
			}
		}
	} else {
		s := make([]int64, 0, maxRequestNum)
		l.store[key] = &s
		*(l.store[key]) = append(*(l.store[key]), now)
	}
	return true
}

// RateLimitReservation includes an in-flight request in success-only admission.
// Failure releases the slot without consuming the completed-request window.
type RateLimitReservation struct {
	limiter *InMemoryRateLimiter
	key     string
	once    sync.Once
}

func (l *InMemoryRateLimiter) Reserve(key string, maximum int, duration int64) *RateLimitReservation {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := time.Now().Unix()
	queue := l.store[key]
	if queue != nil {
		for len(*queue) > 0 && now-(*queue)[0] >= duration {
			*queue = (*queue)[1:]
		}
	}
	completed := 0
	if queue != nil {
		completed = len(*queue)
	}
	if maximum <= 0 || completed+l.reservations[key] >= maximum {
		return nil
	}
	l.reservations[key]++
	return &RateLimitReservation{limiter: l, key: key}
}

func (r *RateLimitReservation) Finish(success bool) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		l := r.limiter
		l.mutex.Lock()
		defer l.mutex.Unlock()
		l.reservations[r.key]--
		if l.reservations[r.key] == 0 {
			delete(l.reservations, r.key)
		}
		if success {
			queue := l.store[r.key]
			if queue == nil {
				queue = new([]int64)
				l.store[r.key] = queue
			}
			*queue = append(*queue, time.Now().Unix())
		}
	})
}
