// Package streamgate preserves one logical response across retryable upstream attempts.
// Prelude delivery does not commit generation; meaningful output, a terminal outcome,
// unsafe execution, or a downstream transport failure does.
package streamgate

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"

	common "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const MaxFrameBytes = 128 << 20

var ErrFrameTooLarge = errors.New("stream event exceeds 128 MiB limit")

type heldEvent struct {
	raw   []byte
	event string
}

// Gate implements gin.ResponseWriter. Callers must still serialize complete
// application events; independently written comment heartbeats may interleave
// with fragmented events and are handled without corrupting their framing.
// TransformEvent and DrainPrelude are the corresponding single-caller WS seam.
type Gate struct {
	gin.ResponseWriter
	mu                           sync.Mutex
	format                       string
	meaningful, terminal, unsafe bool
	created, done, allowDone     bool
	attemptErr                   *types.NewAPIError
	writeErr                     error
	attempt                      int
	chatCreated                  gjson.Result
	sequence                     int64
	sequenced                    bool
	stream, plain                bool
	pending                      []byte
	scan                         int
	held                         []heldEvent
	heldBytes                    int
	ready                        [][]byte
	readyEvents                  []string
	failureRaw                   []byte
	responseID                   string
	upstreamID                   string
	publicID                     string
	onResponse                   func(string, string) error
	boundUpstream                string
}

func New(writer gin.ResponseWriter, format string) *Gate {
	return &Gate{ResponseWriter: writer, format: format}
}

func (g *Gate) SetFormat(format string) { g.mu.Lock(); defer g.mu.Unlock(); g.format = format }

// BindResponseIdentity publishes an opaque public response ID and persists its
// final route before output or completion makes that route observable.
func (g *Gate) BindResponseIdentity(id string, save func(string, string) error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.publicID, g.onResponse = id, save
}
func (g *Gate) Meaningful() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.meaningful }
func (g *Gate) Terminal() bool   { g.mu.Lock(); defer g.mu.Unlock(); return g.terminal }
func (g *Gate) MarkUnsafe()      { g.mu.Lock(); defer g.mu.Unlock(); g.unsafe = true }

// ObserveReplaySafety preserves upstream execution safety when an adaptor
// converts or drops events. It does not claim those bytes were delivered.
func (g *Gate) ObserveReplaySafety(raw []byte, format string) *types.NewAPIError {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !gjson.ValidBytes(raw) {
		g.unsafe = true
		return types.NewError(errors.New("invalid upstream stream event"), types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	result := classify(parse(raw), "", format)
	g.unsafe = g.unsafe || result.unsafe || result.meaningful
	return result.err
}

// FailureEvent commits the hidden protocol failure for final delivery when no
// retryable route remains. It patches identity/sequence only at this boundary
// and returns caller-owned bytes once; the caller must terminate on write error.
func (g *Gate) FailureEvent() ([]byte, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.terminal || g.writeErr != nil || len(g.failureRaw) == 0 {
		return nil, false
	}
	out, err := g.patch(g.failureRaw)
	if err != nil {
		g.attemptErr = types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
		return nil, false
	}
	g.terminal, g.allowDone = true, false
	g.failureRaw = nil
	g.held, g.ready, g.readyEvents = nil, nil, nil
	g.heldBytes = 0
	return out, true
}
func (g *Gate) ResponseID() string         { g.mu.Lock(); defer g.mu.Unlock(); return g.responseID }
func (g *Gate) UpstreamResponseID() string { g.mu.Lock(); defer g.mu.Unlock(); return g.upstreamID }
func (g *Gate) AttemptError() *types.NewAPIError {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.attemptErr
}
func (g *Gate) CanRetry() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.meaningful && !g.terminal && !g.unsafe && g.writeErr == nil && !types.IsSkipRetryError(g.attemptErr)
}

func (g *Gate) BeginAttempt() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.attempt++
	g.attemptErr = nil
	g.upstreamID = ""
	g.boundUpstream = ""
	g.pending = nil
	g.failureRaw = nil
	g.scan = 0
	g.stream, g.plain = false, false
	g.held = nil
	g.heldBytes = 0
	g.ready = nil
	g.readyEvents = nil
}

// The first observed write/event starts an attempt even when its caller does
// not need the explicit selection hook. A later BeginAttempt then means retry.
func (g *Gate) ensureAttempt() {
	if g.attempt == 0 {
		g.attempt = 1
	}
}

