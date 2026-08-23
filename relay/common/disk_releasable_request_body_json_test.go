package common

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/require"
)

func largeResponsesRequestForDiskJSONTest(size int) dto.OpenAIResponsesRequest {
	input := append([]byte(`[{"role":"user","content":"`), bytes.Repeat([]byte("x"), size)...)
	input = append(input, []byte(`"}]`)...)
	stream := true
	return dto.OpenAIResponsesRequest{Model: "gpt-5.4", Input: input, Stream: &stream}
}

func TestDiskReleasableRequestBodyFromJSONMatchesMarshalBytes(t *testing.T) {
	request := largeResponsesRequestForDiskJSONTest(1 << 20)
	want, err := appcommon.Marshal(request)
	require.NoError(t, err)

	body, err := NewDiskReleasableRequestBodyFromJSON(request)
	require.NoError(t, err)
	path := body.path
	reader, err := body.Reader()
	require.NoError(t, err)
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, int64(len(want)), reader.ContentLength())

	body.Release()
	require.NoError(t, reader.Close())
	require.Eventually(t, func() bool { _, err := os.Stat(path); return os.IsNotExist(err) }, time.Second, 10*time.Millisecond)
}

func BenchmarkDiskReleasableRequestBodyJSON(b *testing.B) {
	request := largeResponsesRequestForDiskJSONTest(1 << 20)
	b.Run("marshal_then_disk", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			data, err := appcommon.Marshal(request)
			if err != nil {
				b.Fatal(err)
			}
			body, err := NewDiskReleasableRequestBody(data)
			if err != nil {
				b.Fatal(err)
			}
			body.Release()
		}
	})
	b.Run("encode_directly_to_disk", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			body, err := NewDiskReleasableRequestBodyFromJSON(request)
			if err != nil {
				b.Fatal(err)
			}
			body.Release()
		}
	})
}
