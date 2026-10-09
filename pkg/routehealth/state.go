package routehealth

import (
	"math"
	"time"
)

const (
	maxResources          = 8192
	maxActiveEntries      = 8192
	maxCandidates         = 4096
	maxCandidateResources = 64
	maxSampleBuckets      = 128
	maxRetryAfter         = 24 * time.Hour
	redisTimeout          = 2 * time.Second
	casAttempts           = 4
)

type permit struct {
	Generation uint64
	ExpiresAt  int64
	Probe      bool
}

type sampleBucket struct {
	Start     int64
	Successes uint64
	Failures  uint64
}

type resourceState struct {
	Generation   uint64
	Open         bool
	OpenedUntil  int64
	NextProbe    int64
	Backoff      uint32
	Recovery     int
	ProbeID      string
	Active       map[string]permit
	FailureTimes []int64
	Buckets      []sampleBucket
	BucketWidth  int64
	ExpiresAt    int64
	RemoteSeen   int64
}

func newState() *resourceState {
	return &resourceState{Generation: 1, Active: make(map[string]permit)}
}

func (s *resourceState) clone() *resourceState {
	copy := *s
	copy.Active = make(map[string]permit, len(s.Active))
	for id, active := range s.Active {
		copy.Active[id] = active
	}
	copy.FailureTimes = append([]int64(nil), s.FailureTimes...)
	copy.Buckets = append([]sampleBucket(nil), s.Buckets...)
	return &copy
}

func (s *resourceState) prune(now time.Time, p Policy) {
	ns := now.UnixNano()
	for id, active := range s.Active {
		if active.ExpiresAt <= ns {
			delete(s.Active, id)
		}
	}
	if _, exists := s.Active[s.ProbeID]; !exists {
		s.ProbeID = ""
	}
	cut := ns - int64(p.FailureWindow)
	n := 0
	for _, at := range s.FailureTimes {
		if at > cut {
			s.FailureTimes[n] = at
			n++
		}
	}
	s.FailureTimes = s.FailureTimes[:n]
	if n > p.FailureThreshold {
		s.FailureTimes = s.FailureTimes[n-p.FailureThreshold:]
	}
	width := (int64(p.SampleWindow) + maxSampleBuckets - 1) / maxSampleBuckets
	if width != s.BucketWidth {
		s.Buckets = nil
		s.BucketWidth = width
	}
	// 丢弃已完整离开滑窗的桶；边界误差不超过一个桶宽。
	cut = ns - int64(p.SampleWindow)
	n = 0
	for _, bucket := range s.Buckets {
		if bucket.Start+s.BucketWidth > cut {
			s.Buckets[n] = bucket
			n++
		}
	}
	s.Buckets = s.Buckets[:n]
}

func (s *resourceState) addSample(now time.Time, failure bool) {
	start := now.UnixNano() / s.BucketWidth * s.BucketWidth
	idx := len(s.Buckets) - 1
	if idx < 0 || s.Buckets[idx].Start != start {
		// 窗口可能同时覆盖首尾两个部分桶，因此最多保留 129 个。
		if len(s.Buckets) >= maxSampleBuckets+1 {
			copy(s.Buckets, s.Buckets[1:])
			s.Buckets = s.Buckets[:maxSampleBuckets]
		}
		s.Buckets = append(s.Buckets, sampleBucket{Start: start})
		idx = len(s.Buckets) - 1
	}
	if failure {
		if s.Buckets[idx].Failures < math.MaxUint64/4 {
			s.Buckets[idx].Failures++
		}
	} else if s.Buckets[idx].Successes < math.MaxUint64/4 {
		s.Buckets[idx].Successes++
	}
}

func (s *resourceState) samples() (successes, failures uint64) {
	for _, bucket := range s.Buckets {
		// 饱和计数防止极端吞吐下整数回绕。
		successes = saturatedAdd(successes, bucket.Successes)
		failures = saturatedAdd(failures, bucket.Failures)
	}
	return
}

func saturatedAdd(a, b uint64) uint64 {
	if math.MaxUint64-a < b {
		return math.MaxUint64
	}
	return a + b
}

