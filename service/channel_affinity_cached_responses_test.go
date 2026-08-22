package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type affinityBodyStorageSpy struct {
	data       []byte
	reader     *bytes.Reader
	bytesCalls atomic.Int64
}

func newAffinityBodyStorageSpy(data []byte) *affinityBodyStorageSpy {
	return &affinityBodyStorageSpy{data: data, reader: bytes.NewReader(data)}
}

func (s *affinityBodyStorageSpy) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *affinityBodyStorageSpy) Seek(offset int64, whence int) (int64, error) {
	return s.reader.Seek(offset, whence)
}

func (s *affinityBodyStorageSpy) Close() error { return nil }

func (s *affinityBodyStorageSpy) Bytes() ([]byte, error) {
	s.bytesCalls.Add(1)
	return bytes.Clone(s.data), nil
}

func (s *affinityBodyStorageSpy) Size() int64 { return int64(len(s.data)) }

func (s *affinityBodyStorageSpy) IsDisk() bool { return true }

func newCachedResponsesAffinityContext(body []byte, promptCacheKey json.RawMessage) (*gin.Context, *affinityBodyStorageSpy) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	storage := newAffinityBodyStorageSpy(body)
	c.Set(common.KeyBodyStorage, common.BodyStorage(storage))
	common.SetContextKey(c, constant.ContextKeyOpenAIResponsesRequest, &dto.OpenAIResponsesRequest{
		Model:          "gpt-5",
		PromptCacheKey: promptCacheKey,
	})
	return c, storage
}

func TestExtractChannelAffinityValueUsesCachedResponsesPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	source := operation_setting.ChannelAffinityKeySource{Type: "gjson", Path: "prompt_cache_key"}
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{name: "string", raw: json.RawMessage(`"cache\u002dkey"`), want: "cache-key"},
		{name: "number", raw: json.RawMessage(`123.5`), want: "123.5"},
		{name: "true", raw: json.RawMessage(`true`), want: "true"},
		{name: "false", raw: json.RawMessage(`false`), want: "false"},
		{name: "object", raw: json.RawMessage(`{"tenant":"alpha"}`), want: `{"tenant":"alpha"}`},
		{name: "null", raw: json.RawMessage(`null`), want: "null"},
		{name: "missing", raw: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, storage := newCachedResponsesAffinityContext(
				[]byte(`{"prompt_cache_key":"body-value","metadata":{"user_id":"user-1"}}`),
				tt.raw,
			)

			require.Equal(t, tt.want, extractChannelAffinityValue(c, source))
			require.Zero(t, storage.bytesCalls.Load(), "cached prompt_cache_key must not materialize BodyStorage")
		})
	}
}

func TestExtractChannelAffinityValueOtherPathStillReadsBodyStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, storage := newCachedResponsesAffinityContext(
		[]byte(`{"prompt_cache_key":"body-value","metadata":{"user_id":"user-1"}}`),
		json.RawMessage(`"cached-value"`),
	)

	got := extractChannelAffinityValue(c, operation_setting.ChannelAffinityKeySource{
		Type: "gjson",
		Path: "metadata.user_id",
	})

	require.Equal(t, "user-1", got)
	require.EqualValues(t, 1, storage.bytesCalls.Load())
}

func BenchmarkExtractChannelAffinityValueCachedPromptCacheKey(b *testing.B) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"input":"` + string(bytes.Repeat([]byte("x"), 1<<20)) + `","prompt_cache_key":"body-value"}`)
	c, storage := newCachedResponsesAffinityContext(body, json.RawMessage(`"cached-value"`))
	source := operation_setting.ChannelAffinityKeySource{Type: "gjson", Path: "prompt_cache_key"}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := extractChannelAffinityValue(c, source); got != "cached-value" {
			b.Fatalf("unexpected affinity value %q", got)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(storage.bytesCalls.Load())/float64(b.N), "body_reads/op")
}
