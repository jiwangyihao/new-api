package helper

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dataFrameRecorder struct {
	*httptest.ResponseRecorder
	events      []string
	writeError  error
	shortWrite  bool
	cancelWrite context.CancelFunc
	panicFlush  bool
}

func (w *dataFrameRecorder) Write(p []byte) (int, error) {
	w.events = append(w.events, "write")
	if w.writeError != nil {
		return 0, w.writeError
	}
	if w.cancelWrite != nil {
		w.cancelWrite()
	}
	if w.shortWrite {
		return w.ResponseRecorder.Write(p[:len(p)-1])
	}
	return w.ResponseRecorder.Write(p)
}

func (w *dataFrameRecorder) Flush() {
	w.events = append(w.events, "flush")
	if w.panicFlush {
		panic("downstream flush failed")
	}
	w.ResponseRecorder.Flush()
}

func newDataFrameContext(w http.ResponseWriter) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func TestDataFramePreservesBytesAndFlushOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, value := range []string{"", "[DONE]", "中文\n\"quoted\"\\ <tag>", strings.Repeat("x", 4<<10), strings.Repeat("y", 64<<10)} {
		for _, objectMode := range []bool{false, true} {
			writer := &dataFrameRecorder{ResponseRecorder: httptest.NewRecorder()}
			c := newDataFrameContext(writer)
			var err error
			want := []byte(value)
			if objectMode {
				object := struct {
					Delta string `json:"delta"`
				}{Delta: value}
				want, err = common.Marshal(object)
				require.NoError(t, err)
				err = ObjectData(c, object)
			} else {
				err = StringData(c, value)
			}
			require.NoError(t, err)
			require.Equal(t, "data: "+string(want)+"\n\n", writer.Body.String())
			require.Equal(t, []string{"write", "flush"}, writer.events)
			require.True(t, writer.Flushed)
		}
	}
}

func TestDataFramePreservesFailureBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, objectMode := range []bool{false, true} {
		writeFrame := func(c *gin.Context) error {
			if objectMode {
				return ObjectData(c, struct {
					Delta string `json:"delta"`
				}{Delta: "hello"})
			}
			return StringData(c, "hello")
		}
		require.EqualError(t, writeFrame(nil), "context or writer is nil")
		require.EqualError(t, writeFrame(&gin.Context{}), "context or writer is nil")

		writer := &dataFrameRecorder{ResponseRecorder: httptest.NewRecorder()}
		c := newDataFrameContext(writer)
		ctx, cancel := context.WithCancel(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		cancel()
		require.ErrorIs(t, writeFrame(c), context.Canceled)
		require.Empty(t, writer.events)

		writer = &dataFrameRecorder{ResponseRecorder: httptest.NewRecorder(), writeError: io.ErrClosedPipe}
		err := writeFrame(newDataFrameContext(writer))
		require.ErrorIs(t, err, io.ErrClosedPipe)
		require.ErrorContains(t, err, "write string data failed")
		require.Equal(t, []string{"write"}, writer.events)

		writer = &dataFrameRecorder{ResponseRecorder: httptest.NewRecorder()}
		c = newDataFrameContext(writer)
		ctx, cancel = context.WithCancel(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		writer.cancelWrite = cancel
		err = writeFrame(c)
		cancel()
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, []string{"write"}, writer.events)

		writer = &dataFrameRecorder{ResponseRecorder: httptest.NewRecorder(), panicFlush: true}
		require.EqualError(t, writeFrame(newDataFrameContext(writer)), "flush panic recovered: downstream flush failed")
		require.Equal(t, []string{"write", "flush"}, writer.events)

		// Preserve the historical writer contract; this optimization does not
		// introduce a new error when a writer returns a short count without one.
		writer = &dataFrameRecorder{ResponseRecorder: httptest.NewRecorder(), shortWrite: true}
		require.NoError(t, writeFrame(newDataFrameContext(writer)))
		require.Equal(t, []string{"write", "flush"}, writer.events)
	}
	require.EqualError(t, ObjectData(nil, nil), "object is nil")
	err := ObjectData(nil, make(chan int))
	require.ErrorContains(t, err, "error marshalling object")
	require.NotContains(t, err.Error(), "context or writer is nil")
}

func TestDataFrameConcurrentRequestsStayIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const workers, frames = 8, 50
	writers := make([]*httptest.ResponseRecorder, workers)
	contexts := make([]*gin.Context, workers)
	for i := range writers {
		writers[i] = httptest.NewRecorder()
		contexts[i] = newDataFrameContext(writers[i])
	}
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strings.Repeat(string(rune('a'+i)), 256+i*512)
			for n := 0; n < frames; n++ {
				if err := StringData(contexts[i], value); err != nil {
					results <- err
					return
				}
			}
			if writers[i].Body.String() != strings.Repeat("data: "+value+"\n\n", frames) {
				results <- errors.New("stream payload changed across requests")
			}
		}(i)
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
}
