package common

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeMemoryGuardTestFile(t *testing.T, root, name, value string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(path, []byte(value), 0600))
	return path
}

func TestDetectCgroupMemorySourceV2UsesLowestFiniteBoundary(t *testing.T) {
	v2Root := t.TempDir()
	v1Root := t.TempDir()
	usagePath := writeMemoryGuardTestFile(t, v2Root, "memory.current", "1024\n")
	writeMemoryGuardTestFile(t, v2Root, "memory.high", "4096\n")
	writeMemoryGuardTestFile(t, v2Root, "memory.max", "8192\n")

	source, ok := detectCgroupMemorySource(os.ReadFile, v2Root, v1Root)

	require.True(t, ok)
	require.Equal(t, usagePath, source.usagePath)
	require.Equal(t, int64(4096), source.boundary)
}

func TestDetectCgroupMemorySourceV2FallsBackFromMaxHigh(t *testing.T) {
	v2Root := t.TempDir()
	v1Root := t.TempDir()
	usagePath := writeMemoryGuardTestFile(t, v2Root, "memory.current", "1024")
	writeMemoryGuardTestFile(t, v2Root, "memory.high", "max")
	writeMemoryGuardTestFile(t, v2Root, "memory.max", "8192")

	source, ok := detectCgroupMemorySource(os.ReadFile, v2Root, v1Root)

	require.True(t, ok)
	require.Equal(t, usagePath, source.usagePath)
	require.Equal(t, int64(8192), source.boundary)
}

func TestDetectCgroupMemorySourceV1IgnoresUnlimitedSoftLimit(t *testing.T) {
	v2Root := t.TempDir()
	v1Root := t.TempDir()
	usagePath := writeMemoryGuardTestFile(t, v1Root, "memory.usage_in_bytes", "2048")
	writeMemoryGuardTestFile(t, v1Root, "memory.soft_limit_in_bytes", "9223372036854771712")
	writeMemoryGuardTestFile(t, v1Root, "memory.limit_in_bytes", "16384")

	source, ok := detectCgroupMemorySource(os.ReadFile, v2Root, v1Root)

	require.True(t, ok)
	require.Equal(t, usagePath, source.usagePath)
	require.Equal(t, int64(16384), source.boundary)
}

func TestDetectCgroupMemorySourceWithoutFiniteBoundaryIsDisabled(t *testing.T) {
	v2Root := t.TempDir()
	v1Root := t.TempDir()
	writeMemoryGuardTestFile(t, v2Root, "memory.current", "1024")
	writeMemoryGuardTestFile(t, v2Root, "memory.high", "max")
	writeMemoryGuardTestFile(t, v2Root, "memory.max", "max")

	_, ok := detectCgroupMemorySource(os.ReadFile, v2Root, v1Root)

	require.False(t, ok)
}

func TestApplyRuntimeMemoryLimit(t *testing.T) {
	t.Run("sets eighty percent of cgroup boundary", func(t *testing.T) {
		var setValue int64
		limit := applyRuntimeMemoryLimit(1000, math.MaxInt64, func(value int64) int64 {
			setValue = value
			return math.MaxInt64
		})

		require.Equal(t, int64(800), limit)
		require.Equal(t, int64(800), setValue)
	})

	t.Run("preserves lower existing runtime limit", func(t *testing.T) {
		called := false
		limit := applyRuntimeMemoryLimit(1000, 700, func(int64) int64 {
			called = true
			return 700
		})

		require.Equal(t, int64(700), limit)
		require.False(t, called)
	})
}

func TestMemoryGuardStateRequiresContinuousGracePeriod(t *testing.T) {
	state := memoryGuardState{}
	start := time.Unix(100, 0)
	threshold := int64(900)
	grace := 15 * time.Second

	require.False(t, state.shouldExit(start, 900, threshold, grace))
	require.False(t, state.shouldExit(start.Add(14*time.Second), 950, threshold, grace))
	require.False(t, state.shouldExit(start.Add(15*time.Second), 899, threshold, grace))

	secondStart := start.Add(20 * time.Second)
	require.False(t, state.shouldExit(secondStart, 901, threshold, grace))
	require.False(t, state.shouldExit(secondStart.Add(14*time.Second), 901, threshold, grace))
	require.True(t, state.shouldExit(secondStart.Add(15*time.Second), 901, threshold, grace))
}