func (g *Gate) SetAttemptError(err *types.NewAPIError) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// A protocol error is more precise than a later EOF/transport summary.
	if err != nil && (g.attemptErr == nil || (g.attemptErr.RelayError == nil && err.RelayError != nil)) {
		g.attemptErr = err
	}
}

func (g *Gate) WriteHeader(code int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.writeErr == nil && !g.terminal {
		g.ResponseWriter.WriteHeader(code)
	}
}
func (g *Gate) WriteHeaderNow() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.writeErr == nil && !g.terminal {
		g.ResponseWriter.WriteHeaderNow()
	}
}
func (g *Gate) Flush() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.writeErr != nil {
		return
	}
	defer func() {
		if recover() != nil {
			g.transportError(errors.New("downstream flush failed"))
		}
	}()
	g.ResponseWriter.Flush()
}
func (g *Gate) Written() bool                     { g.mu.Lock(); defer g.mu.Unlock(); return g.ResponseWriter.Written() }
func (g *Gate) Status() int                       { g.mu.Lock(); defer g.mu.Unlock(); return g.ResponseWriter.Status() }
func (g *Gate) Size() int                         { g.mu.Lock(); defer g.mu.Unlock(); return g.ResponseWriter.Size() }
func (g *Gate) WriteString(s string) (int, error) { return g.Write([]byte(s)) }

func (g *Gate) transportError(err error) {
	g.writeErr = err
	if g.attemptErr == nil {
		g.attemptErr = types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
	}
}
func (g *Gate) write(raw []byte) error {
	if g.writeErr != nil {
		return g.writeErr
	}
	n, err := g.ResponseWriter.Write(raw)
	if err == nil && n != len(raw) {
		err = io.ErrShortWrite
	}
	if err != nil {
		g.transportError(err)
	}
	return err
}
func (g *Gate) tooLarge() error {
	g.unsafe = true
	g.attemptErr = types.NewError(ErrFrameTooLarge, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	g.pending = nil
	g.held = nil
	g.heldBytes = 0
	return ErrFrameTooLarge
}

// frameEnd returns the first blank-line boundary, retaining a scan cursor for
// split UTF-8/lines. No decoding or whole-payload copy is needed for full frames.
func frameEnd(raw []byte, start int) (int, int) {
	for start < len(raw) {
		i := bytes.IndexByte(raw[start:], '\n')
		if i < 0 {
			return 0, len(raw)
		}
		i += start
		if i == 0 || raw[i-1] == '\n' || (raw[i-1] == '\r' && (i == 1 || raw[i-2] == '\n')) {
			return i + 1, i + 1
		}
		start = i + 1
	}
	return 0, start
}

func (g *Gate) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.writeErr != nil {
		return 0, g.writeErr
	}
	g.ensureAttempt()
	if !strings.Contains(strings.ToLower(g.ResponseWriter.Header().Get("Content-Type")), "text/event-stream") {
		g.plain = true
		if g.terminal || g.attemptErr != nil {
			return len(p), nil
		}
		if len(p) != 0 {
			g.meaningful = true
		}
		if err := g.write(p); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	g.stream = true
	// Async ping helpers write a complete comment frame in one call. They must
	// not be appended into an application's partially written event/data pair.
	if len(g.pending) != 0 && commentOnly(p) {
		if !g.terminal && g.attemptErr == nil {
			if err := g.write(p); err != nil {
				return 0, err
			}
		}
		return len(p), nil
	}
	original := len(p)
	for len(p) > 0 {
		if len(g.pending) == 0 {
			end, scan := frameEnd(p, 0)
			if end != 0 {
				if end > MaxFrameBytes {
					return 0, g.tooLarge()
				}
				if err := g.frame(p[:end]); err != nil {
					return 0, err
				}
				p = p[end:]
				continue
			}
			if len(p) > MaxFrameBytes {
				return 0, g.tooLarge()
			}
			g.pending = append(g.pending, p...)
			g.scan = scan
			break
		}
		// Append at most through a newline so even one giant Write containing
		// many events is limited per frame, not per transport write.
		n := bytes.IndexByte(p, '\n') + 1
		if n == 0 {
			n = len(p)
		}
		if len(g.pending)+n > MaxFrameBytes {
			return 0, g.tooLarge()
		}
		g.pending = append(g.pending, p[:n]...)
		p = p[n:]
		end, scan := frameEnd(g.pending, g.scan)
		g.scan = scan
		if end != 0 {
			if err := g.frame(g.pending[:end]); err != nil {
				return 0, err
			}
			g.pending = nil
			g.scan = 0
		}
	}
	return original, nil
}

func commentOnly(raw []byte) bool {
	if len(raw) == 0 || raw[0] != ':' {
		return false
	}
	end, _ := frameEnd(raw, 0)
	if end != len(raw) {
		return false
	}
	for len(raw) != 0 {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			return false
		}
		line := bytes.TrimSuffix(raw[:i], []byte{'\r'})
		if len(line) != 0 && line[0] != ':' {
			return false
		}
		raw = raw[i+1:]
	}
	return true
}

