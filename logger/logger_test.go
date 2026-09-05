package logger

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type concurrentLogRecorder struct {
	mu sync.Mutex
	strings.Builder
}

func (w *concurrentLogRecorder) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Builder.Write(p)
}

func TestConcurrentLoggingPreservesMessages(t *testing.T) {
	writer := &concurrentLogRecorder{}
	common.LogWriterMu.Lock()
	oldWriter, oldErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = writer, writer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldWriter, oldErrorWriter
		common.LogWriterMu.Unlock()
	})

	const workers, perWorker = 8, 32
	ctx := context.WithValue(context.Background(), common.RequestIdKey, "logger-concurrent-test")
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for message := 0; message < perWorker; message++ {
				text := fmt.Sprintf("message-%d-%d", worker, message)
				if message%2 == 0 {
					LogInfo(ctx, text)
				} else {
					LogError(ctx, text)
				}
			}
		}(worker)
	}
	close(start)
	wg.Wait()

	messages := make(map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(writer.String()), "\n") {
		_, message, found := strings.Cut(line, " | logger-concurrent-test | ")
		require.True(t, found, "missing request ID in log: %s", line)
		messages[strings.TrimSpace(message)]++
	}
	require.Len(t, messages, workers*perWorker)
	for worker := 0; worker < workers; worker++ {
		for message := 0; message < perWorker; message++ {
			require.Equal(t, 1, messages[fmt.Sprintf("message-%d-%d", worker, message)])
		}
	}
}
