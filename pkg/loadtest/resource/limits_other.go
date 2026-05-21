//go:build !windows

package resource

import "github.com/QuantumNous/new-api/pkg/loadtest/profile"

func ApplyServerLimits(pid int, limits profile.ServerLimits) (ApplyResult, error) {
	return ApplyResult{
		Status:                  "best_effort",
		Reason:                  "first phase only enforces process memory limits with Windows Job Object; non-Windows platforms record best-effort server env limits without OS memory cap",
		MemoryLimitEnforced:     false,
		CPUAffinityEnforced:     false,
		ProcessMemoryLimitBytes: limits.ProcessMemoryLimitBytes,
		CPUAffinityCores:        limits.CPUAffinityCores,
	}, nil
}
