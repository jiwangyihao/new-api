package channel

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"golang.org/x/net/proxy"
	"net/http"
	"net/url"
	"time"
)

func NativeResponsesWebSocketAPI(apiType int) bool {
	return apiType == constant.APITypeOpenAI || apiType == constant.APITypeCodex
}

// DialResponsesWebSocket shares adaptor auth/overrides and production markers,
// but uses a cancellable native websocket handshake, never an HTTP/SSE bridge.
func DialResponsesWebSocket(a Adaptor, c *gin.Context, info *relaycommon.RelayInfo) (*websocket.Conn, *types.NewAPIError) {
	invalid := func(message string) *types.NewAPIError {
		return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if info == nil || !info.ChannelSetting.ResponsesWebSocketEnabled || !NativeResponsesWebSocketAPI(info.ApiType) {
		return nil, invalid("native Responses WebSocket is not enabled or supported")
	}
	rawURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, invalid("cannot resolve native Responses endpoint")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || u.User != nil {
		return nil, invalid("invalid native Responses endpoint")
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return nil, invalid("unsupported native Responses endpoint scheme")
	}
	headers := make(http.Header)
	if err = a.SetupRequestHeader(c, &headers, info); err != nil {
		return nil, invalid("cannot prepare native Responses headers")
	}
	overrides, err := processHeaderOverride(info, c)
	if err != nil {
		return nil, invalid("invalid native Responses header override")
	}
	for key, value := range overrides {
		headers.Set(key, value)
	}
	for _, key := range []string{"Connection", "Upgrade", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol", "Content-Length", "Content-Encoding"} {
		deleteHeaderCaseInsensitive(headers, key)
	}
	FinalizeSubscriptionMarkerHeader(headers, info)
	d := *websocket.DefaultDialer
	d.HandshakeTimeout = 30 * time.Second
	if common.TLSInsecureSkipVerify {
		d.TLSClientConfig = common.InsecureTLSConfig
	}
	if info.ChannelSetting.Proxy != "" {
		p, err := url.Parse(info.ChannelSetting.Proxy)
		if err != nil {
			return nil, invalid("invalid channel proxy")
		}
		switch p.Scheme {
		case "http", "https":
			d.Proxy = http.ProxyURL(p)
		case "socks5", "socks5h":
			var auth *proxy.Auth
			if p.User != nil {
				password, _ := p.User.Password()
				auth = &proxy.Auth{User: p.User.Username(), Password: password}
			}
			socks, err := proxy.SOCKS5("tcp", p.Host, auth, proxy.Direct)
			if err != nil {
				return nil, invalid("invalid channel SOCKS proxy")
			}
			contextDialer, ok := socks.(proxy.ContextDialer)
			if !ok {
				return nil, invalid("channel proxy does not support cancellation")
			}
			d.Proxy = nil
			d.NetDialContext = contextDialer.DialContext
		default:
			return nil, invalid("unsupported channel proxy scheme")
		}
	}
	service.MarkRouteUpstreamStarted(c)
	conn, response, dialErr := d.DialContext(c.Request.Context(), u.String(), headers)
	if response != nil {
		service.ObserveRouteHTTPResponse(c, response)
		if response.Body != nil {
			_ = response.Body.Close()
		}
		if dialErr == nil {
			info.ApplyDynamicBillingMultiplierFromHeaders(response.Header, relaycommon.DynamicBillingMultiplierSourceHeader)
		}
	}
	if dialErr != nil {
		status := http.StatusBadGateway
		if response != nil && response.StatusCode >= 400 {
			status = response.StatusCode
		}
		// url.Error and Gorilla errors may embed API keys; never expose their text.
		return nil, types.NewOpenAIError(errors.New("native Responses WebSocket handshake failed"), types.ErrorCodeDoRequestFailed, status)
	}
	return conn, nil
}
