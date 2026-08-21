package common

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func withMemoryBodyStorageForTest(t *testing.T) {
	t.Helper()
	original := GetDiskCacheConfig()
	SetDiskCacheConfig(DiskCacheConfig{Enabled: false})
	t.Cleanup(func() { SetDiskCacheConfig(original) })
}

func TestCreateBodyStorageFromReaderKnownLengthPreservesPayload(t *testing.T) {
	withMemoryBodyStorageForTest(t)
	payload := bytes.Repeat([]byte("x"), 1<<20)

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), int64(len(payload)), int64(len(payload)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })

	stored, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, payload, stored)
}

func TestCreateBodyStorageFromReaderKnownLengthShortBodyPreservesPayload(t *testing.T) {
	withMemoryBodyStorageForTest(t)
	payload := []byte("small")

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), 11, 10)

	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })
	stored, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, payload, stored)
}

func TestCreateBodyStorageFromReaderKnownLengthRejectsActualOversize(t *testing.T) {
	withMemoryBodyStorageForTest(t)
	payload := bytes.Repeat([]byte("x"), 11)

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), 10, 10)

	require.Nil(t, storage)
	require.ErrorIs(t, err, ErrRequestBodyTooLarge)
}

func TestCreateBodyStorageFromReaderKnownLengthAcceptsShortBody(t *testing.T) {
	withMemoryBodyStorageForTest(t)
	payload := []byte("short")

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), 10, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })

	stored, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, payload, stored)
}

func TestCreateBodyStorageFromReaderKnownLengthAcceptsBodyLongerThanDeclared(t *testing.T) {
	withMemoryBodyStorageForTest(t)
	payload := []byte("longer")

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), 5, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })

	stored, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, payload, stored)
}

func TestCreateBodyStorageFromReaderUnknownLengthPreservesPayload(t *testing.T) {
	withMemoryBodyStorageForTest(t)
	payload := []byte("unknown")

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), -1, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })

	stored, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, payload, stored)
}

func TestCreateBodyStorageFromReaderUnknownLengthRejectsOversize(t *testing.T) {
	withMemoryBodyStorageForTest(t)
	payload := bytes.Repeat([]byte("x"), 11)

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), -1, 10)

	require.Nil(t, storage)
	require.ErrorIs(t, err, ErrRequestBodyTooLarge)
}

func BenchmarkCreateBodyStorageFromReaderKnownLength(b *testing.B) {
	original := GetDiskCacheConfig()
	SetDiskCacheConfig(DiskCacheConfig{Enabled: false})
	b.Cleanup(func() { SetDiskCacheConfig(original) })
	payload := bytes.Repeat([]byte("x"), 1<<20)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage, err := CreateBodyStorageFromReader(bytes.NewReader(payload), int64(len(payload)), int64(len(payload)))
		if err != nil {
			b.Fatal(err)
		}
		if err := storage.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
