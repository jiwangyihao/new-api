package channel

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	codexProIntentHeaderNameForTest  = "X-NewAPI-Codex-Pro-Intent"
	codexProRequestHeaderNameForTest = "X-NewAPI-Pro-Request"
	codexProMarkerValueForTest       = "codex-pro"
)

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func TestFinalizeSubscriptionMarkerHeaderSetsTrialMarker(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
	req.Header["x-newapi-subscription-marker"] = []string{"spoofed"}

	FinalizeSubscriptionMarkerHeader(req.Header, &relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"})

	requireOnlyTrialSubscriptionMarkerHeader(t, req.Header)
}

func TestFinalizeSubscriptionMarkerHeaderRemovesSpoofedNonTrialMarker(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
	req.Header["x-newapi-subscription-marker"] = []string{"trial"}
	req.Header["X-NewAPI-Subscription-Marker"] = []string{"trial"}

	FinalizeSubscriptionMarkerHeader(req.Header, &relaycommon.RelayInfo{})

	requireNoSubscriptionMarkerHeader(t, req.Header)
}

func TestFinalizeSubscriptionMarkerHeaderIgnoresNonTrialMarkerValue(t *testing.T) {
	t.Parallel()

	for _, marker := range []string{"paid", "wrong", ""} {
		marker := marker
		t.Run(subscriptionMarkerTestName(marker), func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
			req.Header["x-newapi-subscription-marker"] = []string{"trial"}
			req.Header["X-NewAPI-Subscription-Marker"] = []string{"wrong"}

			FinalizeSubscriptionMarkerHeader(req.Header, &relaycommon.RelayInfo{SubscriptionTrialMarker: marker})

			requireNoSubscriptionMarkerHeader(t, req.Header)
		})
	}
}

func TestFinalizeSubscriptionMarkerHeaderRemovesRuntimeOverrideSpoof(t *testing.T) {
	t.Parallel()

	t.Run("channel_passthrough_and_client_header_override", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		ctx.Request.Header.Set(SubscriptionMarkerHeaderName, "trial")

		info := &relaycommon.RelayInfo{
			IsChannelTest: false,
			ChannelMeta: &relaycommon.ChannelMeta{
				HeadersOverride: map[string]any{
					"*":                          "",
					SubscriptionMarkerHeaderName: "{client_header:X-NewAPI-Subscription-Marker}",
				},
			},
		}

		headers, err := processHeaderOverride(info, ctx)
		require.NoError(t, err)
		upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
		applyHeaderOverrideToRequest(upstreamReq, headers)
		require.Equal(t, "trial", upstreamReq.Header.Get(SubscriptionMarkerHeaderName))

		FinalizeSubscriptionMarkerHeader(upstreamReq.Header, info)

		requireNoSubscriptionMarkerHeader(t, upstreamReq.Header)
	})

	t.Run("runtime_pass_headers_override", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		info := &relaycommon.RelayInfo{
			IsChannelTest: false,
			RequestHeaders: map[string]string{
				SubscriptionMarkerHeaderName: "trial",
			},
			ChannelMeta: &relaycommon.ChannelMeta{
				ParamOverride: map[string]any{
					"operations": []any{
						map[string]any{
							"mode":  "pass_headers",
							"value": []any{SubscriptionMarkerHeaderName},
						},
					},
				},
			},
		}

		_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
		require.NoError(t, err)
		require.True(t, info.UseRuntimeHeadersOverride)

		headers, err := processHeaderOverride(info, ctx)
		require.NoError(t, err)
		upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
		applyHeaderOverrideToRequest(upstreamReq, headers)
		require.Equal(t, "trial", upstreamReq.Header.Get(SubscriptionMarkerHeaderName))

		FinalizeSubscriptionMarkerHeader(upstreamReq.Header, info)

		requireNoSubscriptionMarkerHeader(t, upstreamReq.Header)
	})
}

func TestFinalizeSubscriptionMarkerHeaderKeepsTrialAfterOverrideDeletes(t *testing.T) {
	t.Parallel()

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
	upstreamReq.Header["x-newapi-subscription-marker"] = []string{"wrong"}
	applyHeaderOverrideToRequest(upstreamReq, map[string]string{
		SubscriptionMarkerHeaderName: "paid",
	})

	FinalizeSubscriptionMarkerHeader(upstreamReq.Header, &relaycommon.RelayInfo{SubscriptionTrialMarker: "trial"})

	requireOnlyTrialSubscriptionMarkerHeader(t, upstreamReq.Header)
}

