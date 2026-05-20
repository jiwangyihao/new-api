package metrics

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"gorm.io/gorm"
)

type DiffInputs struct {
	Before         artifact.Snapshot
	After          artifact.Snapshot
	Summary        artifact.Summary
	SeedOutput     artifact.SeedOutput
	MockDelta      artifact.MockStatsDelta
	RunContext     artifact.RunContext
	StdoutLog      string
	StderrLog      string
	ConsumeLogRows []ConsumeLogRow
	PreConsumeRows []PreConsumeRow
	BusinessBefore artifact.BusinessSnapshot
	BusinessAfter  artifact.BusinessSnapshot
}

type ConsumeLogRow struct {
	RequestID string
	Quota     int
}

type PreConsumeRow struct {
	RequestID   string
	Status      string
	PreConsumed int
}

func LoadBusinessRows(db *gorm.DB, summary artifact.Summary) ([]ConsumeLogRow, []PreConsumeRow, error) {
	ids := make([]string, 0, len(summary.Requests))
	for _, req := range summary.Requests {
		if req.NewAPIRequestID != "" {
			ids = append(ids, req.NewAPIRequestID)
		}
	}
	if len(ids) == 0 {
		return nil, nil, nil
	}
	var logRows []model.Log
	if err := db.Model(&model.Log{}).Where("request_id IN ? AND type = ?", ids, model.LogTypeConsume).Find(&logRows).Error; err != nil {
		return nil, nil, err
	}
	logs := make([]ConsumeLogRow, 0, len(logRows))
	for _, row := range logRows {
		logs = append(logs, ConsumeLogRow{RequestID: row.RequestId, Quota: row.Quota})
	}
	var preRows []model.SubscriptionPreConsumeRecord
	if err := db.Model(&model.SubscriptionPreConsumeRecord{}).Where("request_id IN ?", ids).Find(&preRows).Error; err != nil {
		return nil, nil, err
	}
	records := make([]PreConsumeRow, 0, len(preRows))
	for _, row := range preRows {
		records = append(records, PreConsumeRow{RequestID: row.RequestId, Status: row.Status, PreConsumed: int(row.PreConsumed)})
	}
	return logs, records, nil
}

func LoadBusinessSnapshot(db *gorm.DB, seed artifact.SeedOutput) (artifact.BusinessSnapshot, error) {
	if seed.UserIDSubscription == 0 && seed.UserIDCompat == 0 {
		return artifact.BusinessSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "seed user ids are missing"}}, nil
	}
	snapshot := artifact.BusinessSnapshot{Statused: artifact.Statused{Status: "ok"}}
	if seed.UserIDSubscription != 0 {
		var sub model.UserSubscription
		if err := db.Model(&model.UserSubscription{}).Where("user_id = ? AND status = ?", seed.UserIDSubscription, "active").Order("id ASC").First(&sub).Error; err == nil {
			snapshot.SubscriptionTokenUsed = sub.TokenUsed
		} else if err != gorm.ErrRecordNotFound {
			return artifact.BusinessSnapshot{}, err
		}
		var user model.User
		if err := db.Model(&model.User{}).Select("quota").Where("id = ?", seed.UserIDSubscription).First(&user).Error; err == nil {
			snapshot.SubscriptionUserQuota = user.Quota
		} else if err != gorm.ErrRecordNotFound {
			return artifact.BusinessSnapshot{}, err
		}
		var token model.Token
		if err := db.Model(&model.Token{}).Select("remain_quota").Where("key = ?", seed.TokenDBKeySubscription).First(&token).Error; err == nil {
			snapshot.SubscriptionTokenRemain = token.RemainQuota
		} else if err != gorm.ErrRecordNotFound {
			return artifact.BusinessSnapshot{}, err
		}
	}
	if seed.UserIDCompat != 0 {
		var sub model.UserSubscription
		if err := db.Model(&model.UserSubscription{}).Where("user_id = ? AND status = ?", seed.UserIDCompat, "active").Order("id ASC").First(&sub).Error; err == nil {
			snapshot.CompatSubscriptionTokenUsed = sub.TokenUsed
		} else if err != gorm.ErrRecordNotFound {
			return artifact.BusinessSnapshot{}, err
		}
		var user model.User
		if err := db.Model(&model.User{}).Select("quota").Where("id = ?", seed.UserIDCompat).First(&user).Error; err == nil {
			snapshot.CompatUserQuota = user.Quota
		} else if err != gorm.ErrRecordNotFound {
			return artifact.BusinessSnapshot{}, err
		}
		var token model.Token
		if err := db.Model(&model.Token{}).Select("remain_quota").Where("key = ?", seed.TokenDBKeyCompat).First(&token).Error; err == nil {
			snapshot.CompatTokenRemain = token.RemainQuota
		} else if err != gorm.ErrRecordNotFound {
			return artifact.BusinessSnapshot{}, err
		}
	}
	return snapshot, nil
}

