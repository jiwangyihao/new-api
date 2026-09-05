package helper

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type alphaValidationStorage struct {
	common.BodyStorage
	readCalls  int
	bytesCalls int
}

func (s *alphaValidationStorage) Read(p []byte) (int, error) {
	s.readCalls++
	return s.BodyStorage.Read(p)
}

func (s *alphaValidationStorage) Bytes() ([]byte, error) {
	s.bytesCalls++
	return s.BodyStorage.Bytes()
}

func newAlphaValidationContext(tb testing.TB, payload []byte, disk bool) (*gin.Context, *alphaValidationStorage) {
	tb.Helper()
	originalConfig := common.GetDiskCacheConfig()
	common.SetDiskCacheConfig(common.DiskCacheConfig{
		Enabled: disk, ThresholdMB: 0, MaxSizeMB: 128, Path: tb.TempDir(),
	})
	storage, err := common.CreateBodyStorage(payload)
	common.SetDiskCacheConfig(originalConfig)
	require.NoError(tb, err)
	tb.Cleanup(func() { require.NoError(tb, storage.Close()) })
	require.Equal(tb, disk, storage.IsDisk())
	counted := &alphaValidationStorage{BodyStorage: storage}
	ctx := &gin.Context{Request: httptest.NewRequest(http.MethodPost, "/v1/alpha/search", bytes.NewReader(payload))}
	ctx.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
	ctx.Set(common.KeyBodyStorage, counted)
	return ctx, counted
}

func TestAlphaSearchValidationPreservesRawJSON(t *testing.T) {
	for _, backend := range []struct {
		name string
		disk bool
	}{{"memory", false}, {"disk", true}} {
		t.Run(backend.name, func(t *testing.T) {
			for _, payload := range []string{
				"{\n  \"model\":\"gpt-alpha\", \"id\":\"req-1\", \"future\":18446744073709551615, \"commands\":{\"search_query\":[{\"q\":\"<>&查询\"}]}\n}",
				`{"model":"gpt-alpha","stream":false,"max_output_tokens":0}`,
				`{"model":"gpt-alpha","stream":null,"max_output_tokens":null}`,
				`{"model":"old","\u006dodel":"gpt-alpha","id":"\u0061","stream":true,"stream":false}`,
			} {
				input := []byte(payload)
				ctx, storage := newAlphaValidationContext(t, input, backend.disk)
				var want dto.AlphaSearchRequest
				require.NoError(t, common.Unmarshal(input, &want))
				want.RawBody = input

				got, err := GetAndValidateAlphaSearchRequest(ctx)

				require.NoError(t, err)
				require.Equal(t, want, *got)
				require.Equal(t, input, []byte(got.RawBody))
				if !backend.disk && &input[0] != &got.RawBody[0] {
					t.Fatal("memory-backed RawBody must reuse the stored input")
				}
				forwarded, err := io.ReadAll(ctx.Request.Body)
				require.NoError(t, err)
				require.Equal(t, input, forwarded)
				retried, err := GetAndValidateAlphaSearchRequest(ctx)
				require.NoError(t, err)
				require.Equal(t, *got, *retried)
				stored, err := common.GetBodyStorage(ctx)
				require.NoError(t, err)
				require.Same(t, storage, stored)
			}
		})
	}
}

func TestAlphaSearchValidationRejectsInvalidJSONAndFields(t *testing.T) {
	for _, disk := range []bool{false, true} {
		for _, tc := range []struct {
			name    string
			body    string
			message string
		}{
			{"syntax", `{"model":`, ""},
			{"trailing", `{"model":"gpt-alpha"} {"extra":true}`, ""},
			{"missing_model", `{"id":"req-1"}`, "model is required"},
			{"null", `null`, "model is required"},
			{"wrong_model_type", `{"model":42}`, ""},
			{"wrong_stream_type", `{"model":"gpt-alpha","stream":"false"}`, ""},
			{"negative_tokens", `{"model":"gpt-alpha","max_output_tokens":-1}`, ""},
			{"overflow_tokens", `{"model":"gpt-alpha","max_output_tokens":2147483647}`, "max_output_tokens is invalid"},
		} {
			t.Run(tc.name+map[bool]string{false: "/memory", true: "/disk"}[disk], func(t *testing.T) {
				payload := []byte(tc.body)
				ctx, originalStorage := newAlphaValidationContext(t, payload, disk)
				request, err := GetAndValidateAlphaSearchRequest(ctx)
				require.Error(t, err)
				require.Nil(t, request)
				if tc.message != "" {
					require.EqualError(t, err, tc.message)
				}
				storage, storageErr := common.GetBodyStorage(ctx)
				require.NoError(t, storageErr)
				require.Same(t, originalStorage, storage)
				raw, readErr := io.ReadAll(storage)
				require.NoError(t, readErr)
				require.Equal(t, payload, raw)
			})
		}
	}
}

func TestAlphaSearchValidationDoesNotStreamDecodeThenReadRawBody(t *testing.T) {
	payload := []byte(`{"model":"gpt-alpha","future":"` + strings.Repeat("x", 1<<20) + `"}`)
	ctx, storage := newAlphaValidationContext(t, payload, true)

	request, err := GetAndValidateAlphaSearchRequest(ctx)

	require.NoError(t, err)
	require.Equal(t, payload, []byte(request.RawBody))
	require.Equal(t, 1, storage.bytesCalls)
	require.Zero(t, storage.readCalls, "decoding must reuse required RawBody rather than separately stream-reading the file")
}