func TestFinalizeProRequestHeaderPaidSubscriptionCodexProEligibleModes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		mode   string
		intent string
		want   bool
	}{
		{name: "all_without_intent", mode: "all", want: true},
		{name: "flexible_with_intent", mode: "flexible", intent: "codex-pro", want: true},
		{name: "flexible_without_intent", mode: "flexible", want: false},
		{name: "off_with_intent", mode: "off", intent: "codex-pro", want: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info := newCodexProRelayInfoForHeaderTest(t, tc.mode, "gpt-5.4", constant.RelayModeResponses, true)
			if tc.intent != "" {
				info.RequestHeaders[codexProIntentHeaderNameForTest] = tc.intent
			}
			markCodexProRequestForHeaderTest(t, info)
			req := httptest.NewRequest(http.MethodPost, "https://example.com/backend-api/codex/responses", nil)
			req.Header["x-newapi-pro-request"] = []string{"spoofed"}

			FinalizeSubscriptionMarkerHeader(req.Header, info)

			if tc.want {
				requireOnlyCodexProRequestHeader(t, req.Header)
			} else {
				requireNoCodexProRequestHeader(t, req.Header)
			}
		})
	}
}

func TestFinalizeProRequestHeaderAddsTEForTrailerAck(t *testing.T) {
	t.Parallel()

	info := newCodexProRelayInfoForHeaderTest(t, "all", "gpt-5.4", constant.RelayModeResponses, true)
	markCodexProRequestForHeaderTest(t, info)
	req := httptest.NewRequest(http.MethodPost, "https://example.com/backend-api/codex/responses", nil)
	req.Header.Set("TE", "client-spoof")

	FinalizeSubscriptionMarkerHeader(req.Header, info)

	requireOnlyCodexProRequestHeader(t, req.Header)
	require.True(t, getRelayInfoBoolFieldForHeaderTest(t, info, "CodexProRequestAllowed"))
	require.Equal(t, "trailers", req.Header.Get("TE"))
}

func TestFinalizeProRequestHeaderRemovesSpoofedTrailerDeclaration(t *testing.T) {
	t.Parallel()

	info := newCodexProRelayInfoForHeaderTest(t, "all", "gpt-5.4", constant.RelayModeResponses, true)
	markCodexProRequestForHeaderTest(t, info)
	req := httptest.NewRequest(http.MethodPost, "https://example.com/backend-api/codex/responses", nil)
	req.Header.Set("Trailer", codexProRequestHeaderNameForTest)
	req.Header.Add("Trailer", "X-NewAPI-Pro-Served")

	FinalizeSubscriptionMarkerHeader(req.Header, info)

	require.Empty(t, req.Header.Get("Trailer"))
	requireOnlyCodexProRequestHeader(t, req.Header)
}

func TestFinalizeProRequestHeaderCodexProUnavailableForModelAndPath(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		model     string
		relayMode int
		eligible  bool
	}{
		{name: "ineligible_subscription", model: "gpt-5.4", relayMode: constant.RelayModeResponses, eligible: false},
		{name: "non_gpt_model", model: "claude-3-7-sonnet", relayMode: constant.RelayModeResponses, eligible: true},
		{name: "chat_completions_path", model: "gpt-5.4", relayMode: constant.RelayModeChatCompletions, eligible: true},
		{name: "embeddings_path", model: "gpt-5.4", relayMode: constant.RelayModeEmbeddings, eligible: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info := newCodexProRelayInfoForHeaderTest(t, "all", tc.model, tc.relayMode, tc.eligible)
			markCodexProRequestForHeaderTest(t, info)
			req := httptest.NewRequest(http.MethodPost, "https://example.com/upstream", nil)
			req.Header["X-NewAPI-Pro-Request"] = []string{"codex-pro"}
			req.Header.Set("TE", "client-spoof")

			FinalizeSubscriptionMarkerHeader(req.Header, info)

			requireNoCodexProRequestHeader(t, req.Header)
			require.False(t, getRelayInfoBoolFieldForHeaderTest(t, info, "CodexProRequestAllowed"))
			require.Empty(t, req.Header.Get("TE"))
		})
	}
}

