package client

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

const (
	DefaultPath  = "/v1/responses"
	DefaultModel = "gpt-5.5"

	TransportModeH1KeepAlive   = "h1_keepalive"
	TransportModeH1NoKeepAlive = "h1_no_keepalive"
	TransportModeH2CDiagnostic = "h2c_diagnostic"
)

const defaultMaxClientConnsPerHost = 4

var tokenProfiles = map[string]string{
	"sk-loadtestsub":     "subscription",
	"sk-loadtestcompat":  "compat",
	"sk-loadtestinvalid": "invalid",
}

type ConfigError struct {
	Err error
}

func (e *ConfigError) Error() string { return e.Err.Error() }
func (e *ConfigError) Unwrap() error { return e.Err }

type RuntimeError struct {
	Err error
}

func (e *RuntimeError) Error() string { return e.Err.Error() }
func (e *RuntimeError) Unwrap() error { return e.Err }

func configErrorf(format string, args ...any) error {
	return &ConfigError{Err: fmt.Errorf(format, args...)}
}

func runtimeErrorf(format string, args ...any) error {
	return &RuntimeError{Err: fmt.Errorf(format, args...)}
}

func IsConfigError(err error) bool {
	var target *ConfigError
	return errors.As(err, &target)
}

func IsRuntimeError(err error) bool {
	var target *RuntimeError
	return errors.As(err, &target)
}

type TransportOptions struct {
	Mode                string
	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

type Options struct {
	BaseURL      string
	APIKey       string
	TokenProfile string
	Path         string
	Model        string
	Scenario     string
	Concurrency  int
	RPS          float64
	Duration     time.Duration
	MaxRequests  int
	RampStep     int
	RampInterval time.Duration
	Timeout      time.Duration
	InputBytes   int
	Stream       bool
	Transport    TransportOptions
	RunContext   artifact.RunContext
	HTTPClient   *http.Client
}

type StreamRecord struct {
	DoneReceived           bool
	CompletedEventReceived bool
	FirstTokenReceived     bool
	Usage                  artifact.Usage
	UsageEvents            int
	Chunks                 int
	Bytes                  int64
}

type HealthCheckOptions struct {
	BaseURL       string
	ValidAPIKey   string
	InvalidAPIKey string
	RuntimeURL    string
	PprofURL      string
	Timeout       time.Duration
	HTTPClient    *http.Client
}

type HealthCheckResult struct {
	SchemaVersion int                          `json:"schema_version"`
	Passed        bool                         `json:"passed"`
	Checks        map[string]artifact.Statused `json:"checks"`
}

type responseUsageWire struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type responsesCompletedEnvelope struct {
	Type     string `json:"type"`
	Response struct {
		Usage responseUsageWire `json:"usage"`
	} `json:"response"`
	Usage responseUsageWire `json:"usage"`
}

type responsesKindEnvelope struct {
	Type string `json:"type"`
}

func ParseResponsesStream(r io.Reader) (StreamRecord, error) {
	return parseResponsesStream(r, nil)
}

func ParseChatCompletionsStream(r io.Reader) (StreamRecord, error) {
	return parseChatCompletionsStream(r, nil)
}

func parseResponsesStream(r io.Reader, firstToken func()) (StreamRecord, error) {
	reader := bufio.NewReader(r)
	var rec StreamRecord
	var eventName string
	var dataLines []string
	firstTokenNotified := false

	dispatch := func() error {
		if eventName == "" && len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		trimmedData := strings.TrimSpace(data)
		defer func() {
			eventName = ""
			dataLines = dataLines[:0]
		}()

		if trimmedData == "[DONE]" {
			rec.DoneReceived = true
			return nil
		}
		if trimmedData == "" {
			return nil
		}

		if eventName == "response.output_text.delta" || strings.Contains(trimmedData, `"delta"`) {
			rec.Chunks++
			if !rec.FirstTokenReceived {
				rec.FirstTokenReceived = true
				if !firstTokenNotified && firstToken != nil {
					firstTokenNotified = true
					firstToken()
				}
			}
		}

		kind := responsesKindEnvelope{}
		kindErr := common.Unmarshal([]byte(trimmedData), &kind)
		if eventName != "response.completed" && (kindErr != nil || kind.Type != "response.completed") {
			return nil
		}

		envelope := responsesCompletedEnvelope{}
		if err := common.Unmarshal([]byte(trimmedData), &envelope); err != nil {
			return fmt.Errorf("parse response.completed SSE event: %w", err)
		}
		usage := envelope.Response.Usage
		if usage.TotalTokens == 0 && usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
			usage = envelope.Usage
		}
		rec.Usage = artifactUsage(usage)
		rec.UsageEvents++
		rec.CompletedEventReceived = true
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			rec.Bytes += int64(len(line))
			line = strings.TrimRight(line, "\r\n")
			switch {
			case line == "":
				if dispatchErr := dispatch(); dispatchErr != nil {
					return rec, dispatchErr
				}
			case strings.HasPrefix(line, ":"):
				continue
			case strings.HasPrefix(line, "event:"):
				eventName = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				data := line[len("data:"):]
				data = strings.TrimPrefix(data, " ")
				dataLines = append(dataLines, data)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if dispatchErr := dispatch(); dispatchErr != nil {
					return rec, dispatchErr
				}
				return rec, nil
			}
			return rec, err
		}
	}
}

