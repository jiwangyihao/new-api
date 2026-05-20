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
