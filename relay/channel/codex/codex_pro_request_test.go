package codex

import (
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCodexAdaptorPaidSubscriptionCodexProMarksResponsesAndCompactOnly(t *testing.T) {
	for _, tc := range []struct {
		name      string
		relayMode int
		want      bool
	}{
		{name: "responses", relayMode: relayconstant.RelayModeResponses, want: true},
		{name: "compact", relayMode: relayconstant.RelayModeResponsesCompact, want: true},
		{name: "chat", relayMode: relayconstant.RelayModeChatCompletions, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-5.4",
				RelayMode:       tc.relayMode,
				RelayFormat:     types.RelayFormatOpenAIResponses,
				UserSetting:     dto.UserSetting{},
				RequestHeaders:  map[string]string{"X-NewAPI-Codex-Pro-Intent": "codex-pro"},
				ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCodex, ApiType: constant.APITypeCodex},
			}
			setCodexAdaptorStringFieldForTest(t, info, "CodexProMode", "flexible")
			setCodexAdaptorBoolFieldForTest(t, info, "CodexProEligible", true)

			_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(&gin.Context{}, info, dto.OpenAIResponsesRequest{})

			if tc.relayMode == relayconstant.RelayModeChatCompletions {
				require.NoError(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, getCodexAdaptorStringFieldForTest(t, info, "CodexProRequestMarker") == "codex-pro")
			if tc.relayMode == relayconstant.RelayModeChatCompletions {
				require.Empty(t, getCodexAdaptorStringFieldForTest(t, info, "CodexProRequestMarker"))
			}
		})
	}
}

func TestCodexAdaptorConvertResponsesResetsStaleServedAck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	info := &relaycommon.RelayInfo{
		OriginModelName:         "gpt-5.4",
		RelayMode:               relayconstant.RelayModeResponses,
		RelayFormat:             types.RelayFormatOpenAIResponses,
		RequestHeaders:          map[string]string{"X-NewAPI-Codex-Pro-Intent": "codex-pro"},
		ChannelMeta:             &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCodex, ApiType: constant.APITypeCodex},
		CodexProRequestMarker:   "codex-pro",
		CodexProRequestSent:     true,
		CodexProServedCandidate: true,
		CodexProServed:          true,
	}
	setCodexAdaptorStringFieldForTest(t, info, "CodexProMode", "flexible")
	setCodexAdaptorBoolFieldForTest(t, info, "CodexProEligible", true)

	_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(&gin.Context{}, info, dto.OpenAIResponsesRequest{})

	require.NoError(t, err)
	require.Equal(t, "codex-pro", getCodexAdaptorStringFieldForTest(t, info, "CodexProRequestMarker"))
	require.False(t, getCodexAdaptorBoolFieldForTest(t, info, "CodexProServedCandidate"))
	require.False(t, getCodexAdaptorBoolFieldForTest(t, info, "CodexProServed"))
}

func TestCodexAdaptorConvertResponsesRequiresCodexChannelMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.4",
		RelayMode:       relayconstant.RelayModeResponses,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RequestHeaders:  map[string]string{"X-NewAPI-Codex-Pro-Intent": "codex-pro"},
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ApiType: constant.APITypeOpenAI},
	}
	setCodexAdaptorStringFieldForTest(t, info, "CodexProMode", "flexible")
	setCodexAdaptorBoolFieldForTest(t, info, "CodexProEligible", true)

	_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(&gin.Context{}, info, dto.OpenAIResponsesRequest{})

	require.NoError(t, err)
	require.Empty(t, getCodexAdaptorStringFieldForTest(t, info, "CodexProRequestMarker"))
}

func TestCodexAdaptorConvertResponsesKeepsChatCompletionsViaResponsesProDisabled(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		OriginModelName:         "gpt-5.4",
		RelayMode:               relayconstant.RelayModeResponses,
		RequestHeaders:          map[string]string{"X-NewAPI-Codex-Pro-Intent": "codex-pro"},
		ChannelMeta:             &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCodex, ApiType: constant.APITypeCodex},
		CodexProRequestDisabled: true,
	}
	setCodexAdaptorStringFieldForTest(t, info, "CodexProMode", "flexible")
	setCodexAdaptorBoolFieldForTest(t, info, "CodexProEligible", true)
	setCodexAdaptorBoolFieldForTest(t, info, "CodexProRequestAllowed", false)

	_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(&gin.Context{}, info, dto.OpenAIResponsesRequest{})
	require.NoError(t, err)
	require.Empty(t, getCodexAdaptorStringFieldForTest(t, info, "CodexProRequestMarker"))
	require.False(t, getCodexAdaptorBoolFieldForTest(t, info, "CodexProRequestAllowed"))
}
func setCodexAdaptorBoolFieldForTest(t *testing.T, info *relaycommon.RelayInfo, fieldName string, value bool) {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	if field.CanSet() {
		field.SetBool(value)
	}
}

func setCodexAdaptorStringFieldForTest(t *testing.T, info *relaycommon.RelayInfo, fieldName string, value string) {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	if field.CanSet() {
		field.SetString(value)
	}
}

func getCodexAdaptorStringFieldForTest(t *testing.T, info *relaycommon.RelayInfo, fieldName string) string {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	return field.String()
}

func getCodexAdaptorBoolFieldForTest(t *testing.T, info *relaycommon.RelayInfo, fieldName string) bool {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	return field.Bool()
}
