package analysis

import (
	"strings"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

const (
	FailureCleanupFailed      = "cleanup_failed"
	FailureClientTransport    = "client_transport"
	FailureStreamProtocol     = "stream_protocol"
	FailureClientParser       = "client_parser_failure"
	FailureUpstreamMock       = "upstream_mock"
	FailureBillingInvariant   = "billing_invariant"
	FailureServerResource     = "server_resource_limit"
	FailurePostgresBottleneck = "postgres_bottleneck"
	FailureRedisBottleneck    = "redis_bottleneck"
	FailureCapacityLimit      = "capacity_limit"
	FailureUnknown            = "unknown"
)

const (
	benchmarkMaxConcurrency = 1000
	postgresPoolLimit       = 10
)

type Inputs struct {
	Point           artifact.PointResult
	Summary         artifact.Summary
	Ports           artifact.PortsClosedArtifact
	ResourceSamples artifact.ResourceSamplesArtifact
	BusinessDiff    artifact.Diff
	RequirePorts    bool
	MaxRequests     int
}

func EvaluateBenchmarkPoint(in Inputs) artifact.PointAnalysis {
	hardGate := in.Point.Gate
	if gateResultZero(hardGate) {
		hardGate.Passed = in.Point.Passed
		if !in.Point.Passed {
			hardGate.FailedReasons = append(hardGate.FailedReasons, "point gate unavailable")
		}
	}
	if hardGate.Passed {
		if failed := resourceCoverageFailures(in); len(failed) > 0 {
			hardGate.Passed = false
			hardGate.FailedReasons = append(hardGate.FailedReasons, failed...)
		}
	}

	diagnosticReasons := DiagnosticReasons(in)
	diagnosticGate := artifact.GateResult{}
	if len(diagnosticReasons) > 0 {
		diagnosticGate = artifact.GateResult{DiagnosticReasons: diagnosticReasons}
	}

	return artifact.PointAnalysis{
		SchemaVersion:  artifact.SchemaVersion,
		RunContext:     runContext(in),
		Concurrency:    in.Point.Concurrency,
		FailureClass:   ClassifyFailure(in),
		HardGate:       hardGate,
		DiagnosticGate: diagnosticGate,
	}
}

func ClassifyFailure(in Inputs) string {
	for _, detector := range failureDetectors {
		if detector.match(in) {
			return detector.class
		}
	}
	return FailureUnknown
}

func DiagnosticReasons(in Inputs) []string {
	reasons := make([]string, 0, 4)
	if cleanupFailed(in) {
		reasons = append(reasons, FailureCleanupFailed+": ports did not close")
	}
	if clientTransportDominant(in) {
		reasons = append(reasons, FailureClientTransport+": client transport errors are dominant")
	}
	if streamProtocolFailure(in) {
		reasons = append(reasons, FailureStreamProtocol+": HTTP 200 stream responses missed done events")
	}
	if clientParserFailure(in) {
		reasons = append(reasons, FailureClientParser+": client-side response parsing errors are dominant")
	}
	if upstreamMockFailure(in) {
		reasons = append(reasons, FailureUpstreamMock+": mock delta does not match summary")
	}
	if billingInvariantFailure(in) {
		reasons = append(reasons, FailureBillingInvariant+": token quota billing invariant failed")
	}
	if serverResourceLimit(in) {
		reasons = append(reasons, FailureServerResource+": process memory or runtime resource usage is close to limit")
	}
	if postgresBottleneck(in) {
		reasons = append(reasons, FailurePostgresBottleneck+": PostgreSQL connection pool or locks are elevated")
	}
	if redisBottleneck(in) {
		reasons = append(reasons, FailureRedisBottleneck+": Redis availability, memory, or command rate is abnormal")
	}
	if capacityLimit(in) {
		reasons = append(reasons, FailureCapacityLimit+": observed in-flight requests reached the target concurrency")
	}
	return reasons
}

type failureDetector struct {
	class string
	match func(Inputs) bool
}

var failureDetectors = []failureDetector{
	{class: FailureCleanupFailed, match: cleanupFailed},
	{class: FailureClientTransport, match: clientTransportDominant},
	{class: FailureStreamProtocol, match: streamProtocolFailure},
	{class: FailureClientParser, match: clientParserFailure},
	{class: FailureUpstreamMock, match: upstreamMockFailure},
	{class: FailureBillingInvariant, match: billingInvariantFailure},
	{class: FailureServerResource, match: serverResourceLimit},
	{class: FailurePostgresBottleneck, match: postgresBottleneck},
	{class: FailureRedisBottleneck, match: redisBottleneck},
	{class: FailureCapacityLimit, match: capacityLimit},
}

func runContext(in Inputs) artifact.RunContext {
	if in.Point.MockDelta.RunContext != (artifact.RunContext{}) {
		return in.Point.MockDelta.RunContext
	}
	if in.Summary.RunContext != (artifact.RunContext{}) {
		return in.Summary.RunContext
	}
	if in.ResourceSamples.RunContext != (artifact.RunContext{}) {
		return in.ResourceSamples.RunContext
	}
	if in.Ports.RunContext != (artifact.RunContext{}) {
		return in.Ports.RunContext
	}
	return in.BusinessDiff.RunContext
}

func gateResultZero(gate artifact.GateResult) bool {
	return !gate.Passed && len(gate.FailedReasons) == 0 && len(gate.DiagnosticReasons) == 0
}

func resourceCoverageFailures(in Inputs) []string {
	failed := make([]string, 0, 4)
	if len(in.ResourceSamples.Samples) == 0 {
		failed = append(failed, "resource samples are required")
	}
	if !resourcePeaksPresent(in.ResourceSamples.Peaks) {
		failed = append(failed, "resource peaks are required")
	}
	if in.ResourceSamples.Drain.Status == "failed" {
		failed = append(failed, "resource drain failed")
	}
	if in.RequirePorts && !in.Ports.Passed {
		failed = append(failed, "ports must be closed")
	}
	return failed
}

func resourcePeaksPresent(peaks artifact.ResourcePeaks) bool {
	return peaks.RSSPeakBytes != 0 ||
		peaks.CPUPercentPeak != 0 ||
		peaks.CPUTimeSecondsPeak != 0 ||
		peaks.ThreadCountPeak != 0 ||
		peaks.HandleCountPeak != 0 ||
		peaks.OpenTCPSocketsPeak != 0 ||
		peaks.GoroutinesPeak != 0 ||
		peaks.HeapAllocPeakBytes != 0 ||
		peaks.HeapSysPeakBytes != 0 ||
		peaks.GCCountPeak != 0 ||
		peaks.PauseTotalNSPeak != 0 ||
		peaks.HTTPAcceptTotalPeak != 0 ||
		peaks.HTTPActiveCurrentPeak != 0 ||
		peaks.RedisConnectedClientsPeak != 0 ||
		peaks.RedisUsedMemoryPeakBytes != 0 ||
		peaks.RedisUsedMemoryRSSPeakBytes != 0 ||
		peaks.RedisInstantaneousOpsPeak != 0 ||
		peaks.RedisTotalCommandsProcessedPeak != 0 ||
		peaks.PostgresActiveConnectionsPeak != 0 ||
		peaks.PostgresIdleConnectionsPeak != 0 ||
		peaks.PostgresWaitingLocksPeak != 0 ||
		peaks.PostgresDatabaseSizePeakBytes != 0
}

func cleanupFailed(in Inputs) bool {
	return in.RequirePorts && !in.Ports.Passed
}

func clientTransportDominant(in Inputs) bool {
	return reasonGroupDominant(in, isTransportReason)
}

func streamProtocolFailure(in Inputs) bool {
	return reasonGroupDominant(in, isMissingDoneReason) && hasHTTP200Evidence(in, "missing_done")
}

func clientParserFailure(in Inputs) bool {
	return reasonGroupDominant(in, isParserReason)
}

func upstreamMockFailure(in Inputs) bool {
	if gateReasonMatches(in, isUpstreamMockGateReason) {
		return true
	}
	return mockDeltaMismatch(in.Point.MockDelta, in.Point.SummaryExcerpt) || mockDeltaMismatch(in.BusinessDiff.MockDelta, in.Point.SummaryExcerpt)
}

func billingInvariantFailure(in Inputs) bool {
	if gateReasonMatches(in, containsBillingInvariantName) || invariantsContainBillingFailure(in.Point.Invariants) || invariantsContainBillingFailure(in.BusinessDiff.BusinessDelta.Invariants) {
		return true
	}
	if in.BusinessDiff.BusinessDelta.Status == "failed" && containsBillingInvariantName(in.BusinessDiff.BusinessDelta.Reason) {
		return true
	}
	return false
}

func serverResourceLimit(in Inputs) bool {
	if gateReasonMatches(in, isServerResourceGateReason) || diagnosticReasonContains(in, FailureServerResource) {
		return true
	}
	limit := runtimeMemoryLimit(in.ResourceSamples.Samples)
	if limit > 0 && (nearLimit(in.ResourceSamples.Peaks.RSSPeakBytes, limit) || nearLimit(in.ResourceSamples.Peaks.HeapAllocPeakBytes, limit) || nearLimit(in.ResourceSamples.Peaks.HeapSysPeakBytes, limit)) {
		return true
	}
	if in.ResourceSamples.Peaks.CPUPercentPeak >= 95 {
		return true
	}
	for _, sample := range in.ResourceSamples.Samples {
		if statusReasonContains(sample.Process.Statused, "memory") || statusReasonContains(sample.Process.Statused, "limit") || statusReasonContains(sample.Runtime.Statused, "memory") || statusReasonContains(sample.Runtime.Statused, "limit") {
			return true
		}
	}
	return false
}

func postgresBottleneck(in Inputs) bool {
	if diagnosticReasonContains(in, FailurePostgresBottleneck) {
		return true
	}
	peaks := in.ResourceSamples.Peaks
	if peaks.PostgresWaitingLocksPeak > 0 || peaks.PostgresActiveConnectionsPeak >= postgresPoolLimit*9/10 || peaks.PostgresActiveConnectionsPeak+peaks.PostgresIdleConnectionsPeak >= postgresPoolLimit {
		return true
	}
	for _, sample := range in.ResourceSamples.Samples {
		if sample.Postgres.WaitingLocks > 0 || sample.Postgres.ActiveConnections >= postgresPoolLimit*9/10 || sample.Postgres.ActiveConnections+sample.Postgres.IdleConnections >= postgresPoolLimit {
			return true
		}
		if unavailableOrFailed(sample.Postgres.Statused) && (statusReasonContains(sample.Postgres.Statused, "postgres") || statusReasonContains(sample.Postgres.Statused, "connection") || statusReasonContains(sample.Postgres.Statused, "lock")) {
			return true
		}
	}
	return false
}

func redisBottleneck(in Inputs) bool {
	if diagnosticReasonContains(in, FailureRedisBottleneck) {
		return true
	}
	peaks := in.ResourceSamples.Peaks
	if redisMemoryHigh(peaks.RedisUsedMemoryPeakBytes) || redisMemoryHigh(peaks.RedisUsedMemoryRSSPeakBytes) {
		return true
	}
	success := successCount(in)
	if success > 0 && peaks.RedisTotalCommandsProcessedPeak > 0 && peaks.RedisTotalCommandsProcessedPeak < uint64(success) {
		return true
	}
	for _, sample := range in.ResourceSamples.Samples {
		if unavailableOrFailed(sample.Redis.Statused) {
			return true
		}
		if redisMemoryHigh(sample.Redis.UsedMemoryBytes) || redisMemoryHigh(sample.Redis.UsedMemoryRSSBytes) {
			return true
		}
	}
	return false
}

func capacityLimit(in Inputs) bool {
	if diagnosticReasonContains(in, FailureCapacityLimit) {
		return true
	}
	concurrency := maxPositive(in.Point.Concurrency, in.Summary.TargetConcurrency, in.ResourceSamples.Concurrency)
	observed := maxPositive(in.Point.SummaryExcerpt.MaxObservedInFlight, in.Summary.MaxObservedInFlight)
	if concurrency <= 0 || observed <= 0 || !observedNearConcurrency(observed, concurrency) {
		return false
	}
	return concurrency >= benchmarkMaxConcurrency
}

func reasonGroupDominant(in Inputs, match func(string) bool) bool {
	total := totalReasonCount(in)
	if total == 0 {
		return false
	}
	matched := matchedReasonCount(in, match)
	return matched > 0 && matched*2 > total
}

func totalReasonCount(in Inputs) int {
	if len(in.Summary.ErrorReasons) > 0 {
		total := 0
		for _, count := range in.Summary.ErrorReasons {
			if count > 0 {
				total += count
			}
		}
		return total
	}
	return len(in.Summary.FirstErrorSamples)
}

func matchedReasonCount(in Inputs, match func(string) bool) int {
	if len(in.Summary.ErrorReasons) > 0 {
		matched := 0
		for reason, count := range in.Summary.ErrorReasons {
			if count > 0 && match(reason) {
				matched += count
			}
		}
		return matched
	}
	matched := 0
	for _, sample := range in.Summary.FirstErrorSamples {
		if match(sample.Reason) {
			matched++
		}
	}
	return matched
}

func isTransportReason(reason string) bool {
	switch strings.ToLower(reason) {
	case "connect_refused", "connect_timeout", "request_timeout", "connection_reset", "http_client_do_error":
		return true
	default:
		return false
	}
}

func isMissingDoneReason(reason string) bool {
	return strings.EqualFold(reason, "missing_done")
}

func isParserReason(reason string) bool {
	reason = strings.ToLower(reason)
	return reason == "json_error" || reason == "read_error" || strings.Contains(reason, "stream_parse")
}

func hasHTTP200Evidence(in Inputs, reason string) bool {
	for _, sample := range in.Summary.FirstErrorSamples {
		if strings.EqualFold(sample.Reason, reason) && sample.StatusCode == 200 {
			return true
		}
	}
	statuses := in.Summary.StatusCodes
	if len(statuses) == 0 {
		statuses = in.Point.SummaryExcerpt.StatusCodes
	}
	if len(statuses) == 0 {
		return false
	}
	total := 0
	for _, count := range statuses {
		if count > 0 {
			total += count
		}
	}
	return statuses["200"] > 0 && statuses["200"]*2 >= total
}

func gateReasonMatches(in Inputs, match func(string) bool) bool {
	for _, reason := range in.Point.Gate.FailedReasons {
		if match(reason) {
			return true
		}
	}
	for _, reason := range in.Point.Gate.DiagnosticReasons {
		if match(reason) {
			return true
		}
	}
	return false
}

func isUpstreamMockGateReason(reason string) bool {
	reason = strings.ToLower(reason)
	if !strings.Contains(reason, "mock") && !strings.Contains(reason, "upstream") {
		return false
	}
	return strings.Contains(reason, "delta") || strings.Contains(reason, "attempt") || strings.Contains(reason, "status") || strings.Contains(reason, "mismatch") || strings.Contains(reason, "injected")
}

func isServerResourceGateReason(reason string) bool {
	reason = strings.ToLower(reason)
	return strings.Contains(reason, "rss") || strings.Contains(reason, "goroutine") || strings.Contains(reason, "resource") || strings.Contains(reason, "memory")
}

func mockDeltaMismatch(delta artifact.MockStatsDelta, summary artifact.SummaryExcerpt) bool {
	if !mockDeltaPresent(delta) {
		return false
	}
	expectedAttempts := summary.UpstreamAttemptsTotal
	if expectedAttempts == 0 {
		expectedAttempts = summary.Total
	}
	if expectedAttempts > 0 && delta.UpstreamAttemptsTotal != expectedAttempts {
		return true
	}
	if delta.Actual429 != summary.Actual429 || delta.Actual502 != summary.Actual502 {
		return true
	}
	return false
}

func mockDeltaPresent(delta artifact.MockStatsDelta) bool {
	return delta.UpstreamAttemptsTotal != 0 || delta.Actual429 != 0 || delta.Actual502 != 0 || delta.Path != "" || delta.Hash != ""
}

func invariantsContainBillingFailure(invariants []artifact.Invariant) bool {
	for _, inv := range invariants {
		if inv.Status != "" && inv.Status != "passed" && isBillingInvariantName(inv.Name) {
			return true
		}
	}
	return false
}

func isBillingInvariantName(name string) bool {
	switch name {
	case "subscription_token_used_matches_success_usage", "compat_subscription_token_used_matches_success_usage", "compat_wallet_not_charged", "failure_refund_by_request":
		return true
	default:
		return false
	}
}

func containsBillingInvariantName(reason string) bool {
	return strings.Contains(reason, "subscription_token_used_matches_success_usage") ||
		strings.Contains(reason, "compat_subscription_token_used_matches_success_usage") ||
		strings.Contains(reason, "compat_wallet_not_charged") ||
		strings.Contains(reason, "failure_refund_by_request")
}

func diagnosticReasonContains(in Inputs, value string) bool {
	for _, reason := range in.Point.Gate.DiagnosticReasons {
		if strings.Contains(reason, value) {
			return true
		}
	}
	return false
}

func runtimeMemoryLimit(samples []artifact.ResourceSample) uint64 {
	var limit uint64
	for _, sample := range samples {
		if sample.Runtime.GOMEMLimitBytes <= 0 {
			continue
		}
		current := uint64(sample.Runtime.GOMEMLimitBytes)
		if limit == 0 || current < limit {
			limit = current
		}
	}
	return limit
}

func nearLimit(value, limit uint64) bool {
	if value == 0 || limit == 0 {
		return false
	}
	return value >= limit-limit/10
}

func statusReasonContains(status artifact.Statused, value string) bool {
	return status.Status != "ok" && strings.Contains(strings.ToLower(status.Reason), value)
}

func unavailableOrFailed(status artifact.Statused) bool {
	return status.Status == "failed" || status.Status == "unavailable"
}

func redisMemoryHigh(value uint64) bool {
	const highRedisMemoryBytes = 256 * 1024 * 1024
	return value >= highRedisMemoryBytes
}

func successCount(in Inputs) int {
	if in.Summary.Success > 0 {
		return in.Summary.Success
	}
	return in.Point.SummaryExcerpt.Success
}

func maxPositive(values ...int) int {
	max := 0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

func observedNearConcurrency(observed int, concurrency int) bool {
	return observed >= concurrency*9/10
}
