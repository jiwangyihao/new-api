package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/pkg/routehealth"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectEndpointTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldMemoryCache := common.MemoryCacheEnabled
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Model{}, &model.ChannelGroup{}, &model.ChannelGroupChannel{}))
	t.Cleanup(func() {
		model.DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.MemoryCacheEnabled = oldMemoryCache
		model.InvalidatePricingCache()
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func endpointStringsForServiceTest(t *testing.T, endpoints map[string]any) string {
	t.Helper()
	data, err := common.Marshal(endpoints)
	require.NoError(t, err)
	return string(data)
}

func TestCacheGetRandomSatisfiedChannelFiltersEndpointOnRetry(t *testing.T) {
	db := setupChannelSelectEndpointTestDB(t)
	openAIHigh := int64(100)
	codexLow := int64(10)
	require.NoError(t, db.Create(&model.Channel{Id: 2501, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Priority: &openAIHigh}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 2502, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", Priority: &codexLow}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "gpt-5.5", ChannelId: 2501, Enabled: true, Priority: &openAIHigh},
		{Group: "default", Model: "gpt-5.5", ChannelId: 2502, Enabled: true, Priority: &codexLow},
	}).Error)
	model.RefreshEndpointSupportCache()
	ctx := gin.Context{}

	channel, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: &ctx, TokenGroup: "default", ModelName: "gpt-5.5", Retry: common.GetPointer(0), EndpointType: constant.EndpointTypeOpenAIResponseCompact})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "default", group)
	assert.Equal(t, 2502, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelUsesChannelEndpointSettings(t *testing.T) {
	db := setupChannelSelectEndpointTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 2504, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default", OtherSettings: endpointStringsForServiceTest(t, map[string]any{
		"supported_endpoint_types": []string{string(constant.EndpointTypeOpenAIResponse), string(constant.EndpointTypeOpenAIResponseCompact)},
	})}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2504, Enabled: true}).Error)
	model.RefreshEndpointSupportCache()
	ctx := gin.Context{}

	channel, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: &ctx, TokenGroup: "default", ModelName: "gpt-5.5", Retry: common.GetPointer(0), EndpointType: constant.EndpointTypeOpenAIResponseCompact})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "default", group)
	assert.Equal(t, 2504, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelWithoutEndpointUsesLegacySelection(t *testing.T) {
	db := setupChannelSelectEndpointTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 2503, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-5.5", Group: "default"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-5.5", ChannelId: 2503, Enabled: true}).Error)
	model.RefreshEndpointSupportCache()
	ctx := gin.Context{}

	channel, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: &ctx, TokenGroup: "default", ModelName: "gpt-5.5", Retry: common.GetPointer(0)})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "default", group)
	assert.Equal(t, 2503, channel.Id)
}

func TestRouteHealthRetriesSamePriorityAndIsolatesCredential(t *testing.T) {
	db := setupChannelSelectEndpointTestDB(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	priority := int64(100)
	lower := int64(1)
	for _, ch := range []model.Channel{
		{Id: 9101, Key: "a\nb", Models: "route-test", Status: common.ChannelStatusEnabled, Priority: &priority, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling}},
		{Id: 9102, Key: "c", Models: "route-test", Status: common.ChannelStatusEnabled, Priority: &priority},
		{Id: 9103, Key: "d", Models: "route-test", Status: common.ChannelStatusEnabled, Priority: &lower},
	} {
		require.NoError(t, db.Create(&ch).Error)
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "route-test", ChannelId: ch.Id, Enabled: true, Priority: ch.Priority}).Error)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	defer ReleaseRouteAttempt(c)
	param := &RetryParam{Ctx: c, TokenGroup: "default", ModelName: "route-test", PreferredChannelID: 9101}
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, 9101, channel.Id)
	key, _, ok := SelectedRouteCredential(c, 9101)
	require.True(t, ok)
	require.Equal(t, "a", key)
	MarkRouteUpstreamStarted(c)
	FinishRouteAttempt(c, nil, types.NewErrorWithStatusCode(errors.New("invalid key"), types.ErrorCodeBadResponse, 401))
	param.UsedChannelIds = []int{9101}
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, 9101, channel.Id)
	key, _, _ = SelectedRouteCredential(c, 9101)
	require.Equal(t, "b", key)
	MarkRouteUpstreamStarted(c)
	FinishRouteAttempt(c, nil, types.NewErrorWithStatusCode(errors.New("unavailable"), types.ErrorCodeBadResponse, 503))
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, 9102, channel.Id, "same-priority untried channel must beat lower priority")
}

