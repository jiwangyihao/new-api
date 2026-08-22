package common

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestReleasableRequestBodyDropsBackingDataAfterReadersClose(t *testing.T) {
	body := NewReleasableRequestBody([]byte("request-body"))
	reader := body.Reader()
	replay, err := reader.GetBody()
	require.NoError(t, err)

	body.Release()
	require.NotNil(t, body.data)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "request-body", string(data))
	require.NoError(t, reader.Close())
	require.NotNil(t, body.data)
	require.NoError(t, replay.Close())
	require.Nil(t, body.data)
	_, err = reader.GetBody()
	require.ErrorIs(t, err, errReleasableRequestBodyReleased)
}

func TestReleasableRequestBodyReleaseWithoutReaderDropsImmediately(t *testing.T) {
	body := NewReleasableRequestBody([]byte("request-body"))
	body.Release()
	require.Nil(t, body.data)
}

func withAdaptiveReplayDiskCache(t *testing.T, thresholdMB int) {
	t.Helper()
	original := appcommon.GetDiskCacheConfig()
	appcommon.SetDiskCacheConfig(appcommon.DiskCacheConfig{
		Enabled:     true,
		ThresholdMB: thresholdMB,
		MaxSizeMB:   1024,
		Path:        t.TempDir(),
	})
	appcommon.ResetDiskCacheUsage()
	inFlightReplayMemoryBytes.Store(0)
	t.Cleanup(func() {
		appcommon.SetDiskCacheConfig(original)
		appcommon.ResetDiskCacheUsage()
		inFlightReplayMemoryBytes.Store(0)
	})
}

func TestAdaptiveReplayBodySpillsWhenAggregateBudgetWouldBeExceeded(t *testing.T) {
	withAdaptiveReplayDiskCache(t, 1)
	payload := bytes.Repeat([]byte("x"), 600<<10)
	first := NewAdaptiveReplayableRequestBody(bytes.Clone(payload))
	second := NewAdaptiveReplayableRequestBody(bytes.Clone(payload))

	_, firstInMemory := first.(*ReleasableRequestBodyReader)
	_, secondOnDisk := second.(*DiskReleasableRequestBodyReader)
	require.True(t, firstInMemory)
	require.True(t, secondOnDisk)
	require.Equal(t, int64(len(payload)), inFlightReplayMemoryBytes.Load())

	first.Release()
	require.NoError(t, first.Close())
	second.Release()
	require.NoError(t, second.Close())
	require.Eventually(t, func() bool {
		return inFlightReplayMemoryBytes.Load() == 0 && appcommon.GetDiskCacheStats().ActiveDiskFiles == 0
	}, time.Second, 10*time.Millisecond)
}

func TestAdaptiveReplayMemoryReservationIsConcurrencySafe(t *testing.T) {
	withAdaptiveReplayDiskCache(t, 1)
	const count = 8
	payload := bytes.Repeat([]byte("x"), 256<<10)
	readers := make([]ReplayableRequestBodyReader, count)
	var wg sync.WaitGroup
	for i := range readers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			readers[index] = NewAdaptiveReplayableRequestBody(bytes.Clone(payload))
		}(i)
	}
	wg.Wait()

	memoryReaders := 0
	for _, reader := range readers {
		if _, ok := reader.(*ReleasableRequestBodyReader); ok {
			memoryReaders++
		}
	}
	require.LessOrEqual(t, int64(memoryReaders*len(payload)), int64(1<<20))
	require.Equal(t, int64(memoryReaders*len(payload)), inFlightReplayMemoryBytes.Load())

	for _, reader := range readers {
		reader.Release()
		require.NoError(t, reader.Close())
	}
	require.Eventually(t, func() bool {
		return inFlightReplayMemoryBytes.Load() == 0 && appcommon.GetDiskCacheStats().ActiveDiskFiles == 0
	}, time.Second, 10*time.Millisecond)
}
