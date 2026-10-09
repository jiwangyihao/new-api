package routehealth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

type mutation func(map[string]*resourceState, time.Time) (bool, error)

type storageError struct{ err error }

func (e *storageError) Error() string { return e.err.Error() }
func (e *storageError) Unwrap() error { return e.err }

var errContention = errors.New("routehealth: concurrent state update")
var errCapacity = errors.New("routehealth: local resource capacity exhausted")

func resourceKey(r Resource) string {
	// 保留调用方的 Redis hash tag，不解析资源身份或原始秘密。
	return "routehealth:v1:" + string(r.Scope) + ":" + r.Key
}

func resourceKeys(resources []Resource) []string {
	keys := make([]string, len(resources))
	for i, resource := range resources {
		keys[i] = resourceKey(resource)
	}
	return keys
}

func decodeStates(keys []string, values []any, now time.Time, p Policy) (map[string]*resourceState, error) {
	states := make(map[string]*resourceState, len(keys))
	for i, key := range keys {
		s := newState()
		if values[i] != nil {
			raw, ok := values[i].(string)
			if !ok || len(raw) > 2*1024*1024 {
				return nil, errors.New("routehealth: invalid stored state")
			}
			if err := common.UnmarshalJsonStr(raw, s); err != nil {
				return nil, fmt.Errorf("routehealth: decode stored state: %w", err)
			}
			if s.Generation == 0 || len(s.Active) > maxActiveEntries || len(s.FailureTimes) > maxActiveEntries || len(s.Buckets) > maxSampleBuckets+1 {
				return nil, errors.New("routehealth: stored state exceeds safety bounds")
			}
			if s.Active == nil {
				s.Active = make(map[string]permit)
			}
		}
		s.prune(now, p)
		states[key] = s
	}
	return states, nil
}

func (m *Manager) remoteSnapshot(ctx context.Context, resources []Resource, p Policy) (map[string]*resourceState, error) {
	now := m.now()
	states, err := m.remoteSnapshotAt(ctx, resources, p, now)
	if err == nil {
		m.remember(states, now, p)
	}
	return states, err
}

// remoteSnapshotAt shares the storage decoder without populating the shadow
// cache. Administrative observation must not create state or extend retention.
func (m *Manager) remoteSnapshotAt(ctx context.Context, resources []Resource, p Policy, now time.Time) (map[string]*resourceState, error) {
	ctx, cancel := context.WithTimeout(ctx, redisTimeout)
	defer cancel()
	states := make(map[string]*resourceState, len(resources))
	const batchSize = 256
	for start := 0; start < len(resources); start += batchSize {
		keys := resourceKeys(resources[start:min(start+batchSize, len(resources))])
		values, err := m.client.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, &storageError{err}
		}
		batch, err := decodeStates(keys, values, now, p)
		if err != nil {
			return nil, err
		}
		for key, state := range batch {
			states[key] = state
		}
	}
	return states, nil
}

func (m *Manager) remoteUpdate(ctx context.Context, resources []Resource, p Policy, mutate mutation) error {
	ctx, cancel := context.WithTimeout(ctx, redisTimeout)
	defer cancel()
	keys := resourceKeys(resources)
	for range casAttempts {
		var observed map[string]*resourceState
		var at time.Time
		var callbackFailure error
		err := m.client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.MGet(ctx, keys...).Result()
			if err != nil {
				return &storageError{err}
			}
			at = m.now()
			states, err := decodeStates(keys, values, at, p)
			if err != nil {
				callbackFailure = err
				return err
			}
			changed, err := mutate(states, at)
			if err != nil {
				callbackFailure = err
				return err
			}
			if !changed {
				observed = states
				return nil
			}
			encoded := make([][]byte, len(keys))
			for i, key := range keys {
				s := states[key]
				s.ExpiresAt = at.Add(retention(at, p, s)).UnixNano()
				encoded[i], err = common.Marshal(s)
				if err != nil {
					callbackFailure = fmt.Errorf("routehealth: encode stored state: %w", err)
					return callbackFailure
				}
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				for i, key := range keys {
					pipe.Set(ctx, key, encoded[i], retention(at, p, states[key]))
				}
				return nil
			})
			if errors.Is(err, redis.TxFailedErr) {
				return err
			}
			if err != nil {
				return &storageError{err}
			}
			observed = states
			return nil
		}, keys...)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		if err != nil {
			if callbackFailure != nil {
				return callbackFailure
			}
			var storage *storageError
			if errors.As(err, &storage) {
				return err
			}
			// 包括 WATCH 的 EOF/服务器错误；数据或调用方错误已在上面区分。
			return &storageError{err}
		}
		m.remember(observed, at, p)
		return nil
	}
	return errContention
}

