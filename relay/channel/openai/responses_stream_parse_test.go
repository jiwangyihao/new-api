package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/require"
)

func TestParseResponsesStreamEventFastMatchesFullDecode(t *testing.T) {
	cases := []string{
		`{"type":"response.output_text.delta","delta":"hello"}`,
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`,
		`{"type":"response.failed","response":{"error":{"type":"server_error","message":"failed"}}}`,
		`{"type":"response.output_item.done","item":{"type":"web_search_call"}}`,
	}
	for _, data := range cases {
		var full dto.ResponsesStreamResponse
		require.NoError(t, common.UnmarshalJsonStr(data, &full))
		fast, err := parseResponsesStreamEvent(data)
		require.NoError(t, err)
		require.Equal(t, full.Type, fast.Type)
		if requiresFullResponsesStreamDecode(full.Type) {
			require.Equal(t, full, fast)
		}
	}
}

func TestParseResponsesStreamEventFastRejectsMalformedJSON(t *testing.T) {
	_, err := parseResponsesStreamEvent(`{"type":"response.output_text.delta"`)
	require.Error(t, err)
}

func BenchmarkResponsesStreamEventDecode(b *testing.B) {
	data := `{"type":"response.output_text.delta","delta":"` + strings.Repeat("x", 64<<10) + `","sequence_number":123}`
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		for i := 0; i < b.N; i++ {
			var event dto.ResponsesStreamResponse
			if err := common.UnmarshalJsonStr(data, &event); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("fast", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		for i := 0; i < b.N; i++ {
			if _, err := parseResponsesStreamEvent(data); err != nil {
				b.Fatal(err)
			}
		}
	})
}
