package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"

	gzipmiddleware "github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newResponseGzipTestRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(handler)
	router.GET("/json", func(c *gin.Context) {
		c.Header("Content-Length", "999")
		c.String(http.StatusCreated, "hello gzip")
	})
	router.GET("/events", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/event-stream", []byte("data: hello\n\n"))
	})
	router.GET("/image.png", func(c *gin.Context) {
		c.Data(http.StatusOK, "image/png", []byte("png"))
	})
	return router
}

func TestResponseGzipCompressesAndPreservesStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/json", nil)
	request.Header.Set("Accept-Encoding", "gzip")

	newResponseGzipTestRouter(ResponseGzip()).ServeHTTP(recorder, request)

	require.Equal(t, http.StatusCreated, recorder.Code)
	require.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))
	require.Equal(t, "Accept-Encoding", recorder.Header().Get("Vary"))
	contentLength, err := strconv.Atoi(recorder.Header().Get("Content-Length"))
	require.NoError(t, err)
	require.Equal(t, recorder.Body.Len(), contentLength)
	reader, err := gzip.NewReader(bytes.NewReader(recorder.Body.Bytes()))
	require.NoError(t, err)
	decoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, "hello gzip", string(decoded))
}

func TestResponseGzipSkipsUnsupportedRequests(t *testing.T) {
	for _, test := range []struct {
		name    string
		path    string
		headers map[string]string
	}{
		{name: "no_accept_encoding", path: "/json"},
		{name: "event_stream", path: "/events", headers: map[string]string{"Accept-Encoding": "gzip", "Accept": "text/event-stream"}},
		{name: "upgrade", path: "/json", headers: map[string]string{"Accept-Encoding": "gzip", "Connection": "Upgrade"}},
		{name: "excluded_extension", path: "/image.png", headers: map[string]string{"Accept-Encoding": "gzip"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			for key, value := range test.headers {
				request.Header.Set(key, value)
			}
			newResponseGzipTestRouter(ResponseGzip()).ServeHTTP(recorder, request)
			require.Empty(t, recorder.Header().Get("Content-Encoding"))
			require.Empty(t, recorder.Header().Get("Vary"))
		})
	}
}

func TestResponseGzipPoolRetainsBoundedWriterAcrossGC(t *testing.T) {
	pool := newResponseGzipPool(gzip.DefaultCompression, 1)
	writer := pool.get()
	pool.put(writer)
	runtime.GC()
	runtime.GC()

	reused := pool.get()
	require.Same(t, writer, reused)
	pool.put(reused)
}

func TestResponseGzipPoolBoundsPersistentWriters(t *testing.T) {
	pool := newResponseGzipPool(gzip.DefaultCompression, 2)
	writers := []*gzip.Writer{pool.get(), pool.get(), pool.get(), pool.get()}
	for _, writer := range writers {
		pool.put(writer)
	}
	require.Len(t, pool.persistent, 2)
}

func TestResponseGzipMatchesGinContribCompressedBytes(t *testing.T) {
	payload := strings.Repeat("equivalent-response-", 1024)
	compress := func(handler gin.HandlerFunc) []byte {
		router := gin.New()
		router.Use(handler)
		router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, payload) })
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Accept-Encoding", "gzip")
		router.ServeHTTP(recorder, request)
		return recorder.Body.Bytes()
	}

	require.Equal(t, compress(gzipmiddleware.Gzip(gzip.DefaultCompression)), compress(ResponseGzip()))
}

var responseGzipBenchmarkPayload = []byte(`{"success":true,"data":["` + strings.Repeat("subscription-usage-ranking-", 4096) + `"]}`)

func BenchmarkResponseGzipWriter(b *testing.B) {
	b.Run("cold_writer", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(responseGzipBenchmarkPayload)))
		for i := 0; i < b.N; i++ {
			writer, err := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = writer.Write(responseGzipBenchmarkPayload)
			_ = writer.Close()
		}
	})
	b.Run("bounded_persistent_pool", func(b *testing.B) {
		pool := newResponseGzipPool(gzip.DefaultCompression, 1)
		b.ReportAllocs()
		b.SetBytes(int64(len(responseGzipBenchmarkPayload)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			writer := pool.get()
			writer.Reset(io.Discard)
			_, _ = writer.Write(responseGzipBenchmarkPayload)
			_ = writer.Close()
			pool.put(writer)
		}
	})
}