func parseFrame(raw []byte) (event string, data []byte, hasData bool) {
	for len(raw) != 0 {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			i = len(raw)
		}
		line := bytes.TrimSuffix(raw[:i], []byte{'\r'})
		if i == len(raw) {
			raw = nil
		} else {
			raw = raw[i+1:]
		}
		if bytes.HasPrefix(line, []byte("event:")) {
			event = strings.TrimSpace(string(line[6:]))
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		v := line[5:]
		if len(v) != 0 && v[0] == ' ' {
			v = v[1:]
		}
		if !hasData {
			data = v
			hasData = true
		} else {
			// The first data line aliases the input; joining multiline data must
			// not mutate either that frame or its sibling frames.
			joined := make([]byte, len(data)+1+len(v))
			copy(joined, data)
			joined[len(data)] = '\n'
			copy(joined[len(data)+1:], v)
			data = joined
		}
	}
	return
}

func (g *Gate) frame(raw []byte) error {
	event, data, hasData := parseFrame(raw)
	if !hasData {
		if !g.terminal && g.attemptErr == nil {
			return g.write(raw)
		}
		return nil
	}
	out, emit, err := g.transform(data, event)
	if err != nil {
		return err
	}
	if !emit {
		return nil
	}
	for i, prelude := range g.ready {
		if err := g.writeSSE(prelude, g.readyEvents[i]); err != nil {
			return err
		}
	}
	g.ready, g.readyEvents = nil, nil
	if bytes.Equal(out, data) {
		return g.write(raw)
	}
	if typ := gjson.GetBytes(out, "type").String(); typ != "" {
		event = typ
	}
	return g.writeSSE(out, event)
}

func (g *Gate) writeSSE(raw []byte, event string) error {
	if event != "" {
		if err := g.write([]byte("event: " + event + "\n")); err != nil {
			return err
		}
	}
	for {
		line := raw
		i := bytes.IndexByte(raw, '\n')
		if i >= 0 {
			line = raw[:i]
		}
		if err := g.write([]byte("data: ")); err != nil {
			return err
		}
		if err := g.write(line); err != nil {
			return err
		}
		if i < 0 {
			break
		}
		if err := g.write([]byte("\n")); err != nil {
			return err
		}
		raw = raw[i+1:]
	}
	return g.write([]byte("\n\n"))
}

// TransformEvent classifies a complete JSON WebSocket event. Returning emit=true
// commits delivery intent; the caller must terminate if its WebSocket write fails.
// Before writing out, send the events returned by DrainPrelude in order.
func (g *Gate) TransformEvent(raw []byte) ([]byte, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.transform(raw, "")
}

// DrainPrelude transfers ownership of previously empty item/block scaffolding
// made visible by the last TransformEvent. It is empty for ordinary events.
func (g *Gate) DrainPrelude() [][]byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := g.ready
	g.ready, g.readyEvents = nil, nil
	return out
}

// EndAttempt closes one upstream attempt. It never commits a logical response;
// an incomplete frame or missing terminal is retained as an attempt error so
// the caller can select another route when no visible output was delivered.
func (g *Gate) EndAttempt() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.terminal || g.attemptErr != nil {
		g.pending, g.scan = nil, 0
		return
	}
	if !g.stream && (g.plain || !strings.Contains(strings.ToLower(g.ResponseWriter.Header().Get("Content-Type")), "text/event-stream")) {
		// A normal JSON response has no stream terminal event to wait for.
		g.terminal = g.meaningful
		return
	}
	if len(g.pending) != 0 {
		g.pending, g.scan = nil, 0
		g.attemptErr = types.NewError(errors.New("upstream stream truncated"), types.ErrorCodeBadResponseBody)
		return
	}
	g.attemptErr = types.NewError(errors.New("upstream stream ended before terminal event"), types.ErrorCodeBadResponseBody)
}

