package common

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiskReleasableRequestBodyReaderWriteToPreservesRemainingBytes(t *testing.T) {
	payload := bytes.Repeat([]byte("payload"), 1024)
	owner, err := NewDiskReleasableRequestBody(payload)
	require.NoError(t, err)
	reader, err := owner.Reader()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = reader.Close()
		owner.Release()
	})
	prefix := make([]byte, 17)
	_, err = io.ReadFull(reader, prefix)
	require.NoError(t, err)
	var output bytes.Buffer
	written, err := reader.WriteTo(&output)
	require.NoError(t, err)
	require.Equal(t, int64(len(payload)-len(prefix)), written)
	require.Equal(t, payload[len(prefix):], output.Bytes())
	_, err = reader.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

func BenchmarkDiskReleasableRequestBodyCopy(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 1<<20)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		owner, err := NewDiskReleasableRequestBody(payload)
		if err != nil {
			b.Fatal(err)
		}
		reader, err := owner.Reader()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, reader); err != nil {
			b.Fatal(err)
		}
		_ = reader.Close()
		owner.Release()
	}
}