func TestFinalizeProRequestHeaderRequiresCodexChannel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		channelType int
		apiType     int
	}{
		{name: "openai_responses", channelType: appconstant.ChannelTypeOpenAI, apiType: appconstant.APITypeOpenAI},
		{name: "xai_responses", channelType: appconstant.ChannelTypeXai, apiType: appconstant.APITypeXai},
		{name: "codex_channel_openai_api_type", channelType: appconstant.ChannelTypeCodex, apiType: appconstant.APITypeOpenAI},
		{name: "openai_channel_codex_api_type", channelType: appconstant.ChannelTypeOpenAI, apiType: appconstant.APITypeCodex},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info := newCodexProRelayInfoForHeaderTest(t, "all", "gpt-5.4", constant.RelayModeResponses, true)
			info.ChannelMeta.ChannelType = tc.channelType
			info.ChannelMeta.ApiType = tc.apiType
			req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
			req.Header["X-NewAPI-Pro-Request"] = []string{"spoofed"}

			FinalizeSubscriptionMarkerHeader(req.Header, info)

			requireNoCodexProRequestHeader(t, req.Header)
			require.False(t, getRelayInfoBoolFieldForHeaderTest(t, info, "CodexProRequestAllowed"))
			require.Empty(t, getRelayInfoStringFieldForHeaderTest(t, info, "CodexProRequestMarker"))
		})
	}
}

func TestFinalizeProRequestHeaderResetsAttemptRuntimeState(t *testing.T) {
	t.Parallel()

	info := newCodexProRelayInfoForHeaderTest(t, "all", "gpt-5.4", constant.RelayModeResponses, true)
	setRelayInfoStringFieldForHeaderTest(t, info, "CodexProRequestMarker", "codex-pro")
	setRelayInfoBoolFieldForHeaderTest(t, info, "CodexProRequestSent", true)
	setRelayInfoBoolFieldForHeaderTest(t, info, "CodexProServedCandidate", true)
	setRelayInfoBoolFieldForHeaderTest(t, info, "CodexProServed", true)
	req := httptest.NewRequest(http.MethodPost, "https://example.com/backend-api/codex/responses", nil)

	FinalizeSubscriptionMarkerHeader(req.Header, info)

	requireOnlyCodexProRequestHeader(t, req.Header)
	require.False(t, getRelayInfoBoolFieldForHeaderTest(t, info, "CodexProServedCandidate"))
	require.False(t, getRelayInfoBoolFieldForHeaderTest(t, info, "CodexProServed"))
	require.True(t, getRelayInfoBoolFieldForHeaderTest(t, info, "CodexProRequestAllowed"))
}

func TestFinalizeProRequestHeaderKeepsServerFinalValueAfterOverrideSpoofs(t *testing.T) {
	t.Parallel()

	t.Run("passthrough_set_header_and_delete_header", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		ctx.Request.Header.Set("x-newapi-pro-request", "client-spoof")

		info := newCodexProRelayInfoForHeaderTest(t, "all", "gpt-5.4", constant.RelayModeResponses, true)
		info.ChannelMeta.ParamOverride = map[string]any{
			"operations": []any{
				map[string]any{"mode": "pass_headers", "value": []any{"X-NewAPI-Pro-Request"}},
				map[string]any{"mode": "set_header", "path": "X-NewAPI-Pro-Request", "value": "override-spoof"},
				map[string]any{"mode": "delete_header", "path": "X-NewAPI-Pro-Request"},
			},
		}
		_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-5.4"}`), info)
		require.NoError(t, err)
		markCodexProRequestForHeaderTest(t, info)

		headers, err := processHeaderOverride(info, ctx)
		require.NoError(t, err)
		upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/backend-api/codex/responses", nil)
		applyHeaderOverrideToRequest(upstreamReq, headers)

		FinalizeSubscriptionMarkerHeader(upstreamReq.Header, info)

		requireOnlyCodexProRequestHeader(t, upstreamReq.Header)
	})

	t.Run("runtime_override_delete_cannot_remove_server_marker", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		info := newCodexProRelayInfoForHeaderTest(t, "all", "gpt-5.4", constant.RelayModeResponses, true)
		info.UseRuntimeHeadersOverride = true
		info.RuntimeHeadersOverride = map[string]any{
			"x-newapi-pro-request": "runtime-spoof",
			"x-static":             "runtime-value",
		}
		markCodexProRequestForHeaderTest(t, info)

		headers, err := processHeaderOverride(info, ctx)
		require.NoError(t, err)
		upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/backend-api/codex/responses", nil)
		applyHeaderOverrideToRequest(upstreamReq, headers)
		require.Equal(t, "runtime-spoof", upstreamReq.Header.Get(codexProRequestHeaderNameForTest))

		FinalizeSubscriptionMarkerHeader(upstreamReq.Header, info)

		requireOnlyCodexProRequestHeader(t, upstreamReq.Header)
		require.Equal(t, "runtime-value", upstreamReq.Header.Get("X-Static"))
	})
}

