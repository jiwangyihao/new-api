package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func setupNativeResponsesWSFixture(t *testing.T, upstream string) {
	t.Helper()
	setupRelayGPTAbuseRepeatBlockTest(t, upstream)
	var groups int64
	require.NoError(t, model.DB.Model(&model.ChannelGroup{}).Where("name = ?", model.DefaultChannelGroupName).Count(&groups).Error)
	if groups == 0 {
		require.NoError(t, model.DB.Create(&model.ChannelGroup{Name: model.DefaultChannelGroupName, Enabled: true}).Error)
	}
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 87103).Updates(map[string]any{"setting": `{"responses_websocket_enabled":true}`, "token_billing_multiplier": 1}).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 87102).Updates(map[string]any{"key": "nativewstestkey", "token_limit_enabled": true, "token_limit": 1000000}).Error)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", 87105).Update("token_limit", 1000000).Error)
	oldRate := setting.ModelRequestRateLimitEnabled
	setting.ModelRequestRateLimitEnabled = false
	t.Cleanup(func() { setting.ModelRequestRateLimitEnabled = oldRate })
}

func nativeResponsesWSClient(t *testing.T) *websocket.Conn {
	t.Helper()
	engine := gin.New()
	engine.GET("/v1/responses", middleware.TokenAuth(), ResponsesWebSocket)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	header := http.Header{"Authorization": []string{"Bearer sk-nativewstestkey"}}
	dialer := *websocket.DefaultDialer
	dialer.Subprotocols = []string{"responses"}
	conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	require.NoError(t, err)
	require.Equal(t, "responses", conn.Subprotocol())
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	return conn
}

func readNativeResponsesTerminal(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	for {
		_, body, err := conn.ReadMessage()
		require.NoError(t, err)
		switch gjson.GetBytes(body, "type").String() {
		case "error", "response.completed", "response.failed", "response.incomplete", "response.cancelled":
			return body
		}
	}
}

func TestNativeResponsesWebSocketReuseIndependentBillingAndRevocation(t *testing.T) {
	var dials atomic.Int32
	var creates atomic.Int32
	var firstUpstreamID string
	upgrade := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer sk-upstream" {
			http.Error(w, "wrong upstream route or credential", 400)
			return
		}
		conn, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		dials.Add(1)
		for {
			_, body, err := conn.ReadMessage()
			if err != nil {
				return
			}
			n := creates.Add(1)
			id := fmt.Sprintf("upstream_%d", n)
			if n == 2 && gjson.GetBytes(body, "previous_response_id").String() != firstUpstreamID {
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","status":400,"error":{"message":"previous ID was not translated"}}`))
				return
			}
			if n == 1 {
				firstUpstreamID = id
			}
			_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"type":"response.created","sequence_number":0,"response":{"id":%q,"status":"in_progress","output":[]}}`, id)))
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.output_text.delta","sequence_number":1,"delta":"hello"}`))
			_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"type":"response.completed","sequence_number":2,"response":{"id":%q,"status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`, id)))
		}
	}))
	defer upstream.Close()
	setupNativeResponsesWSFixture(t, upstream.URL)
	client := nativeResponsesWSClient(t)
	previous := ""
	for n := range 2 {
		require.NoError(t, client.WriteJSON(map[string]any{"type": "response.create", "model": "gpt-4o", "input": "hello", "max_output_tokens": 8, "previous_response_id": previous}))
		terminal := readNativeResponsesTerminal(t, client)
		require.Equal(t, "response.completed", gjson.GetBytes(terminal, "type").String(), string(terminal))
		previous = gjson.GetBytes(terminal, "response.id").String()
		require.True(t, strings.HasPrefix(previous, "resp_route_"))
		var token model.Token
		require.NoError(t, model.DB.First(&token, 87102).Error)
		require.Equal(t, int64((n+1)*15), token.TokenUsed)
		var subscription model.UserSubscription
		require.NoError(t, model.DB.First(&subscription, 87105).Error)
		require.Equal(t, int64((n+1)*15), subscription.TokenUsed)
	}
	require.Equal(t, int32(1), dials.Load())
	require.Equal(t, int32(2), creates.Load())
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 87102).Update("status", common.TokenStatusDisabled).Error)
	require.NoError(t, client.WriteJSON(map[string]any{"type": "response.create", "model": "gpt-4o", "input": "denied"}))
	terminal := readNativeResponsesTerminal(t, client)
	require.Equal(t, "error", gjson.GetBytes(terminal, "type").String(), string(terminal))
	require.Equal(t, int32(2), creates.Load())
}

func TestNativeResponsesWebSocketSubscriptionAndTokenCapAreNotBypassed(t *testing.T) {
	for _, scenario := range []string{"no_subscription", "token_cap"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "must not reach upstream", 500)
			}))
			defer upstream.Close()
			setupNativeResponsesWSFixture(t, upstream.URL)
			client := nativeResponsesWSClient(t)
			if scenario == "no_subscription" {
				require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", 87105).Update("status", "cancelled").Error)
			} else {
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 87102).Updates(map[string]any{"token_limit": 1, "token_used": 1}).Error)
			}
			require.NoError(t, client.WriteJSON(map[string]any{"type": "response.create", "model": "gpt-4o", "input": "hello", "max_output_tokens": 8}))
			terminal := readNativeResponsesTerminal(t, client)
			require.Equal(t, "error", gjson.GetBytes(terminal, "type").String(), string(terminal))
			require.Zero(t, calls.Load())
		})
	}
}

