package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesToChatStreamHandlerTextToolPrecedence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const toolEvent = `{"type":"response.output_item.added","item":{"id":"item_1","call_id":"call_1","type":"function_call","name":"lookup","arguments":""}}`
	const argsEvent = `{"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"q\":\"天气\"}"}`
	const textEvent = `{"type":"response.output_text.delta","delta":"你\n\"好\""}`
	const emptyEvent = `{"type":"response.output_text.delta","delta":""}`
	const nullEvent = `{"type":"response.output_text.delta","delta":null}`
	const reasoningEvent = `{"type":"response.reasoning_summary_text.delta","delta":"先分析"}`
	const reasoningDone = `{"type":"response.reasoning_summary_text.done"}`
	const moreReasoning = `{"type":"response.reasoning_summary_text.delta","delta":"再检查"}`

	for _, tc := range []struct {
		name      string
		events    []string
		text      []string
		reasoning []string
		tool      bool
		finish    string
	}{
		{name: "tools_only", events: []string{toolEvent, argsEvent}, tool: true, finish: "tool_calls"},
		{name: "empty_text_allows_tools", events: []string{emptyEvent, nullEvent, toolEvent, argsEvent}, tool: true, finish: "tool_calls"},
		{name: "reasoning_allows_tools", events: []string{reasoningEvent, reasoningDone, moreReasoning, toolEvent, argsEvent}, reasoning: []string{"先分析", "\n\n再检查"}, tool: true, finish: "tool_calls"},
		{name: "text_suppresses_tools", events: []string{textEvent, toolEvent, argsEvent}, text: []string{"你\n\"好\""}, finish: "stop"},
		{name: "text_after_tools_changes_finish", events: []string{toolEvent, argsEvent, textEvent}, text: []string{"你\n\"好\""}, tool: true, finish: "stop"},
		{name: "empty_delta_does_not_reset_text", events: []string{textEvent, emptyEvent, nullEvent, toolEvent, argsEvent}, text: []string{"你\n\"好\""}, finish: "stop"},
		{name: "whitespace_is_output", events: []string{`{"type":"response.output_text.delta","delta":" "}`, toolEvent, argsEvent}, text: []string{" "}, finish: "stop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, completed := range []bool{false, true} {
				t.Run(fmt.Sprintf("completed=%t", completed), func(t *testing.T) {
					var input strings.Builder
					for _, event := range tc.events {
						input.WriteString("data: " + event + "\n\n")
					}
					if completed {
						input.WriteString("data: " + `{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":17},"output":[]}}` + "\n\n")
					}
					input.WriteString("data: [DONE]\n\n")
					recorder := flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
					info := &relaycommon.RelayInfo{
						RelayFormat:        types.RelayFormatOpenAI,
						ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
						ShouldIncludeUsage: true,
						DisablePing:        true,
					}
					resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(input.String()))}
					usage, apiErr := OaiResponsesToChatStreamHandler(ctx, info, resp)
					require.Nil(t, apiErr)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.True(t, recorder.Flushed)

					var text, reasoning, finish []string
					var tools []dto.ToolCallResponse
					startCount, doneCount, usageCount, billingCount := 0, 0, 0, 0
					for _, line := range strings.Split(recorder.Body.String(), "\n") {
						if !strings.HasPrefix(line, "data: ") {
							continue
						}
						data := strings.TrimPrefix(line, "data: ")
						if data == "[DONE]" {
							doneCount++
							continue
						}
						var chunk dto.ChatCompletionsStreamResponse
						require.NoError(t, common.UnmarshalJsonStr(data, &chunk))
						if chunk.Usage != nil {
							usageCount++
							if completed {
								require.Equal(t, 17, chunk.Usage.TotalTokens)
							}
						}
						if chunk.NewAPIBilling != nil {
							billingCount++
						}
						for _, choice := range chunk.Choices {
							if choice.Delta.Role == "assistant" {
								startCount++
							}
							if value := choice.Delta.GetContentString(); value != "" {
								text = append(text, value)
							}
							if value := choice.Delta.GetReasoningContent(); value != "" {
								reasoning = append(reasoning, value)
							}
							tools = append(tools, choice.Delta.ToolCalls...)
							if choice.FinishReason != nil && *choice.FinishReason != "" {
								finish = append(finish, *choice.FinishReason)
							}
						}
					}
					require.Equal(t, tc.text, text)
					require.Equal(t, tc.reasoning, reasoning)
					require.Equal(t, []string{tc.finish}, finish)
					require.Equal(t, 1, startCount)
					require.Positive(t, doneCount)
					require.Equal(t, 1, usageCount)
					if tc.tool {
						require.Len(t, tools, 2)
						require.Equal(t, "lookup", tools[0].Function.Name)
						require.Equal(t, "", tools[1].Function.Name)
						require.Equal(t, `{"q":"天气"}`, tools[1].Function.Arguments)
						for _, tool := range tools {
							require.Equal(t, "call_1", tool.ID)
							require.NotNil(t, tool.Index)
							require.Zero(t, *tool.Index)
						}
					} else {
						require.Empty(t, tools)
					}
					if completed {
						require.NotNil(t, usage)
						require.Equal(t, 3, usage.PromptTokens)
						require.Equal(t, 2, usage.CompletionTokens)
						require.Equal(t, 17, usage.TotalTokens)
						require.True(t, info.HasTrustedUsage)
						require.Equal(t, 1, billingCount)
					} else {
						require.Nil(t, usage)
						require.False(t, info.HasTrustedUsage)
						require.Zero(t, billingCount)
					}
				})
			}
		})
	}
}

func BenchmarkOaiResponsesToChatStreamText(b *testing.B) {
	gin.SetMode(gin.TestMode)
	for _, size := range []int{4 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			const chunkSize = 1024
			delta := strings.Repeat("x", chunkSize)
			event := "data: " + `{"type":"response.output_text.delta","delta":"` + delta + `"}` + "\n\n"
			input := strings.Repeat(event, size/chunkSize) + "data: " + `{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}` + "\n\ndata: [DONE]\n\n"
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				recorder := httptest.NewRecorder()
				// Exercise the actual streaming writer without retaining client output.
				recorder.Body = nil
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				info := &relaycommon.RelayInfo{
					RelayFormat:        types.RelayFormatOpenAI,
					ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
					ShouldIncludeUsage: true,
					DisablePing:        true,
				}
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(input))}
				usage, apiErr := OaiResponsesToChatStreamHandler(ctx, info, resp)
				if apiErr != nil || usage == nil || usage.TotalTokens != 5 || !recorder.Flushed {
					b.Fatalf("unexpected stream result: usage=%+v error=%v flushed=%t", usage, apiErr, recorder.Flushed)
				}
			}
		})
	}
}