var alphaValidationBenchmarkSink *dto.AlphaSearchRequest

func BenchmarkAlphaSearchValidation(b *testing.B) {
	for _, tc := range []struct {
		name string
		size int
		disk bool
	}{
		{"memory_4KiB", 4 << 10, false},
		{"disk_4KiB", 4 << 10, true},
		{"memory_1MiB", 1 << 20, false},
		{"disk_1MiB", 1 << 20, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			payload := []byte(`{"model":"gpt-alpha","id":"req-bench","stream":false,"commands":{"search_query":[{"q":"weather"}]},"future":"` + strings.Repeat("x", tc.size) + `"}`)
			ctx, storage := newAlphaValidationContext(b, payload, tc.disk)
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			for b.Loop() {
				request, err := GetAndValidateAlphaSearchRequest(ctx)
				if err != nil {
					b.Fatal(err)
				}
				alphaValidationBenchmarkSink = request
			}
			b.ReportMetric(float64(storage.bytesCalls)/float64(b.N), "bytes_calls/op")
			b.ReportMetric(float64(storage.readCalls)/float64(b.N), "storage_reads/op")
			alphaValidationBenchmarkSink = nil
		})
	}
}

func TestAlphaSearchValidationPreservesNonJSONPaths(t *testing.T) {
	var multipartBody bytes.Buffer
	form := multipart.NewWriter(&multipartBody)
	require.NoError(t, form.WriteField("model", "gpt-alpha"))
	require.NoError(t, form.WriteField("id", "req<1"))
	require.NoError(t, form.WriteField("future", "unknown field"))
	require.NoError(t, form.Close())

	for _, backend := range []struct {
		name string
		disk bool
	}{{"memory", false}, {"disk", true}} {
		for _, tc := range []struct {
			name        string
			contentType string
			body        string
			wantError   bool
		}{
			{"form", "application/x-www-form-urlencoded", "model=gpt-alpha&id=req%3C1&future=unknown+field", false},
			{"multipart", form.FormDataContentType(), multipartBody.String(), false},
			{"text", "text/plain", `{"model":"gpt-alpha"}`, true},
			{"missing_content_type", "", `{"model":"gpt-alpha"}`, true},
		} {
			t.Run(backend.name+"/"+tc.name, func(t *testing.T) {
				payload := []byte(tc.body)
				ctx, storage := newAlphaValidationContext(t, payload, backend.disk)
				ctx.Request.Header.Set("Content-Type", tc.contentType)

				request, err := GetAndValidateAlphaSearchRequest(ctx)

				if tc.wantError {
					require.EqualError(t, err, "model is required")
					require.Nil(t, request)
				} else {
					require.NoError(t, err)
					require.Equal(t, "gpt-alpha", request.Model)
					require.Equal(t, "req<1", request.Id)
					require.Equal(t, payload, []byte(request.RawBody))
				}
				forwarded, readErr := io.ReadAll(ctx.Request.Body)
				require.NoError(t, readErr)
				require.Equal(t, payload, forwarded)
				cached, exists := ctx.Get(common.KeyBodyStorage)
				require.True(t, exists)
				require.Same(t, storage, cached)
			})
		}
	}
}

type alphaValidationFailingStorage struct {
	common.BodyStorage
	failure    error
	failBytes  bool
	failSeekAt int
	seekCalls  int
}

func (s *alphaValidationFailingStorage) Bytes() ([]byte, error) {
	if s.failBytes {
		return nil, s.failure
	}
	return s.BodyStorage.Bytes()
}

func (s *alphaValidationFailingStorage) Seek(offset int64, whence int) (int64, error) {
	s.seekCalls++
	if s.seekCalls == s.failSeekAt {
		return 0, s.failure
	}
	return s.BodyStorage.Seek(offset, whence)
}

func TestAlphaSearchValidationPropagatesStorageErrors(t *testing.T) {
	for _, backend := range []struct {
		name string
		disk bool
	}{{"memory", false}, {"disk", true}} {
		for _, tc := range []struct {
			name       string
			failBytes  bool
			failSeekAt int
		}{
			{"read", true, 0},
			{"initial_seek", false, 1},
			{"reset_seek", false, 2},
		} {
			t.Run(backend.name+"/"+tc.name, func(t *testing.T) {
				payload := []byte(`{"model":"gpt-alpha","future":18446744073709551615}`)
				ctx, stored := newAlphaValidationContext(t, payload, backend.disk)
				originalBody := ctx.Request.Body
				failure := errors.New("test body storage failure")
				storage := &alphaValidationFailingStorage{
					BodyStorage: stored,
					failure:     failure,
					failBytes:   tc.failBytes,
					failSeekAt:  tc.failSeekAt,
				}
				ctx.Set(common.KeyBodyStorage, storage)

				request, err := GetAndValidateAlphaSearchRequest(ctx)

				require.ErrorIs(t, err, failure)
				require.Nil(t, request)
				require.True(t, originalBody == ctx.Request.Body, "failed validation must not replace Request.Body")
				cached, exists := ctx.Get(common.KeyBodyStorage)
				require.True(t, exists)
				require.Same(t, storage, cached)
				original, readErr := stored.Bytes()
				require.NoError(t, readErr)
				require.Equal(t, payload, original)
			})
		}
	}
}
