package routehealth

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testContext = context.Background()

type healthFixture struct {
	manager *Manager
	other   *Manager
	server  *miniredis.Miniredis
	clock   atomic.Int64
}

func healthStores(t *testing.T, run func(*testing.T, *healthFixture)) {
	t.Helper()
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			f := &healthFixture{}
			f.clock.Store(time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC).UnixNano())
			if backend == "redis" {
				f.server = miniredis.RunT(t)
				f.manager = New(testRedisClient(t, f.server))
				f.other = New(testRedisClient(t, f.server))
			} else {
				f.manager = New(nil)
				f.other = f.manager
			}
			for _, m := range []*Manager{f.manager, f.other} {
				m.now = func() time.Time { return time.Unix(0, f.clock.Load()) }
				m.random = func() float64 { return 0.5 }
			}
			run(t, f)
		})
	}
}

func testRedisClient(t *testing.T, server *miniredis.Miniredis) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1,
		DialTimeout: 100 * time.Millisecond, ReadTimeout: 100 * time.Millisecond, WriteTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func (f *healthFixture) advance(delta time.Duration) {
	f.clock.Add(int64(delta))
	if f.server != nil {
		f.server.FastForward(delta)
	}
}

func testCandidate(id, channel, model, credential string) Candidate {
	prefix := "{" + channel + "}:"
	return Candidate{ID: id, Weight: 1, Resources: []Resource{
		{Key: prefix + model + ":chat", Scope: ScopeRoute},
		{Key: prefix + credential, Scope: ScopeCredential},
		{Key: prefix + "service", Scope: ScopeChannel},
	}}
}

func take(t *testing.T, m *Manager, p Policy, candidates ...Candidate) *Lease {
	t.Helper()
	lease, wait, err := m.Acquire(testContext, candidates, p)
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Zero(t, wait)
	return lease
}

func unavailable(t *testing.T, m *Manager, p Policy, candidates ...Candidate) time.Duration {
	t.Helper()
	lease, wait, err := m.Acquire(testContext, candidates, p)
	require.NoError(t, err)
	require.Nil(t, lease)
	require.Positive(t, wait)
	return wait
}

func snapshot(t *testing.T, m *Manager, p Policy, r Resource) Snapshot {
	t.Helper()
	s, err := m.Inspect(testContext, r, p)
	require.NoError(t, err)
	return s
}

func finish(t *testing.T, lease *Lease, verdict Verdict, scope Scope) {
	t.Helper()
	require.NoError(t, lease.Finish(testContext, Result{Verdict: verdict, Scope: scope}))
}

func TestPolicyDefaultsAndValidation(t *testing.T) {
	p := DefaultPolicy()
	require.NoError(t, p.Validate())
	for _, mutate := range []func(*Policy){
		func(p *Policy) { p.FailureThreshold = 0 },
		func(p *Policy) { p.FailureRatio = math.NaN() },
		func(p *Policy) { p.FailureRatio = 1.1 },
		func(p *Policy) { p.MaxInflight = 0 },
		func(p *Policy) { p.MaxInflight = math.MaxInt },
		func(p *Policy) { p.SampleWindow = 0 },
		func(p *Policy) { p.MaxCooldown = time.Second },
		func(p *Policy) { p.RecoverySuccesses = 0 },
		func(p *Policy) { p.LeaseDuration = 25 * time.Hour },
	} {
		invalid := p
		mutate(&invalid)
		_, _, err := New(nil).Acquire(testContext, []Candidate{testCandidate("a", "c", "m", "k")}, invalid)
		require.Error(t, err)
	}
}