func (g *Gate) finalErrorBody(apiErr *types.NewAPIError) ([]byte, error) {
	if g.format == string(types.RelayFormatClaude) {
		return common.Marshal(struct {
			Type  string            `json:"type"`
			Error types.ClaudeError `json:"error"`
		}{Type: "error", Error: apiErr.ToClaudeError()})
	}
	if g.format == string(types.RelayFormatGemini) {
		return common.Marshal(struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}{Error: struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		}{Code: finalStatus(apiErr), Message: apiErr.MaskSensitiveError(), Status: googleStatus(finalStatus(apiErr))}})
	}
	return common.Marshal(struct {
		Error types.OpenAIError `json:"error"`
	}{Error: apiErr.ToOpenAIError()})
}

func (g *Gate) finalResponseFailure(apiErr *types.NewAPIError) ([]byte, error) {
	response := map[string]any{
		"status": "failed",
		"error":  apiErr.ToOpenAIError(),
	}
	if g.responseID != "" {
		response["id"] = g.responseID
	}
	result := map[string]any{"type": "response.failed", "response": response}
	if g.sequenced {
		result["sequence_number"] = g.sequence
	}
	return common.Marshal(result)
}

// Fail commits one final protocol error. Before the response writer is
// committed it preserves the HTTP status and JSON envelope; after an SSE
// prelude it emits the format-specific terminal event without attempting to
// rewrite the already-sent status line.
func (g *Gate) Fail(apiErr *types.NewAPIError) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.writeErr != nil {
		return g.writeErr
	}
	if g.terminal {
		return nil
	}
	if apiErr == nil {
		apiErr = g.attemptErr
	}
	if apiErr == nil {
		apiErr = types.NewError(errors.New("upstream stream failed"), types.ErrorCodeBadResponse)
	}
	g.attemptErr = apiErr
	g.terminal, g.allowDone = true, false

	if !g.ResponseWriter.Written() {
		status := finalStatus(apiErr)
		g.ResponseWriter.Header().Set("Content-Type", "application/json")
		g.ResponseWriter.WriteHeader(status)
		body, err := g.finalErrorBody(apiErr)
		if err != nil {
			return err
		}
		return g.write(body)
	}

	var body []byte
	var err error
	if len(g.failureRaw) > 0 {
		body, err = g.patch(g.failureRaw)
		g.failureRaw = nil
	} else if isResponsesFormat(g.format) {
		body, err = g.finalResponseFailure(apiErr)
		if err == nil {
			body, err = g.patch(body)
		}
	} else {
		body, err = g.finalErrorBody(apiErr)
	}
	if err != nil {
		return err
	}
	event := ""
	if isResponsesFormat(g.format) || g.format == string(types.RelayFormatClaude) {
		event = gjson.GetBytes(body, "type").String()
		if event == "" {
			event = "error"
		}
	}
	if err := g.writeSSE(body, event); err != nil {
		return err
	}
	if g.format == string(types.RelayFormatOpenAI) {
		g.done = true
		return g.writeSSE([]byte("[DONE]"), "")
	}
	return nil
}

func finalStatus(apiErr *types.NewAPIError) int {
	if apiErr.StatusCode >= 400 && apiErr.StatusCode <= 599 {
		return apiErr.StatusCode
	}
	return 502
}

func googleStatus(status int) string {
	switch status {
	case 400:
		return "INVALID_ARGUMENT"
	case 401:
		return "UNAUTHENTICATED"
	case 403:
		return "PERMISSION_DENIED"
	case 404:
		return "NOT_FOUND"
	case 409:
		return "ABORTED"
	case 429:
		return "RESOURCE_EXHAUSTED"
	case 499:
		return "CANCELLED"
	case 503:
		return "UNAVAILABLE"
	case 504:
		return "DEADLINE_EXCEEDED"
	default:
		return "INTERNAL"
	}
}

// FinishPlainResponse closes a JSON response already delivered as one document.
// Its protocol failure remains observable without appending an SSE/JSON suffix.
func (g *Gate) FinishPlainResponse(apiErr *types.NewAPIError) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.plain || g.writeErr != nil {
		return
	}
	if apiErr != nil {
		g.attemptErr = apiErr
	}
	g.terminal, g.allowDone = true, false
}
