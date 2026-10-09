// Package routehealth 对上游资源提供有界准入、故障冷却和受控恢复。
package routehealth

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

type Scope string

const (
	ScopeRoute      Scope = "route"
	ScopeCredential Scope = "credential"
	ScopeChannel    Scope = "channel"
)

type Resource struct {
	Key   string
	Scope Scope
}

type Candidate struct {
	ID              string
	Resources       []Resource
	Priority        int64
	Weight          int
	Pool            string
	CredentialOrder int
	Preferred       bool
}

type Policy struct {
	Enabled           bool
	FailureThreshold  int
	FailureWindow     time.Duration
	SampleWindow      time.Duration
	MinimumSamples    int
	FailureRatio      float64
	BaseCooldown      time.Duration
	MaxCooldown       time.Duration
	MaxInflight       int
	LeaseDuration     time.Duration
	RecoverySuccesses int
	ProbeInterval     time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		Enabled: true, FailureThreshold: 3, FailureWindow: 30 * time.Second,
		SampleWindow: 60 * time.Second, MinimumSamples: 20, FailureRatio: 0.5,
		BaseCooldown: 30 * time.Second, MaxCooldown: 5 * time.Minute,
		MaxInflight: 64, LeaseDuration: 30 * time.Second,
		RecoverySuccesses: 3, ProbeInterval: 5 * time.Second,
	}
}

// Validate 对状态大小和时长同时设限，避免错误配置成为无界缓存或租约。
func (p Policy) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.FailureThreshold < 1 || p.FailureThreshold > maxActiveEntries || p.MinimumSamples < 1 || p.MinimumSamples > 1000000 {
		return errors.New("routehealth: invalid failure or sample threshold")
	}
	if math.IsNaN(p.FailureRatio) || math.IsInf(p.FailureRatio, 0) || p.FailureRatio <= 0 || p.FailureRatio > 1 {
		return errors.New("routehealth: failure ratio must be in (0, 1]")
	}
	if p.MaxInflight < 1 || p.MaxInflight > maxActiveEntries/2 || p.RecoverySuccesses < 1 || p.RecoverySuccesses > maxActiveEntries {
		return errors.New("routehealth: invalid inflight or recovery limit")
	}
	for _, duration := range [...]struct {
		name  string
		value time.Duration
	}{
		{"failure window", p.FailureWindow}, {"sample window", p.SampleWindow},
		{"base cooldown", p.BaseCooldown}, {"max cooldown", p.MaxCooldown},
		{"lease duration", p.LeaseDuration}, {"probe interval", p.ProbeInterval},
	} {
		if duration.value <= 0 || duration.value > maxRetryAfter {
			return fmt.Errorf("routehealth: invalid %s", duration.name)
		}
	}
	if p.MaxCooldown < p.BaseCooldown {
		return errors.New("routehealth: max cooldown is smaller than base cooldown")
	}
	return nil
}

type Verdict string

const (
	VerdictSuccess Verdict = "success"
	VerdictFailure Verdict = "failure"
	VerdictIgnored Verdict = "ignored"
)

type Result struct {
	Verdict    Verdict
	Scope      Scope
	RetryAfter time.Duration
	Reason     string
}

// ErrLeaseExpired 表示许可已到期；迟到结果不会成为新的健康样本。
var ErrLeaseExpired = errors.New("routehealth: lease expired")

// Lease 只绑定一次尝试；调用方负责长请求 Renew，所有退出路径都应 Finish。
// Degraded 表示只有本实例的保护，不能据此推断集群探测唯一性。
// 不得与 Renew/Finish 并发读取 Degraded；方法自身支持并发调用。
type Lease struct {
	CandidateID string
	Probe       bool
	Degraded    bool

	mu        sync.Mutex
	manager   *Manager
	id        string
	resources []Resource
	policy    Policy
	remote    bool
	disabled  bool
	finished  bool
}

func (l *Lease) Finish(ctx context.Context, result Result) error {
	if l == nil {
		return errors.New("routehealth: nil lease")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.finished {
		return nil
	}
	if result.Verdict != VerdictSuccess && result.Verdict != VerdictFailure && result.Verdict != VerdictIgnored {
		return errors.New("routehealth: invalid verdict")
	}
	if result.Verdict == VerdictFailure {
		found := false
		for _, r := range l.resources {
			found = found || r.Scope == result.Scope
		}
		if !found && !l.disabled {
			return errors.New("routehealth: failure scope is not a leased resource")
		}
	}
	if l.disabled {
		l.finished = true
		return nil
	}
	// 客户端取消也要释放许可，不把取消本身计为上游失败。
	ctx = context.WithoutCancel(ctx)
	err := l.update(ctx, func(states map[string]*resourceState, now time.Time) (bool, error) {
		for _, r := range l.resources {
			s := states[resourceKey(r)]
			permit, exists := s.Active[l.id]
			if !exists {
				continue
			}
			delete(s.Active, l.id)
			if s.ProbeID == l.id {
				s.ProbeID = ""
			}
			if permit.Generation != s.Generation {
				continue
			}
			s.finish(now, l.policy, permit.Probe, r.Scope, result, l.manager.random())
		}
		return true, nil
	})
	if err == nil {
		l.finished = true
	}
	return err
}

func (l *Lease) Renew(ctx context.Context) error {
	if l == nil {
		return errors.New("routehealth: nil lease")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.finished {
		return ErrLeaseExpired
	}
	if l.disabled {
		return nil
	}
	return l.update(ctx, func(states map[string]*resourceState, now time.Time) (bool, error) {
		for _, r := range l.resources {
			if _, exists := states[resourceKey(r)].Active[l.id]; !exists {
				return false, ErrLeaseExpired
			}
		}
		for _, r := range l.resources {
			s := states[resourceKey(r)]
			permit := s.Active[l.id]
			permit.ExpiresAt = now.Add(l.policy.LeaseDuration).UnixNano()
			s.Active[l.id] = permit
		}
		return true, nil
	})
}

// Snapshot 是诊断快照，不是准入许可。窗口统计使用最多 128 个时间桶。
type Snapshot struct {
	Resource            Resource
	Generation          uint64
	OpenedUntil         time.Time
	NextProbe           time.Time
	Open                bool
	Inflight            int
	Probe               bool
	RecoverySuccesses   int
	ConsecutiveFailures int
	Successes           uint64
	Failures            uint64
	Degraded            bool
}
