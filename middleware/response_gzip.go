package middleware

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const persistentGzipWriterCount = 2

var defaultResponseGzipPool = newResponseGzipPool(gzip.DefaultCompression, persistentGzipWriterCount)

type responseGzipPool struct {
	level      int
	persistent chan *gzip.Writer
	ephemeral  sync.Pool
}

func newResponseGzipPool(level, persistent int) *responseGzipPool {
	pool := &responseGzipPool{
		level:      level,
		persistent: make(chan *gzip.Writer, persistent),
	}
	pool.ephemeral.New = func() any {
		writer, err := gzip.NewWriterLevel(io.Discard, level)
		if err != nil {
			panic(err)
		}
		return writer
	}
	return pool
}

func (p *responseGzipPool) get() *gzip.Writer {
	select {
	case writer := <-p.persistent:
		return writer
	default:
		return p.ephemeral.Get().(*gzip.Writer)
	}
}

func (p *responseGzipPool) put(writer *gzip.Writer) {
	if writer == nil {
		return
	}
	writer.Reset(io.Discard)
	select {
	case p.persistent <- writer:
	default:
		p.ephemeral.Put(writer)
	}
}

type responseGzipWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

func (w *responseGzipWriter) WriteString(value string) (int, error) {
	w.Header().Del("Content-Length")
	return w.writer.Write([]byte(value))
}

func (w *responseGzipWriter) Write(data []byte) (int, error) {
	w.Header().Del("Content-Length")
	return w.writer.Write(data)
}

func (w *responseGzipWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseGzipWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// ResponseGzip preserves the existing default gzip behavior while retaining a
// small, fixed number of writers across garbage collections. Extra concurrent
// writers remain in sync.Pool and may be reclaimed normally.
func ResponseGzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldGzipResponse(c.Request) {
			return
		}

		writer := defaultResponseGzipPool.get()
		writer.Reset(c.Writer)
		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		c.Writer = &responseGzipWriter{ResponseWriter: c.Writer, writer: writer}
		defer func() {
			_ = writer.Close()
			c.Header("Content-Length", fmt.Sprint(c.Writer.Size()))
			defaultResponseGzipPool.put(writer)
		}()
		c.Next()
	}
}

func shouldGzipResponse(request *http.Request) bool {
	if request == nil ||
		!strings.Contains(request.Header.Get("Accept-Encoding"), "gzip") ||
		strings.Contains(request.Header.Get("Connection"), "Upgrade") ||
		strings.Contains(request.Header.Get("Accept"), "text/event-stream") {
		return false
	}

	switch filepath.Ext(request.URL.Path) {
	case ".png", ".gif", ".jpeg", ".jpg":
		return false
	default:
		return true
	}
}
