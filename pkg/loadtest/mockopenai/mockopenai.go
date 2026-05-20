package mockopenai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
)

const (
	requestIDHeader         = "X-Oneapi-Request-Id"
	upstreamRequestIDHeader = "X-Upstream-Request-Id"
	defaultModel            = "gpt-5.5"
)

type Config struct {
	RunContext       artifact.RunContext
	FirstTokenDelay  time.Duration
	StreamDuration   time.Duration
	ChunkInterval    time.Duration
	OutputBytes      int
	PromptTokens     int
	CompletionTokens int
	StatusRate       map[int]float64
	Seed             int64
	StatsOut         string
}

type Server struct {
	cfg Config

	mu                   sync.Mutex
	attemptsTotal        int
	injectedStatusCounts map[string]int
	attempts             []artifact.MockAttempt
}

func NewServer(cfg Config) http.Handler {
	cfg = normalizeConfig(cfg)
	return &Server{cfg: cfg, injectedStatusCounts: make(map[string]int)}
}

func MockStatsForTest(rc artifact.RunContext) artifact.MockStats {
	return artifact.MockStats{SchemaVersion: artifact.SchemaVersion, RunContext: rc, InjectedStatusCounts: map[string]int{}, Attempts: []artifact.MockAttempt{}}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		s.handleModels(w)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/responses":
		s.handleMain(w, r, true)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		s.handleMain(w, r, false)
	case r.Method == http.MethodGet && r.URL.Path == "/debug/loadtest/mock-stats":
		s.handleStats(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) Snapshot() artifact.MockStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Server) WriteStats() error {
	if s.cfg.StatsOut == "" {
		return nil
	}
	return writeStatsFile(s.cfg.StatsOut, s.Snapshot())
}

func (s *Server) handleModels(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]any{{
			"id":       s.model(),
			"object":   "model",
			"created":  1710000000,
			"owned_by": "loadtest",
		}},
	})
}

func (s *Server) handleMain(w http.ResponseWriter, r *http.Request, responses bool) {
	attempt, requestID, injectedStatus := s.recordAttempt(r.Method, r.URL.Path)
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set(upstreamRequestIDHeader, requestID)
	if injectedStatus != 0 {
		s.writeInjectedError(w, injectedStatus)
		_ = s.WriteStats()
		return
	}

	if responses {
		s.writeResponsesStream(w, attempt)
	} else {
		s.writeChatCompletionsStream(w, attempt)
	}
	_ = s.WriteStats()
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "mock stats endpoint is loopback-only"})
		return
	}
	writeJSON(w, http.StatusOK, s.Snapshot())
}

func (s *Server) writeInjectedError(w http.ResponseWriter, status int) {
	switch status {
	case http.StatusTooManyRequests:
		writeJSON(w, status, map[string]any{"error": map[string]any{"message": "loadtest injected rate limit", "type": "rate_limit_error", "code": "rate_limit_exceeded"}})
	case http.StatusBadGateway:
		writeJSON(w, status, map[string]any{"error": map[string]any{"message": "loadtest injected upstream failure", "type": "upstream_error", "code": "bad_gateway"}})
	default:
		writeJSON(w, status, map[string]any{"error": map[string]any{"message": "loadtest injected upstream failure", "type": "upstream_error", "code": http.StatusText(status)}})
	}
}

func (s *Server) writeResponsesStream(w http.ResponseWriter, attempt int64) {
	setSSEHeaders(w)
	id := fmt.Sprintf("resp_loadtest_%d", attempt)
	messageID := fmt.Sprintf("msg_loadtest_%d", attempt)
	writeSSEEvent(w, "response.created", map[string]any{"type": "response.created", "response": map[string]any{"id": id, "model": s.model(), "status": "in_progress"}})
	s.sleepFirstToken()
	for _, chunk := range s.outputChunks() {
		writeSSEEvent(w, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": messageID, "output_index": 0, "content_index": 0, "delta": chunk})
		s.sleepChunk()
	}
	writeSSEEvent(w, "response.completed", map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "model": s.model(), "status": "completed", "usage": map[string]any{"input_tokens": s.cfg.PromptTokens, "output_tokens": s.cfg.CompletionTokens, "total_tokens": s.totalTokens()}}})
	writeSSEDone(w)
}

