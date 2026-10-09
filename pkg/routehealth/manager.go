package routehealth

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// Manager 不创建后台任务。内存和 shadow 都有容量上限与惰性 TTL 清理。
// nil Redis 使用单实例内存；非 nil 时逐资源 CAS 提供跨实例准入。
type Manager struct {
	client   *redis.Client
	mu       sync.Mutex
	local    map[string]*resourceState
	shadow   map[string]*resourceState
	now      func() time.Time
	random   func() float64
	instance string
	sequence atomic.Uint64
}

func New(client *redis.Client) *Manager {
	return &Manager{
		client: client, local: make(map[string]*resourceState), shadow: make(map[string]*resourceState),
		now: time.Now, random: rand.Float64, instance: uuid.NewString(),
	}
}

type rankedCandidate struct {
	index           int
	probe           bool
	due             int64
	score           float64
	credentialScore float64
}

func normalizeCandidates(candidates []Candidate) ([]Candidate, []Resource, error) {
	if len(candidates) > maxCandidates {
		return nil, nil, errors.New("routehealth: too many candidates")
	}
	normalized := make([]Candidate, len(candidates))
	all := make([]Resource, 0, len(candidates)*3)
	seen := make(map[string]bool)
	ids := make(map[string]bool, len(candidates))
	for i, c := range candidates {
		if c.ID == "" || ids[c.ID] || c.Weight < 0 || len(c.Resources) == 0 || len(c.Resources) > maxCandidateResources {
			return nil, nil, errors.New("routehealth: invalid candidate")
		}
		ids[c.ID] = true
		resources := make([]Resource, 0, len(c.Resources))
		for _, r := range c.Resources {
			if err := validateResource(r); err != nil {
				return nil, nil, err
			}
			duplicate := false
			for _, existing := range resources {
				if existing == r {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			resources = append(resources, r)
			key := resourceKey(r)
			if !seen[key] {
				all = append(all, r)
				seen[key] = true
			}
		}
		c.Resources = resources
		normalized[i] = c
	}
	if len(all) > maxResources {
		return nil, nil, errors.New("routehealth: too many candidate resources")
	}
	return normalized, all, nil
}

func validateResource(r Resource) error {
	if r.Key == "" || len(r.Key) > 4096 || (r.Scope != ScopeRoute && r.Scope != ScopeCredential && r.Scope != ScopeChannel) {
		return errors.New("routehealth: invalid resource")
	}
	return nil
}

func shorterWait(a, b time.Duration) time.Duration {
	if a == 0 || (b > 0 && b < a) {
		return b
	}
	return a
}

func (m *Manager) rank(candidates []Candidate, states map[string]*resourceState, p Policy) ([]rankedCandidate, time.Duration) {
	now := m.now()
	ranked := make([]rankedCandidate, 0, len(candidates))
	wait := time.Duration(0)
	poolScores := make(map[string]float64)
	for i, candidate := range candidates {
		available, probe, candidateWait, due := true, false, time.Duration(0), int64(0)
		if p.Enabled {
			available, probe, candidateWait, due = availability(states, candidate.Resources, now, p)
		}
		if !available {
			wait = shorterWait(wait, candidateWait)
			continue
		}
		// 每个渠道只参与一次加权抽签，不因凭证数量改变渠道权重。
		pool := candidate.Pool
		if pool == "" {
			pool = candidate.ID
		}
		score, exists := poolScores[pool]
		if !exists {
			u := min(max(m.random(), 0), math.Nextafter(1, 0))
			score = -math.Log1p(-u)
			if candidate.Weight > 0 {
				score /= float64(candidate.Weight)
			}
			poolScores[pool] = score
		}
		ranked = append(ranked, rankedCandidate{index: i, probe: probe, due: due, score: score, credentialScore: m.random()})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		ca, cb := candidates[a.index], candidates[b.index]
		if a.probe != b.probe {
			return a.probe
		}
		if a.probe {
			if a.due != b.due {
				return a.due < b.due
			}
		} else {
			if ca.Priority != cb.Priority {
				return ca.Priority > cb.Priority
			}
			if ca.Preferred != cb.Preferred {
				return ca.Preferred
			}
		}
		if (ca.Weight > 0) != (cb.Weight > 0) {
			return ca.Weight > 0
		}
		if a.score != b.score {
			return a.score < b.score
		}
		if ca.Pool != cb.Pool {
			return ca.Pool < cb.Pool
		}
		if ca.Pool != "" && ca.CredentialOrder != cb.CredentialOrder {
			return ca.CredentialOrder < cb.CredentialOrder
		}
		return a.credentialScore < b.credentialScore
	})
	return ranked, wait
}

// Acquire 没有可用候选时返回 nil、最早等待、nil；不会强行放开冷却资源。
// 快照仅用于排名，最终逐候选再次原子检查，竞争失败继续其他候选。
func (m *Manager) Acquire(ctx context.Context, candidates []Candidate, p Policy) (*Lease, time.Duration, error) {
	if err := p.Validate(); err != nil {
		return nil, 0, err
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	candidates, resources, err := normalizeCandidates(candidates)
	if err != nil || len(candidates) == 0 {
		return nil, 0, err
	}
	if !p.Enabled {
		ranked, _ := m.rank(candidates, nil, p)
		return &Lease{CandidateID: candidates[ranked[0].index].ID, manager: m, disabled: true}, 0, nil
	}
	remote := m.client != nil
	var states map[string]*resourceState
	if remote {
		states, err = m.remoteSnapshot(ctx, resources, p)
		if err != nil {
			var storage *storageError
			if !errors.As(err, &storage) || ctx.Err() != nil {
				return nil, 0, err
			}
			remote = false
		}
	}
	if !remote {
		states = m.localSnapshot(resources, p)
	}
	// 最多一次从 remote 切到 local；每个后端每个候选至多一次准入尝试。
	for round := range 2 {
		ranked, wait := m.rank(candidates, states, p)
		fallback := false
		for _, choice := range ranked {
			candidate := candidates[choice.index]
			id := m.instance + ":" + strconv.FormatUint(m.sequence.Add(1), 36)
			granted := false
			actualProbe := false
			mutate := func(current map[string]*resourceState, now time.Time) (bool, error) {
				// WATCH 重试必须重算结果，不能沿用失败事务的 granted 标志。
				granted = false
				available, probe, candidateWait, _ := availability(current, candidate.Resources, now, p)
				if !available || probe != choice.probe {
					wait = shorterWait(wait, max(candidateWait, time.Millisecond))
					return false, nil
				}
				for _, r := range candidate.Resources {
					s := current[resourceKey(r)]
					isProbe := s.Open
					s.Active[id] = permit{Generation: s.Generation, ExpiresAt: now.Add(p.LeaseDuration).UnixNano(), Probe: isProbe}
					if isProbe {
						s.ProbeID = id
						s.NextProbe = now.Add(p.ProbeInterval).UnixNano()
					}
				}
				granted, actualProbe = true, probe
				return true, nil
			}
			if remote {
				err = m.remoteUpdate(ctx, candidate.Resources, p, mutate)
			} else {
				err = m.localUpdate(ctx, candidate.Resources, p, mutate)
			}
			if err != nil {
				if errors.Is(err, errContention) || errors.Is(err, errCapacity) {
					wait = shorterWait(wait, p.LeaseDuration)
					continue
				}
				var storage *storageError
				if remote && errors.As(err, &storage) && ctx.Err() == nil {
					remote = false
					states = m.localSnapshot(resources, p)
					fallback = true
					break
				}
				return nil, 0, err
			}
			if granted {
				return &Lease{CandidateID: candidate.ID, Probe: actualProbe, Degraded: m.client != nil && !remote,
					manager: m, id: id, resources: candidate.Resources, policy: p, remote: remote}, 0, nil
			}
		}
		if !fallback || round == 1 {
			return nil, wait, nil
		}
	}
	return nil, 0, nil
}

func (m *Manager) Inspect(ctx context.Context, resource Resource, p Policy) (Snapshot, error) {
	if err := validateResource(resource); err != nil {
		return Snapshot{}, err
	}
	if err := p.Validate(); err != nil {
		return Snapshot{}, err
	}
	if !p.Enabled {
		return Snapshot{Resource: resource}, nil
	}
	resources := []Resource{resource}
	var states map[string]*resourceState
	degraded := false
	if m.client != nil {
		var err error
		states, err = m.remoteSnapshot(ctx, resources, p)
		if err != nil {
			var storage *storageError
			if !errors.As(err, &storage) || ctx.Err() != nil {
				return Snapshot{}, fmt.Errorf("routehealth inspect: %w", err)
			}
			degraded = true
		}
	}
	if states == nil {
		states = m.localSnapshot(resources, p)
	}
	s := states[resourceKey(resource)]
	successes, failures := s.samples()
	snapshot := Snapshot{Resource: resource, Generation: s.Generation, Open: s.Open,
		Inflight: len(s.Active), Probe: s.ProbeID != "", RecoverySuccesses: s.Recovery,
		ConsecutiveFailures: len(s.FailureTimes), Successes: successes, Failures: failures, Degraded: degraded}
	if s.OpenedUntil != 0 {
		snapshot.OpenedUntil = time.Unix(0, s.OpenedUntil)
	}
	if s.NextProbe != 0 {
		snapshot.NextProbe = time.Unix(0, s.NextProbe)
	}
	return snapshot, nil
}
