package streamgate

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"unsafe"

	"github.com/QuantumNous/new-api/types"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type classification struct {
	meaningful bool
	terminal   bool
	hold       bool
	created    bool
	unsafe     bool
	failed     bool
	err        *types.NewAPIError
}

func parse(raw []byte) gjson.Result {
	return gjson.Parse(unsafe.String(unsafe.SliceData(raw), len(raw)))
}

func nonempty(value gjson.Result) bool {
	if !value.Exists() || value.Type == gjson.Null {
		return false
	}
	if value.Type == gjson.String {
		return value.Str != ""
	}
	if value.IsArray() || value.IsObject() {
		found := false
		value.ForEach(func(_, child gjson.Result) bool {
			if nonempty(child) {
				found = true
			}
			return !found
		})
		return found
	}
	return true
}

func business(value gjson.Result) bool {
	if !nonempty(value) {
		return false
	}
	if value.IsArray() {
		found := false
		value.ForEach(func(_, child gjson.Result) bool {
			found = business(child)
			return !found
		})
		return found
	}
	if !value.IsObject() {
		return true
	}
	typ := value.Get("type").String()
	switch typ {
	case "", "message", "reasoning", "text", "output_text", "input_text", "thinking", "redacted_thinking", "summary_text", "refusal", "text_delta", "thinking_delta", "input_json_delta", "signature_delta":
	default:
		return true
	}
	found := false
	value.ForEach(func(key, child gjson.Result) bool {
		switch key.Str {
		case "type", "id", "role", "status", "index", "output_index", "content_index":
		case "content", "summary", "text", "thinking", "reasoning", "refusal":
			found = business(child)
		default:
			found = nonempty(child)
		}
		return !found
	})
	return found
}

func hasUnknown(value gjson.Result, allowed string) bool {
	found := false
	value.ForEach(func(key, child gjson.Result) bool {
		if !strings.Contains("|"+allowed+"|", "|"+key.Str+"|") && nonempty(child) {
			found = true
		}
		return !found
	})
	return found
}

func responseBusiness(root gjson.Result) bool {
	for _, path := range []string{"response.output", "response.output_text", "output", "item", "part", "delta", "text", "refusal", "audio", "image", "result", "arguments", "name"} {
		if business(root.Get(path)) {
			return true
		}
	}
	return false
}

// ResponsesMeaningfulOutput applies the same payload boundary to JSON responses.
func ResponsesMeaningfulOutput(raw []byte) bool {
	root := parse(raw)
	return responseBusiness(root) || business(root.Get("output_text"))
}

// ResponsesFailure retains the shared provider error/status classification.
func ResponsesFailure(raw []byte) *types.NewAPIError {
	return failure(parse(raw), types.RelayFormatOpenAIResponses)
}