func TestFinalizeProRequestHeaderRemovesSpoofWhenCodexProUnavailable(t *testing.T) {
	t.Parallel()

	gintestRecorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(gintestRecorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("X-NewAPI-Pro-Request", "codex-pro")
	info := newCodexProRelayInfoForHeaderTest(t, "off", "gpt-5.4", constant.RelayModeResponses, true)
	info.ChannelMeta.HeadersOverride = map[string]any{
		"*":                        "",
		"re:(?i)^x-newapi-pro-.*$": "",
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/backend-api/codex/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)

	FinalizeSubscriptionMarkerHeader(upstreamReq.Header, info)

	requireNoCodexProRequestHeader(t, upstreamReq.Header)
}

func newCodexProRelayInfoForHeaderTest(t *testing.T, mode string, model string, relayMode int, eligible bool) *relaycommon.RelayInfo {
	t.Helper()
	info := &relaycommon.RelayInfo{
		OriginModelName: model,
		RelayMode:       relayMode,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		UserSetting:     dto.UserSetting{},
		RequestHeaders:  map[string]string{},
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: appconstant.ChannelTypeCodex, ApiType: appconstant.APITypeCodex},
	}
	setRelayInfoStringFieldForHeaderTest(t, info, "CodexProMode", mode)
	setRelayInfoBoolFieldForHeaderTest(t, info, "CodexProEligible", eligible)
	return info
}

func markCodexProRequestForHeaderTest(t *testing.T, info *relaycommon.RelayInfo) {
	t.Helper()
	method := reflect.ValueOf(info).MethodByName("FinalizeCodexProRequestMarker")
	if method.IsValid() {
		method.Call(nil)
		return
	}
	setRelayInfoStringFieldForHeaderTest(t, info, "CodexProRequestMarker", "codex-pro")
	require.NotEmpty(t, getRelayInfoStringFieldForHeaderTest(t, info, "CodexProRequestMarker"), "RelayInfo must expose CodexProRequestMarker for finalized Pro request headers")
}

func setRelayInfoStringFieldForHeaderTest(t *testing.T, info *relaycommon.RelayInfo, fieldName string, value string) {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	if field.CanSet() {
		field.SetString(value)
	}
}

func getRelayInfoStringFieldForHeaderTest(t *testing.T, info *relaycommon.RelayInfo, fieldName string) string {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	return field.String()
}

func setRelayInfoBoolFieldForHeaderTest(t *testing.T, info *relaycommon.RelayInfo, fieldName string, value bool) {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	if field.CanSet() {
		field.SetBool(value)
	}
}

func getRelayInfoBoolFieldForHeaderTest(t *testing.T, info *relaycommon.RelayInfo, fieldName string) bool {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	return field.Bool()
}

func requireNoCodexProRequestHeader(t *testing.T, headers http.Header) {
	t.Helper()

	require.Empty(t, headers.Get(codexProRequestHeaderNameForTest))
	markerHeaderName := strings.ToLower(codexProRequestHeaderNameForTest)
	for key := range headers {
		require.NotEqual(t, markerHeaderName, strings.ToLower(key))
	}
}

func requireOnlyCodexProRequestHeader(t *testing.T, headers http.Header) {
	t.Helper()

	require.Equal(t, codexProMarkerValueForTest, headers.Get(codexProRequestHeaderNameForTest))
	markerHeaderName := strings.ToLower(codexProRequestHeaderNameForTest)
	matchedKeys := 0
	for key, values := range headers {
		if strings.ToLower(key) != markerHeaderName {
			continue
		}
		matchedKeys++
		require.Equal(t, []string{codexProMarkerValueForTest}, values)
	}
	require.Equal(t, 1, matchedKeys)
}

func subscriptionMarkerTestName(marker string) string {
	if marker == "" {
		return "empty"
	}
	return marker
}

func requireNoSubscriptionMarkerHeader(t *testing.T, headers http.Header) {
	t.Helper()

	require.Empty(t, headers.Get(SubscriptionMarkerHeaderName))
	markerHeaderName := strings.ToLower(SubscriptionMarkerHeaderName)
	for key := range headers {
		require.NotEqual(t, markerHeaderName, strings.ToLower(key))
	}
}

func requireOnlyTrialSubscriptionMarkerHeader(t *testing.T, headers http.Header) {
	t.Helper()

	require.Equal(t, "trial", headers.Get(SubscriptionMarkerHeaderName))
	markerHeaderName := strings.ToLower(SubscriptionMarkerHeaderName)
	matchedKeys := 0
	for key, values := range headers {
		if strings.ToLower(key) != markerHeaderName {
			continue
		}
		matchedKeys++
		require.Equal(t, []string{"trial"}, values)
	}
	require.Equal(t, 1, matchedKeys)
}
