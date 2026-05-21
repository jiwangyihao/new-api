package artifact

import (
	"strings"
	"testing"
)

func testRunContext() RunContext {
	return RunContext{SchemaVersion: 1, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "s2-short-stream", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
}

func TestArtifactRoundTripIncludesRunContextAndSeedOutput(t *testing.T) {
	rc := testRunContext()
	summary := Summary{SchemaVersion: 1, RunContext: rc, Path: "/v1/responses", Scenario: "s2-short-stream", TokenProfile: "subscription", Model: "gpt-5.5", Total: 1, Success: 1, Requests: []RequestRecord{{RequestIndex: 1, ClientRequestID: "client-1", NewAPIRequestID: "rid-1", UpstreamRequestID: "upstream-loadtest-1", StatusCode: 200, Success: true, PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}}}
	seed := SeedOutput{SchemaVersion: 1, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), UserIDSubscription: 1001, UserIDCompat: 1002, TokenSubscription: "sk-loadtestsub", TokenCompat: "sk-loadtestcompat", TokenDBKeySubscription: "loadtestsub", TokenDBKeyCompat: "loadtestcompat", ChannelID: 1, Model: "gpt-5.5", Group: "default", MockBaseURL: "http://127.0.0.1:19080", ExpectedUsagePerSuccess: Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}, RatioConfig: map[string]any{"ModelRatio": map[string]any{"gpt-5.5": float64(1)}}, FeatureOptions: map[string]any{"LogConsumeEnabled": true, "DataExportEnabled": true, "perf_metrics_setting.enabled": true, "RetryTimes": float64(0), "AutomaticRetryStatusCodes": ""}}
	mockDelta := MockStatsDelta{SchemaVersion: 1, RunContext: rc, Path: "c100-mock-stats-delta.json", Hash: "sha256:mockdelta", Actual429: 5, Actual502: 1, UpstreamAttemptsTotal: 100}
	diff := Diff{SchemaVersion: 1, RunContext: rc, SummaryPath: "c100-summary.json", MockStatsDeltaPath: mockDelta.Path, MockStatsDeltaHash: mockDelta.Hash, MockDelta: mockDelta, BusinessDelta: BusinessDelta{ChargesByRequest: []ChargeByRequest{{NewAPIRequestID: "rid-1", ClientRequestID: "client-1", UpstreamRequestID: "upstream-loadtest-1", StatusCode: 200, Success: true, LogQuota: 28, SubscriptionTokenDelta: 28, NetSubscriptionTokenDelta: 28}}}}
	for name, v := range map[string]any{"summary": summary, "seed": seed, "mockDelta": mockDelta, "diff": diff} {
		b, err := MarshalCanonical(v)
		if err != nil {
			t.Fatalf("%s marshal: %v", name, err)
		}
		if !strings.Contains(string(b), "run_context") {
			t.Fatalf("%s missing run_context: %s", name, b)
		}
	}
	if seed.RunContext.MockHash != "" {
		t.Fatalf("seed context contains scenario mock hash: %#v", seed.RunContext)
	}
}

func TestSeedOutputHashExcludesSelfReference(t *testing.T) {
	rc := testRunContext()
	seed := SeedOutput{SchemaVersion: 1, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), TokenDBKeySubscription: "loadtestsub", TokenDBKeyCompat: "loadtestcompat"}
	hash1, err := HashCanonical(seed)
	if err != nil {
		t.Fatal(err)
	}
	seed.RunContext.SeedOutputHash = hash1
	hash2, err := HashSeedOutput(seed)
	if err != nil {
		t.Fatal(err)
	}
	if hash1 != hash2 {
		t.Fatalf("self-referential seed hash: %s != %s", hash1, hash2)
	}
	if seed.RunContext.MockHash != "" {
		t.Fatalf("seed hash input contains scenario mock hash: %#v", seed.RunContext)
	}
}

func TestRedactRemovesSecretsAndProductionURLs(t *testing.T) {
	input := "postgresql://user:secret@example.com:5432/prod OPENAI_API_KEY=sk-real-production"
	redacted := Redact(input)
	if strings.Contains(redacted, "secret") || strings.Contains(redacted, "example.com") || strings.Contains(redacted, "sk-real") {
		t.Fatalf("not redacted: %s", redacted)
	}
}

