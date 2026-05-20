package metrics

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testRunContext() artifact.RunContext {
	return artifact.RunContext{SchemaVersion: 1, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "s2-short-stream", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
}

func TestBuildDiffRequiresSeedAndMockContext(t *testing.T) {
	rc := testRunContext()
	seed := artifact.SeedOutput{SchemaVersion: 1, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), ExpectedUsagePerSuccess: artifact.Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}}
	seedHash, _ := artifact.HashSeedOutput(seed)
	rc.SeedOutputHash = seedHash
	seed.RunContext = rc.WithoutSeedOutputHash().WithoutMockHash()
	before := artifact.Snapshot{SchemaVersion: 1, RunContext: rc, Process: artifact.ProcessSnapshot{Statused: artifact.Statused{Status: "ok"}, RSSBytes: 100}, Business: artifact.BusinessSnapshot{Statused: artifact.Statused{Status: "ok"}, SubscriptionTokenUsed: 100}}
	after := artifact.Snapshot{SchemaVersion: 1, RunContext: rc, Process: artifact.ProcessSnapshot{Statused: artifact.Statused{Status: "ok"}, RSSBytes: 130}, Business: artifact.BusinessSnapshot{Statused: artifact.Statused{Status: "ok"}, SubscriptionTokenUsed: 100 + 94*28}}
	requests := []artifact.RequestRecord{{NewAPIRequestID: "rid-1", StatusCode: 200, Success: true, TotalTokens: 28}}
	logs := []ConsumeLogRow{{RequestID: "rid-1", Quota: 28}}
	records := []PreConsumeRow{{RequestID: "rid-1", Status: "consumed", PreConsumed: 28}}
	summary := artifact.Summary{RunContext: rc, Total: 100, Success: 94, Requests: requests}
	mock := artifact.MockStatsDelta{SchemaVersion: 1, RunContext: rc, Path: "c100-mock-stats-delta.json", Hash: "sha256:mockdelta", Actual429: 5, Actual502: 1, UpstreamAttemptsTotal: 100}
	diff, inv := BuildDiff(DiffInputs{Before: before, After: after, Summary: summary, SeedOutput: seed, MockDelta: mock, RunContext: rc, ConsumeLogRows: logs, PreConsumeRows: records, BusinessBefore: before.Business, BusinessAfter: after.Business})
	if inv.Status != "passed" {
		t.Fatalf("invariant failed: %#v", inv)
	}
	if diff.MockStatsDeltaPath == "" || diff.MockDelta.UpstreamAttemptsTotal != 100 {
		t.Fatalf("missing mock delta: %#v", diff)
	}
}

func TestBuildDiffFailsOnRunContextMismatch(t *testing.T) {
	rc := testRunContext()
	seed := artifact.SeedOutput{SchemaVersion: 1, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash()}
	seedHash, _ := artifact.HashSeedOutput(seed)
	rc.SeedOutputHash = seedHash
	seed.RunContext = rc.WithoutSeedOutputHash().WithoutMockHash()
	other := rc
	other.Scenario = "s3-long-stream"
	cases := map[string]func(*DiffInputs){
		"before":     func(in *DiffInputs) { in.Before.RunContext = other },
		"after":      func(in *DiffInputs) { in.After.RunContext = other },
		"summary":    func(in *DiffInputs) { in.Summary.RunContext = other },
		"mock_delta": func(in *DiffInputs) { in.MockDelta.RunContext = other },
	}
	for name, mutate := range cases {
		input := DiffInputs{Before: artifact.Snapshot{SchemaVersion: 1, RunContext: rc}, After: artifact.Snapshot{SchemaVersion: 1, RunContext: rc}, Summary: artifact.Summary{RunContext: rc}, SeedOutput: seed, MockDelta: artifact.MockStatsDelta{SchemaVersion: 1, RunContext: rc}, RunContext: rc}
		mutate(&input)
		_, inv := BuildDiff(input)
		if inv.Status != "failed" {
			t.Fatalf("%s context mismatch should fail: %#v", name, inv)
		}
	}
}

func TestBuildDiffFailsOnSeedHashMismatch(t *testing.T) {
	rc := testRunContext()
	rc.SeedOutputHash = "sha256:wrong"
	_, inv := BuildDiff(DiffInputs{RunContext: rc, SeedOutput: artifact.SeedOutput{RunContext: rc.WithoutSeedOutputHash().WithoutMockHash()}})
	if inv.Status != "failed" {
		t.Fatalf("want failed: %#v", inv)
	}
}