func BuildDiff(in DiffInputs) (artifact.Diff, artifact.Invariant) {
	if err := validateDiffRunContexts(in); err != nil {
		return artifact.Diff{SchemaVersion: artifact.SchemaVersion, RunContext: in.RunContext}, artifact.Invariant{Name: "run_context_consistency", Status: "failed", Reason: err.Error()}
	}
	seedHash, err := artifact.HashSeedOutput(in.SeedOutput)
	if err != nil {
		return artifact.Diff{SchemaVersion: artifact.SchemaVersion, RunContext: in.RunContext}, artifact.Invariant{Name: "seed_output_hash", Status: "failed", Reason: err.Error()}
	}
	if in.RunContext.SeedOutputHash == "" || seedHash != in.RunContext.SeedOutputHash {
		return artifact.Diff{SchemaVersion: artifact.SchemaVersion, RunContext: in.RunContext}, artifact.Invariant{Name: "seed_output_hash", Status: "failed", Reason: fmt.Sprintf("got %s want %s", seedHash, in.RunContext.SeedOutputHash)}
	}
	mockHash := in.MockDelta.Hash
	if mockHash == "" {
		mockHash, _ = artifact.HashCanonical(in.MockDelta)
	}
	charges, chargeInv := BuildChargesByRequest(in.Summary, in.ConsumeLogRows, in.PreConsumeRows)
	logs := ScanServerLogs(in.StdoutLog, in.StderrLog)
	invariants := buildInvariants(in, chargeInv, logs)
	diff := artifact.Diff{SchemaVersion: artifact.SchemaVersion, RunContext: in.RunContext, MockStatsDeltaPath: in.MockDelta.Path, MockStatsDeltaHash: mockHash, MockDelta: in.MockDelta, BusinessSnapshot: in.BusinessAfter, BusinessDelta: artifact.BusinessDelta{Statused: statusFromInvariants(invariants), ChargesByRequest: charges, Invariants: invariants}, ResourceDelta: artifact.ResourceDelta{RSSBeforeBytes: in.Before.Process.RSSBytes, RSSAfterDrainBytes: in.After.Process.RSSBytes, GoroutinesBefore: in.Before.Runtime.Goroutines, GoroutinesAfterDrain: in.After.Runtime.Goroutines}, Logs: logs}
	return diff, artifact.Invariant{Name: "diff_context_and_seed", Status: "passed"}
}

func DiffFailedInvariant(diff artifact.Diff) artifact.Invariant {
	if diff.BusinessDelta.Statused.Status == "" || diff.BusinessDelta.Statused.Status == "passed" {
		return artifact.Invariant{Name: "business_invariants", Status: "passed"}
	}
	reason := diff.BusinessDelta.Statused.Reason
	if reason == "" {
		reason = "business invariants did not pass"
	}
	return artifact.Invariant{Name: "business_invariants", Status: "failed", Reason: reason}
}