func TestRouteCooldownSharedAndModelIsolation(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		bad := testCandidate("group-a", "channel-a", "bad-model", "key")
		for range p.FailureThreshold {
			finish(t, take(t, f.manager, p, bad), VerdictFailure, ScopeRoute)
		}
		shared := bad
		shared.ID = "group-b-alias"
		assert.Equal(t, p.BaseCooldown, unavailable(t, f.other, p, shared))
		good := testCandidate("good", "channel-a", "good-model", "key")
		lease := take(t, f.other, p, bad, good)
		assert.Equal(t, good.ID, lease.CandidateID)
		finish(t, lease, VerdictSuccess, "")
		assert.True(t, snapshot(t, f.other, p, bad.Resources[0]).Open)
		assert.False(t, snapshot(t, f.manager, p, good.Resources[0]).Open)
		assert.False(t, snapshot(t, f.manager, p, bad.Resources[1]).Open)
		assert.False(t, snapshot(t, f.manager, p, bad.Resources[2]).Open)
	})
}

func TestCredentialAndChannelIsolation(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		p.FailureThreshold = 1
		one := testCandidate("one", "channel", "model", "key-one")
		two := testCandidate("two", "channel", "model", "key-two")
		finish(t, take(t, f.manager, p, one), VerdictFailure, ScopeCredential)
		unavailable(t, f.other, p, one)
		lease := take(t, f.other, p, one, two)
		assert.Equal(t, two.ID, lease.CandidateID)
		finish(t, lease, VerdictSuccess, "")
		assert.False(t, snapshot(t, f.manager, p, one.Resources[0]).Open)
		finish(t, take(t, f.manager, p, two), VerdictFailure, ScopeChannel)
		unavailable(t, f.other, p, two)
		sibling := testCandidate("sibling", "channel", "other-model", "third-key")
		unavailable(t, f.other, p, sibling)
		separate := testCandidate("separate", "another-channel", "model", "key-one")
		finish(t, take(t, f.other, p, separate), VerdictSuccess, "")
	})
}

func TestIgnoredIdempotentAndLateResults(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		p.FailureThreshold = 1
		c := testCandidate("a", "channel", "model", "key")
		ignored := take(t, f.manager, p, c)
		cancelled, cancel := context.WithCancel(testContext)
		cancel()
		require.NoError(t, ignored.Finish(cancelled, Result{Verdict: VerdictIgnored}))
		s := snapshot(t, f.other, p, c.Resources[0])
		assert.Zero(t, s.Successes)
		assert.Zero(t, s.Failures)
		assert.Zero(t, s.Inflight)
		lateSuccess := take(t, f.manager, p, c)
		lateFailure := take(t, f.other, p, c)
		failed := take(t, f.manager, p, c)
		finish(t, failed, VerdictFailure, ScopeRoute)
		finish(t, failed, VerdictFailure, ScopeRoute)
		opened := snapshot(t, f.other, p, c.Resources[0])
		finish(t, lateSuccess, VerdictSuccess, "")
		finish(t, lateFailure, VerdictFailure, ScopeRoute)
		s = snapshot(t, f.other, p, c.Resources[0])
		assert.True(t, s.Open)
		assert.Equal(t, opened.Generation, s.Generation)
		assert.Equal(t, opened.OpenedUntil, s.OpenedUntil)
		assert.Equal(t, uint64(1), s.Failures)
		assert.Zero(t, s.Successes)
		for _, r := range c.Resources {
			assert.Zero(t, snapshot(t, f.other, p, r).Inflight)
		}
	})
}

