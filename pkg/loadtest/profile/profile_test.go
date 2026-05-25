package profile

import (
	"strings"
	"testing"
	"time"
)

func TestBenchmarkProfileMatchesWorkbenchMatrix(t *testing.T) {
	p := Benchmark()
	wantPoints := []int{250, 500, 750, 1000, 1250, 1500, 1750, 2000}
	if len(p.Points) != len(wantPoints) {
		t.Fatalf("points len = %d", len(p.Points))
	}
	for i := range wantPoints {
		if p.Points[i] != wantPoints[i] {
			t.Fatalf("point[%d] = %d want %d", i, p.Points[i], wantPoints[i])
		}
	}
	maxPoint := wantPoints[len(wantPoints)-1]
	if p.RequestsPerPoint != 3000 || p.RampStep != 125 || p.RampInterval != 200*time.Millisecond || p.Duration != 75*time.Second || p.Timeout != 120*time.Second {
		t.Fatalf("benchmark timings mismatch: %#v", p)
	}
	minRequiredRampStep := maxPoint / 16
	if p.RampStep < minRequiredRampStep {
		t.Fatalf("benchmark ramp too slow for full matrix: ramp_step=%d min=%d max_point=%d", p.RampStep, minRequiredRampStep, maxPoint)
	}
	if p.Transport.Mode != TransportH1KeepAlive {
		t.Fatalf("transport mode = %q", p.Transport.Mode)
	}
	if p.Transport.MaxConnsPerHost != maxPoint || p.Transport.MaxIdleConns != maxPoint || p.Transport.MaxIdleConnsPerHost != maxPoint {
		t.Fatalf("benchmark transport limits must drive full matrix max point %d: %#v", maxPoint, p.Transport)
	}
	if p.Relay.MaxIdleConns != 1024 || p.Relay.MaxIdleConnsPerHost != 1024 {
		t.Fatalf("benchmark relay limits mismatch: %#v", p.Relay)
	}
	if p.ServerLimits.GOMAXPROCS != "2" || p.ServerLimits.GOGC != "100" || p.ServerLimits.GOMEMLIMIT != "384MiB" || p.ServerLimits.ProcessMemoryLimitBytes != 512*1024*1024 || p.ServerLimits.CPUAffinityCores != 2 {
		t.Fatalf("server limits mismatch: %#v", p.ServerLimits)
	}
	if p.ServerLimits.SQLMaxOpenConns != "64" || p.ServerLimits.SQLMaxIdleConns != "64" {
		t.Fatalf("benchmark SQL pool must reuse a bounded connection set: %#v", p.ServerLimits)
	}
	if p.ServerLimits.RedisPoolSize != "256" {
		t.Fatalf("benchmark Redis pool must stay below managed Redis crash threshold: %#v", p.ServerLimits)
	}
	if p.ServerLimits.RedisIdleTimeoutSeconds != "1" {
		t.Fatalf("benchmark Redis idle timeout must drain idle sockets quickly: %#v", p.ServerLimits)
	}
}

func TestSmokeProfileKeepsLocalSafeConnectionLimits(t *testing.T) {
	p := Smoke()
	if p.Transport.MaxConnsPerHost > 4 || p.Transport.MaxIdleConns > 4 || p.Transport.MaxIdleConnsPerHost > 4 || p.Relay.MaxIdleConns != 64 || p.Relay.MaxIdleConnsPerHost != 16 {
		t.Fatalf("smoke profile is unsafe for local loopback: %#v", p)
	}
	if p.RequestsPerPoint >= 3000 || len(p.Points) != 1 || p.Points[0] >= 250 {
		t.Fatalf("smoke profile must not be benchmark: %#v", p)
	}
}

func TestH2CDiagnosticIsNotImplementedProfile(t *testing.T) {
	_, err := ProfileByName("h2c_diagnostic")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("h2c diagnostic should be explicitly unavailable in first stage: %v", err)
	}
}