func parseChatCompletionsStream(r io.Reader, firstToken func()) (StreamRecord, error) {
	reader := bufio.NewReader(r)
	var rec StreamRecord
	var dataLines []string
	firstTokenNotified := false

	dispatch := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		defer func() { dataLines = dataLines[:0] }()
		if data == "[DONE]" {
			rec.DoneReceived = true
			return nil
		}
		if data == "" {
			return nil
		}
		var envelope struct {
			Choices []struct {
				Delta map[string]any `json:"delta"`
			} `json:"choices"`
			Usage responseUsageWire `json:"usage"`
		}
		if err := common.Unmarshal([]byte(data), &envelope); err != nil {
			return fmt.Errorf("parse chat.completion.chunk SSE data: %w", err)
		}
		for _, choice := range envelope.Choices {
			if content, ok := choice.Delta["content"].(string); ok && content != "" {
				rec.Chunks++
				if !rec.FirstTokenReceived {
					rec.FirstTokenReceived = true
					if !firstTokenNotified && firstToken != nil {
						firstTokenNotified = true
						firstToken()
					}
				}
			}
		}
		usage := artifactUsage(envelope.Usage)
		if usage.TotalTokens != 0 || usage.PromptTokens != 0 || usage.CompletionTokens != 0 {
			rec.Usage = usage
			rec.UsageEvents++
			rec.CompletedEventReceived = true
		}
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			rec.Bytes += int64(len(line))
			line = strings.TrimRight(line, "\r\n")
			switch {
			case line == "":
				if dispatchErr := dispatch(); dispatchErr != nil {
					return rec, dispatchErr
				}
			case strings.HasPrefix(line, ":"):
				continue
			case strings.HasPrefix(line, "data:"):
				data := strings.TrimPrefix(line[len("data:"):], " ")
				dataLines = append(dataLines, data)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if dispatchErr := dispatch(); dispatchErr != nil {
					return rec, dispatchErr
				}
				return rec, nil
			}
			return rec, err
		}
	}
}