func (s *resourceState) finish(now time.Time, p Policy, probe bool, scope Scope, result Result, random float64) {
	if result.Verdict == VerdictIgnored || (result.Verdict == VerdictFailure && scope != result.Scope) {
		return
	}
	if result.Verdict == VerdictSuccess {
		s.FailureTimes = s.FailureTimes[:0]
		s.addSample(now, false)
		if probe && s.Open {
			s.Recovery++
			if s.Recovery >= p.RecoverySuccesses {
				s.Open = false
				s.OpenedUntil = 0
				s.NextProbe = 0
				s.Backoff = 0
				s.Recovery = 0
				s.Generation++
				s.Buckets = s.Buckets[:0]
				s.addSample(now, false)
			}
		}
		return
	}
	s.addSample(now, true)
	if len(s.FailureTimes) >= p.FailureThreshold {
		copy(s.FailureTimes, s.FailureTimes[1:])
		s.FailureTimes = s.FailureTimes[:p.FailureThreshold-1]
	}
	s.FailureTimes = append(s.FailureTimes, now.UnixNano())
	successes, failures := s.samples()
	samples := saturatedAdd(successes, failures)
	ratio := float64(failures) / (float64(successes) + float64(failures))
	if probe || len(s.FailureTimes) >= p.FailureThreshold || (samples >= uint64(p.MinimumSamples) && ratio >= p.FailureRatio) || result.RetryAfter > 0 {
		s.open(now, p, result.RetryAfter, random)
	}
}

func (s *resourceState) open(now time.Time, p Policy, retryAfter time.Duration, random float64) {
	cooldown := p.BaseCooldown
	for i := uint32(0); i < s.Backoff && cooldown < p.MaxCooldown; i++ {
		if cooldown > p.MaxCooldown/2 {
			cooldown = p.MaxCooldown
		} else {
			cooldown *= 2
		}
	}
	cooldown = min(time.Duration(float64(cooldown)*(0.9+0.2*random)), p.MaxCooldown)
	// Retry-After 是等待下限，不受普通退避上限截短；异常提示最多 24h。
	cooldown = max(cooldown, min(retryAfter, maxRetryAfter))
	s.Open = true
	s.OpenedUntil = max(s.OpenedUntil, now.Add(cooldown).UnixNano())
	s.NextProbe = s.OpenedUntil
	s.Recovery = 0
	s.ProbeID = ""
	s.Generation++
	if s.Backoff < 32 {
		s.Backoff++
	}
}

// availability 同时考虑冷却、单探测和正常并发；多个资源的等待取最大值。
func availability(states map[string]*resourceState, resources []Resource, now time.Time, p Policy) (bool, bool, time.Duration, int64) {
	ns := now.UnixNano()
	wait := time.Duration(0)
	probe := false
	due := int64(math.MaxInt64)
	for _, r := range resources {
		s := states[resourceKey(r)]
		if s.Open {
			probe = true
			due = min(due, max(s.OpenedUntil, s.NextProbe))
			blockedUntil := max(s.OpenedUntil, s.NextProbe)
			if active, exists := s.Active[s.ProbeID]; exists {
				blockedUntil = max(blockedUntil, active.ExpiresAt)
			}
			if blockedUntil > ns {
				wait = max(wait, time.Duration(blockedUntil-ns))
			}
		}
		if len(s.Active) >= p.MaxInflight {
			earliest := int64(math.MaxInt64)
			for _, active := range s.Active {
				earliest = min(earliest, active.ExpiresAt)
			}
			wait = max(wait, time.Duration(earliest-ns))
		}
	}
	return wait == 0, probe, wait, due
}

func retention(now time.Time, p Policy, s *resourceState) time.Duration {
	ttl := 2 * max(p.SampleWindow, p.FailureWindow, p.MaxCooldown, p.LeaseDuration, p.ProbeInterval)
	if s.OpenedUntil > now.UnixNano() {
		ttl = max(ttl, time.Duration(s.OpenedUntil-now.UnixNano())+p.LeaseDuration+p.SampleWindow)
	}
	return ttl
}