func TestBuildChargesByRequestJoinsOnlyNewAPIRequestID(t *testing.T) {
	summary := artifact.Summary{Requests: []artifact.RequestRecord{{ClientRequestID: "client-1", NewAPIRequestID: "rid-1", UpstreamRequestID: "upstream-1", StatusCode: 200, Success: true}}}
	logs := []ConsumeLogRow{{RequestID: "rid-1", Quota: 28}}
	records := []PreConsumeRow{{RequestID: "rid-1", Status: "settled", PreConsumed: 28}}
	charges, inv := BuildChargesByRequest(summary, logs, records)
	if inv.Status != "passed" || charges[0].NewAPIRequestID != "rid-1" {
		t.Fatalf("bad join: %#v %#v", charges, inv)
	}
	wrongLogs := []ConsumeLogRow{{RequestID: "client-1", Quota: 28}}
	_, inv = BuildChargesByRequest(summary, wrongLogs, records)
	if inv.Status != "failed" {
		t.Fatalf("wrong id join should fail: %#v", inv)
	}
}

func TestDiffFailedInvariantReportsBusinessInvariantFailure(t *testing.T) {
	diff := artifact.Diff{BusinessDelta: artifact.BusinessDelta{Statused: artifact.Statused{Status: "failed", Reason: "consume_logs_by_request: missing"}}}
	inv := DiffFailedInvariant(diff)
	if inv.Status != "failed" || !strings.Contains(inv.Reason, "consume_logs_by_request") {
		t.Fatalf("business failure was not propagated: %#v", inv)
	}
}

func TestLoadBusinessRowsAndSnapshotUseDatabaseRows(t *testing.T) {
	db := openMetricsTestDB(t)
	seed := artifact.SeedOutput{UserIDSubscription: 1, UserIDCompat: 2, TokenDBKeySubscription: "loadtestsub", TokenDBKeyCompat: "loadtestcompat"}
	if err := db.Create(&model.User{Id: 1, Username: "sub", Quota: 100, AffCode: "sub-aff"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{Id: 2, Username: "compat", Quota: 200, AffCode: "compat-aff"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserSubscription{Id: 10, UserId: 1, Status: "active", TokenUsed: 123}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserSubscription{Id: 11, UserId: 2, Status: "active", TokenUsed: 456}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Token{UserId: 1, Key: "loadtestsub", RemainQuota: 300}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Token{UserId: 2, Key: "loadtestcompat", RemainQuota: 400}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Log{RequestId: "rid-1", Type: model.LogTypeConsume, Quota: 28}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SubscriptionPreConsumeRecord{RequestId: "rid-1", PreConsumed: 28, Status: "consumed"}).Error; err != nil {
		t.Fatal(err)
	}
	summary := artifact.Summary{Requests: []artifact.RequestRecord{{NewAPIRequestID: "rid-1"}}}
	logs, records, err := LoadBusinessRows(db, summary)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || len(records) != 1 || logs[0].Quota != 28 || records[0].PreConsumed != 28 {
		t.Fatalf("bad rows: %#v %#v", logs, records)
	}
	snapshot, err := LoadBusinessSnapshot(db, seed)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SubscriptionTokenUsed != 123 || snapshot.CompatSubscriptionTokenUsed != 456 || snapshot.CompatTokenRemain != 400 {
		t.Fatalf("bad snapshot: %#v", snapshot)
	}
}

func TestServerLogScanningUsesStdoutAndStderr(t *testing.T) {
	got := ScanServerLogs("record consume log: userId=1", "failed to flush perf metric bucket: column reference")
	if got.StdoutFullParamsLines != 0 || got.PerfMetricUpsertErrors != 1 {
		t.Fatalf("bad log scan: %#v", got)
	}
	got = ScanServerLogs("record consume log: userId=1, params={large_payload", "")
	if got.StdoutFullParamsLines != 1 {
		t.Fatalf("missing stdout params detection: %#v", got)
	}
}

func openMetricsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserSubscription{}, &model.Token{}, &model.Log{}, &model.SubscriptionPreConsumeRecord{}); err != nil {
		t.Fatal(err)
	}
	return db
}