func TestResourceSweepArtifactsRoundTrip(t *testing.T) {
	rc := testRunContext()
	limits := ResourceLimitsArtifact{
		SchemaVersion:                 SchemaVersion,
		RunContext:                    rc,
		TargetProcess:                 "new-api.exe",
		OSProcessMemoryLimitEnforced:  true,
		OSCPUAffinityEnforced:         true,
		ServerCPUAffinityCores:        2,
		ServerCPUAffinityMask:         3,
		ServerProcessMemoryLimitBytes: 512 * 1024 * 1024,
		ServerEnv:                     map[string]string{"GOMAXPROCS": "2", "GOGC": "100", "GOMEMLIMIT": "384MiB"},
		Scope:                         "server_only",
		Statused:                      Statused{Status: "applied"},
	}
	summary := Summary{
		SchemaVersion:       SchemaVersion,
		RunContext:          rc,
		Path:                "/v1/responses",
		Scenario:            "s2-short-stream",
		TokenProfile:        "subscription",
		Model:               "gpt-5.5",
		TargetConcurrency:   250,
		Total:               3000,
		Success:             2999,
		Errors:              1,
		StatusCodes:         map[string]int{"200": 2999, "500": 1},
		ProtocolCounts:      map[string]int{"HTTP/1.1": 3000},
		FirstErrorSamples:   []ErrorSample{{RequestIndex: 42, Phase: "read", Reason: "missing_done", StatusCode: 500, LatencyMS: 123.4, RequestID: "rid-42"}},
		Transport:           TransportProfile{Mode: "h1_keepalive", MaxConnsPerHost: 1024, MaxIdleConns: 1024, MaxIdleConnsPerHost: 1024},
		MaxObservedInFlight: 250,
		StopReason:          "error",
	}
	samples := ResourceSamplesArtifact{
		SchemaVersion: SchemaVersion,
		RunContext:    rc,
		Concurrency:   250,
		Samples: []ResourceSample{{
			UnixMilli: 1710000000000,
			Process:   ProcessSnapshot{Statused: Statused{Status: "ok"}, PID: 1234, RSSBytes: 64 * 1024 * 1024, CPUPercent: 80.5, ThreadCount: 12, HandleCount: 128, OpenTCPSockets: 250, CPUTimeSeconds: 12.25},
			Runtime:   RuntimeSnapshot{Statused: Statused{Status: "ok"}, Goroutines: 512, HeapAllocBytes: 32 * 1024 * 1024, HeapSysBytes: 64 * 1024 * 1024, GOMAXPROCS: 2, GOMEMLimitBytes: 384 * 1024 * 1024, GCCount: 7, LastGCUnixMS: 1710000000000, PauseTotalNS: 123456, HTTPConnState: map[string]int64{"active": 250}, HTTPAcceptTotal: 3000, HTTPActiveCurrent: 250},
			Postgres:  PostgresSnapshot{Statused: Statused{Status: "ok"}, Rows: map[string]int64{"consume_logs": 2999}, ActiveConnections: 10, IdleConnections: 4, WaitingLocks: 1, DatabaseSizeBytes: 2048},
			Redis:     RedisSnapshot{Statused: Statused{Status: "ok"}, ConnectedClients: 5, UsedMemoryBytes: 1024, UsedMemoryRSSBytes: 2048, MemFragmentationRatio: 1.5, InstantaneousOpsPerSec: 100, TotalCommandsProcessed: 5000, Keyspace: map[string]int64{"db0": 3}},
		}},
		Peaks: ResourcePeaks{RSSPeakBytes: 128 * 1024 * 1024, CPUPercentPeak: 95.5, CPUTimeSecondsPeak: 15.5, ThreadCountPeak: 14, HandleCountPeak: 140, OpenTCPSocketsPeak: 260, GoroutinesPeak: 600, HeapAllocPeakBytes: 48 * 1024 * 1024, HeapSysPeakBytes: 96 * 1024 * 1024, GCCountPeak: 9, PauseTotalNSPeak: 234567, HTTPAcceptTotalPeak: 3000, HTTPActiveCurrentPeak: 250, RedisConnectedClientsPeak: 6, RedisUsedMemoryPeakBytes: 2048, RedisUsedMemoryRSSPeakBytes: 4096, RedisInstantaneousOpsPeak: 120, RedisTotalCommandsProcessedPeak: 6000, PostgresActiveConnectionsPeak: 12, PostgresIdleConnectionsPeak: 5, PostgresWaitingLocksPeak: 2, PostgresDatabaseSizePeakBytes: 4096},
		Drain: Statused{Status: "passed"},
	}
	analysis := PointAnalysis{
		SchemaVersion:  SchemaVersion,
		RunContext:     rc,
		Concurrency:    250,
		FailureClass:   "server_resource_limit",
		HardGate:       GateResult{Passed: false, FailedReasons: []string{"errors must be zero"}},
		DiagnosticGate: GateResult{Passed: true, DiagnosticReasons: []string{"postgres active connections elevated"}},
	}
	ports := PortsClosedArtifact{
		SchemaVersion: SchemaVersion,
		RunContext:    rc,
		Ports:         map[string]string{"13080": "closed", "19080": "closed"},
		Passed:        true,
	}
	point := PointResult{
		Concurrency:    250,
		Passed:         false,
		SummaryExcerpt: SummaryExcerpt{Total: 3000, Success: 2999, Errors: 1, StopReason: "error"},
		ResourcePeaks:  samples.Peaks,
		Gate:           GateResult{Passed: false, FailedReasons: []string{"errors must be zero"}},
	}
	artifacts := map[string]any{
		"limits":   limits,
		"summary":  summary,
		"samples":  samples,
		"analysis": analysis,
		"ports":    ports,
		"point":    point,
	}
	for name, v := range artifacts {
		b, err := MarshalCanonical(v)
		if err != nil {
			t.Fatalf("%s marshal: %v", name, err)
		}
		json := string(b)
		if !strings.Contains(json, "run_context") {
			t.Fatalf("%s missing run_context: %s", name, b)
		}
	}
	checks := []string{
		"target_process",
		"GOMEMLIMIT",
		"protocol_counts",
		"HTTP/1.1",
		"first_error_samples",
		"reason",
		"samples",
		"failure_class",
		"hard_gate",
		"failed_reasons",
		"13080",
		"passed",
		"diagnostic_reasons",
		"resource_peaks",
		"postgres_database_size_peak_bytes",
		"gomemlimit_bytes",
	}
	combinedParts := make([]string, 0, len(artifacts))
	for _, v := range artifacts {
		b, err := MarshalCanonical(v)
		if err != nil {
			t.Fatal(err)
		}
		combinedParts = append(combinedParts, string(b))
	}
	combined := strings.Join(combinedParts, "\n")
	for _, check := range checks {
		if !strings.Contains(combined, check) {
			t.Fatalf("missing %q in resource sweep artifacts: %s", check, combined)
		}
	}
}

