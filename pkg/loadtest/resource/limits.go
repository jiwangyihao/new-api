package resource

import (
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/QuantumNous/new-api/pkg/loadtest/profile"
)

const serverOnlyScope = "new-api server process only; load generator, mock upstream, PostgreSQL, Redis, and orchestrator remain uncapped except normal OS scheduling"

// ApplyResult records the platform resource-limit operations that actually took
// effect for the tested new-api server process.
type ApplyResult struct {
	Status                  string
	Reason                  string
	MemoryLimitEnforced     bool
	CPUAffinityEnforced     bool
	CPUAffinityMask         uintptr
	ProcessMemoryLimitBytes uint64
	CPUAffinityCores        int
}

func ServerEnv(limits profile.ServerLimits) map[string]string {
	return map[string]string{
		"GOMAXPROCS": limits.GOMAXPROCS,
		"GOGC":       limits.GOGC,
		"GOMEMLIMIT": limits.GOMEMLIMIT,
	}
}

func BuildLimitsArtifact(rc artifact.RunContext, limits profile.ServerLimits, result ApplyResult) artifact.ResourceLimitsArtifact {
	memoryLimitBytes := result.ProcessMemoryLimitBytes
	if memoryLimitBytes == 0 {
		memoryLimitBytes = limits.ProcessMemoryLimitBytes
	}
	cpuCores := result.CPUAffinityCores
	if cpuCores == 0 {
		cpuCores = limits.CPUAffinityCores
	}
	return artifact.ResourceLimitsArtifact{
		SchemaVersion:                 artifact.SchemaVersion,
		RunContext:                    rc,
		TargetProcess:                 "server",
		OSProcessMemoryLimitEnforced:  result.MemoryLimitEnforced,
		OSCPUAffinityEnforced:         result.CPUAffinityEnforced,
		ServerCPUAffinityCores:        cpuCores,
		ServerCPUAffinityMask:         result.CPUAffinityMask,
		ServerProcessMemoryLimitBytes: memoryLimitBytes,
		ServerEnv:                     ServerEnv(limits),
		Scope:                         serverOnlyScope,
		Statused: artifact.Statused{
			Status: result.Status,
			Reason: result.Reason,
		},
	}
}