func buildInvariants(in DiffInputs, chargeInv artifact.Invariant, logs artifact.LogsSnapshot) []artifact.Invariant {
	invariants := []artifact.Invariant{
		chargeInv,
		{Name: "diff_context_and_seed", Status: "passed"},
		logInvariant("perf_metrics_no_upsert_error", logs.PerfMetricUpsertErrors == 0, "perf metric upsert errors present"),
		logInvariant("stdout_no_full_params", logs.StdoutFullParamsLines == 0, "consume stdout contains full params"),
		{Name: "quota_data_pending_or_unavailable", Status: "passed", Reason: "quota data runtime pending counter unavailable in first-stage collector"},
	}
	expected := int64(in.Summary.Success) * int64(in.SeedOutput.ExpectedUsagePerSuccess.TotalTokens)
	switch in.RunContext.TokenProfile {
	case "subscription":
		actual := in.BusinessAfter.SubscriptionTokenUsed - in.BusinessBefore.SubscriptionTokenUsed
		invariants = append(invariants, tokenDeltaInvariant("subscription_token_used_matches_success_usage", actual, expected))
		invariants = append(invariants, artifact.Invariant{Name: "compat_subscription_token_used_matches_success_usage", Status: "passed", Reason: "not applicable for subscription token profile"})
		invariants = append(invariants, artifact.Invariant{Name: "compat_wallet_not_charged", Status: "passed", Reason: "not applicable for subscription token profile"})
	case "compat":
		actual := in.BusinessAfter.CompatSubscriptionTokenUsed - in.BusinessBefore.CompatSubscriptionTokenUsed
		invariants = append(invariants, artifact.Invariant{Name: "subscription_token_used_matches_success_usage", Status: "passed", Reason: "not applicable for compat token profile"})
		invariants = append(invariants, tokenDeltaInvariant("compat_subscription_token_used_matches_success_usage", actual, expected))
		walletUnchanged := in.BusinessAfter.CompatUserQuota == in.BusinessBefore.CompatUserQuota && in.BusinessAfter.CompatTokenRemain == in.BusinessBefore.CompatTokenRemain
		invariants = append(invariants, logInvariant("compat_wallet_not_charged", walletUnchanged, "compat wallet or token quota changed"))
	default:
		invariants = append(invariants,
			artifact.Invariant{Name: "subscription_token_used_matches_success_usage", Status: "failed", Reason: "unknown token profile"},
			artifact.Invariant{Name: "compat_subscription_token_used_matches_success_usage", Status: "failed", Reason: "unknown token profile"},
			artifact.Invariant{Name: "compat_wallet_not_charged", Status: "failed", Reason: "unknown token profile"},
		)
	}
	if in.Summary.Errors == 0 {
		invariants = append(invariants, artifact.Invariant{Name: "failure_refund_by_request", Status: "passed", Reason: "no failed requests"})
	} else {
		invariants = append(invariants, failureRefundInvariant(in.PreConsumeRows))
	}
	return invariants
}

func tokenDeltaInvariant(name string, actual int64, expected int64) artifact.Invariant {
	if actual == expected {
		return artifact.Invariant{Name: name, Status: "passed"}
	}
	return artifact.Invariant{Name: name, Status: "failed", Reason: fmt.Sprintf("token delta got %d want %d", actual, expected)}
}

func logInvariant(name string, ok bool, reason string) artifact.Invariant {
	if ok {
		return artifact.Invariant{Name: name, Status: "passed"}
	}
	return artifact.Invariant{Name: name, Status: "failed", Reason: reason}
}

func failureRefundInvariant(records []PreConsumeRow) artifact.Invariant {
	for _, row := range records {
		if row.Status != "consumed" && row.Status != "refunded" {
			return artifact.Invariant{Name: "failure_refund_by_request", Status: "failed", Reason: "unexpected pre-consume status " + row.Status}
		}
	}
	return artifact.Invariant{Name: "failure_refund_by_request", Status: "passed"}
}

