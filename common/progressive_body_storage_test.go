package common

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type spillObservingReader struct {
	data      []byte
	offset    int
	chunkSize int
	threshold int
	sawDisk   bool
}

func (r *spillObservingReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	if r.offset >= r.threshold {
		if count, _, err := GetDiskCacheInfo(); err == nil && count > 0 {
			r.sawDisk = true
		}
	}
	n := len(r.data) - r.offset
	if n > len(p) {
		n = len(p)
	}
	if n > r.chunkSize {
		n = r.chunkSize
	}
	copy(p[:n], r.data[r.offset:r.offset+n])
	r.offset += n
	return n, nil
}

func withDiskCacheForProgressiveBodyTest(t *testing.T) {
	t.Helper()
	original := GetDiskCacheConfig()
	SetDiskCacheConfig(DiskCacheConfig{
		Enabled:     true,
		ThresholdMB: 1,
		MaxSizeMB:   64,
		Path:        t.TempDir(),
	})
	ResetDiskCacheUsage()
	t.Cleanup(func() {
		SetDiskCacheConfig(original)
		ResetDiskCacheUsage()
	})
}

func TestCreateBodyStorageFromReaderUnknownLengthSpillsBeforeEOF(t *testing.T) {
	withDiskCacheForProgressiveBodyTest(t)
	payload := bytes.Repeat([]byte("x"), 4<<20)
	reader := &spillObservingReader{data: payload, chunkSize: 64 << 10, threshold: 1 << 20}

	storage, err := CreateBodyStorageFromReader(reader, -1, 8<<20)

	require.NoError(t, err)
	require.True(t, reader.sawDisk, "unknown-length body should spill once it crosses the disk threshold")
	require.True(t, storage.IsDisk())
	stored, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, payload, stored)
	require.NoError(t, storage.Close())
	count, size, err := GetDiskCacheInfo()
	require.NoError(t, err)
	require.Zero(t, count)
	require.Zero(t, size)
}

func TestCreateBodyStorageFromReaderUnknownLengthKeepsSmallBodyInMemory(t *testing.T) {
	withDiskCacheForProgressiveBodyTest(t)
	payload := bytes.Repeat([]byte("x"), 512<<10)

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), -1, 8<<20)

	require.NoError(t, err)
	require.False(t, storage.IsDisk())
	stored, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, payload, stored)
	require.NoError(t, storage.Close())
	count, size, err := GetDiskCacheInfo()
	require.NoError(t, err)
	require.Zero(t, count)
	require.Zero(t, size)
}

func TestCreateBodyStorageFromReaderUnknownLengthRejectsOversizeAfterSpill(t *testing.T) {
	withDiskCacheForProgressiveBodyTest(t)
	payload := bytes.Repeat([]byte("x"), 3<<20)

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), -1, 2<<20)

	require.Nil(t, storage)
	require.ErrorIs(t, err, ErrRequestBodyTooLarge)
	count, size, statErr := GetDiskCacheInfo()
	require.NoError(t, statErr)
	require.Zero(t, count)
	require.Zero(t, size)
}

func BenchmarkCreateBodyStorageFromReaderUnknownLengthDisk(b *testing.B) {
	original := GetDiskCacheConfig()
	SetDiskCacheConfig(DiskCacheConfig{
		Enabled:     true,
		ThresholdMB: 1,
		MaxSizeMB:   1024,
		Path:        b.TempDir(),
	})
	ResetDiskCacheUsage()
	b.Cleanup(func() {
		SetDiskCacheConfig(original)
		ResetDiskCacheUsage()
	})
	payload := bytes.Repeat([]byte("x"), 4<<20)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), -1, 8<<20)
		if err != nil {
			b.Fatal(err)
		}
		if err := storage.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
