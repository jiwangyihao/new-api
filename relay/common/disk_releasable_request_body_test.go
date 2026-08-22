package common

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiskReleasableRequestBodyReplaysAndDeletesAfterLastReader(t *testing.T) {
	payload := bytes.Repeat([]byte("request-body"), 1024)
	body, err := NewDiskReleasableRequestBody(payload)
	require.NoError(t, err)
	path := body.path
	primary, err := body.Reader()
	require.NoError(t, err)
	replay, err := primary.GetBody()
	require.NoError(t, err)

	primaryData, err := io.ReadAll(primary)
	require.NoError(t, err)
	replayData, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, payload, primaryData)
	require.Equal(t, payload, replayData)

	body.Release()
	_, statErr := os.Stat(path)
	require.NoError(t, statErr)
	require.NoError(t, primary.Close())
	_, statErr = os.Stat(path)
	require.NoError(t, statErr)
	require.NoError(t, replay.Close())
	require.Eventually(t, func() bool { _, err := os.Stat(path); return os.IsNotExist(err) }, time.Second, 10*time.Millisecond)
	_, err = primary.GetBody()
	require.ErrorIs(t, err, errReleasableRequestBodyReleased)
}

func TestDiskReleasableRequestBodyKeepsFileUntilRelease(t *testing.T) {
	body, err := NewDiskReleasableRequestBody([]byte("retry-body"))
	require.NoError(t, err)
	path := body.path
	reader, err := body.Reader()
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	_, err = os.Stat(path)
	require.NoError(t, err)
	replay, err := body.Reader()
	require.NoError(t, err)
	data, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, "retry-body", string(data))
	require.NoError(t, replay.Close())
	body.Release()
	require.Eventually(t, func() bool { _, err := os.Stat(path); return os.IsNotExist(err) }, time.Second, 10*time.Millisecond)
}

func TestDiskReleasableRequestBodyFailureCleanupDeletesFile(t *testing.T) {
	body, err := NewDiskReleasableRequestBody([]byte("failed-request"))
	require.NoError(t, err)
	path := body.path
	reader, err := body.Reader()
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	reader.Release()
	require.Eventually(t, func() bool { _, err := os.Stat(path); return os.IsNotExist(err) }, time.Second, 10*time.Millisecond)
}

func retainedHeapAfterDiskRequestBodies(t *testing.T, count, payloadSize int) uint64 {
	t.Helper()
	payload := bytes.Repeat([]byte("x"), payloadSize)
	bodies := make([]*DiskReleasableRequestBody, 0, count)
	readers := make([]*DiskReleasableRequestBodyReader, 0, count)
	for i := 0; i < count; i++ {
		body, err := NewDiskReleasableRequestBody(payload)
		require.NoError(t, err)
		reader, err := body.Reader()
		require.NoError(t, err)
		bodies = append(bodies, body)
		readers = append(readers, reader)
	}
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	for i, body := range bodies {
		body.Release()
		_ = readers[i].Close()
	}
	runtime.KeepAlive(bodies)
	runtime.KeepAlive(readers)
	return stats.HeapAlloc
}

func retainedHeapAfterMemoryRequestBodies(count, payloadSize int) uint64 {
	bodies := make([]*ReleasableRequestBody, 0, count)
	readers := make([]*ReleasableRequestBodyReader, 0, count)
	for i := 0; i < count; i++ {
		payload := bytes.Repeat([]byte("x"), payloadSize)
		body := NewReleasableRequestBody(payload)
		bodies = append(bodies, body)
		readers = append(readers, body.Reader())
	}
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	for i, body := range bodies {
		body.Release()
		_ = readers[i].Close()
	}
	runtime.KeepAlive(bodies)
	runtime.KeepAlive(readers)
	return stats.HeapAlloc
}

func heapDelta(current, baseline uint64) uint64 {
	if current <= baseline {
		return 0
	}
	return current - baseline
}

func TestDiskReleasableRequestBodyUsesLessRetainedHeap(t *testing.T) {
	const count = 16
	const payloadSize = 1 << 20
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	memoryHeap := retainedHeapAfterMemoryRequestBodies(count, payloadSize)
	runtime.GC()
	diskHeap := retainedHeapAfterDiskRequestBodies(t, count, payloadSize)
	require.Greater(t, heapDelta(memoryHeap, baseline.HeapAlloc), uint64(12<<20))
	require.Less(t, heapDelta(diskHeap, baseline.HeapAlloc), uint64(4<<20))
}