func TestResponsesWSRunnerKeepsOriginalCredentialAndTrustedIP(t *testing.T) {
	setupNativeResponsesWSFixture(t, "http://127.0.0.1:1")
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 87102).Update("allow_ips", "203.0.113.8").Error)
	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies([]string{"127.0.0.1"}))
	engine.GET("/capture", func(c *gin.Context) {
		runner := newResponsesWSRequestRunner(c)
		c.Request.Header.Set("Authorization", "Bearer upstream-secret")
		for n := range 2 {
			request := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-4o","input":"hello"}`))
			apiErr := runner(request, fmt.Sprintf("isolated-%d", n), func(inner *gin.Context) *types.NewAPIError {
				require.Equal(t, "203.0.113.8", inner.ClientIP())
				require.Equal(t, 87102, common.GetContextKeyInt(inner, constant.ContextKeyTokenId))
				_, leaked := inner.Get("previous_turn")
				require.False(t, leaked)
				inner.Set("previous_turn", true)
				return nil
			})
			require.Nil(t, apiErr)
		}
	})
	request := httptest.NewRequest("GET", "/capture", nil)
	request.RemoteAddr = "127.0.0.1:1000"
	request.Header.Set("Authorization", "Bearer sk-nativewstestkey")
	request.Header.Set("X-Forwarded-For", "203.0.113.8")
	engine.ServeHTTP(httptest.NewRecorder(), request)
}

func TestNativeResponsesWebSocketPreludeFailoverAndOutputCommit(t *testing.T) {
	for _, outputFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(outputFirst), func(t *testing.T) {
			var backupCalls atomic.Int32
			upgrade := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrade.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				if _, _, err = conn.ReadMessage(); err != nil {
					return
				}
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.created","sequence_number":0,"response":{"id":"a","output":[]}}`))
				if outputFirst {
					_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.output_text.delta","sequence_number":1,"delta":"committed"}`))
				}
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.failed","sequence_number":2,"response":{"id":"a","error":{"type":"server_error","code":"server_error","message":"unavailable"},"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`))
			}))
			defer failed.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrade.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				backupCalls.Add(1)
				if _, _, err = conn.ReadMessage(); err != nil {
					return
				}
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.created","sequence_number":0,"response":{"id":"b","output":[]}}`))
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","sequence_number":1,"response":{"id":"b","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`))
			}))
			defer backup.Close()
			setupNativeResponsesWSFixture(t, failed.URL)
			oldRanges := operation_setting.AutomaticRetryStatusCodeRanges
			operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 599}}
			t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = oldRanges })
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 87103).Update("priority", 10).Error)
			require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 87103).Update("priority", 10).Error)
			require.NoError(t, model.DB.Create(&model.Channel{Id: 87106, Type: constant.ChannelTypeOpenAI, Key: "backup", Status: common.ChannelStatusEnabled, Name: "native-backup", Models: "gpt-4o", BaseURL: common.GetPointer(backup.URL), Setting: common.GetPointer(`{"responses_websocket_enabled":true}`), TokenBillingMultiplier: 1, AutoBan: common.GetPointer(0)}).Error)
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 87106).Update("settings", `{"supported_endpoint_types":["openai","openai-response"]}`).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: model.DefaultChannelGroupName, Model: "gpt-4o", ChannelId: 87106, Enabled: true}).Error)
			client := nativeResponsesWSClient(t)
			require.NoError(t, client.WriteJSON(map[string]any{"type": "response.create", "model": "gpt-4o", "input": "hello", "max_output_tokens": 8}))
			created := 0
			for {
				_, body, err := client.ReadMessage()
				require.NoError(t, err)
				kind := gjson.GetBytes(body, "type").String()
				if kind == "response.created" {
					created++
				}
				if kind == "response.completed" || kind == "response.failed" || kind == "error" {
					if outputFirst {
						require.Equal(t, "response.failed", kind, string(body))
						require.Zero(t, backupCalls.Load())
					} else {
						require.Equal(t, "response.completed", kind, string(body))
						require.Equal(t, int32(1), backupCalls.Load())
					}
					require.Equal(t, 1, created)
					break
				}
			}
		})
	}
}

func TestNativeResponsesWebSocketFailedPreludeDoesNotConsumeSuccessLimit(t *testing.T) {
	var calls atomic.Int32
	upgrade := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			n := calls.Add(1)
			if n == 1 {
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.created","response":{"id":"limit-failed","status":"in_progress"}}`))
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","status":400,"error":{"type":"invalid_request_error","code":"context_length_exceeded","message":"input too long"}}`))
			} else {
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"limit-success","status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`))
			}
		}
	}))
	defer upstream.Close()
	setupNativeResponsesWSFixture(t, upstream.URL)
	oldDuration, oldTotal, oldSuccess := setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount = 1, 0, 1
	t.Cleanup(func() {
		setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount = oldDuration, oldTotal, oldSuccess
	})
	client := nativeResponsesWSClient(t)
	for _, want := range []string{"error", "response.completed", "error"} {
		require.NoError(t, client.WriteJSON(map[string]any{"type": "response.create", "model": "gpt-4o", "input": "hello"}))
		terminal := readNativeResponsesTerminal(t, client)
		require.Equal(t, want, gjson.GetBytes(terminal, "type").String(), string(terminal))
		if calls.Load() == 2 && want == "error" {
			require.Equal(t, int64(http.StatusTooManyRequests), gjson.GetBytes(terminal, "status").Int())
		}
	}
	require.Equal(t, int32(2), calls.Load())
}