func artifactUsage(usage responseUsageWire) artifact.Usage {
	promptTokens := usage.PromptTokens
	if promptTokens == 0 {
		promptTokens = usage.InputTokens
	}
	completionTokens := usage.CompletionTokens
	if completionTokens == 0 {
		completionTokens = usage.OutputTokens
	}
	totalTokens := usage.TotalTokens
	if totalTokens == 0 && (promptTokens != 0 || completionTokens != 0) {
		totalTokens = promptTokens + completionTokens
	}
	return artifact.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

func ValidateTokenProfile(apiKey string, profile string) error {
	want, ok := tokenProfiles[apiKey]
	if !ok {
		return configErrorf("api key is not a loadtest key")
	}
	if profile != want {
		return configErrorf("api key profile mismatch: key requires %q, got %q", want, profile)
	}
	return nil
}

func RunOnceForTest(baseURL string, apiKey string, tokenProfile string) (artifact.Summary, error) {
	return RunLoad(context.Background(), Options{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		TokenProfile: tokenProfile,
		Path:         DefaultPath,
		Model:        DefaultModel,
		Scenario:     "test",
		Concurrency:  1,
		MaxRequests:  1,
		Timeout:      5 * time.Second,
		InputBytes:   16,
		Stream:       true,
	})
}

func RunLoad(ctx context.Context, opts Options) (artifact.Summary, error) {
	if err := normalizeAndValidateOptions(&opts); err != nil {
		return artifact.Summary{}, err
	}

	httpClient := opts.HTTPClient
	var ownedTransport *http.Transport
	if httpClient == nil {
		var err error
		ownedTransport, _, err = newTransport(opts.Transport)
		if err != nil {
			return artifact.Summary{}, err
		}
		httpClient = &http.Client{Transport: ownedTransport}
	}
	if ownedTransport != nil {
		defer ownedTransport.CloseIdleConnections()
	}

	runCtx, cancel := context.WithCancel(ctx)
	if opts.Duration > 0 {
		runCtx, cancel = context.WithTimeout(ctx, opts.Duration)
	}
	defer cancel()

	startedAt := time.Now()
	jobs := make(chan int, opts.Concurrency)
	results := make(chan requestResult, opts.Concurrency)
	var wg sync.WaitGroup
	var state requestState

	for workerID := 1; workerID <= opts.Concurrency; workerID++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				if err := waitForRamp(runCtx, workerID, startedAt, opts); err != nil {
					return
				}
				select {
				case <-runCtx.Done():
					return
				case requestIndex, ok := <-jobs:
					if !ok {
						return
					}
					results <- doOne(runCtx, httpClient, opts, requestIndex, &state)
				}
			}
		}(workerID)
	}

	go func() {
		defer close(jobs)
		sent := 0
		var ticker *time.Ticker
		if opts.RPS > 0 {
			interval := time.Duration(float64(time.Second) / opts.RPS)
			if interval <= 0 {
				interval = time.Nanosecond
			}
			ticker = time.NewTicker(interval)
			defer ticker.Stop()
		}
		for {
			if opts.MaxRequests > 0 && sent >= opts.MaxRequests {
				return
			}
			if opts.MaxRequests == 0 && opts.Duration <= 0 {
				return
			}
			if ticker != nil {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
				}
			}
			select {
			case <-runCtx.Done():
				return
			default:
			}
			sent++
			select {
			case <-runCtx.Done():
				return
			case jobs <- sent:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	requestResults := make([]requestResult, 0, maxInt(opts.MaxRequests, opts.Concurrency))
	for result := range results {
		requestResults = append(requestResults, result)
	}
	endedAt := time.Now()

	summary := buildSummary(opts, requestResults, state.maxInFlight.Load(), startedAt, endedAt, runCtx.Err())
	if hasNetworkRuntimeFailure(requestResults) {
		return summary, runtimeErrorf("one or more requests failed before receiving an HTTP response")
	}
	return summary, nil
}

func normalizeAndValidateOptions(opts *Options) error {
	if strings.TrimSpace(opts.BaseURL) == "" {
		return configErrorf("--url is required")
	}
	if err := ValidateLoopbackURL(opts.BaseURL); err != nil {
		return err
	}
	if err := ValidateTokenProfile(opts.APIKey, opts.TokenProfile); err != nil {
		return err
	}
	if opts.Path == "" {
		opts.Path = DefaultPath
	}
	if strings.Contains(opts.Path, "://") || !strings.HasPrefix(opts.Path, "/") {
		return configErrorf("--path must be an absolute HTTP path")
	}
	if opts.Model == "" {
		opts.Model = DefaultModel
	}
	if opts.Concurrency <= 0 {
		return configErrorf("--concurrency must be greater than zero")
	}
	if opts.RPS < 0 {
		return configErrorf("--rps must be zero or greater")
	}
	if opts.Duration <= 0 && opts.MaxRequests <= 0 {
		return configErrorf("at least one of --duration or --max-requests is required")
	}
	if opts.MaxRequests < 0 {
		return configErrorf("--max-requests must be zero or greater")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.InputBytes < 0 {
		return configErrorf("--input-bytes must be zero or greater")
	}
	if opts.RampStep < 0 {
		return configErrorf("--ramp-step must be zero or greater")
	}
	if err := normalizeTransportOptions(&opts.Transport); err != nil {
		return err
	}
	if opts.RunContext.SchemaVersion == 0 {
		opts.RunContext.SchemaVersion = artifact.SchemaVersion
	}
	if opts.RunContext.Path == "" {
		opts.RunContext.Path = opts.Path
	}
	if opts.RunContext.TokenProfile == "" {
		opts.RunContext.TokenProfile = opts.TokenProfile
	}
	if opts.RunContext.Model == "" {
		opts.RunContext.Model = opts.Model
	}
	if opts.RunContext.Scenario == "" {
		opts.RunContext.Scenario = opts.Scenario
	}
	return nil
}

type requestState struct {
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
}

type requestResult struct {
	record                artifact.RequestRecord
	latencyMS             float64
	ttftMS                float64
	protocol              string
	phase                 string
	stream                StreamRecord
	networkRuntimeFailure bool
}

func waitForRamp(ctx context.Context, workerID int, startedAt time.Time, opts Options) error {
	for workerID > allowedConcurrency(time.Since(startedAt), opts) {
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func allowedConcurrency(elapsed time.Duration, opts Options) int {
	if opts.RampStep <= 0 || opts.RampInterval <= 0 || opts.RampStep >= opts.Concurrency {
		return opts.Concurrency
	}
	steps := int(elapsed/opts.RampInterval) + 1
	allowed := steps * opts.RampStep
	if allowed < 1 {
		return 1
	}
	if allowed > opts.Concurrency {
		return opts.Concurrency
	}
	return allowed
}

func doOne(parent context.Context, httpClient *http.Client, opts Options, requestIndex int, state *requestState) requestResult {
	clientRequestID := "client-loadtest-" + strconv.Itoa(requestIndex)
	record := artifact.RequestRecord{
		RequestIndex:    requestIndex,
		ClientRequestID: clientRequestID,
	}

	requestURL, err := joinBaseAndPath(opts.BaseURL, opts.Path)
	if err != nil {
		record.ErrorReason = "request_build_error"
		return requestResult{record: record, phase: "request_build"}
	}
	body, err := buildRequestBody(opts)
	if err != nil {
		record.ErrorReason = "request_build_error"
		return requestResult{record: record, phase: "request_build"}
	}

	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		record.ErrorReason = "request_build_error"
		return requestResult{record: record, phase: "request_build"}
	}
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	req.Header.Set("X-Loadtest-Client-Request-Id", clientRequestID)

	beginRequest(state)
	startedAt := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		endRequest(state)
		reason := classifyHTTPError(err, ctx, parent, opts.Duration)
		record.ErrorReason = reason
		return requestResult{record: record, latencyMS: elapsedMS(startedAt), phase: "http_client_do", networkRuntimeFailure: reason != "client_duration"}
	}
	defer resp.Body.Close()

	record.StatusCode = resp.StatusCode
	record.NewAPIRequestID = resp.Header.Get(common.RequestIdKey)
	record.UpstreamRequestID = resp.Header.Get(common.UpstreamRequestIdKey)
	record.MockRequestID = resp.Header.Get(common.UpstreamRequestIdKey)
	protocol := resp.Proto

	var firstTokenAt time.Time
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024*1024))
		endRequest(state)
		record.ErrorReason = "status_non_2xx"
		return requestResult{record: record, latencyMS: elapsedMS(startedAt), protocol: protocol, phase: "status_code"}
	}

	if opts.Stream || strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		var streamRecord StreamRecord
		var parseErr error
		if strings.Contains(opts.Path, "/chat/completions") {
			streamRecord, parseErr = parseChatCompletionsStream(resp.Body, func() { firstTokenAt = time.Now() })
		} else {
			streamRecord, parseErr = parseResponsesStream(resp.Body, func() { firstTokenAt = time.Now() })
		}
		endRequest(state)
		latency := elapsedMS(startedAt)
		if parseErr != nil {
			record.ErrorReason = classifyStreamParseError(parseErr, parent, opts.Duration)
			return requestResult{record: record, latencyMS: latency, protocol: protocol, phase: "stream_parse", stream: streamRecord}
		}
		if !streamRecord.DoneReceived {
			record.ErrorReason = "missing_done"
			return requestResult{record: record, latencyMS: latency, protocol: protocol, phase: "stream_parse", stream: streamRecord}
		}
		record.Success = true
		record.PromptTokens = streamRecord.Usage.PromptTokens
		record.CompletionTokens = streamRecord.Usage.CompletionTokens
		record.TotalTokens = streamRecord.Usage.TotalTokens
		ttft := latency
		if !firstTokenAt.IsZero() {
			ttft = firstTokenAt.Sub(startedAt).Seconds() * 1000
		}
		return requestResult{record: record, latencyMS: latency, ttftMS: ttft, protocol: protocol, stream: streamRecord}
	}

	bodyBytes, readErr := io.ReadAll(resp.Body)
	endRequest(state)
	latency := elapsedMS(startedAt)
	if readErr != nil {
		record.ErrorReason = "read_error"
		return requestResult{record: record, latencyMS: latency, protocol: protocol, phase: "response_read"}
	}
	usage := parseUsageFromJSON(bodyBytes)
	record.Success = true
	record.PromptTokens = usage.PromptTokens
	record.CompletionTokens = usage.CompletionTokens
	record.TotalTokens = usage.TotalTokens
	return requestResult{record: record, latencyMS: latency, ttftMS: latency, protocol: protocol}
}

