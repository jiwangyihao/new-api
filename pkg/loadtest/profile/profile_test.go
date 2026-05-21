package profile

import (
	"strings"
	"testing"
	"time"
)

func TestBenchmarkProfileMatchesWorkbenchMatrix(t *testing.T) {
	p := Benchmark()
	wantPoints := []int{250, 500, 750, 1000}
	if len(p.Points) != len(wantPoints) {
		t.Fatalf("points len = %d", len(p.Points))
	}
	for i := range wantPoints {
		if p.Points[i] != wantPoints[i] {
			t.Fatalf("point[%d] = %d want %d", i, p.Points[i], wantPoints[i])
		}
	}
	if p.RequestsPerPoint != 3000 || p.RampStep != 25 || p.RampInterval != 200*time.Millisecond || p.Duration != 45*time.Second || p.Timeout != 120*time.Second {
		t.Fatalf("benchmark timings mismatch: %#v", p)
	}
	if p.Transport.Mode != TransportH1KeepAlive {
		t.Fatalf("transport mode = %q", p.Transport.Mode)
	}
	if p.Transport.MaxConnsPerHost != 1024 || p.Transport.MaxIdleConns != 1024 || p.Transport.MaxIdleConnsPerHost != 1024 || p.Relay.MaxIdleConns != 1024 || p.Relay.MaxIdleConnsPerHost != 1024 {
		t.Fatalf("benchmark connection limits mismatch: transport=%#v relay=%#v", p.Transport, p.Relay)
	}
	if p.ServerLimits.GOMAXPROCS != "2" || p.ServerLimits.GOGC != "100" || p.ServerLimits.GOMEMLIMIT != "384MiB" || p.ServerLimits.ProcessMemoryLimitBytes != 512*1024*1024 || p.ServerLimits.CPUAffinityCores != 2 {
		t.Fatalf("server limits mismatch: %#v", p.ServerLimits)
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
