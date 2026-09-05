package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type alphaPreludeStorage struct {
	common.BodyStorage
	bytesCalls int
	readCalls  int
}

func (s *alphaPreludeStorage) Bytes() ([]byte, error) {
	s.bytesCalls++
	return s.BodyStorage.Bytes()
}

func (s *alphaPreludeStorage) Read(p []byte) (int, error) {
	s.readCalls++
	return s.BodyStorage.Read(p)
}

func newAlphaPreludeContext(tb testing.TB, body []byte, disk bool) (*gin.Context, *alphaPreludeStorage) {
	tb.Helper()
	original := common.GetDiskCacheConfig()
	common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 128, Path: tb.TempDir()})
	storage, err := common.CreateBodyStorage(body)
	common.SetDiskCacheConfig(original)
	require.NoError(tb, err)
	tb.Cleanup(func() { require.NoError(tb, storage.Close()) })
	require.Equal(tb, disk, storage.IsDisk())
	counted := &alphaPreludeStorage{BodyStorage: storage}
	ctx := &gin.Context{Request: httptest.NewRequest(http.MethodPost, "/v1/alpha/search", bytes.NewReader(body))}
	ctx.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
	ctx.Set(common.KeyBodyStorage, counted)
	return ctx, counted
}

func TestAlphaModelPreludeAndValidationUseSingleDecode(t *testing.T) {
	for _, backend := range []struct {
		name string
		disk bool
	}{{"memory", false}, {"disk", true}} {
		t.Run(backend.name, func(t *testing.T) {
			for _, body := range []string{
				`{"model":"gpt-alpha","group":"vip","id":"req-1","stream":false,"max_output_tokens":0,"future":18446744073709551615}`,
				`{"model":"old","\u006dodel":"gpt-alpha","group":"first","group":"vip","id":null,"stream":null,"max_output_tokens":null}`,
				"{\n\t\"model\":\"gpt-alpha\",\"commands\":{\"search_query\":[{\"q\":\"<>&查询\"}]},\"group\":null\n}",
			} {
				payload := []byte(body)
				ctx, storage := newAlphaPreludeContext(t, payload, backend.disk)
				var wantModel ModelRequest
				var wantRequest dto.AlphaSearchRequest
				require.NoError(t, common.Unmarshal(payload, &wantModel))
				require.NoError(t, common.Unmarshal(payload, &wantRequest))
				wantRequest.RawBody = payload

				modelRequest, err := getModelFromRequest(ctx)
				require.NoError(t, err)
				require.Equal(t, wantModel, *modelRequest)
				reads, materializations := storage.readCalls, storage.bytesCalls
				request, err := helper.GetAndValidateAlphaSearchRequest(ctx)
				require.NoError(t, err)
				require.Equal(t, wantRequest, *request)
				require.Equal(t, materializations, storage.bytesCalls, "validation must reuse prelude bytes")
				require.Equal(t, reads, storage.readCalls, "validation must not re-read storage")
				require.Equal(t, 1, storage.bytesCalls)
				require.Zero(t, storage.readCalls, "prelude must decode the required RawBody without a separate disk Decoder")
				replay, err := io.ReadAll(ctx.Request.Body)
				require.NoError(t, err)
				require.Equal(t, payload, replay)
			}
		})
	}
}

func TestAlphaModelPreludePreservesValidationErrorStage(t *testing.T) {
	for _, disk := range []bool{false, true} {
		for _, body := range []string{
			`{"model":"gpt-alpha","group":"vip","stream":"invalid"}`,
			`{"model":"gpt-alpha","id":123}`,
			`{"model":"gpt-alpha","max_output_tokens":-1}`,
			`{"model":"gpt-alpha","max_output_tokens":2147483647}`,
			`{"group":"vip"}`,
		} {
			ctx, _ := newAlphaPreludeContext(t, []byte(body), disk)
			directCtx, _ := newAlphaPreludeContext(t, []byte(body), disk)
			_, directErr := helper.GetAndValidateAlphaSearchRequest(directCtx)
			require.Error(t, directErr)
			var want ModelRequest
			require.NoError(t, common.Unmarshal([]byte(body), &want))

			modelRequest, preludeErr := getModelFromRequest(ctx)
			require.NoError(t, preludeErr, "non-routing field errors belong to validation")
			require.Equal(t, want, *modelRequest)
			_, validationErr := helper.GetAndValidateAlphaSearchRequest(ctx)
			require.EqualError(t, validationErr, directErr.Error())
		}
	}
}