func failure(root gjson.Result, format string) *types.NewAPIError {
	errValue := root.Get("response.error")
	if !nonempty(errValue) {
		errValue = root.Get("error")
	}
	if !nonempty(errValue) {
		errValue = root
	}
	status := int(errValue.Get("status_code").Int())
	if status < 400 || status > 599 {
		status = int(errValue.Get("status").Int())
	}
	if status < 400 || status > 599 {
		status = int(root.Get("status").Int())
	}
	if status < 400 || status > 599 {
		status = int(errValue.Get("code").Int())
	}
	if status < 400 || status > 599 {
		switch errValue.Get("type").String() + "/" + errValue.Get("code").String() {
		case "authentication_error/", "invalid_api_key/", "invalid_request_error/invalid_api_key":
			status = http.StatusUnauthorized
		case "permission_error/":
			status = http.StatusForbidden
		case "rate_limit_error/", "rate_limit_exceeded/", "tokens/rate_limit_exceeded":
			status = http.StatusTooManyRequests
		case "invalid_request_error/":
			status = http.StatusBadRequest
		case "overloaded_error/":
			status = http.StatusServiceUnavailable
		default:
			status = http.StatusBadGateway
		}
	}
	if format == types.RelayFormatClaude {
		typ := strings.Clone(errValue.Get("type").String())
		if typ == "" || typ == "error" {
			typ = "api_error"
		}
		message := strings.Clone(errValue.Get("message").String())
		if message == "" {
			message = "Upstream stream failed"
		}
		return types.WithClaudeError(types.ClaudeError{Type: typ, Message: message}, status)
	}
	var code any
	if value := errValue.Get("code"); value.Exists() {
		code = value.Value()
		if text, ok := code.(string); ok {
			code = strings.Clone(text)
		}
	}
	if code == nil {
		code = "upstream_stream_error"
	}
	typ := strings.Clone(errValue.Get("type").String())
	if typ == "" || typ == "error" || strings.HasPrefix(typ, "response.") {
		typ = "upstream_error"
	}
	message := strings.Clone(errValue.Get("message").String())
	if message == "" {
		message = "Upstream stream failed"
	}
	var options []types.NewAPIErrorOptions
	codeText, _ := code.(string)
	if codeText == "content_filter" || codeText == "content_policy_violation" || codeText == "safety" || codeText == "prompt_blocked" {
		options = append(options, types.ErrOptionWithSkipRetry())
		status = http.StatusBadRequest
	}
	return types.WithOpenAIError(types.OpenAIError{Type: typ, Code: code, Message: message}, status, options...)
}

