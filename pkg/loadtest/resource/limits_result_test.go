package resource

import "testing"

func TestApplyResultTerminalErrorRequiresFailure(t *testing.T) {
	result := ApplyResult{Status: "partial", Reason: "assign process to job object: Access is denied.", CPUAffinityEnforced: true}
	if result.ShouldFailOrchestrator() != true {
		t.Fatalf("terminal partial result did not fail: %#v", result)
	}
}

func TestApplyResultNestedJobPartialIsReportOnly(t *testing.T) {
	result := ApplyResult{Status: "partial", Reason: "job object assignment denied by current Windows job; env limits and CPU affinity still applied", CPUAffinityEnforced: true}
	if result.ShouldFailOrchestrator() {
		t.Fatalf("nested-job partial result should be report-only: %#v", result)
	}
}

func TestMarkNestedJobAssignmentDeniedRecordsPartialReportOnly(t *testing.T) {
	result := ApplyResult{Status: "applied", CPUAffinityEnforced: true}
	result.markNestedJobAssignmentDenied()
	if result.Status != "partial" {
		t.Fatalf("status = %q, want partial", result.Status)
	}
	if result.Reason != nestedJobAssignmentDeniedReason {
		t.Fatalf("reason = %q", result.Reason)
	}
	if result.ShouldFailOrchestrator() {
		t.Fatalf("nested-job partial should stay report-only: %#v", result)
	}
}
