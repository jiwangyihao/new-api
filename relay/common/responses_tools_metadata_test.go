package common

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func largeResponsesToolsJSON() json.RawMessage {
	parameters := `{"type":"object","properties":{"payload":{"type":"string","description":"` + strings.Repeat("x", 1<<20) + `"}}}`
	return json.RawMessage(`[{"type":"web_search_preview","search_context_size":"high","parameters":` + parameters + `},{"type":"file_search","vector_store_ids":["vs_1"]}]`)
}

func newResponsesToolsContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

func TestGenRelayInfoResponsesExtractsBuiltInToolMetadata(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{Model: "gpt-5", Tools: largeResponsesToolsJSON()}

	info := GenRelayInfoResponses(newResponsesToolsContext(), request)

	require.NotNil(t, info.ResponsesUsageInfo)
	require.Equal(t, "high", info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].SearchContextSize)
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, dto.BuildInToolFileSearch)
}

func TestGenRelayInfoResponsesDefaultsWebSearchContextSize(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{Model: "gpt-5", Tools: json.RawMessage(`[{"type":"web_search_preview"}]`)}

	info := GenRelayInfoResponses(newResponsesToolsContext(), request)

	require.Equal(t, "medium", info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].SearchContextSize)
}

func BenchmarkGenRelayInfoResponsesLargeTools(b *testing.B) {
	gin.SetMode(gin.TestMode)
	request := &dto.OpenAIResponsesRequest{Model: "gpt-5", Tools: largeResponsesToolsJSON()}
	b.ReportAllocs()
	b.SetBytes(int64(len(request.Tools)))
	for i := 0; i < b.N; i++ {
		info := GenRelayInfoResponses(newResponsesToolsContext(), request)
		if len(info.ResponsesUsageInfo.BuiltInTools) != 2 {
			b.Fatal("unexpected tools metadata")
		}
	}
}
