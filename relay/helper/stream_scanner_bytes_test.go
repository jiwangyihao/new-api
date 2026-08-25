package helper

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamScannerBytesHandlerPreservesOrderAndDoneSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := strings.Join([]string{
		`data:   {"id":1}   `,
		`event: ignored`,
		`data: {"id":2}`,
		`data: [DONE]`,
		`data: {"id":3}`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	var got []string

	StreamScannerBytesHandler(c, resp, info, func(data []byte, _ *StreamResult) {
		got = append(got, string(data))
	})

	require.Equal(t, []string{`{"id":1}`, `{"id":2}`}, got)
	require.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
}

func TestStreamScannerBytesHandlerDoesNotLeakPreviousPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := "data: first-secret\ndata: x\ndata: [DONE]\n"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	var got []string

	StreamScannerBytesHandler(c, resp, info, func(data []byte, _ *StreamResult) {
		got = append(got, string(bytes.Clone(data)))
	})

	require.Equal(t, []string{"first-secret", "x"}, got)
}

func TestStreamScannerBytesHandlerReleasesQueuedPayloadsAfterStop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body strings.Builder
	for i := 0; i < 256; i++ {
		body.WriteString("data: payload-")
		body.WriteString(strings.Repeat("x", 1024))
		body.WriteByte('\n')
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body.String()))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	var count int

	StreamScannerBytesHandler(c, resp, info, func([]byte, *StreamResult) {
		count++
	})

	require.Equal(t, 256, count)
}

func TestStreamScannerBytesHandlerDrainsQueuedPayloadsAfterStop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body strings.Builder
	for i := 0; i < 256; i++ {
		body.WriteString("data: payload-")
		body.WriteString(strings.Repeat("x", 1024))
		body.WriteByte('\n')
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body.String()))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	var count int

	StreamScannerBytesHandler(c, resp, info, func(_ []byte, sr *StreamResult) {
		count++
		if count == 1 {
			sr.Stop(io.EOF)
		}
	})

	require.Equal(t, 1, count)
	require.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
}

func BenchmarkStreamScannerBytesPayload(b *testing.B) {
	gin.SetMode(gin.TestMode)
	oldWriter := gin.DefaultWriter
	oldErrorWriter := gin.DefaultErrorWriter
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard
	b.Cleanup(func() {
		gin.DefaultWriter = oldWriter
		gin.DefaultErrorWriter = oldErrorWriter
	})
	payload := strings.Repeat("x", 4<<10)
	body := []byte("data: " + payload + "\ndata: [DONE]\n")
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{Body: io.NopCloser(bytes.NewReader(body))}
		StreamScannerBytesHandler(c, resp, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, func([]byte, *StreamResult) {})
	}
}

func BenchmarkStreamScannerStringPayload(b *testing.B) {
	gin.SetMode(gin.TestMode)
	oldWriter := gin.DefaultWriter
	oldErrorWriter := gin.DefaultErrorWriter
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard
	b.Cleanup(func() {
		gin.DefaultWriter = oldWriter
		gin.DefaultErrorWriter = oldErrorWriter
	})
	payload := strings.Repeat("x", 4<<10)
	body := []byte("data: " + payload + "\ndata: [DONE]\n")
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{Body: io.NopCloser(bytes.NewReader(body))}
		StreamScannerHandler(c, resp, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, func(string, *StreamResult) {})
	}
}
