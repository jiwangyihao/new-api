package common

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type writerOnly struct {
	io.Writer
}

func TestReleasableRequestBodyReaderWriteToPreservesRemainingBytes(t *testing.T) {
	payload := bytes.Repeat([]byte("payload"), 1024)
	owner := NewReleasableRequestBody(payload)
	reader := owner.Reader()
	t.Cleanup(func() {
		_ = reader.Close()
		owner.Release()
	})
	prefix := make([]byte, 17)
	_, err := io.ReadFull(reader, prefix)
	require.NoError(t, err)
	var output bytes.Buffer
	written, err := reader.WriteTo(&output)
	require.NoError(t, err)
	require.Equal(t, int64(len(payload)-len(prefix)), written)
	require.Equal(t, payload[len(prefix):], output.Bytes())
	_, err = reader.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

func BenchmarkReleasableRequestBodyCopy(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 1<<20)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		owner := NewReleasableRequestBody(payload)
		reader := owner.Reader()
		if _, err := io.Copy(io.Discard, reader); err != nil {
			b.Fatal(err)
		}
		_ = reader.Close()
		owner.Release()
	}
}

func BenchmarkReleasableRequestBodyCopyBufferBaseline(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 1<<20)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		owner := NewReleasableRequestBody(payload)
		reader := owner.Reader()
		if _, err := io.Copy(writerOnly{Writer: io.Discard}, struct{ io.Reader }{Reader: reader}); err != nil {
			b.Fatal(err)
		}
		_ = reader.Close()
		owner.Release()
	}
}