func TestResourceSweepOptionalStructFieldsOmitZeroValues(t *testing.T) {
	rc := testRunContext()
	artifacts := map[string]any{
		"summary": Summary{SchemaVersion: SchemaVersion, RunContext: rc, Path: "/v1/responses", Scenario: "s1-smoke", TokenProfile: "subscription", Model: "gpt-5.5", Total: 1, Success: 1},
		"analysis": PointAnalysis{SchemaVersion: SchemaVersion, RunContext: rc, Concurrency: 2, FailureClass: "unknown", HardGate: GateResult{Passed: true}},
		"point": PointResult{Concurrency: 2, Passed: true, SummaryExcerpt: SummaryExcerpt{Total: 1, Success: 1}},
	}
	for name, v := range artifacts {
		b, err := MarshalCanonical(v)
		if err != nil {
			t.Fatalf("%s marshal: %v", name, err)
		}
		json := string(b)
		for _, unexpected := range []string{"transport", "diagnostic_gate", "resource_peaks", "resource_delta", "profile_paths"} {
			if strings.Contains(json, unexpected) {
				t.Fatalf("%s should omit zero optional field %q: %s", name, unexpected, json)
			}
		}
		if name == "point" && strings.Contains(json, "\"gate\":{\"passed\":false") {
			t.Fatalf("point should omit zero gate instead of serializing failed gate: %s", json)
		}
	}
}
