package common

import (
	"net/http"
	"testing"

	appcommon "github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestFinalizeCodexProRequestMarkerRequiresCodexChannelMetadata(t *testing.T) {
	for _, tc := range []struct {
		name        string
		channelType int
		apiType     int
		wantMarker  bool
	}{
		{name: "codex", channelType: appconstant.ChannelTypeCodex, apiType: appconstant.APITypeCodex, wantMarker: true},
		{name: "openai", channelType: appconstant.ChannelTypeOpenAI, apiType: appconstant.APITypeOpenAI},
		{name: "xai", channelType: appconstant.ChannelTypeXai, apiType: appconstant.APITypeXai},
		{name: "codex_channel_wrong_api", channelType: appconstant.ChannelTypeCodex, apiType: appconstant.APITypeOpenAI},
		{name: "codex_api_wrong_channel", channelType: appconstant.ChannelTypeOpenAI, apiType: appconstant.APITypeCodex},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &RelayInfo{
				OriginModelName:         "gpt-5.4",
				RelayMode:               relayconstant.RelayModeResponses,
				RequestHeaders:          map[string]string{},
				ChannelMeta:             &ChannelMeta{ChannelType: tc.channelType, ApiType: tc.apiType},
				CodexProMode:            appcommon.CodexProModeAll,
				CodexProEligible:        true,
				CodexProRequestMarker:   "stale",
				CodexProRequestSent:     true,
				CodexProServedCandidate: true,
				CodexProServed:          true,
			}

			info.FinalizeCodexProRequestMarker()

			if tc.wantMarker {
				require.Equal(t, "codex-pro", info.CodexProRequestMarker)
				require.True(t, info.CodexProRequestSent)
			} else {
				require.Empty(t, info.CodexProRequestMarker)
				require.False(t, info.CodexProRequestSent)
			}
			require.False(t, info.CodexProServedCandidate)
			require.False(t, info.CodexProServed)
		})
	}
}

func TestFinalizeCodexProRequestMarkerClearsStaleServedStateBeforeAttempt(t *testing.T) {
	info := &RelayInfo{
		OriginModelName:         "gpt-5.4",
		RelayMode:               relayconstant.RelayModeResponses,
		RequestHeaders:          map[string]string{},
		ChannelMeta:             &ChannelMeta{ChannelType: appconstant.ChannelTypeCodex, ApiType: appconstant.APITypeCodex},
		CodexProMode:            appcommon.CodexProModeAll,
		CodexProEligible:        true,
		CodexProRequestMarker:   "codex-pro",
		CodexProRequestSent:     true,
		CodexProServedCandidate: true,
		CodexProServed:          true,
	}

	info.FinalizeCodexProRequestMarker()

	require.Equal(t, "codex-pro", info.CodexProRequestMarker)
	require.True(t, info.CodexProRequestSent)
	require.False(t, info.CodexProServedCandidate)
	require.False(t, info.CodexProServed)
}

func TestFinalizeCodexProRequestMarkerHonorsRequestDisabled(t *testing.T) {
	info := &RelayInfo{
		OriginModelName:         "gpt-5.4",
		RelayMode:               relayconstant.RelayModeResponses,
		RequestHeaders:          map[string]string{},
		ChannelMeta:             &ChannelMeta{ChannelType: appconstant.ChannelTypeCodex, ApiType: appconstant.APITypeCodex},
		CodexProMode:            appcommon.CodexProModeAll,
		CodexProEligible:        true,
		CodexProRequestDisabled: true,
		CodexProRequestAllowed:  true,
		CodexProRequestMarker:   "stale",
		CodexProRequestSent:     true,
		CodexProServedCandidate: true,
		CodexProServed:          true,
	}

	info.FinalizeCodexProRequestMarker()

	require.Empty(t, info.CodexProRequestMarker)
	require.False(t, info.CodexProRequestSent)
	require.False(t, info.CodexProRequestAllowed)
	require.False(t, info.CodexProServedCandidate)
	require.False(t, info.CodexProServed)
}

func TestCodexProAckFromFailedAttemptDoesNotPolluteFallbackAttempt(t *testing.T) {
	info := &RelayInfo{
		OriginModelName:  "gpt-5.4",
		RelayMode:        relayconstant.RelayModeResponses,
		RequestHeaders:   map[string]string{},
		ChannelMeta:      &ChannelMeta{ChannelType: appconstant.ChannelTypeCodex, ApiType: appconstant.APITypeCodex},
		CodexProMode:     appcommon.CodexProModeAll,
		CodexProEligible: true,
	}
	info.FinalizeCodexProRequestMarker()
	info.MarkCodexProServedCandidateFromTrailers(http.Header{"X-NewAPI-Pro-Served": []string{"codex-pro"}})
	require.True(t, info.CodexProServedCandidate)
	require.False(t, info.CodexProServed)

	info.ChannelMeta = &ChannelMeta{ChannelType: appconstant.ChannelTypeOpenAI, ApiType: appconstant.APITypeOpenAI}
	info.FinalizeCodexProRequestMarker()
	info.ConfirmCodexProServed()

	require.Empty(t, info.CodexProRequestMarker)
	require.False(t, info.CodexProRequestSent)
	require.False(t, info.CodexProServedCandidate)
	require.False(t, info.CodexProServed)
}

func TestCodexProTrailerAckRequiresRequestSentState(t *testing.T) {
	info := &RelayInfo{
		CodexProRequestMarker: "codex-pro",
		CodexProRequestSent:   false,
	}

	info.MarkCodexProServedCandidateFromTrailers(http.Header{"X-NewAPI-Pro-Served": []string{"codex-pro"}})
	info.ConfirmCodexProServed()

	require.False(t, info.CodexProServedCandidate)
	require.False(t, info.CodexProServed)
}