func TestRouteHealthIdentityAndCancellation(t *testing.T) {
	c := &gin.Context{}
	mapping := `{"alias-a":"actual","alias-b":"actual"}`
	ch := &model.Channel{Id: 9201, Key: "key", Status: common.ChannelStatusEnabled, ModelMapping: &mapping}
	a, err := buildRouteSelections(c, []*model.Channel{ch}, "group-a", "alias-a", "/v1/responses", 0)
	require.NoError(t, err)
	b, err := buildRouteSelections(c, []*model.Channel{ch}, "group-b", "alias-b", "/v1/responses", 0)
	require.NoError(t, err)
	require.Equal(t, a[0].candidate.ID, b[0].candidate.ID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Equal(t, routehealth.VerdictIgnored, ClassifyRouteAttempt(ctx, nil, types.NewError(errors.New("transport"), types.ErrorCodeDoRequestFailed), true).Verdict)
	require.Equal(t, routehealth.VerdictIgnored, ClassifyRouteAttempt(context.Background(), nil, types.NewError(errors.New("cap"), types.ErrorCodeAPIKeyTokenLimitExhausted), true).Verdict)
	stream := &relaycommon.StreamStatus{}
	stream.MarkFailed("server_error", "server_error", 503)
	require.Equal(t, routehealth.VerdictFailure, ClassifyRouteAttempt(context.Background(), &relaycommon.RelayInfo{StreamStatus: stream}, nil, true).Verdict)
	state := routeState(c)
	state.started = time.Now().Add(-time.Duration(operation_setting.GetRouteHealthSetting().RetryBudgetSeconds+1) * time.Second)
	require.False(t, RouteRetryBudgetAvailable(c))
}

func TestResponseRouteBindingPinsIdentityAndFailsClosed(t *testing.T) {
	db := setupChannelSelectEndpointTestDB(t)
	oldRedis, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldRedis, oldRDB })
	ch := &model.Channel{Id: 9301, Key: "secret-key", Models: "response-test", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(ch).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "original", Model: "response-test", ChannelId: ch.Id, Enabled: true}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyUserId, 777)
	common.SetContextKey(c, constant.ContextKeyTokenGroups, []string{"original"})
	_, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: c, TokenGroup: "original", ModelName: "response-test"})
	require.NoError(t, err)
	defer ReleaseRouteAttempt(c)
	info := &relaycommon.RelayInfo{UserId: 777, OriginModelName: "response-test", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: ch.Id, ApiKey: ch.Key}}
	require.NoError(t, SaveResponseBinding(c, info, "resp_route_fixture", "resp_upstream_actual"))
	entry, found, err := LoadResponseBinding(c, "resp_route_fixture")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "resp_upstream_actual", entry.UpstreamID)
	require.NotEqual(t, ch.Key, entry.KeyFingerprint)
	require.Nil(t, ResponseRouteBinding(c, "resp_route_fixture", "response-test"))
	require.NotNil(t, ResponseRouteBinding(c, "resp_route_fixture", "other-model"))
	common.SetContextKey(c, constant.ContextKeyTokenGroups, []string{"different"})
	require.NotNil(t, ResponseRouteBinding(c, "resp_route_fixture", "response-test"))
	common.SetContextKey(c, constant.ContextKeyUserId, 778)
	require.Equal(t, 404, ResponseRouteBinding(c, "resp_route_fixture", "response-test").StatusCode)
	require.Nil(t, ResponseRouteBinding(c, "resp_original_legacy", "response-test"))
	common.RedisEnabled, common.RDB = true, nil
	_, _, err = LoadResponseBinding(c, "resp_route_fixture")
	require.Error(t, err)
}

func TestBoundRouteUnavailableDoesNotFallback(t *testing.T) {
	db := setupChannelSelectEndpointTestDB(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	for _, id := range []int{9501, 9502} {
		require.NoError(t, db.Create(&model.Channel{Id: id, Key: "key", Models: "bound-test", Status: common.ChannelStatusEnabled}).Error)
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "bound-test", ChannelId: id, Enabled: true}).Error)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ch, err := model.CacheGetChannel(9501)
	require.NoError(t, err)
	selections, err := buildRouteSelections(c, []*model.Channel{ch}, "default", "bound-test", "/v1/responses", 0)
	require.NoError(t, err)
	for _, resource := range selections[0].candidate.Resources {
		if resource.Scope == routehealth.ScopeRoute {
			routeState(c).excluded[resource.Key] = struct{}{}
		}
	}
	selected, _, err := SelectRouteForChannel(c, ch, &RetryParam{Ctx: c, TokenGroup: "default", ModelName: "bound-test"})
	require.Nil(t, selected)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "route_temporarily_unavailable", string(apiErr.GetErrorCode()))
	require.Equal(t, "1", c.Writer.Header().Get("Retry-After"))
	selected, _, err = CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: c, TokenGroup: "default", ModelName: "bound-test"})
	require.NoError(t, err)
	require.Equal(t, 9502, selected.Id)
	ReleaseRouteAttempt(c)
}