func (s *Server) writeChatCompletionsStream(w http.ResponseWriter, attempt int64) {
	setSSEHeaders(w)
	id := fmt.Sprintf("chatcmpl-loadtest-%d", attempt)
	writeSSEData(w, map[string]any{"id": id, "object": "chat.completion.chunk", "created": int64(1710000000), "model": s.model(), "choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}}})
	s.sleepFirstToken()
	for _, chunk := range s.outputChunks() {
		writeSSEData(w, map[string]any{"id": id, "object": "chat.completion.chunk", "created": int64(1710000000), "model": s.model(), "choices": []map[string]any{{"index": 0, "delta": map[string]any{"content": chunk}, "finish_reason": nil}}})
		s.sleepChunk()
	}
	writeSSEData(w, map[string]any{"id": id, "object": "chat.completion.chunk", "created": int64(1710000000), "model": s.model(), "choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": artifact.Usage{PromptTokens: s.cfg.PromptTokens, CompletionTokens: s.cfg.CompletionTokens, TotalTokens: s.totalTokens()}})
	writeSSEDone(w)
}

func (s *Server) recordAttempt(method, path string) (int64, string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.attemptsTotal++
	attempt := int64(s.attemptsTotal)
	requestID := fmt.Sprintf("upstream-loadtest-%d", attempt)
	injectedStatus := s.injectedStatus(attempt)
	if injectedStatus != 0 {
		s.injectedStatusCounts[strconv.Itoa(injectedStatus)]++
	}
	s.attempts = append(s.attempts, artifact.MockAttempt{AttemptIndex: int(attempt), Method: method, Path: path, UpstreamRequestID: requestID, InjectedStatus: injectedStatus})
	return attempt, requestID, injectedStatus
}

func (s *Server) injectedStatus(attempt int64) int {
	if len(s.cfg.StatusRate) == 0 {
		return 0
	}
	statuses := make([]int, 0, len(s.cfg.StatusRate))
	for status := range s.cfg.StatusRate {
		statuses = append(statuses, status)
	}
	sort.Ints(statuses)
	for _, status := range statuses {
		if loadtestconfig.ShouldInjectStatus(s.cfg.Seed, attempt, status, s.cfg.StatusRate[status]) {
			return status
		}
	}
	return 0
}

func (s *Server) snapshotLocked() artifact.MockStats {
	counts := make(map[string]int, len(s.injectedStatusCounts))
	for status, count := range s.injectedStatusCounts {
		counts[status] = count
	}
	attempts := make([]artifact.MockAttempt, len(s.attempts))
	copy(attempts, s.attempts)
	return artifact.MockStats{SchemaVersion: artifact.SchemaVersion, RunContext: s.cfg.RunContext, AttemptsTotal: s.attemptsTotal, InjectedStatusCounts: counts, Attempts: attempts, MockHash: s.cfg.RunContext.MockHash}
}

func (s *Server) outputChunks() []string {
	if s.cfg.OutputBytes <= 0 {
		return nil
	}
	chunkBytes := s.chunkSize()
	chunks := make([]string, 0, (s.cfg.OutputBytes+chunkBytes-1)/chunkBytes)
	remaining := s.cfg.OutputBytes
	for remaining > 0 {
		n := chunkBytes
		if remaining < n {
			n = remaining
		}
		chunks = append(chunks, strings.Repeat("x", n))
		remaining -= n
	}
	return chunks
}

func (s *Server) chunkSize() int {
	if s.cfg.StreamDuration <= 0 || s.cfg.ChunkInterval <= 0 || s.cfg.OutputBytes <= 0 {
		return s.cfg.OutputBytes
	}
	chunks := int(s.cfg.StreamDuration / s.cfg.ChunkInterval)
	if chunks < 1 {
		chunks = 1
	}
	chunkBytes := (s.cfg.OutputBytes + chunks - 1) / chunks
	if chunkBytes < 1 {
		return 1
	}
	return chunkBytes
}

func (s *Server) sleepFirstToken() {
	if s.cfg.FirstTokenDelay > 0 {
		time.Sleep(s.cfg.FirstTokenDelay)
	}
}

func (s *Server) sleepChunk() {
	if s.cfg.ChunkInterval > 0 {
		time.Sleep(s.cfg.ChunkInterval)
	}
}

func (s *Server) model() string {
	if s.cfg.RunContext.Model != "" {
		return s.cfg.RunContext.Model
	}
	return defaultModel
}

func (s *Server) totalTokens() int {
	return s.cfg.PromptTokens + s.cfg.CompletionTokens
}

func normalizeConfig(cfg Config) Config {
	if cfg.RunContext.SchemaVersion == 0 {
		cfg.RunContext.SchemaVersion = artifact.SchemaVersion
	}
	if cfg.RunContext.Model == "" {
		cfg.RunContext.Model = defaultModel
	}
	if cfg.OutputBytes < 0 {
		cfg.OutputBytes = 0
	}
	if cfg.PromptTokens < 0 {
		cfg.PromptTokens = 0
	}
	if cfg.CompletionTokens < 0 {
		cfg.CompletionTokens = 0
	}
	if cfg.StatusRate == nil {
		cfg.StatusRate = map[int]float64{}
	}
	return cfg
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func writeSSEEvent(w http.ResponseWriter, event string, v any) {
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	writeSSEDataLine(w, v)
	_, _ = fmt.Fprint(w, "\n")
	flush(w)
}

func writeSSEData(w http.ResponseWriter, v any) {
	writeSSEDataLine(w, v)
	_, _ = fmt.Fprint(w, "\n")
	flush(w)
}

func writeSSEDataLine(w http.ResponseWriter, v any) {
	b, err := common.Marshal(v)
	if err != nil {
		_, _ = fmt.Fprint(w, "data: {\"error\":{\"message\":\"marshal failure\"}}\n")
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n", b)
}

func writeSSEDone(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flush(w)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := common.Marshal(v)
	if err != nil {
		http.Error(w, "marshal failure", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeStatsFile(path string, stats artifact.MockStats) error {
	b, err := artifact.MarshalCanonical(stats)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write([]byte("\n")); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func isLoopbackRequest(r *http.Request) bool {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			first, _, _ := strings.Cut(value, ",")
			if !isLoopbackHost(strings.TrimSpace(first)) {
				return false
			}
		}
	}
	return isLoopbackHost(r.RemoteAddr)
}

func isLoopbackHost(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ValidateRunContextHash(rc artifact.RunContext, cfg Config) error {
	profile := loadtestconfig.MockProfile{FirstTokenDelay: cfg.FirstTokenDelay, StreamDuration: cfg.StreamDuration, ChunkInterval: cfg.ChunkInterval, OutputBytes: cfg.OutputBytes, PromptTokens: cfg.PromptTokens, CompletionTokens: cfg.CompletionTokens, StatusRate: cfg.StatusRate, Seed: cfg.Seed}
	hash, err := loadtestconfig.HashMockProfile(profile)
	if err != nil {
		return err
	}
	if rc.MockHash == "" {
		return errors.New("run_context.mock_hash is required")
	}
	if rc.MockHash != hash {
		return fmt.Errorf("run_context.mock_hash %s does not match current mock profile %s", rc.MockHash, hash)
	}
	return nil
}

func Serve(ctx context.Context, addr string, cfg Config) error {
	server := &http.Server{Addr: addr, Handler: NewServer(cfg)}
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}