func hasNetworkRuntimeFailure(results []requestResult) bool {
	for _, result := range results {
		if result.networkRuntimeFailure {
			return true
		}
	}
	return false
}

func beginRequest(state *requestState) {
	current := state.inFlight.Add(1)
	for {
		maxObserved := state.maxInFlight.Load()
		if current <= maxObserved || state.maxInFlight.CompareAndSwap(maxObserved, current) {
			return
		}
	}
}

func endRequest(state *requestState) {
	state.inFlight.Add(-1)
}

func newTransport(opts TransportOptions) (*http.Transport, artifact.TransportProfile, error) {
	if err := normalizeTransportOptions(&opts); err != nil {
		return nil, artifact.TransportProfile{}, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = opts.MaxIdleConns
	transport.MaxIdleConnsPerHost = opts.MaxIdleConnsPerHost
	transport.MaxConnsPerHost = opts.MaxConnsPerHost
	transport.IdleConnTimeout = 5 * time.Second
	if opts.Mode == TransportModeH1NoKeepAlive {
		transport.DisableKeepAlives = true
	}
	return transport, transportProfile(opts), nil
}

func normalizeTransportOptions(opts *TransportOptions) error {
	if opts.Mode == "" {
		opts.Mode = TransportModeH1KeepAlive
	}
	switch opts.Mode {
	case TransportModeH1KeepAlive, TransportModeH1NoKeepAlive:
	case TransportModeH2CDiagnostic:
		return configErrorf("h2c diagnostic transport is not implemented in this phase")
	default:
		return configErrorf("unsupported transport mode %q", opts.Mode)
	}
	if opts.MaxConnsPerHost < 0 {
		return configErrorf("--max-conns-per-host must be zero or greater")
	}
	if opts.MaxIdleConns < 0 {
		return configErrorf("--max-idle-conns must be zero or greater")
	}
	if opts.MaxIdleConnsPerHost < 0 {
		return configErrorf("--max-idle-conns-per-host must be zero or greater")
	}
	if opts.MaxConnsPerHost == 0 {
		opts.MaxConnsPerHost = defaultMaxClientConnsPerHost
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = opts.MaxConnsPerHost
	}
	if opts.MaxIdleConnsPerHost == 0 {
		opts.MaxIdleConnsPerHost = opts.MaxConnsPerHost
	}
	return nil
}

func transportProfile(opts TransportOptions) artifact.TransportProfile {
	return artifact.TransportProfile{
		Mode:                opts.Mode,
		MaxConnsPerHost:     opts.MaxConnsPerHost,
		MaxIdleConns:        opts.MaxIdleConns,
		MaxIdleConnsPerHost: opts.MaxIdleConnsPerHost,
	}
}

func classifyHTTPError(err error, ctx context.Context, loadCtx context.Context, loadDuration time.Duration) string {
	if loadDuration > 0 && loadCtx != nil && errors.Is(loadCtx.Err(), context.DeadlineExceeded) {
		return "client_duration"
	}
	if ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "request_timeout"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "request_timeout"
	}
	if errorIsErrno(err, 111, 61, 10061) {
		return "connect_refused"
	}
	if errorIsErrno(err, 104, 54, 10054) {
		return "connection_reset"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		var opErr *net.OpError
		if errors.As(err, &opErr) && (opErr.Op == "dial" || opErr.Op == "connect") {
			return "connect_timeout"
		}
		return "request_timeout"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "connection refused") || strings.Contains(message, "actively refused") || strings.Contains(message, "no connection could be made") {
		return "connect_refused"
	}
	if strings.Contains(message, "connection reset") || strings.Contains(message, "wsarecv") || strings.Contains(message, "forcibly closed") {
		return "connection_reset"
	}
	return "http_client_do_error"
}