func classify(root gjson.Result, event, format string) classification {
	result := classification{}
	typ := root.Get("type").String()
	if typ == "" {
		typ = event
	}
	if typ == "ping" || typ == "heartbeat" || typ == "keepalive" {
		result.meaningful = hasUnknown(root, "type|timestamp")
		return result
	}
	if strings.HasPrefix(typ, "response.") || format == types.RelayFormatOpenAIResponses || format == "responses" {
		result.meaningful = responseBusiness(root)
		if nonempty(root.Get("error")) || typ == "error" || typ == "response.failed" || typ == "response.error" {
			result.failed = true
			result.err = failure(root, format)
			result.terminal = true
			return result
		}
		switch typ {
		case "response.created", "response.in_progress", "response.queued":
			result.created = typ == "response.created"
			result.unsafe = typ == "response.queued" || root.Get("response.background").Bool()
			result.meaningful = result.meaningful || hasUnknown(root, "type|sequence_number|response|response_id|stream_id|event_id") || hasUnknown(root.Get("response"), "id|object|created_at|status|background|error|incomplete_details|instructions|max_output_tokens|max_tool_calls|model|output|output_text|parallel_tool_calls|previous_response_id|prompt|prompt_cache_key|prompt_cache_retention|reasoning|safety_identifier|service_tier|store|temperature|text|tool_choice|tools|top_logprobs|top_p|truncation|usage|user|metadata|conversation|completed_at")
		case "response.output_item.added", "response.content_part.added", "response.reasoning_summary_part.added", "response.output_item.done", "response.content_part.done", "response.reasoning_summary_part.done":
			result.meaningful = result.meaningful || hasUnknown(root, "type|sequence_number|response_id|output_index|content_index|summary_index|item_id|item|part")
			result.hold = !result.meaningful
		case "response.output_text.delta", "response.output_text.done", "response.refusal.delta", "response.refusal.done", "response.reasoning_text.delta", "response.reasoning_text.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done", "response.function_call_arguments.delta", "response.function_call_arguments.done":
			result.meaningful = result.meaningful || hasUnknown(root, "type|sequence_number|response_id|item_id|output_index|content_index|summary_index|delta|text|refusal|arguments")
			result.hold = !result.meaningful
		case "response.completed", "response.done", "response.cancelled", "response.canceled":
			result.terminal = true
		case "response.incomplete":
			result.terminal = true
			reason := root.Get("response.incomplete_details.reason").String()
			if reason == "server_error" || reason == "overloaded" || reason == "rate_limit_exceeded" || reason == "timeout" {
				result.failed = true
				result.err = types.WithOpenAIError(types.OpenAIError{Type: "upstream_error", Code: reason, Message: "Upstream response incomplete: " + reason}, http.StatusBadGateway)
			}
		default:
			result.meaningful = true
		}
		if strings.Contains(typ, "web_search_call.") || strings.Contains(typ, "file_search_call.") || strings.Contains(typ, "code_interpreter_call.") || strings.Contains(typ, "computer_call.") || strings.Contains(typ, "mcp_call.") {
			result.unsafe = true
		}
		itemType := root.Get("item.type").String()
		if itemType == "web_search_call" || itemType == "file_search_call" || itemType == "code_interpreter_call" || itemType == "computer_call" || itemType == "mcp_call" {
			result.unsafe = true
		}
		return result
	}
	if typ == "error" || nonempty(root.Get("error")) {
		result.failed = true
		result.terminal = true
		result.err = failure(root, format)
		return result
	}
	if format == types.RelayFormatClaude || strings.HasPrefix(typ, "message_") || strings.HasPrefix(typ, "content_block_") {
		switch typ {
		case "message_start":
			result.created = true
			result.meaningful = business(root.Get("message.content")) || hasUnknown(root, "type|message") || hasUnknown(root.Get("message"), "id|type|role|content|model|stop_reason|stop_sequence|usage|container")
		case "content_block_start":
			block := root.Get("content_block")
			result.meaningful = business(block)
			result.unsafe = block.Get("type").String() == "server_tool_use"
			result.hold = !result.meaningful
		case "content_block_delta":
			result.meaningful = business(root.Get("delta"))
			result.hold = !result.meaningful
		case "content_block_stop":
			result.hold = true
		case "message_delta":
			result.meaningful = hasUnknown(root.Get("delta"), "stop_reason|stop_sequence")
		case "message_stop":
			result.terminal = true
		default:
			result.meaningful = true
		}
		return result
	}
	if format == types.RelayFormatGemini {
		if root.Get("promptFeedback.blockReason").String() != "" {
			result.terminal = true
		}
		candidates := root.Get("candidates")
		candidates.ForEach(func(_, candidate gjson.Result) bool {
			if business(candidate.Get("content.parts")) || hasUnknown(candidate, "content|finishReason|index|safetyRatings|citationMetadata|tokenCount|avgLogprobs|groundingMetadata|finishMessage") {
				result.meaningful = true
			}
			if reason := candidate.Get("finishReason"); reason.Exists() && reason.String() != "" {
				result.terminal = true
			}
			if nonempty(candidate.Get("citationMetadata")) || nonempty(candidate.Get("groundingMetadata")) {
				result.meaningful = true
			}
			return true
		})
		result.meaningful = result.meaningful || hasUnknown(root, "candidates|usageMetadata|modelVersion|responseId|promptFeedback|createTime")
		if !candidates.Exists() && !root.Get("usageMetadata").Exists() && !root.Get("promptFeedback").Exists() {
			result.meaningful = true
		}
		return result
	}
	choices := root.Get("choices")
	if choices.Exists() {
		choices.ForEach(func(_, choice gjson.Result) bool {
			delta := choice.Get("delta")
			if business(delta) || business(choice.Get("message")) || nonempty(choice.Get("text")) || nonempty(choice.Get("logprobs")) {
				result.meaningful = true
			}
			if finish := choice.Get("finish_reason"); finish.Exists() && finish.Type != gjson.Null {
				result.terminal = true
			}
			if hasUnknown(choice, "delta|message|text|index|finish_reason|logprobs") {
				result.meaningful = true
			}
			return true
		})
		result.meaningful = result.meaningful || hasUnknown(root, "id|object|created|model|choices|usage|system_fingerprint|service_tier")
		return result
	}
	result.meaningful = true
	return result
}