func TestFailureWindowsAndRatio(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		c := testCandidate("a", "channel", "model", "key")
		finish(t, take(t, f.manager, p, c), VerdictFailure, ScopeRoute)
		f.advance(p.FailureWindow + time.Nanosecond)
		finish(t, take(t, f.manager, p, c), VerdictFailure, ScopeRoute)
		finish(t, take(t, f.manager, p, c), VerdictFailure, ScopeRoute)
		assert.False(t, snapshot(t, f.other, p, c.Resources[0]).Open)
		assert.Equal(t, 2, snapshot(t, f.other, p, c.Resources[0]).ConsecutiveFailures)
		finish(t, take(t, f.manager, p, c), VerdictSuccess, "")
		assert.Zero(t, snapshot(t, f.other, p, c.Resources[0]).ConsecutiveFailures)
		p.FailureThreshold = 100
		p.MinimumSamples = 4
		p.FailureRatio = 0.5
		ratio := testCandidate("ratio", "ratio-channel", "model", "key")
		finish(t, take(t, f.manager, p, ratio), VerdictFailure, ScopeRoute)
		finish(t, take(t, f.manager, p, ratio), VerdictSuccess, "")
		finish(t, take(t, f.manager, p, ratio), VerdictFailure, ScopeRoute)
		assert.False(t, snapshot(t, f.other, p, ratio.Resources[0]).Open)
		finish(t, take(t, f.manager, p, ratio), VerdictFailure, ScopeRoute)
		assert.True(t, snapshot(t, f.other, p, ratio.Resources[0]).Open)
		// 样本完整离开滑窗后不参与失败率。
		window := testCandidate("window", "window-channel", "model", "key")
		for range 3 {
			finish(t, take(t, f.manager, p, window), VerdictFailure, ScopeRoute)
		}
		f.advance(p.SampleWindow + time.Second)
		finish(t, take(t, f.manager, p, window), VerdictFailure, ScopeRoute)
		s := snapshot(t, f.other, p, window.Resources[0])
		assert.False(t, s.Open)
		assert.Equal(t, uint64(1), s.Failures)
	})
}

func TestConcurrentSingleProbe(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		p.FailureThreshold = 1
		c := testCandidate("a", "channel", "model", "key")
		finish(t, take(t, f.manager, p, c), VerdictFailure, ScopeRoute)
		f.advance(p.BaseCooldown)
		const workers = 12
		start := make(chan struct{})
		type acquired struct {
			lease *Lease
			wait  time.Duration
			err   error
		}
		results := make(chan acquired, workers)
		var ready, done sync.WaitGroup
		ready.Add(workers)
		done.Add(workers)
		for i := range workers {
			go func() {
				defer done.Done()
				m := f.manager
				if i%2 != 0 {
					m = f.other
				}
				ready.Done()
				<-start
				lease, wait, err := m.Acquire(testContext, []Candidate{c}, p)
				results <- acquired{lease, wait, err}
			}()
		}
		ready.Wait()
		close(start)
		done.Wait()
		close(results)
		var winner *Lease
		for r := range results {
			require.NoError(t, r.err)
			if r.lease != nil {
				require.Nil(t, winner, "only one instance may acquire a recovery probe")
				winner = r.lease
				assert.True(t, winner.Probe)
				assert.False(t, winner.Degraded)
			} else {
				assert.Positive(t, r.wait)
			}
		}
		require.NotNil(t, winner)
		finish(t, winner, VerdictIgnored, "")
		assert.Zero(t, snapshot(t, f.other, p, c.Resources[0]).RecoverySuccesses)
		assert.Equal(t, p.ProbeInterval, unavailable(t, f.other, p, c))
	})
}

func TestLeaseRenewExpiryAndAbandonedProbe(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		p.MaxInflight = 1
		p.FailureThreshold = 1
		c := testCandidate("a", "channel", "model", "key")
		old := take(t, f.manager, p, c)
		assert.Equal(t, p.LeaseDuration, unavailable(t, f.other, p, c))
		f.advance(p.LeaseDuration - time.Second)
		require.NoError(t, old.Renew(testContext))
		f.advance(2 * time.Second)
		assert.Equal(t, p.LeaseDuration-2*time.Second, unavailable(t, f.other, p, c))
		f.advance(p.LeaseDuration - 2*time.Second)
		fresh := take(t, f.other, p, c)
		require.ErrorIs(t, old.Renew(testContext), ErrLeaseExpired)
		finish(t, old, VerdictFailure, ScopeRoute)
		assert.False(t, snapshot(t, f.other, p, c.Resources[0]).Open)
		finish(t, fresh, VerdictFailure, ScopeRoute)
		f.advance(p.BaseCooldown)
		abandoned := take(t, f.manager, p, c)
		assert.True(t, abandoned.Probe)
		f.advance(p.LeaseDuration)
		probe := take(t, f.other, p, c)
		assert.True(t, probe.Probe)
		finish(t, abandoned, VerdictSuccess, "")
		assert.Zero(t, snapshot(t, f.other, p, c.Resources[0]).RecoverySuccesses)
		finish(t, probe, VerdictSuccess, "")
		assert.Equal(t, 1, snapshot(t, f.other, p, c.Resources[0]).RecoverySuccesses)
	})
}

