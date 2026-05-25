package resource

import (
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/QuantumNous/new-api/pkg/loadtest/profile"
)

const serverOnlyScope = "new-api server process only; load generator, mock upstream, PostgreSQL, Redis, and orchestrator remain uncapped except normal OS scheduling"

const nestedJobAssignmentDeniedReason = "job object assignment denied by current Windows job; env limits and CPU affinity still applied"

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

func (r ApplyResult) ShouldFailOrchestrator() bool {
	switch r.Status {
	case "applied", "best_effort", "ok":
		return false
	case "partial":
		return r.Reason != nestedJobAssignmentDeniedReason
	default:
		return true
	}
}

func (r *ApplyResult) markNestedJobAssignmentDenied() {
	r.Status = "partial"
	r.Reason = nestedJobAssignmentDeniedReason
}

func ServerEnv(limits profile.ServerLimits) map[string]string {
	return map[string]string{
		"GOMAXPROCS":                    limits.GOMAXPROCS,
		"GOGC":                          limits.GOGC,
		"GOMEMLIMIT":                    limits.GOMEMLIMIT,
		"SQL_MAX_OPEN_CONNS":            limits.SQLMaxOpenConns,
		"SQL_MAX_IDLE_CONNS":            limits.SQLMaxIdleConns,
		"REDIS_POOL_SIZE":               limits.RedisPoolSize,
		"REDIS_IDLE_TIMEOUT_SECONDS":    limits.RedisIdleTimeoutSeconds,
		"RELAY_MAX_IDLE_CONNS":          limits.RelayMaxIdleConns,
		"RELAY_MAX_IDLE_CONNS_PER_HOST": limits.RelayMaxIdleConnsPerHost,
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
