package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
)

type ResponseBinding struct {
	ChannelID         int    `json:"channel_id"`
	KeyFingerprint    string `json:"key_fingerprint"`
	ConfigFingerprint string `json:"config_fingerprint"`
	UpstreamID        string `json:"upstream_id"`
	Model             string `json:"model"`
	Group             string `json:"group"`
}

var responseBindings = hot.NewHotCache[string, ResponseBinding](hot.LRU, 100000).WithTTL(24 * time.Hour).Build()

func responseBindingKey(userID int, responseID string) string {
	return fmt.Sprintf("new-api:response-route:v1:%d:%s", userID, routeFingerprint(responseID))
}

func SaveResponseBinding(c *gin.Context, info *relaycommon.RelayInfo, publicID, upstreamID string) error {
	if c == nil || c.Request == nil || info == nil || info.ChannelMeta == nil || info.UserId <= 0 || info.ChannelId <= 0 || publicID == "" || upstreamID == "" {
		return errors.New("response routing context is incomplete")
	}
	if common.RedisEnabled && common.RDB == nil {
		return errors.New("response routing storage is unavailable")
	}
	value, ok := c.Get(routeAttemptContextKey)
	if !ok {
		return errors.New("response route was not admitted")
	}
	state := value.(*routeAttemptState)
	if state.selected == nil || state.selected.channel.Id != info.ChannelId {
		return errors.New("response route does not match the admitted channel")
	}
	group := state.selected.group
	if group == "" || info.OriginModelName == "" {
		return errors.New("response routing identity is incomplete")
	}
	entry := ResponseBinding{ChannelID: info.ChannelId, KeyFingerprint: routeFingerprint(state.selected.key), UpstreamID: upstreamID, Model: info.OriginModelName, Group: group}
	entry.ConfigFingerprint = RouteConfigurationFingerprint(state.selected.channel)
	key := responseBindingKey(info.UserId, publicID)
	if common.RedisEnabled {
		encoded, err := common.Marshal(entry)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), time.Second)
		defer cancel()
		if err := common.RDB.Set(ctx, key, encoded, 24*time.Hour).Err(); err != nil {
			return err
		}
	}
	responseBindings.SetWithTTL(key, entry, 24*time.Hour)
	return nil
}

func LoadResponseBinding(c *gin.Context, responseID string) (ResponseBinding, bool, error) {
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if userID <= 0 || responseID == "" || len(responseID) > 1024 {
		return ResponseBinding{}, false, nil
	}
	key := responseBindingKey(userID, responseID)
	if common.RedisEnabled {
		if common.RDB == nil {
			return ResponseBinding{}, false, errors.New("response routing storage is unavailable")
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		value, err := common.RDB.Get(ctx, key).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return ResponseBinding{}, false, nil
			}
			return ResponseBinding{}, false, err
		}
		var entry ResponseBinding
		if err := common.UnmarshalJsonStr(value, &entry); err != nil {
			return entry, false, err
		}
		return entry, true, nil
	}
	return responseBindings.Get(key)
}

func ResponseRouteBinding(c *gin.Context, responseID, modelName string) *types.NewAPIError {
	entry, found, err := LoadResponseBinding(c, responseID)
	if err != nil {
		return types.NewErrorWithStatusCode(errors.New("response routing is temporarily unavailable"), types.ErrorCodeGetChannelFailed, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	if !found {
		if strings.HasPrefix(responseID, "resp_route_") {
			return types.NewErrorWithStatusCode(errors.New("response reference is missing, expired or not accessible"), "response_not_found", http.StatusNotFound, types.ErrOptionWithSkipRetry())
		}
		return nil
	}
	if entry.ChannelID <= 0 || entry.KeyFingerprint == "" || entry.ConfigFingerprint == "" || entry.UpstreamID == "" || entry.Group == "" {
		return types.NewErrorWithStatusCode(errors.New("response reference is invalid"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	if entry.Model != modelName {
		return types.NewErrorWithStatusCode(errors.New("response reference model does not match request model"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	channel, err := model.CacheGetChannel(entry.ChannelID)
	if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
		return types.NewErrorWithStatusCode(errors.New("response reference channel is unavailable"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	if entry.ConfigFingerprint != RouteConfigurationFingerprint(channel) {
		return types.NewErrorWithStatusCode(errors.New("response route configuration has changed"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	credentialValid := false
	for _, credential := range channel.EnabledCredentials() {
		if RouteCredentialFingerprint(credential.Key) == entry.KeyFingerprint {
			credentialValid = true
			break
		}
	}
	if !credentialValid {
		return types.NewErrorWithStatusCode(errors.New("response reference credential is unavailable"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	groups := c.GetStringSlice(string(constant.ContextKeyTokenGroups))
	if len(groups) == 0 {
		groups = []string{model.DefaultChannelGroupName}
	}
	if !slices.Contains(groups, entry.Group) {
		return types.NewErrorWithStatusCode(errors.New("response reference group is not allowed"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	candidates, candidateErr := model.GetSatisfiedChannelCandidates([]string{entry.Group}, modelName, "", nil, 0, false, model.ChannelBillingProfile{}, false)
	if candidateErr != nil {
		return types.NewErrorWithStatusCode(errors.New("response route authorization is unavailable"), types.ErrorCodeGetChannelFailed, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	if !slices.ContainsFunc(candidates, func(ch *model.Channel) bool { return ch.Id == entry.ChannelID }) {
		return types.NewErrorWithStatusCode(errors.New("response is not available in the original model group"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	if pin, pinned := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); pinned && fmt.Sprint(pin) != strconv.Itoa(entry.ChannelID) {
		return types.NewErrorWithStatusCode(errors.New("response channel conflicts with the token binding"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	c.Set("response_route_binding", entry)
	return nil
}