func TestGradualRecoveryAndBackoff(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		p.FailureThreshold = 1
		c := testCandidate("a", "channel", "model", "key")
		finish(t, take(t, f.manager, p, c), VerdictFailure, ScopeRoute)
		f.advance(p.BaseCooldown)
		finish(t, take(t, f.other, p, c), VerdictFailure, ScopeRoute)
		assert.Equal(t, 2*p.BaseCooldown, unavailable(t, f.manager, p, c))
		f.advance(2 * p.BaseCooldown)
		for i := range p.RecoverySuccesses {
			probe := take(t, f.other, p, c)
			assert.True(t, probe.Probe)
			finish(t, probe, VerdictSuccess, "")
			s := snapshot(t, f.manager, p, c.Resources[0])
			if i+1 < p.RecoverySuccesses {
				assert.True(t, s.Open)
				assert.Equal(t, i+1, s.RecoverySuccesses)
				assert.Equal(t, p.ProbeInterval, unavailable(t, f.manager, p, c))
				f.advance(p.ProbeInterval)
			} else {
				assert.False(t, s.Open)
			}
		}
		regular := take(t, f.manager, p, c)
		assert.False(t, regular.Probe)
		finish(t, regular, VerdictFailure, ScopeRoute)
		assert.Equal(t, p.BaseCooldown, unavailable(t, f.other, p, c))
	})
}

func TestEarliestCooldownAndRetryAfter(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		p.FailureThreshold = 1
		one := testCandidate("one", "channel-one", "model", "key")
		two := testCandidate("two", "channel-two", "model", "key")
		finish(t, take(t, f.manager, p, one), VerdictFailure, ScopeRoute)
		require.NoError(t, take(t, f.manager, p, two).Finish(testContext,
			Result{Verdict: VerdictFailure, Scope: ScopeRoute, RetryAfter: 10 * time.Minute}))
		assert.Equal(t, p.BaseCooldown, unavailable(t, f.other, p, one, two))
		assert.Equal(t, 10*time.Minute, unavailable(t, f.other, p, two))
		f.advance(10 * time.Minute)
		probe := take(t, f.other, p, two)
		require.NoError(t, probe.Finish(testContext,
			Result{Verdict: VerdictFailure, Scope: ScopeRoute, RetryAfter: 100 * time.Hour}))
		assert.Equal(t, 24*time.Hour, unavailable(t, f.manager, p, two))
	})
}

func TestPriorityPreferenceLowPriorityProbeAndZeroWeights(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		p.FailureThreshold = 1
		high := testCandidate("high", "high-channel", "model", "key")
		high.Priority = 100
		low := testCandidate("low", "low-channel", "model", "key")
		low.Preferred = true
		lease := take(t, f.manager, p, low, high)
		assert.Equal(t, high.ID, lease.CandidateID)
		finish(t, lease, VerdictSuccess, "")
		peer := testCandidate("peer", "peer-channel", "model", "key")
		peer.Priority, peer.Preferred = high.Priority, true
		lease = take(t, f.manager, p, high, peer)
		assert.Equal(t, peer.ID, lease.CandidateID)
		finish(t, lease, VerdictSuccess, "")
		finish(t, take(t, f.manager, p, low), VerdictFailure, ScopeRoute)
		lease = take(t, f.other, p, low, high)
		assert.Equal(t, high.ID, lease.CandidateID)
		finish(t, lease, VerdictSuccess, "")
		f.advance(p.BaseCooldown)
		lease = take(t, f.other, p, high, low)
		assert.Equal(t, low.ID, lease.CandidateID)
		assert.True(t, lease.Probe)
		finish(t, lease, VerdictSuccess, "")
		lease = take(t, f.manager, p, low, high)
		assert.Equal(t, high.ID, lease.CandidateID)
		finish(t, lease, VerdictSuccess, "")
		high.Weight, peer.Weight, peer.Preferred = 0, 0, false
		lease = take(t, f.manager, p, high, peer)
		assert.Contains(t, []string{high.ID, peer.ID}, lease.CandidateID)
		finish(t, lease, VerdictSuccess, "")
		high.Weight, peer.Weight = math.MaxInt, math.MaxInt
		finish(t, take(t, f.manager, p, high, peer), VerdictSuccess, "")
	})
}