func (g *Gate) rememberUpstream(root gjson.Result) {
	for _, path := range []string{"response.id", "response_id", "message.id", "responseId", "id"} {
		if path == "id" && (strings.HasPrefix(root.Get("type").String(), "response.") || root.Get("type").String() == "error") {
			continue
		}
		if id := root.Get(path).String(); id != "" {
			g.upstreamID = strings.Clone(id)
			if g.responseID == "" {
				g.responseID = g.publicID
				if g.responseID == "" {
					g.responseID = strings.Clone(id)
				}
			}
			break
		}
	}
	if root.Get("choices").Exists() {
		if created := root.Get("created"); created.Exists() && !g.chatCreated.Exists() {
			g.chatCreated = gjson.Parse(strings.Clone(created.Raw))
		}
	}
}

func postTerminalMetadata(root gjson.Result, format string) bool {
	if format == string(types.RelayFormatOpenAI) {
		return root.Get("choices").IsArray() && root.Get("choices.#").Int() == 0 && nonempty(root.Get("usage")) && !hasUnknown(root, "id|object|created|model|choices|usage|system_fingerprint|service_tier")
	}
	if format == string(types.RelayFormatGemini) {
		return !root.Get("candidates").Exists() && nonempty(root.Get("usageMetadata"))
	}
	return false
}

func (g *Gate) patch(raw []byte) ([]byte, error) {
	root := parse(raw)
	var err error
	patch := func(path string, value any) {
		if err == nil {
			raw, err = sjson.SetBytes(raw, path, value)
		}
	}
	for _, path := range []string{"response.id", "response_id", "message.id", "responseId", "id"} {
		id := root.Get(path).String()
		if id == "" {
			continue
		}
		if path == "id" && (strings.HasPrefix(root.Get("type").String(), "response.") || root.Get("type").String() == "error") {
			continue
		}
		if g.responseID == "" {
			g.responseID = g.publicID
			if g.responseID == "" {
				g.responseID = strings.Clone(id)
			}
		}
		if id != g.responseID {
			patch(path, g.responseID)
		}
	}
	if root.Get("choices").Exists() {
		if created := root.Get("created"); created.Exists() {
			if !g.chatCreated.Exists() {
				g.chatCreated = gjson.Parse(strings.Clone(created.Raw))
			} else if created.Raw != g.chatCreated.Raw {
				patch("created", g.chatCreated.Value())
			}
		}
	}
	if typ := root.Get("type").String(); strings.HasPrefix(typ, "response.") || typ == "error" && isResponsesFormat(g.format) {
		incoming := root.Get("sequence_number")
		if incoming.Exists() {
			g.sequenced = true
		}
		if incoming.Exists() || g.attempt > 1 && g.sequenced {
			next := g.sequence
			if g.attempt <= 1 && incoming.Exists() {
				next = max(next, incoming.Int())
			}
			if !incoming.Exists() || incoming.Int() != next {
				patch("sequence_number", next)
			}
			g.sequence = next + 1
		} else {
			g.sequence++
		}
	}
	return raw, err
}
func isResponsesFormat(format string) bool {
	return strings.HasPrefix(format, "openai_responses") || format == "responses"
}

