package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type alphaResourceTransport func(*http.Request) (*http.Response, error)

func (f alphaResourceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type alphaResourceReadGate struct {
	io.ReadCloser
	response *http.Response
	entered  chan struct{}
	resume   chan struct{}
	once     sync.Once
}

func (r *alphaResourceReadGate) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.resume
	return r.ReadCloser.Read(p)
}

func withAlphaResourceClient(tb testing.TB, transport http.RoundTripper) {
	tb.Helper()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	client := service.GetHttpClient()
	original := *client
	*client = http.Client{Transport: transport, Timeout: 15 * time.Second}
	tb.Cleanup(func() {
		client.CloseIdleConnections()
		*client = original
	})
}

func newAlphaResourceContext(tb testing.TB, upstreamURL string, payload []byte) (*gin.Context, *relaycommon.RelayInfo, *dto.AlphaSearchRequest, common.BodyStorage, *httptest.ResponseRecorder) {
	tb.Helper()
	recorder := httptest.NewRecorder()
	ctx := &gin.Context{Request: httptest.NewRequest(http.MethodPost, "/v1/alpha/search", bytes.NewReader(payload))}
	wrapped, _ := gin.CreateTestContext(recorder)
	ctx.Writer = wrapped.Writer
	ctx.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(payload)
	require.NoError(tb, err)
	ctx.Set(common.KeyBodyStorage, storage)
	request, err := helper.GetAndValidateAlphaSearchRequest(ctx)
	require.NoError(tb, err)
	common.SetContextKey(ctx, constant.ContextKeyOpenAIAlphaSearchRequest, request)
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstreamURL)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "test-key")
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "alpha-test")
	info := &relaycommon.RelayInfo{
		Request: request, OriginModelName: "alpha-test", DisablePing: true,
		RelayFormat:         types.RelayFormatOpenAIAlphaSearch,
		RelayMode:           relayconstant.Path2RelayMode("/v1/alpha/search"),
		RequestURLPath:      "/v1/alpha/search",
		BillingRequestInput: &billingexpr.RequestInput{},
	}
	return ctx, info, request, storage, recorder
}

func TestAlphaSearchHelperReleasesInputBeforeCopyingSuccessfulResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, backend := range []struct {
		name string
		disk bool
	}{{"memory", false}, {"disk", true}} {
		t.Run(backend.name, func(t *testing.T) {
			originalConfig := common.GetDiskCacheConfig()
			common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: backend.disk, ThresholdMB: 0, MaxSizeMB: 128, Path: t.TempDir()})
			t.Cleanup(func() { common.SetDiskCacheConfig(originalConfig) })
			payload := []byte(`{"model":"alpha-test","future":"` + strings.Repeat("x", 1<<20) + `","exact":18446744073709551615}`)
			requestMatches := make(chan bool, 1)
			const partialResponse = "partial search response"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requestMatches <- err == nil && bytes.Equal(body, payload)
				w.Header().Set("Content-Type", "application/json")
				// A truncated success response exercises the real copy path, then
				// returns before settlement. Billing is tested independently.
				w.Header().Set("Content-Length", strconv.Itoa(len(partialResponse)+1))
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, partialResponse)
			}))
			t.Cleanup(upstream.Close)
			transport := http.DefaultTransport.(*http.Transport).Clone()
			t.Cleanup(transport.CloseIdleConnections)
			gateReady := make(chan *alphaResourceReadGate, 1)
			withAlphaResourceClient(t, alphaResourceTransport(func(r *http.Request) (*http.Response, error) {
				response, err := transport.RoundTrip(r)
				if err != nil {
					return response, err
				}
				gate := &alphaResourceReadGate{ReadCloser: response.Body, response: response, entered: make(chan struct{}), resume: make(chan struct{})}
				response.Body = gate
				gateReady <- gate
				return response, nil
			}))
			ctx, info, request, storage, recorder := newAlphaResourceContext(t, upstream.URL, payload)
			billingInput := info.BillingRequestInput
			t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
			done := make(chan *types.NewAPIError, 1)
			go func() { done <- AlphaSearchHelper(ctx, info) }()
			var gate *alphaResourceReadGate
			select {
			case gate = <-gateReady:
			case err := <-done:
				t.Fatalf("helper exited before receiving response: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("upstream did not return headers")
			}
			// Always release the blocked read and join the helper, including on
			// assertion failure, before restoring global client/config state.
			joined := false
			t.Cleanup(func() {
				select {
				case <-gate.resume:
				default:
					close(gate.resume)
				}
				if !joined {
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("helper did not finish after releasing response")
					}
				}
			})
			select {
			case <-gate.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("helper did not start copying the response")
			}
			require.True(t, <-requestMatches)
			_, bodyErr := storage.Bytes()
			require.ErrorIs(t, bodyErr, common.ErrStorageClosed)
			require.Nil(t, request.RawBody)
			require.Nil(t, info.Request)
			cached, _ := ctx.Get(string(constant.ContextKeyOpenAIAlphaSearchRequest))
			require.Nil(t, cached)
			require.Same(t, billingInput, info.BillingRequestInput)
			require.Nil(t, gate.response.Request.Body)
			require.Nil(t, gate.response.Request.GetBody)
			close(gate.resume)
			select {
			case apiErr := <-done:
				joined = true
				require.NotNil(t, apiErr)
				require.Equal(t, partialResponse, recorder.Body.String())
			case <-time.After(5 * time.Second):
				t.Fatal("helper did not propagate truncated response error")
			}
		})
	}
}

func TestAlphaSearchHelperPreservesInputForRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, backend := range []struct {
		name string
		disk bool
	}{{"memory", false}, {"disk", true}} {
		t.Run(backend.name, func(t *testing.T) {
			originalConfig := common.GetDiskCacheConfig()
			common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: backend.disk, ThresholdMB: 0, MaxSizeMB: 128, Path: t.TempDir()})
			t.Cleanup(func() { common.SetDiskCacheConfig(originalConfig) })
			for _, status := range []int{0, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
				t.Run(strconv.Itoa(status), func(t *testing.T) {
					payload := []byte(`{"model":"alpha-test","future":"` + strings.Repeat("x", 4096) + `","exact":18446744073709551615}`)
					ctx, info, request, storage, _ := newAlphaResourceContext(t, "http://alpha-retry.invalid", payload)
					t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
					billingInput := info.BillingRequestInput
					originalFiles := common.GetDiskCacheStats().ActiveDiskFiles
					attempts := 0
					withAlphaResourceClient(t, alphaResourceTransport(func(r *http.Request) (*http.Response, error) {
						defer r.Body.Close()
						body, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						require.Equal(t, payload, body)
						attempts++
						if status == 0 {
							return nil, io.ErrUnexpectedEOF
						}
						return &http.Response{
							StatusCode: status,
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"retry later","type":"upstream_error"}}`)),
							Request:    r,
						}, nil
					}))
					for attempt := 0; attempt < 2; attempt++ {
						require.NotNil(t, AlphaSearchHelper(ctx, info))
						require.Same(t, request, info.Request)
						require.True(t, bytes.Equal(payload, request.RawBody), "original request bytes changed")
						cached, _ := ctx.Get(string(constant.ContextKeyOpenAIAlphaSearchRequest))
						require.Same(t, request, cached)
						require.Same(t, billingInput, info.BillingRequestInput)
						body, err := storage.Bytes()
						require.NoError(t, err)
						require.Equal(t, payload, body)
						require.Equal(t, originalFiles, common.GetDiskCacheStats().ActiveDiskFiles)
					}
					require.Equal(t, 2, attempts)
				})
			}
		})
	}
}
