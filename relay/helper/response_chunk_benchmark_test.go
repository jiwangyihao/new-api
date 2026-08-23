package helper

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"
)

type discardResponseChunkWriter struct {
	header http.Header
	writes int
}

func (w *discardResponseChunkWriter) Header() http.Header {
	return w.header
}

func (w *discardResponseChunkWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

func (w *discardResponseChunkWriter) WriteHeader(int) {}

func (w *discardResponseChunkWriter) Flush() {}

func BenchmarkResponseChunkDataFrameAllocation(b *testing.B) {
	gin.SetMode(gin.TestMode)
	for _, size := range []int{256, 4 << 10, 64 << 10} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			writer := &discardResponseChunkWriter{header: make(http.Header)}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			response := dto.ResponsesStreamResponse{Type: "response.output_text.delta"}
			data := strings.Repeat("x", size)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := ResponseChunkData(c, response, data); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(writer.writes)/float64(b.N), "writes/op")
		})
	}
}
