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
	usagePath       string
	statPath        string
	inactiveFileKey string
	boundary        int64
}

type memoryGuardState struct {
	aboveThresholdSince time.Time
}

var startMemoryGuardOnce sync.Once

// StartMemoryGuard configures Go's soft memory limit from the container cgroup
// and exits after sustained working-set pressure, allowing the container
// restart policy to recover instead of remaining indefinitely throttled.
func StartMemoryGuard() {
	startMemoryGuardOnce.Do(func() {
		if !cgroupMemoryRootAvailable() {
			return
		}
		go runMemoryGuardWithDiscovery(
			func() (cgroupMemorySource, bool) {
				return detectCgroupMemorySource(os.ReadFile, cgroupV2MemoryRoot, cgroupV1MemoryRoot)
			},
			os.ReadFile,
			func() int64 { return debug.SetMemoryLimit(-1) },
			debug.SetMemoryLimit,
			os.Exit,
			memoryGuardPollInterval,
			memoryGuardGracePeriod,
		)
	})
}

func cgroupMemoryRootAvailable() bool {
	for _, root := range []string{cgroupV2MemoryRoot, cgroupV1MemoryRoot} {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func detectCgroupMemorySource(readFile func(string) ([]byte, error), v2Root, v1Root string) (cgroupMemorySource, bool) {
	candidates := []struct {
		usagePath       string
		statPath        string
		inactiveFileKey string
		boundaryPaths   []string
	}{
		{
			usagePath:       filepath.Join(v2Root, "memory.current"),
			statPath:        filepath.Join(v2Root, "memory.stat"),
			inactiveFileKey: "inactive_file",
			boundaryPaths: []string{
				filepath.Join(v2Root, "memory.high"),
				filepath.Join(v2Root, "memory.max"),
			},
		},
		{
			usagePath:       filepath.Join(v1Root, "memory.usage_in_bytes"),
			statPath:        filepath.Join(v1Root, "memory.stat"),
			inactiveFileKey: "total_inactive_file",
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
		statData, err := readFile(candidate.statPath)
		if err != nil {
			continue
		}
		if _, ok := parseCgroupMemoryStatValue(statData, candidate.inactiveFileKey); !ok {
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
			return cgroupMemorySource{
				usagePath:       candidate.usagePath,
				statPath:        candidate.statPath,
				inactiveFileKey: candidate.inactiveFileKey,
				boundary:        boundary,
			}, true
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

func parseCgroupMemoryStatValue(data []byte, key string) (int64, bool) {
	var fallback int64
	var fallbackOK bool
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || value < 0 {
			continue
		}
		if fields[0] == key {
			return value, true
		}
		if fields[0] == "inactive_file" || fields[0] == "total_inactive_file" {
			fallback = value
			fallbackOK = true
		}
	}
	return fallback, fallbackOK
}

func readCgroupWorkingSet(readFile func(string) ([]byte, error), source cgroupMemorySource) (int64, bool) {
	usageData, err := readFile(source.usagePath)
	if err != nil {
		return 0, false
	}
	current, ok := parseCgroupUsage(usageData)
	if !ok {
		return 0, false
	}
	statData, err := readFile(source.statPath)
	if err != nil {
		return 0, false
	}
	inactiveFile, ok := parseCgroupMemoryStatValue(statData, source.inactiveFileKey)
	if !ok {
		return 0, false
	}
	if inactiveFile >= current {
		return 0, true
	}
	return current - inactiveFile, true
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
	if softLimit <= 0 {
		return currentLimit
	}
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

func runMemoryGuardWithDiscovery(
	detect func() (cgroupMemorySource, bool),
	readFile func(string) ([]byte, error),
	readCurrentLimit func() int64,
	setLimit func(int64) int64,
	exit func(int),
	pollInterval, gracePeriod time.Duration,
) {
	if pollInterval <= 0 {
		pollInterval = memoryGuardPollInterval
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	state := memoryGuardState{}
	var source cgroupMemorySource
	var exitThreshold int64
	configured := false

	for {
		if !configured {
			candidate, ok := detect()
			if ok {
				softLimit := applyRuntimeMemoryLimit(candidate.boundary, readCurrentLimit(), setLimit)
				exitThreshold = fractionOf(candidate.boundary, memoryGuardExitNumerator, memoryGuardExitDenominator)
				if softLimit > 0 && exitThreshold > 0 {
					source = candidate
					configured = true
					state = memoryGuardState{}
					SysLog(fmt.Sprintf(
						"cgroup memory guard enabled: boundary=%d MiB, working_set_limit=%d MiB, go_soft_limit=%d MiB, exit_threshold=%d MiB, grace=%s",
						candidate.boundary/(1024*1024),
						candidate.boundary/(1024*1024),
						softLimit/(1024*1024),
						exitThreshold/(1024*1024),
						gracePeriod,
					))
				}
			}
		}

		if configured {
			workingSet, ok := readCgroupWorkingSet(readFile, source)
			if !ok {
				SysError("cgroup memory guard will retry after working-set read failed")
				configured = false
				state = memoryGuardState{}
			} else if state.shouldExit(time.Now(), workingSet, exitThreshold, gracePeriod) {
				SysError(fmt.Sprintf(
					"cgroup memory guard exiting after sustained working-set pressure: working_set=%d MiB, threshold=%d MiB, grace=%s",
					workingSet/(1024*1024),
					exitThreshold/(1024*1024),
					gracePeriod,
				))
				exit(1)
				return
			}
		}

		<-ticker.C
	}
}