func statusFromInvariants(invariants []artifact.Invariant) artifact.Statused {
	for _, inv := range invariants {
		if inv.Status != "passed" {
			return artifact.Statused{Status: "failed", Reason: inv.Name + ": " + inv.Reason}
		}
	}
	return artifact.Statused{Status: "passed"}
}

func validateDiffRunContexts(in DiffInputs) error {
	rc := in.RunContext
	if in.Before.RunContext != rc {
		return fmt.Errorf("before run_context mismatch")
	}
	if in.After.RunContext != rc {
		return fmt.Errorf("after run_context mismatch")
	}
	if in.Summary.RunContext != rc {
		return fmt.Errorf("summary run_context mismatch")
	}
	if in.MockDelta.RunContext != rc {
		return fmt.Errorf("mock_delta run_context mismatch")
	}
	return nil
}

func BuildChargesByRequest(summary artifact.Summary, logs []ConsumeLogRow, records []PreConsumeRow) ([]artifact.ChargeByRequest, artifact.Invariant) {
	logByRequest := make(map[string]ConsumeLogRow, len(logs))
	for _, row := range logs {
		if row.RequestID != "" {
			logByRequest[row.RequestID] = row
		}
	}
	preByRequest := make(map[string]PreConsumeRow, len(records))
	for _, row := range records {
		if row.RequestID != "" {
			preByRequest[row.RequestID] = row
		}
	}
	charges := make([]artifact.ChargeByRequest, 0, len(summary.Requests))
	for _, req := range summary.Requests {
		if req.NewAPIRequestID == "" {
			return charges, artifact.Invariant{Name: "consume_logs_by_request", Status: "failed", Reason: "missing new_api_request_id"}
		}
		charge := artifact.ChargeByRequest{NewAPIRequestID: req.NewAPIRequestID, ClientRequestID: req.ClientRequestID, UpstreamRequestID: req.UpstreamRequestID, StatusCode: req.StatusCode, Success: req.Success}
		if len(logs) != 0 || len(summary.Requests) != 0 {
			row, ok := logByRequest[req.NewAPIRequestID]
			if !ok {
				return charges, artifact.Invariant{Name: "consume_logs_by_request", Status: "failed", Reason: "consume log missing for " + req.NewAPIRequestID}
			}
			charge.LogQuota = row.Quota
		}
		if len(records) != 0 || len(summary.Requests) != 0 {
			row, ok := preByRequest[req.NewAPIRequestID]
			if !ok {
				return charges, artifact.Invariant{Name: "consume_logs_by_request", Status: "failed", Reason: "pre-consume record missing for " + req.NewAPIRequestID}
			}
			charge.SubscriptionTokenDelta = row.PreConsumed
			charge.NetSubscriptionTokenDelta = row.PreConsumed
		}
		charges = append(charges, charge)
	}
	return charges, artifact.Invariant{Name: "consume_logs_by_request", Status: "passed"}
}

func ScanServerLogs(stdout, stderr string) artifact.LogsSnapshot {
	logs := artifact.LogsSnapshot{Statused: artifact.Statused{Status: "ok"}}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "record consume log:") && strings.Contains(line, "params={") {
			logs.StdoutFullParamsLines++
		}
		if isPerfMetricFlushError(line) {
			logs.PerfMetricUpsertErrors++
		}
	}
	for _, line := range strings.Split(stderr, "\n") {
		if isPerfMetricFlushError(line) {
			logs.PerfMetricUpsertErrors++
		}
	}
	return logs
}

func isPerfMetricFlushError(line string) bool {
	return strings.Contains(line, "failed to flush perf metric bucket") || strings.Contains(line, "column reference \"generation_ms\" is ambiguous")
}
