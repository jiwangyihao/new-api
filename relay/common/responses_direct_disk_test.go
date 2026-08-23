package common

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/require"
)

func TestCanEncodeResponsesRequestDirectlyToDisk(t *testing.T) {
	base := dto.OpenAIResponsesRequest{Model: "gpt-5.4"}
	t.Run("no transforms", func(t *testing.T) {
		require.True(t, CanEncodeResponsesRequestDirectlyToDisk(base, &RelayInfo{}))
	})
	t.Run("header only override", func(t *testing.T) {
		info := &RelayInfo{ChannelMeta: &ChannelMeta{ParamOverride: map[string]any{
			"operations": []any{map[string]any{"mode": "set_header", "path": "X-Test", "value": "yes"}},
		}}}
		require.True(t, CanEncodeResponsesRequestDirectlyToDisk(base, info))
	})
	t.Run("json override", func(t *testing.T) {
		info := &RelayInfo{ChannelMeta: &ChannelMeta{ParamOverride: map[string]any{
			"operations": []any{map[string]any{"mode": "set", "path": "temperature", "value": 0}},
		}}}
		require.False(t, CanEncodeResponsesRequestDirectlyToDisk(base, info))
	})
	t.Run("disabled fields", func(t *testing.T) {
		request := base
		request.ServiceTier = "flex"
		require.False(t, CanEncodeResponsesRequestDirectlyToDisk(request, &RelayInfo{}))
		request = base
		request.Store = []byte("true")
		require.False(t, CanEncodeResponsesRequestDirectlyToDisk(request, &RelayInfo{ChannelMeta: &ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{DisableStore: true}}}))
		request = base
		request.SafetyIdentifier = []byte(`"user"`)
		require.False(t, CanEncodeResponsesRequestDirectlyToDisk(request, &RelayInfo{}))
		request = base
		request.StreamOptions = &dto.StreamOptions{IncludeObfuscation: true}
		require.False(t, CanEncodeResponsesRequestDirectlyToDisk(request, &RelayInfo{}))
	})
}

func TestApplyResponsesHeaderOnlyOverrideUpdatesHeaders(t *testing.T) {
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{map[string]any{"mode": "set_header", "path": "X-Test", "value": "yes"}},
			},
			HeadersOverride: map[string]any{},
		},
		RequestHeaders: map[string]string{},
	}
	require.NoError(t, ApplyResponsesHeaderOnlyOverride(info))
	require.Equal(t, "yes", info.RuntimeHeadersOverride["x-test"])
	require.True(t, info.UseRuntimeHeadersOverride)
}