func TestDisabledPolicyAndWeightedSelection(t *testing.T) {
	m := New(nil)
	one := testCandidate("one", "one", "model", "key")
	two := testCandidate("two", "two", "model", "key")
	one.Weight, two.Weight = 1, 100
	values := []float64{0.5, 0.5}
	i := 0
	m.random = func() float64 { v := values[i%len(values)]; i++; return v }
	lease := take(t, m, Policy{}, one, two)
	assert.Equal(t, two.ID, lease.CandidateID)
	finish(t, lease, VerdictFailure, ScopeRoute)
	require.ErrorIs(t, lease.Renew(testContext), ErrLeaseExpired, "a finished attempt cannot retain admission")
	assert.False(t, snapshot(t, m, DefaultPolicy(), two.Resources[0]).Open)
	two.Priority = -1
	lease = take(t, m, Policy{}, one, two)
	assert.Equal(t, one.ID, lease.CandidateID)
	finish(t, lease, VerdictIgnored, "")
}

func TestRedisDisconnectedPreservesLocalProtection(t *testing.T) {
	server := miniredis.RunT(t)
	m := New(testRedisClient(t, server))
	at := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return at }
	m.random = func() float64 { return 0.5 }
	p := DefaultPolicy()
	p.FailureThreshold = 1
	cold := testCandidate("cold", "cold", "model", "key")
	finish(t, take(t, m, p, cold), VerdictFailure, ScopeRoute)
	server.Close()
	assert.Equal(t, p.BaseCooldown, unavailable(t, m, p, cold))
	assert.True(t, snapshot(t, m, p, cold.Resources[0]).Degraded)
	unknown := testCandidate("unknown", "unknown", "model", "key")
	lease := take(t, m, p, unknown)
	assert.True(t, lease.Degraded)
	finish(t, lease, VerdictFailure, ScopeRoute)
	unavailable(t, m, p, unknown)
}

func TestDegradedLeaseNeverWritesRecoveredRedis(t *testing.T) {
	server := miniredis.RunT(t)
	client := testRedisClient(t, server)
	m := New(client)
	m.random = func() float64 { return 0.5 }
	p := DefaultPolicy()
	p.FailureThreshold = 1
	c := testCandidate("a", "channel", "model", "key")
	lease := take(t, m, p, c)
	require.NoError(t, client.Close())
	// 更新失败后，这张许可永久归属本地；新连接恢复后也不能写回。
	require.NoError(t, lease.Renew(testContext))
	assert.True(t, lease.Degraded)
	m.client = testRedisClient(t, server)
	fresh := take(t, m, p, c)
	finish(t, fresh, VerdictSuccess, "")
	finish(t, lease, VerdictFailure, ScopeRoute)
	s := snapshot(t, m, p, c.Resources[0])
	assert.False(t, s.Open)
	assert.Equal(t, uint64(1), s.Successes)
	assert.Zero(t, s.Failures)
}

