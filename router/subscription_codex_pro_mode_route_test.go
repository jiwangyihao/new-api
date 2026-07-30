package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionSelfCodexProModeRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.GPTAbuseSignalLog{}, &model.GPTAbuseUserSuspension{}))
	accessToken := "codex-pro-route-token"
	require.NoError(t, model.DB.Create(&model.User{Id: 9941, Username: "codex-pro-route", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/subscription/self/codex-pro-mode", bytes.NewBufferString(`{"mode":"all"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("New-Api-User", "9941")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotEqual(t, "404 page not found", recorder.Body.String())
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	var rawSetting string
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9941).Select("setting").Scan(&rawSetting).Error)
	var setting map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(rawSetting), &setting))
	assert.Equal(t, "all", setting["codex_pro_mode"])
}

func TestSubscriptionCreditBalanceLedgerRouteIsAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.CreditBalanceLedger{}, &model.SubscriptionOrder{}))
	accessToken := "credit-ledger-route-token"
	require.NoError(t, model.DB.Create(&model.User{Id: 9942, Username: "credit-ledger-route", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}).Error)
	require.NoError(t, model.DB.Create(&model.CreditBalanceLedger{UserId: 9942, UserSubscriptionId: 9943, Type: model.CreditBalanceLedgerTypePurchase, IdempotencyKey: "route-ledger", SourceType: model.CreditBalanceLedgerSourceSubscriptionOrder, SourceId: 9944, GrossCredit: 100, BalanceBefore: 0, BalanceAfter: 100, AvailableCreditAfter: 100, CreatedAt: common.GetTimestamp()}).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	unauthorized := httptest.NewRecorder()
	engine.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/subscription/self/credit-balance/ledger", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/subscription/self/credit-balance/ledger", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("New-Api-User", "9942")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"gross_credit":100`)
	assert.NotContains(t, recorder.Body.String(), `"user_id":9941`)
}

func TestSubscriptionBillingStrategyRouteIsAuthenticatedAndRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	accessToken := "billing-strategy-route-token"
	require.NoError(t, model.DB.Create(&model.User{Id: 9943, Username: "billing-strategy-route", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	unauthorized := httptest.NewRecorder()
	unauthorizedReq := httptest.NewRequest(http.MethodPut, "/api/subscription/self/billing-strategy", bytes.NewBufferString(`{"billing_strategy":"timed_first"}`))
	unauthorizedReq.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(unauthorized, unauthorizedReq)
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/subscription/self/billing-strategy", bytes.NewBufferString(`{"billing_strategy":"timed_first"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("New-Api-User", "9943")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"billing_strategy":"timed_first"`)
}
