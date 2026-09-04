package relay

import (
	"bytes"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAlphaSearchRequestBodyPreservesUnknownFields(t *testing.T) {
	raw := []byte(`{"id":"req_1","model":"gpt-5.1","input":[{"role":"user","content":"hi"}],"commands":{"search_query":[{"q":"weather","recency":1}]},"settings":{"locale":"en"},"future_field":{"nested":true,"exact_integer":18446744073709551615}}`)

	out, err := BuildAlphaSearchRequestBody(raw, "gpt-5.1", "gpt-5.1-mapped")
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, common.Unmarshal(out, &body))
	assert.Equal(t, "gpt-5.1-mapped", body["model"])
	assert.Equal(t, "req_1", body["id"])
	assert.Contains(t, body, "input")
	assert.Contains(t, body, "commands")
	assert.Contains(t, body, "settings")
	assert.Contains(t, body, "future_field")
	assert.True(t, bytes.Contains(out, []byte(`"exact_integer":18446744073709551615`)), "unknown integer must not pass through float64")
}

func TestBuildAlphaSearchRequestBodyWithoutMappingKeepsRawBytes(t *testing.T) {
	raw := []byte("{\n  \"model\": \"gpt-5.1\",\n  \"future_field\": 1\n}")

	out, err := BuildAlphaSearchRequestBody(raw, "gpt-5.1", "gpt-5.1")
	require.NoError(t, err)

	assert.Equal(t, raw, out)
}

func TestBuildAlphaSearchRequestBodyRejectsEmptyBody(t *testing.T) {
	_, err := BuildAlphaSearchRequestBody(nil, "gpt-5.1", "gpt-5.1")
	require.ErrorContains(t, err, "empty alpha search request body")
}
