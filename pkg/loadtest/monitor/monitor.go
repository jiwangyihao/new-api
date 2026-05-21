package monitor

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/QuantumNous/new-api/pkg/loadtest/localguard"
)

type SamplerOptions struct {
	Interval time.Duration
	Process  func() artifact.ProcessSnapshot
	Runtime  func() artifact.RuntimeSnapshot
	Postgres func() artifact.PostgresSnapshot
	Redis    func() artifact.RedisSnapshot
}

type Sampler struct {
	interval time.Duration
	process  func() artifact.ProcessSnapshot
	runtime  func() artifact.RuntimeSnapshot
	postgres func() artifact.PostgresSnapshot
	redis    func() artifact.RedisSnapshot
	done     chan struct{}
	stopped  chan struct{}
	once     sync.Once
	mu       sync.Mutex
	samples  []artifact.ResourceSample
}

func ReadRuntimeSnapshot(ctx context.Context, rawURL string) artifact.RuntimeSnapshot {
	if err := localguard.ValidateURL(rawURL); err != nil {
		return artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "config: " + err.Error()}}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "runtime request: " + err.Error()}}
	}
	client := http.Client{Timeout: time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "runtime GET: " + err.Error()}}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: fmt.Sprintf("runtime status %d", resp.StatusCode)}}
	}
	var snapshot artifact.RuntimeSnapshot
	if err := common.DecodeJson(resp.Body, &snapshot); err != nil {
		return artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "runtime decode: " + err.Error()}}
	}
	if snapshot.Goroutines <= 0 || snapshot.HeapAllocBytes == 0 {
		return artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "runtime response missing required fields"}}
	}
	snapshot.Statused = artifact.Statused{Status: "ok"}
	return snapshot
}

func NewSampler(opts SamplerOptions) *Sampler {
	interval := opts.Interval
	if interval <= 0 {
		interval = time.Second
	}
	return &Sampler{
		interval: interval,
		process:  processSampler(opts.Process),
		runtime:  runtimeSampler(opts.Runtime),
		postgres: postgresSampler(opts.Postgres),
		redis:    redisSampler(opts.Redis),
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

func (s *Sampler) Start() func() []artifact.ResourceSample {
	s.recordSample()
	go func() {
		defer close(s.stopped)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				s.recordSample()
			}
		}
	}()
	return func() []artifact.ResourceSample {
		s.once.Do(func() {
			close(s.done)
			<-s.stopped
		})
		return s.samplesCopy()
	}
}