func (m *Manager) remember(states map[string]*resourceState, now time.Time, p Policy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, s := range states {
		if _, exists := m.shadow[key]; !exists && len(m.shadow) >= maxResources {
			m.expire(m.shadow, now)
			if len(m.shadow) >= maxResources {
				// 优先保存真实冷却，而不是挤掉仍有许可的保护状态。
				if !s.Open {
					continue
				}
				for oldKey, old := range m.shadow {
					if !old.Open && len(old.Active) == 0 {
						delete(m.shadow, oldKey)
						break
					}
				}
				if len(m.shadow) >= maxResources {
					continue
				}
			}
		}
		copy := s.clone()
		copy.RemoteSeen = now.UnixNano()
		copy.ExpiresAt = now.Add(retention(now, p, copy)).UnixNano()
		m.shadow[key] = copy
	}
}

func (m *Manager) expire(cache map[string]*resourceState, now time.Time) {
	for key, state := range cache {
		if state.ExpiresAt <= now.UnixNano() {
			delete(cache, key)
		}
	}
}

// localState 只在持有 m.mu 时调用。降级时合并最近观察到的冷却和未过期许可。
func (m *Manager) localState(key string, now time.Time, p Policy) *resourceState {
	s := m.local[key]
	if s != nil && s.ExpiresAt <= now.UnixNano() {
		s = nil
	}
	shadow := m.shadow[key]
	if shadow != nil && shadow.ExpiresAt <= now.UnixNano() {
		shadow = nil
	}
	if s == nil {
		if shadow != nil {
			s = shadow.clone()
		} else {
			s = newState()
		}
	} else {
		s = s.clone()
		if shadow != nil && shadow.RemoteSeen > s.RemoteSeen {
			if shadow.Open && (!s.Open || shadow.OpenedUntil > s.OpenedUntil) {
				active := s.Active
				s = shadow.clone()
				for id, permit := range active {
					if len(s.Active) < maxActiveEntries {
						s.Active[id] = permit
					}
				}
			} else {
				for id, permit := range shadow.Active {
					if len(s.Active) < maxActiveEntries {
						s.Active[id] = permit
					}
				}
				s.RemoteSeen = shadow.RemoteSeen
			}
		}
	}
	s.prune(now, p)
	return s
}

func (m *Manager) localSnapshot(resources []Resource, p Policy) map[string]*resourceState {
	return m.localSnapshotAt(resources, p, m.now())
}

func (m *Manager) localSnapshotAt(resources []Resource, p Policy, now time.Time) map[string]*resourceState {
	m.mu.Lock()
	defer m.mu.Unlock()
	// localState ignores expired entries and clones only requested resources.
	states := make(map[string]*resourceState, len(resources))
	for _, r := range resources {
		key := resourceKey(r)
		states[key] = m.localState(key, now, p)
	}
	return states
}

func (m *Manager) localUpdate(ctx context.Context, resources []Resource, p Policy, mutate mutation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	states := make(map[string]*resourceState, len(resources))
	missing := 0
	for _, r := range resources {
		key := resourceKey(r)
		// Mutation retains the previous lazy cleanup behavior. Read-only
		// snapshots merely ignore expired entries without altering either map.
		if state := m.local[key]; state != nil && state.ExpiresAt <= now.UnixNano() {
			delete(m.local, key)
		}
		if state := m.shadow[key]; state != nil && state.ExpiresAt <= now.UnixNano() {
			delete(m.shadow, key)
		}
		states[key] = m.localState(key, now, p)
		if _, exists := m.local[key]; !exists {
			missing++
		}
	}
	if len(m.local)+missing > maxResources {
		m.expire(m.local, now)
		missing = 0
		for key := range states {
			if _, exists := m.local[key]; !exists {
				missing++
			}
		}
		if len(m.local)+missing > maxResources {
			return errCapacity
		}
	}
	changed, err := mutate(states, now)
	if err != nil || !changed {
		return err
	}
	for key, s := range states {
		s.ExpiresAt = now.Add(retention(now, p, s)).UnixNano()
		m.local[key] = s
	}
	return nil
}

func (l *Lease) update(ctx context.Context, mutate mutation) error {
	if l.remote {
		err := l.manager.remoteUpdate(ctx, l.resources, l.policy, mutate)
		var storage *storageError
		if !errors.As(err, &storage) || ctx.Err() != nil {
			return err
		}
		// 后续操作固定本地；不能把迟到本地结果写进已恢复的共享状态。
		l.remote = false
		l.Degraded = true
	}
	return l.manager.localUpdate(ctx, l.resources, l.policy, mutate)
}