func (g *Gate) bindResponse() error {
	if g.onResponse == nil || g.responseID == "" || g.upstreamID == "" || g.boundUpstream == g.upstreamID {
		return nil
	}
	if err := g.onResponse(g.responseID, g.upstreamID); err != nil {
		g.attemptErr = types.NewError(errors.New("response routing storage is unavailable"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return g.attemptErr
	}
	g.boundUpstream = g.upstreamID
	return nil
}

func (g *Gate) queueHeld() error {
	for _, held := range g.held {
		out, err := g.patch(held.raw)
		if err != nil {
			return err
		}
		g.ready = append(g.ready, out)
		g.readyEvents = append(g.readyEvents, held.event)
	}
	g.held = nil
	g.heldBytes = 0
	return nil
}

func (g *Gate) transform(raw []byte, event string) ([]byte, bool, error) {
	g.ensureAttempt()
	g.stream = true
	if g.writeErr != nil {
		return nil, false, g.writeErr
	}
	if g.done {
		return nil, false, nil
	}
	if len(raw) > MaxFrameBytes {
		return nil, false, g.tooLarge()
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("[DONE]")) {
		if g.terminal && g.allowDone && g.attemptErr == nil {
			g.done = true
			return raw, true, nil
		}
		if !g.terminal && g.attemptErr == nil {
			g.attemptErr = types.NewOpenAIError(errors.New("upstream stream ended before terminal event"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		return nil, false, nil
	}
	// A failed attempt cannot later become a successful logical response.
	if g.attemptErr != nil {
		return nil, false, nil
	}
	if !gjson.ValidBytes(trimmed) {
		g.attemptErr = types.NewOpenAIError(errors.New("invalid upstream stream event"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		return nil, false, g.attemptErr
	}
	root := parse(trimmed)
	if g.terminal {
		if postTerminalMetadata(root, g.format) {
			out, err := g.patch(trimmed)
			return out, err == nil, err
		}
		return nil, false, nil
	}
	result := classify(root, event, g.format)
	g.rememberUpstream(root)
	if result.unsafe {
		g.unsafe = true
	}
	if result.failed {
		g.attemptErr = result.err
		// Preserve the provider's failure shape and usage, but never forward an
		// unmasked diagnostic message or provider error metadata.
		for _, path := range []string{"response.error", "error"} {
			if message := root.Get(path + ".message"); message.Exists() {
				masked := result.err.MaskSensitiveError()
				if masked != message.String() {
					var err error
					trimmed, err = sjson.SetBytes(trimmed, path+".message", masked)
					if err != nil {
						return nil, false, err
					}
				}
			}
			if root.Get(path + ".metadata").Exists() {
				var err error
				trimmed, err = sjson.DeleteBytes(trimmed, path+".metadata")
				if err != nil {
					return nil, false, err
				}
			}
		}
		if root.Get("type").String() == "error" && root.Get("message").Exists() {
			var err error
			trimmed, err = sjson.SetBytes(trimmed, "message", result.err.MaskSensitiveError())
			if err != nil {
				return nil, false, err
			}
		}
		if !result.meaningful && !g.meaningful {
			g.failureRaw = bytes.Clone(trimmed)
			g.held, g.heldBytes = nil, 0
			return nil, false, nil
		}
		if err := g.bindResponse(); err != nil {
			return nil, false, err
		}
		if err := g.queueHeld(); err != nil {
			return nil, false, err
		}
		out, err := g.patch(trimmed)
		if err != nil {
			return nil, false, err
		}
		g.meaningful, g.terminal, g.allowDone = true, true, false
		return out, true, nil
	}
	if result.created && g.created {
		if !result.meaningful {
			return nil, false, nil
		}
		if isResponsesFormat(g.format) {
			var err error
			trimmed, err = sjson.SetBytes(trimmed, "type", "response.in_progress")
			if err != nil {
				return nil, false, err
			}
		} else {
			g.attemptErr = types.NewOpenAIError(errors.New("duplicate stream creation contains output"), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
			return nil, false, g.attemptErr
		}
	}
	if result.hold && !g.meaningful && !result.meaningful && !result.terminal {
		if g.heldBytes+len(trimmed) > MaxFrameBytes {
			return nil, false, g.tooLarge()
		}
		if event == "" {
			event = root.Get("type").String()
		}
		g.held = append(g.held, heldEvent{raw: bytes.Clone(trimmed), event: strings.Clone(event)})
		g.heldBytes += len(trimmed)
		return nil, false, nil
	}
	if err := g.bindResponse(); err != nil {
		return nil, false, err
	}
	if result.meaningful || result.terminal {
		if err := g.queueHeld(); err != nil {
			return nil, false, err
		}
		g.failureRaw = nil
	}
	out, err := g.patch(trimmed)
	if err != nil {
		return nil, false, err
	}
	g.created = g.created || result.created
	g.meaningful = g.meaningful || result.meaningful
	if result.terminal {
		g.terminal, g.allowDone = true, true
	}
	return out, true, nil
}