func Peaks(samples []artifact.ResourceSample) artifact.ResourcePeaks {
	var peaks artifact.ResourcePeaks
	for _, sample := range samples {
		if sample.Process.RSSBytes > peaks.RSSPeakBytes {
			peaks.RSSPeakBytes = sample.Process.RSSBytes
		}
		if sample.Process.CPUPercent > peaks.CPUPercentPeak {
			peaks.CPUPercentPeak = sample.Process.CPUPercent
		}
		if sample.Process.CPUTimeSeconds > peaks.CPUTimeSecondsPeak {
			peaks.CPUTimeSecondsPeak = sample.Process.CPUTimeSeconds
		}
		if sample.Process.ThreadCount > peaks.ThreadCountPeak {
			peaks.ThreadCountPeak = sample.Process.ThreadCount
		}
		if sample.Process.HandleCount > peaks.HandleCountPeak {
			peaks.HandleCountPeak = sample.Process.HandleCount
		}
		if sample.Process.OpenTCPSockets > peaks.OpenTCPSocketsPeak {
			peaks.OpenTCPSocketsPeak = sample.Process.OpenTCPSockets
		}
		if sample.Runtime.Goroutines > peaks.GoroutinesPeak {
			peaks.GoroutinesPeak = sample.Runtime.Goroutines
		}
		if sample.Runtime.HeapAllocBytes > peaks.HeapAllocPeakBytes {
			peaks.HeapAllocPeakBytes = sample.Runtime.HeapAllocBytes
		}
		if sample.Runtime.HeapSysBytes > peaks.HeapSysPeakBytes {
			peaks.HeapSysPeakBytes = sample.Runtime.HeapSysBytes
		}
		if sample.Runtime.GCCount > peaks.GCCountPeak {
			peaks.GCCountPeak = sample.Runtime.GCCount
		}
		if sample.Runtime.PauseTotalNS > peaks.PauseTotalNSPeak {
			peaks.PauseTotalNSPeak = sample.Runtime.PauseTotalNS
		}
		if sample.Runtime.HTTPAcceptTotal > peaks.HTTPAcceptTotalPeak {
			peaks.HTTPAcceptTotalPeak = sample.Runtime.HTTPAcceptTotal
		}
		if sample.Runtime.HTTPActiveCurrent > peaks.HTTPActiveCurrentPeak {
			peaks.HTTPActiveCurrentPeak = sample.Runtime.HTTPActiveCurrent
		}
		if sample.Redis.ConnectedClients > peaks.RedisConnectedClientsPeak {
			peaks.RedisConnectedClientsPeak = sample.Redis.ConnectedClients
		}
		if sample.Redis.UsedMemoryBytes > peaks.RedisUsedMemoryPeakBytes {
			peaks.RedisUsedMemoryPeakBytes = sample.Redis.UsedMemoryBytes
		}
		if sample.Redis.UsedMemoryRSSBytes > peaks.RedisUsedMemoryRSSPeakBytes {
			peaks.RedisUsedMemoryRSSPeakBytes = sample.Redis.UsedMemoryRSSBytes
		}
		if sample.Redis.InstantaneousOpsPerSec > peaks.RedisInstantaneousOpsPeak {
			peaks.RedisInstantaneousOpsPeak = sample.Redis.InstantaneousOpsPerSec
		}
		if sample.Redis.TotalCommandsProcessed > peaks.RedisTotalCommandsProcessedPeak {
			peaks.RedisTotalCommandsProcessedPeak = sample.Redis.TotalCommandsProcessed
		}
		if sample.Postgres.ActiveConnections > peaks.PostgresActiveConnectionsPeak {
			peaks.PostgresActiveConnectionsPeak = sample.Postgres.ActiveConnections
		}
		if sample.Postgres.IdleConnections > peaks.PostgresIdleConnectionsPeak {
			peaks.PostgresIdleConnectionsPeak = sample.Postgres.IdleConnections
		}
		if sample.Postgres.WaitingLocks > peaks.PostgresWaitingLocksPeak {
			peaks.PostgresWaitingLocksPeak = sample.Postgres.WaitingLocks
		}
		if sample.Postgres.DatabaseSizeBytes > peaks.PostgresDatabaseSizePeakBytes {
			peaks.PostgresDatabaseSizePeakBytes = sample.Postgres.DatabaseSizeBytes
		}
	}
	return peaks
}

func (s *Sampler) recordSample() {
	sample := artifact.ResourceSample{
		UnixMilli: time.Now().UnixMilli(),
		Process:   s.process(),
		Runtime:   s.runtime(),
		Postgres:  s.postgres(),
		Redis:     s.redis(),
	}
	s.mu.Lock()
	s.samples = append(s.samples, sample)
	s.mu.Unlock()
}

func (s *Sampler) samplesCopy() []artifact.ResourceSample {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]artifact.ResourceSample, len(s.samples))
	copy(out, s.samples)
	return out
}

func processSampler(fn func() artifact.ProcessSnapshot) func() artifact.ProcessSnapshot {
	if fn != nil {
		return fn
	}
	return func() artifact.ProcessSnapshot {
		return artifact.ProcessSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "process sampler is not configured"}}
	}
}

func runtimeSampler(fn func() artifact.RuntimeSnapshot) func() artifact.RuntimeSnapshot {
	if fn != nil {
		return fn
	}
	return func() artifact.RuntimeSnapshot {
		return artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "runtime sampler is not configured"}}
	}
}

func postgresSampler(fn func() artifact.PostgresSnapshot) func() artifact.PostgresSnapshot {
	if fn != nil {
		return fn
	}
	return func() artifact.PostgresSnapshot {
		return artifact.PostgresSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "postgres sampler is not configured"}}
	}
}

func redisSampler(fn func() artifact.RedisSnapshot) func() artifact.RedisSnapshot {
	if fn != nil {
		return fn
	}
	return func() artifact.RedisSnapshot {
		return artifact.RedisSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "redis sampler is not configured"}}
	}
}
