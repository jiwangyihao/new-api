package relay

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type trackedResponseRequestBody struct {
	*bytes.Reader
	closed bool
}

func (b *trackedResponseRequestBody) Close() error {
	b.closed = true
	return nil
}

func TestReleaseResponsesRequestResourcesDropsLongLivedRequestReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	payload := bytes.Repeat([]byte("x"), 1<<20)
	storage, err := common.CreateBodyStorage(payload)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	c.Set(common.KeyRequestBody, payload)
	regularRequest := &dto.OpenAIResponsesRequest{Model: "gpt-5.4", Input: payload}
	compactRequest := &dto.OpenAIResponsesCompactionRequest{Model: "gpt-5.4", Input: payload}
	c.Set(string(constant.ContextKeyOpenAIResponsesRequest), regularRequest)
	c.Set(string(constant.ContextKeyOpenAIResponsesCompactionRequest), compactRequest)
	billingInput := &billingexpr.RequestInput{}
	info := &relaycommon.RelayInfo{Request: regularRequest, BillingRequestInput: billingInput}
	requestBody := &trackedResponseRequestBody{Reader: bytes.NewReader(payload)}
	httpResp := &http.Response{
		Request: &http.Request{
			Body:    requestBody,
			GetBody: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(payload)), nil },
		},
	}

	releaseResponsesRequestResources(c, info, httpResp)

	bodyStorage, exists := c.Get(common.KeyBodyStorage)
	require.True(t, exists)
	require.Nil(t, bodyStorage)
	legacyBody, exists := c.Get(common.KeyRequestBody)
	require.True(t, exists)
	require.Nil(t, legacyBody)
	cachedRegular, exists := c.Get(string(constant.ContextKeyOpenAIResponsesRequest))
	require.True(t, exists)
	require.Nil(t, cachedRegular)
	cachedCompact, exists := c.Get(string(constant.ContextKeyOpenAIResponsesCompactionRequest))
	require.True(t, exists)
	require.Nil(t, cachedCompact)
	require.Nil(t, info.Request)
	require.Same(t, billingInput, info.BillingRequestInput)
	require.True(t, requestBody.closed)
	require.Nil(t, httpResp.Request.Body)
	require.Nil(t, httpResp.Request.GetBody)
}

func newResponsesResourceTestContext(tb testing.TB, upstreamURL string, payload []byte) (*gin.Context, *dto.OpenAIResponsesRequest, common.BodyStorage) {
	tb.Helper()
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(payload)
	require.NoError(tb, err)
	c.Set(common.KeyBodyStorage, storage)
	request := &dto.OpenAIResponsesRequest{Model: "gpt-5.4", Input: payload}
	common.SetContextKey(c, constant.ContextKeyOpenAIResponsesRequest, request)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstreamURL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-5.4")
	return c, request, storage
}

func newResponsesResourceTestInfo(request *dto.OpenAIResponsesRequest) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		Request:         request,
		RelayMode:       relayconstant.RelayModeResponses,
		RequestURLPath:  "/v1/responses",
		OriginModelName: "gpt-5.4",
		IsStream:        true,
		DisablePing:     true,
		FreeModel:       true,
		StreamStatus:    relaycommon.NewStreamStatus(),
	}
}

func TestResponsesHelperReleasesRequestResourcesWhileUpstreamStreamIsOpen(t *testing.T) {
	originalLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = originalLogConsumeEnabled })
	headersSent := make(chan struct{})
	releaseBody := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		close(headersSent)
		<-releaseBody
	}))
	t.Cleanup(func() {
		select {
		case <-releaseBody:
		default:
			close(releaseBody)
		}
		upstream.Close()
	})

	payload := append([]byte(`{"model":"gpt-5.4","stream":true,"input":`), bytes.Repeat([]byte(" "), 1<<20)...)
	payload = append(payload, []byte(`[]}`)...)
	c, request, storage := newResponsesResourceTestContext(t, upstream.URL, payload)
	requestCtx, cancelRequest := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestCtx)
	info := newResponsesResourceTestInfo(request)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = ResponsesHelper(c, info)
	}()

	select {
	case <-headersSent:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream response headers were not received")
	}
	require.Eventually(t, func() bool {
		_, ok := common.GetContextKeyType[*dto.OpenAIResponsesRequest](c, constant.ContextKeyOpenAIResponsesRequest)
		return !ok && info.Request == nil
	}, time.Second, 10*time.Millisecond)
	_, err := storage.Bytes()
	require.ErrorIs(t, err, common.ErrStorageClosed)

	// The observable contract is release-before-stream-completion. Let the
	// helper unwind asynchronously; settlement behavior is covered elsewhere.
	cancelRequest()
	close(releaseBody)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ResponsesHelper did not finish after request cancellation")
	}
}

func TestResponsesHelperKeepsRequestResourcesWhenDoRequestFails(t *testing.T) {
	service.InitHttpClient()
	payload := []byte(`{"model":"gpt-5.4","stream":true,"input":[]}`)
	c, request, storage := newResponsesResourceTestContext(t, "http://127.0.0.1:1", payload)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	info := newResponsesResourceTestInfo(request)

	apiErr := ResponsesHelper(c, info)

	require.NotNil(t, apiErr)
	require.Same(t, request, info.Request)
	cached, ok := common.GetContextKeyType[*dto.OpenAIResponsesRequest](c, constant.ContextKeyOpenAIResponsesRequest)
	require.True(t, ok)
	require.Same(t, request, cached)
	_, err := storage.Bytes()
	require.NoError(t, err)
}

func BenchmarkReleaseResponsesRequestResources(b *testing.B) {
	gin.SetMode(gin.TestMode)
	payload := bytes.Repeat([]byte("x"), 1<<20)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))

	for b.Loop() {
		ownedPayload := bytes.Clone(payload)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		storage, err := common.CreateBodyStorage(ownedPayload)
		if err != nil {
			b.Fatal(err)
		}
		c.Set(common.KeyBodyStorage, storage)
		request := &dto.OpenAIResponsesRequest{Model: "gpt-5.4", Input: bytes.Clone(ownedPayload)}
		common.SetContextKey(c, constant.ContextKeyOpenAIResponsesRequest, request)
		info := &relaycommon.RelayInfo{Request: request}
		httpResp := &http.Response{Request: &http.Request{Body: io.NopCloser(bytes.NewReader(bytes.Clone(ownedPayload)))}}

		releaseResponsesRequestResources(c, info, httpResp)
	}
}
