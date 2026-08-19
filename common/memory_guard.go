package common

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cgroupV2MemoryRoot = "/sys/fs/cgroup"
	cgroupV1MemoryRoot = "/sys/fs/cgroup/memory"

	memoryGuardSoftLimitNumerator   = int64(4)
	memoryGuardSoftLimitDenominator = int64(5)
	memoryGuardExitNumerator        = int64(9)
	memoryGuardExitDenominator      = int64(10)
	memoryGuardPollInterval         = 2 * time.Second
	memoryGuardGracePeriod          = 15 * time.Second

	// Linux cgroup v1 represents an unlimited boundary with a value close to
	// MaxInt64. No practical container memory limit approaches one exbibyte.
	cgroupUnlimitedBoundary = int64(1 << 60)
)

type cgroupMemorySource struct {
	usagePath string
	boundary  int64
}

type memoryGuardState struct {
	aboveThresholdSince time.Time
}

var startMemoryGuardOnce sync.Once

// StartMemoryGuard configures Go's soft memory limit from the container cgroup
// and exits after sustained pressure, allowing the container restart policy to
// recover instead of remaining indefinitely throttled above memory.high.
func StartMemoryGuard() {
	startMemoryGuardOnce.Do(func() {
		source, ok := detectCgroupMemorySource(os.ReadFile, cgroupV2MemoryRoot, cgroupV1MemoryRoot)
		if !ok {
			return
		}

		currentLimit := debug.SetMemoryLimit(-1)
		softLimit := applyRuntimeMemoryLimit(source.boundary, currentLimit, debug.SetMemoryLimit)
		exitThreshold := fractionOf(source.boundary, memoryGuardExitNumerator, memoryGuardExitDenominator)
		SysLog(fmt.Sprintf(
			"cgroup memory guard enabled: boundary=%d MiB, go_soft_limit=%d MiB, exit_threshold=%d MiB, grace=%s",
			source.boundary/(1024*1024),
			softLimit/(1024*1024),
			exitThreshold/(1024*1024),
			memoryGuardGracePeriod,
		))

		go runMemoryGuard(source, exitThreshold, memoryGuardPollInterval, memoryGuardGracePeriod, os.Exit)
	})
}

func detectCgroupMemorySource(readFile func(string) ([]byte, error), v2Root, v1Root string) (cgroupMemorySource, bool) {
	candidates := []struct {
		usagePath     string
		boundaryPaths []string
	}{
		{
			usagePath: filepath.Join(v2Root, "memory.current"),
			boundaryPaths: []string{
				filepath.Join(v2Root, "memory.high"),
				filepath.Join(v2Root, "memory.max"),
			},
		},
		{
			usagePath: filepath.Join(v1Root, "memory.usage_in_bytes"),
			boundaryPaths: []string{
				filepath.Join(v1Root, "memory.soft_limit_in_bytes"),
				filepath.Join(v1Root, "memory.limit_in_bytes"),
			},
		},
	}

	for _, candidate := range candidates {
		usageData, err := readFile(candidate.usagePath)
		if err != nil {
			continue
		}
		if _, ok := parseCgroupUsage(usageData); !ok {
			continue
		}

		var boundary int64
		for _, path := range candidate.boundaryPaths {
			data, err := readFile(path)
			if err != nil {
				continue
			}
			value, ok := parseCgroupBoundary(data)
			if !ok {
				continue
			}
			if boundary == 0 || value < boundary {
				boundary = value
			}
		}
		if boundary > 0 {
			return cgroupMemorySource{usagePath: candidate.usagePath, boundary: boundary}, true
		}
	}

	return cgroupMemorySource{}, false
}

func parseCgroupUsage(data []byte) (int64, bool) {
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func parseCgroupBoundary(data []byte) (int64, bool) {
	text := strings.TrimSpace(string(data))
	if text == "" || strings.EqualFold(text, "max") {
		return 0, false
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value <= 0 || value >= cgroupUnlimitedBoundary {
		return 0, false
	}
	return value, true
}

func fractionOf(value, numerator, denominator int64) int64 {
	if value <= 0 || numerator <= 0 || denominator <= 0 {
		return 0
	}
	return (value/denominator)*numerator + ((value%denominator)*numerator)/denominator
}

func applyRuntimeMemoryLimit(boundary, currentLimit int64, setLimit func(int64) int64) int64 {
	softLimit := fractionOf(boundary, memoryGuardSoftLimitNumerator, memoryGuardSoftLimitDenominator)
	if currentLimit > 0 && currentLimit <= softLimit {
		return currentLimit
	}
	setLimit(softLimit)
	return softLimit
}

func (state *memoryGuardState) shouldExit(now time.Time, usage, threshold int64, grace time.Duration) bool {
	if usage < threshold {
		state.aboveThresholdSince = time.Time{}
		return false
	}
	if state.aboveThresholdSince.IsZero() || now.Before(state.aboveThresholdSince) {
		state.aboveThresholdSince = now
		return grace <= 0
	}
	return now.Sub(state.aboveThresholdSince) >= grace
}

func runMemoryGuard(source cgroupMemorySource, exitThreshold int64, pollInterval, gracePeriod time.Duration, exit func(int)) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	state := memoryGuardState{}

	check := func(now time.Time) bool {
		data, err := os.ReadFile(source.usagePath)
		if err != nil {
			SysError("cgroup memory guard disabled after usage read failed: " + err.Error())
			return true
		}
		usage, ok := parseCgroupUsage(data)
		if !ok {
			SysError("cgroup memory guard disabled after invalid usage value")
			return true
		}
		if !state.shouldExit(now, usage, exitThreshold, gracePeriod) {
			return false
		}
		SysError(fmt.Sprintf(
			"cgroup memory guard exiting after sustained pressure: usage=%d MiB, threshold=%d MiB, grace=%s",
			usage/(1024*1024),
			exitThreshold/(1024*1024),
			gracePeriod,
		))
		exit(1)
		return true
	}

	if check(time.Now()) {
		return
	}
	for now := range ticker.C {
		if check(now) {
			return
		}
	}
}