func TestAlphaModelPreludeRejectsInvalidRoutingFields(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, body := range []string{`{"model":7}`, `{"model":"gpt-alpha","group":4}`, `{"model":`, `{"model":"gpt-alpha"} {}`} {
		for _, disk := range []bool{false, true} {
			ctx, _ := newAlphaPreludeContext(t, []byte(body), disk)
			_, err := getModelFromRequest(ctx)
			require.Error(t, err)
		}
	}
}

func BenchmarkAlphaPreludeAndValidation(b *testing.B) {
	for _, tc := range []struct {
		name string
		disk bool
		size int
	}{{"memory_4KiB", false, 4 << 10}, {"disk_4KiB", true, 4 << 10}, {"memory_1MiB", false, 1 << 20}, {"disk_1MiB", true, 1 << 20}} {
		b.Run(tc.name, func(b *testing.B) {
			payload := []byte(`{"model":"gpt-alpha","group":"vip","stream":false,"max_output_tokens":0,"future":"` + strings.Repeat("x", tc.size) + `"}`)
			template, storage := newAlphaPreludeContext(b, payload, tc.disk)
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			for b.Loop() {
				ctx := &gin.Context{Request: template.Request}
				ctx.Set(common.KeyBodyStorage, storage)
				modelRequest, err := getModelFromRequest(ctx)
				if err != nil || modelRequest.Model != "gpt-alpha" || modelRequest.Group != "vip" {
					b.Fatalf("prelude: %v, %v", modelRequest, err)
				}
				request, err := helper.GetAndValidateAlphaSearchRequest(ctx)
				if err != nil || len(request.RawBody) != len(payload) {
					b.Fatalf("validation: %v", err)
				}
			}
			b.ReportMetric(float64(storage.bytesCalls)/float64(b.N), "body_materializations/op")
			b.ReportMetric(float64(storage.readCalls)/float64(b.N), "storage_reads/op")
		})
	}
}

func TestAlphaModelPreludeCachesOnlyExactJSONEndpoint(t *testing.T) {
	for _, backend := range []struct {
		name string
		disk bool
	}{{"memory", false}, {"disk", true}} {
		for _, tc := range []struct {
			name        string
			path        string
			contentType string
			body        string
			wantCached  bool
		}{
			{"json", "/v1/alpha/search", "application/json; charset=utf-8", `{"model":"gpt-alpha","group":"vip"}`, true},
			{"path_suffix", "/v1/alpha/searchXYZ", "application/json", `{"model":"gpt-alpha","group":"vip"}`, false},
			{"path_child", "/v1/alpha/search/other", "application/json", `{"model":"gpt-alpha","group":"vip"}`, false},
			{"other_endpoint", "/v1/chat/completions", "application/json", `{"model":"gpt-alpha","group":"vip"}`, false},
			{"form", "/v1/alpha/search", "application/x-www-form-urlencoded", "model=gpt-alpha&group=vip", false},
			{"multipart", "/v1/alpha/search", "multipart/form-data; boundary=alpha-boundary", "--alpha-boundary\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\ngpt-alpha\r\n--alpha-boundary\r\nContent-Disposition: form-data; name=\"group\"\r\n\r\nvip\r\n--alpha-boundary--\r\n", false},
		} {
			t.Run(backend.name+"/"+tc.name, func(t *testing.T) {
				ctx, _ := newAlphaPreludeContext(t, []byte(tc.body), backend.disk)
				ctx.Request.URL.Path = tc.path
				ctx.Request.Header.Set("Content-Type", tc.contentType)

				modelRequest, err := getModelFromRequest(ctx)

				require.NoError(t, err)
				require.Equal(t, ModelRequest{Model: "gpt-alpha", Group: "vip"}, *modelRequest)
				cached, exists := common.GetContextKeyType[*dto.AlphaSearchRequest](ctx, constant.ContextKeyOpenAIAlphaSearchRequest)
				require.Equal(t, tc.wantCached, exists)
				if tc.wantCached {
					require.NotNil(t, cached)
					require.Equal(t, tc.body, string(cached.RawBody))
					validated, err := helper.GetAndValidateAlphaSearchRequest(ctx)
					require.NoError(t, err)
					require.Same(t, cached, validated)
				}
			})
		}
	}
}