func errorIsErrno(err error, values ...syscall.Errno) bool {
	for _, value := range values {
		if errors.Is(err, value) {
			return true
		}
	}
	return false
}

func classifyStreamParseError(err error, loadCtx context.Context, loadDuration time.Duration) string {
	if err == nil {
		return ""
	}
	if loadDuration > 0 && loadCtx != nil && errors.Is(loadCtx.Err(), context.DeadlineExceeded) {
		return "client_duration"
	}
	if errors.Is(err, io.EOF) {
		return "read_error"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "read") || strings.Contains(message, "unexpected eof") {
		return "read_error"
	}
	return "json_error"
}

func buildRequestBody(opts Options) ([]byte, error) {
	input := deterministicInput(opts.InputBytes)
	if strings.Contains(opts.Path, "/chat/completions") {
		payload := chatCompletionsRequest{
			Model: opts.Model,
			Messages: []chatMessage{{
				Role:    "user",
				Content: input,
			}},
			Stream:        opts.Stream,
			StreamOptions: streamOptions{IncludeUsage: true},
		}
		return common.Marshal(payload)
	}
	payload := responsesRequest{
		Model:  opts.Model,
		Input:  input,
		Stream: opts.Stream,
	}
	return common.Marshal(payload)
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type responsesRequest struct {
	Model  string `json:"model"`
	Input  string `json:"input"`
	Stream bool   `json:"stream"`
}

type chatCompletionsRequest struct {
	Model         string        `json:"model"`
	Messages      []chatMessage `json:"messages"`
	Stream        bool          `json:"stream"`
	StreamOptions streamOptions `json:"stream_options,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func deterministicInput(size int) string {
	if size <= 0 {
		return "loadtest"
	}
	return strings.Repeat("x", size)
}

func parseUsageFromJSON(body []byte) artifact.Usage {
	var envelope struct {
		Usage    responseUsageWire `json:"usage"`
		Response struct {
			Usage responseUsageWire `json:"usage"`
		} `json:"response"`
	}
	if err := common.Unmarshal(body, &envelope); err != nil {
		return artifact.Usage{}
	}
	usage := envelope.Usage
	if usage.TotalTokens == 0 && usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		usage = envelope.Response.Usage
	}
	return artifactUsage(usage)
}

func buildSummary(opts Options, results []requestResult, maxObservedInFlight int64, startedAt time.Time, endedAt time.Time, ctxErr error) artifact.Summary {
	sort.Slice(results, func(i int, j int) bool {
		return results[i].record.RequestIndex < results[j].record.RequestIndex
	})

	statusCodes := make(map[string]int)
	errorReasons := make(map[string]int)
	protocolCounts := make(map[string]int)
	errorSamples := make([]artifact.ErrorSample, 0, 10)
	requests := make([]artifact.RequestRecord, 0, len(results))
	latencies := make([]float64, 0, len(results))
	ttfts := make([]float64, 0, len(results))
	streamBytes := int64(0)
	streamUsageEvents := 0
	streamDone := false
	success := 0

	for _, result := range results {
		requests = append(requests, result.record)
		if result.record.StatusCode != 0 {
			statusCodes[strconv.Itoa(result.record.StatusCode)]++
		}
		if result.protocol != "" {
			protocolCounts[result.protocol]++
		}
		if result.record.Success {
			success++
			latencies = append(latencies, result.latencyMS)
			if result.ttftMS > 0 {
				ttfts = append(ttfts, result.ttftMS)
			}
		} else if result.record.ErrorReason != "" {
			errorReasons[result.record.ErrorReason]++
			if len(errorSamples) < 10 {
				errorSamples = append(errorSamples, artifact.ErrorSample{
					RequestIndex: result.record.RequestIndex,
					Phase:        result.phase,
					Reason:       result.record.ErrorReason,
					StatusCode:   result.record.StatusCode,
					LatencyMS:    result.latencyMS,
					RequestID:    result.record.NewAPIRequestID,
				})
			}
		}
		if result.stream.Bytes != 0 || result.stream.DoneReceived || result.stream.UsageEvents != 0 {
			streamBytes += result.stream.Bytes
			streamUsageEvents += result.stream.UsageEvents
			streamDone = streamDone || result.stream.DoneReceived
		}
	}

	durationSeconds := endedAt.Sub(startedAt).Seconds()
	requestsPerSecond := 0.0
	if durationSeconds > 0 {
		requestsPerSecond = float64(len(results)) / durationSeconds
	}
	stopReason := stopReason(len(results), opts, ctxErr)

	return artifact.Summary{
		SchemaVersion:       artifact.SchemaVersion,
		RunContext:          opts.RunContext,
		Path:                opts.Path,
		Scenario:            opts.Scenario,
		TokenProfile:        opts.TokenProfile,
		Model:               opts.Model,
		TargetConcurrency:   opts.Concurrency,
		Total:               len(results),
		Success:             success,
		Errors:              len(results) - success,
		StatusCodes:         statusCodes,
		ErrorReasons:        errorReasons,
		ProtocolCounts:      protocolCounts,
		MaxObservedInFlight: int(maxObservedInFlight),
		LatencyP95MS:        percentile(latencies, 0.95),
		TTFTP95MS:           percentile(ttfts, 0.95),
		RequestsPerSecond:   requestsPerSecond,
		Stream: artifact.StreamStats{
			DoneReceived: streamDone,
			UsageEvents:  streamUsageEvents,
			Bytes:        streamBytes,
		},
		Requests:          requests,
		FirstErrorSamples: errorSamples,
		Transport:         transportProfile(opts.Transport),
		StopReason:        stopReason,
	}
}

func stopReason(total int, opts Options, ctxErr error) string {
	if opts.MaxRequests > 0 && total >= opts.MaxRequests {
		return "max_requests"
	}
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		return "duration"
	}
	if errors.Is(ctxErr, context.Canceled) {
		return "context_cancelled"
	}
	if opts.Duration > 0 && opts.MaxRequests == 0 {
		return "duration"
	}
	return "max_requests"
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	idx := int(math.Ceil(float64(len(values))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func elapsedMS(startedAt time.Time) float64 {
	return time.Since(startedAt).Seconds() * 1000
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func HealthCheck(ctx context.Context, opts HealthCheckOptions) (HealthCheckResult, error) {
	if err := validateHealthCheckOptions(&opts); err != nil {
		return HealthCheckResult{}, err
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	checks := map[string]artifact.Statused{
		"api_status":             healthGET(ctx, httpClient, opts.Timeout, mustJoin(opts.BaseURL, "/api/status"), "", expectStatus2xx),
		"runtime_stats":          healthGET(ctx, httpClient, opts.Timeout, opts.RuntimeURL, "", expectRuntimeStats),
		"pprof_goroutine":        healthGET(ctx, httpClient, opts.Timeout, opts.PprofURL, "", expectPprofGoroutine),
		"models_valid_token":     healthGET(ctx, httpClient, opts.Timeout, mustJoin(opts.BaseURL, "/v1/models"), opts.ValidAPIKey, expectStatus2xx),
		"invalid_token_rejected": healthGET(ctx, httpClient, opts.Timeout, mustJoin(opts.BaseURL, "/v1/models"), opts.InvalidAPIKey, expectStatus401),
	}
	passed := true
	for _, check := range checks {
		if check.Status != "passed" {
			passed = false
			break
		}
	}
	return HealthCheckResult{SchemaVersion: artifact.SchemaVersion, Passed: passed, Checks: checks}, nil
}

func validateHealthCheckOptions(opts *HealthCheckOptions) error {
	if strings.TrimSpace(opts.BaseURL) == "" {
		return configErrorf("--url is required")
	}
	if err := ValidateLoopbackURL(opts.BaseURL); err != nil {
		return err
	}
	if _, ok := tokenProfiles[opts.ValidAPIKey]; !ok || opts.ValidAPIKey == "sk-loadtestinvalid" {
		return configErrorf("--valid-api-key must be a valid fixed loadtest key")
	}
	if err := ValidateTokenProfile(opts.InvalidAPIKey, "invalid"); err != nil {
		return err
	}
	if opts.RuntimeURL == "" {
		opts.RuntimeURL = mustJoin(opts.BaseURL, "/debug/loadtest/runtime")
	}
	if opts.PprofURL == "" {
		opts.PprofURL = mustJoin(opts.BaseURL, "/debug/pprof/goroutine?debug=1")
	}
	for _, rawURL := range []string{opts.RuntimeURL, opts.PprofURL} {
		if err := ValidateLoopbackURL(rawURL); err != nil {
			return err
		}
	}
	return nil
}

type healthExpectation func(status int, body []byte) artifact.Statused

func healthGET(ctx context.Context, httpClient *http.Client, timeout time.Duration, rawURL string, apiKey string, expect healthExpectation) artifact.Statused {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return artifact.Statused{Status: "failed", Reason: "request build failed: " + err.Error()}
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return artifact.Statused{Status: "failed", Reason: "request failed: " + err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return artifact.Statused{Status: "failed", Reason: "read failed: " + err.Error()}
	}
	return expect(resp.StatusCode, body)
}

func expectStatus2xx(status int, _ []byte) artifact.Statused {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return artifact.Statused{Status: "passed"}
	}
	return artifact.Statused{Status: "failed", Reason: "unexpected status " + strconv.Itoa(status)}
}

func expectStatus401(status int, _ []byte) artifact.Statused {
	if status == http.StatusUnauthorized {
		return artifact.Statused{Status: "passed"}
	}
	return artifact.Statused{Status: "failed", Reason: "unexpected status " + strconv.Itoa(status)}
}

func expectRuntimeStats(status int, body []byte) artifact.Statused {
	if status != http.StatusOK {
		return artifact.Statused{Status: "failed", Reason: "unexpected status " + strconv.Itoa(status)}
	}
	var payload struct {
		Goroutines int `json:"goroutines"`
	}
	if err := common.Unmarshal(body, &payload); err != nil {
		return artifact.Statused{Status: "failed", Reason: "invalid runtime JSON"}
	}
	if payload.Goroutines <= 0 {
		return artifact.Statused{Status: "failed", Reason: "goroutines missing"}
	}
	return artifact.Statused{Status: "passed"}
}

func expectPprofGoroutine(status int, body []byte) artifact.Statused {
	if status != http.StatusOK {
		return artifact.Statused{Status: "failed", Reason: "unexpected status " + strconv.Itoa(status)}
	}
	if !bytes.Contains(bytes.ToLower(body), []byte("goroutine")) {
		return artifact.Statused{Status: "failed", Reason: "goroutine profile missing"}
	}
	return artifact.Statused{Status: "passed"}
}

func HealthResultHasRuntimeFailure(result HealthCheckResult) bool {
	for _, check := range result.Checks {
		if check.Status != "passed" && (strings.Contains(check.Reason, "request failed:") || strings.Contains(check.Reason, "read failed:")) {
			return true
		}
	}
	return false
}

func ValidateLoopbackURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return configErrorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return configErrorf("URL scheme must be http or https")
	}
	if parsed.User != nil {
		return configErrorf("URL userinfo is not allowed")
	}
	host := parsed.Hostname()
	if host == "" || !hostIsLoopback(host) {
		return configErrorf("URL host must be loopback")
	}
	return nil
}

func hostIsLoopback(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func joinBaseAndPath(baseURL string, requestPath string) (string, error) {
	if strings.Contains(requestPath, "://") || !strings.HasPrefix(requestPath, "/") {
		return "", configErrorf("request path must be an absolute path")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	parsed.Path = requestPath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func mustJoin(baseURL string, requestPath string) string {
	joined, err := joinBaseAndPath(baseURL, requestPath)
	if err != nil {
		return baseURL
	}
	return joined
}
