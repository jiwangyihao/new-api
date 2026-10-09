package controller

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type responsesWSRequestContextKey struct{}

type responsesWSRequestState struct {
	requestID string
	handle    func(*gin.Context) *types.NewAPIError
	apiError  *types.NewAPIError
}

// Each response.create runs the ordinary request middleware to completion,
// without routing another HTTP request or retaining a pooled Gin context.
var responsesWSRequestEngine = sync.OnceValue(func() *gin.Engine {
	engine := gin.New()
	engine.ForwardedByClientIP = false
	_ = engine.SetTrustedProxies(nil)
	engine.POST("/v1/responses", func(c *gin.Context) {
		state := c.Request.Context().Value(responsesWSRequestContextKey{}).(*responsesWSRequestState)
		c.Set(common.RequestIdKey, state.requestID)
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		c.Next()
	}, middleware.BodyStorageCleanup(), middleware.TokenAuth(), refreshResponsesWSAuthorization, middleware.ModelRequestRateLimit(), func(c *gin.Context) {
		state := c.Request.Context().Value(responsesWSRequestContextKey{}).(*responsesWSRequestState)
		groups, err := model.GetEffectiveGroupNamesByToken(common.GetContextKeyInt(c, constant.ContextKeyTokenId))
		if err != nil {
			state.apiError = types.NewErrorWithStatusCode(errors.New("cannot refresh token groups"), types.ErrorCodeAccessDenied, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
			c.Status(http.StatusServiceUnavailable)
			return
		}
		common.SetContextKey(c, constant.ContextKeyTokenGroups, groups)
		state.apiError = state.handle(c)
		if state.apiError != nil {
			status := state.apiError.StatusCode
			if status < http.StatusBadRequest {
				status = http.StatusInternalServerError
			}
			c.Status(status)
		}
	})
	return engine
})

// Persistent connections revalidate revocation and permissions against the DB;
// a cached handshake identity must not authorize later response.create events.
func refreshResponsesWSAuthorization(c *gin.Context) {
	state := c.Request.Context().Value(responsesWSRequestContextKey{}).(*responsesWSRequestState)
	token, err := model.GetTokenByIds(c.GetInt("token_id"), c.GetInt("id"))
	if err != nil || token.Key != c.GetString("token_key") ||
		(token.Status != common.TokenStatusEnabled && token.Status != common.TokenStatusExhausted) ||
		(token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp()) {
		state.apiError = types.NewErrorWithStatusCode(errors.New("token is no longer valid"), types.ErrorCodeAccessDenied, http.StatusUnauthorized, types.ErrOptionWithSkipRetry())
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	user, err := model.GetUserById(token.UserId, false)
	if err != nil || user.Status != common.UserStatusEnabled {
		state.apiError = types.NewErrorWithStatusCode(errors.New("user is no longer allowed"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if _, pinned := c.Get("specific_channel_id"); pinned && user.Role < common.RoleAdminUser {
		state.apiError = types.NewErrorWithStatusCode(errors.New("channel binding is no longer allowed"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if allowed := token.GetIpLimits(); len(allowed) > 0 && !common.IsIpInCIDRList(net.ParseIP(c.ClientIP()), allowed) {
		state.apiError = types.NewErrorWithStatusCode(errors.New("client address is not allowed"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	user.ToBaseUser().WriteContext(c)
	if err := middleware.SetupContextForToken(c, token); err != nil {
		state.apiError = types.NewErrorWithStatusCode(errors.New("token permissions cannot be refreshed"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Next()
}

type responsesWSResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *responsesWSResponseWriter) Header() http.Header {
	return w.header
}

func (w *responsesWSResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *responsesWSResponseWriter) Write(data []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(data)
}

func newResponsesWSRequestRunner(c *gin.Context) relay.ResponsesWSRequestRunner {
	// Capture credentials before channel selection or header overrides. Resolve
	// the peer once with the public router's trusted-proxy configuration.
	headers := c.Request.Header.Clone()
	for _, name := range []string{"Connection", "Upgrade", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol", "Content-Length", "Content-Encoding"} {
		headers.Del(name)
	}
	remoteAddr := net.JoinHostPort(c.ClientIP(), "0")
	return func(request *http.Request, requestID string, handle func(*gin.Context) *types.NewAPIError) *types.NewAPIError {
		state := &responsesWSRequestState{requestID: requestID, handle: handle}
		ctx := context.WithValue(request.Context(), responsesWSRequestContextKey{}, state)
		ctx = context.WithValue(ctx, common.RequestIdKey, requestID)
		request = request.Clone(ctx)
		request.Method = http.MethodPost
		request.URL.Path = "/v1/responses"
		request.URL.RawPath = ""
		request.Header = headers.Clone()
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = remoteAddr
		response := &responsesWSResponseWriter{header: make(http.Header)}
		responsesWSRequestEngine().ServeHTTP(response, request)
		if state.apiError != nil {
			return state.apiError
		}
		if response.status < http.StatusBadRequest {
			return nil
		}
		var body struct {
			Error *types.OpenAIError `json:"error"`
		}
		if common.Unmarshal(response.body.Bytes(), &body) == nil && body.Error != nil {
			return types.WithOpenAIError(*body.Error, response.status, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		// The existing in-memory rate limiter returns a bare 429 response.
		return types.NewErrorWithStatusCode(errors.New(http.StatusText(response.status)), types.ErrorCodeInvalidRequest, response.status, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
}

func ResponsesWebSocket(c *gin.Context) {
	runner := newResponsesWSRequestRunner(c)
	upgrade := websocket.Upgrader{Subprotocols: []string{"responses"}, CheckOrigin: upgrader.CheckOrigin}
	ws, err := upgrade.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	_ = relay.ResponsesWebSocketHelper(c, ws, runner)
}