func TestIdleStateTTLAndInvalidInputs(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		c := testCandidate("a", "channel", "model", "key")
		finish(t, take(t, f.manager, p, c), VerdictSuccess, "")
		f.advance(2*p.MaxCooldown + time.Second)
		assert.Zero(t, snapshot(t, f.other, p, c.Resources[0]).Successes)
		invalid := c
		invalid.Weight = -1
		_, _, err := f.manager.Acquire(testContext, []Candidate{invalid}, p)
		require.Error(t, err)
		_, _, err = f.manager.Acquire(testContext, []Candidate{c, c}, p)
		require.Error(t, err)
		lease, wait, err := f.manager.Acquire(testContext, nil, p)
		require.NoError(t, err)
		assert.Nil(t, lease)
		assert.Zero(t, wait)
		cancelled, cancel := context.WithCancel(testContext)
		cancel()
		_, _, err = f.manager.Acquire(cancelled, []Candidate{c}, p)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestInspectManyIsReadOnlyAndPreservesProbe(t *testing.T) {
	healthStores(t, func(t *testing.T, f *healthFixture) {
		p := DefaultPolicy()
		p.FailureThreshold = 1
		candidate := testCandidate("observed", "channel", "model", "credential")
		resources := append(append([]Resource{}, candidate.Resources...), candidate.Resources[0])
		initial, err := f.manager.InspectMany(testContext, resources, p)
		require.NoError(t, err)
		require.Len(t, initial, 4)
		assert.Empty(t, f.manager.local)
		assert.Empty(t, f.manager.shadow)
		if f.server != nil {
			assert.Empty(t, f.server.Keys())
		}
		finish(t, take(t, f.manager, p, candidate), VerdictFailure, ScopeRoute)
		f.advance(p.BaseCooldown + time.Second)
		beforeLocal := make(map[string]*resourceState)
		beforeShadow := make(map[string]*resourceState)
		for key, state := range f.manager.local {
			beforeLocal[key] = state.clone()
		}
		for key, state := range f.manager.shadow {
			beforeShadow[key] = state.clone()
		}
		beforeRedis := make(map[string]string)
		if f.server != nil {
			for _, key := range f.server.Keys() {
				beforeRedis[key], err = f.server.Get(key)
				require.NoError(t, err)
			}
		}
		batch, err := f.manager.InspectMany(testContext, resources, p)
		require.NoError(t, err)
		for i, resource := range resources {
			assert.Equal(t, snapshot(t, f.manager, p, resource), batch[i])
		}
		assert.Equal(t, beforeLocal, f.manager.local)
		assert.Equal(t, beforeShadow, f.manager.shadow)
		if f.server != nil {
			assert.Len(t, f.server.Keys(), len(beforeRedis))
			for key, value := range beforeRedis {
				actual, getErr := f.server.Get(key)
				require.NoError(t, getErr)
				assert.Equal(t, value, actual)
			}
		}
		assert.True(t, batch[0].Open)
		assert.False(t, batch[0].Probe)
		probe := take(t, f.manager, p, candidate)
		assert.True(t, probe.Probe, "inspection must not consume the due recovery permit")
		finish(t, probe, VerdictSuccess, "")
	})
}

func TestInspectManyBatchesWithoutInventoryTruncation(t *testing.T) {
	server := miniredis.RunT(t)
	m := New(testRedisClient(t, server))
	resources := make([]Resource, maxResources+1)
	for i := range resources {
		resources[i] = Resource{Key: fmt.Sprintf("inventory:%d", i), Scope: ScopeRoute}
	}
	before := server.CommandCount()
	got, err := m.InspectMany(testContext, append(resources, resources[0]), DefaultPolicy())
	require.NoError(t, err)
	require.Len(t, got, len(resources)+1)
	assert.Equal(t, got[0], got[len(resources)])
	assert.LessOrEqual(t, server.CommandCount()-before, (len(resources)+255)/256+4)
	assert.Empty(t, server.Keys())
	assert.Empty(t, m.shadow)
	assert.Empty(t, m.local)
}

func TestInspectManyRedisFailureUsesLocalProtection(t *testing.T) {
	server := miniredis.RunT(t)
	m := New(testRedisClient(t, server))
	p := DefaultPolicy()
	p.FailureThreshold = 1
	candidate := testCandidate("fallback", "channel", "model", "key")
	finish(t, take(t, m, p, candidate), VerdictFailure, ScopeRoute)
	server.Close()
	got, err := m.InspectMany(testContext, candidate.Resources, p)
	require.NoError(t, err)
	for _, snapshot := range got {
		assert.True(t, snapshot.Degraded)
	}
	assert.True(t, got[0].Open)
	assert.Equal(t, uint64(1), got[0].Failures)
	cancelled, cancel := context.WithCancel(testContext)
	cancel()
	_, err = m.InspectMany(cancelled, candidate.Resources, p)
	require.ErrorIs(t, err, context.Canceled)
}
