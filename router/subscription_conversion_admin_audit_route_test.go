package router

import (
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

type adminSubscriptionAuditRouteResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		Subscription struct {
			Id                        int    `json:"id"`
			Status                    string `json:"status"`
			ConversionId              any    `json:"conversion_id"`
			ConvertedToSubscriptionId any    `json:"converted_to_subscription_id"`
		} `json:"subscription"`
		ConversionAudit *struct {
			ConversionId         string `json:"conversion_id"`
			SourceSubscriptionId string `json:"source_subscription_id"`
			TargetSubscriptionId string `json:"target_subscription_id"`
			SourceStatusBefore   string `json:"source_status_before"`
			SourceStatusAfter    string `json:"source_status_after"`
			TargetStatus         string `json:"target_status"`
			ConvertedAt          string `json:"converted_at"`
		} `json:"conversion_audit"`
	} `json:"data"`
}

func TestAdminUserSubscriptionsExposeConvertedAuditRelationship(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.UserSubscription{},
		&model.SubscriptionConversion{},
	))

	const adminID = 10_201
	const userID = 10_202
	const sourceID = 10_203
	const targetID = 9_007_199_254_740_997
	const conversionID = 9_007_199_254_740_993
	const convertedAt = int64(1_789_000_000)
	accessToken := "subscription-audit-admin-token"
	require.NoError(t, db.Create(&model.User{
		Id: adminID, Username: "subscription-audit-admin", Status: common.UserStatusEnabled,
		Role: common.RoleAdminUser, AccessToken: &accessToken, AffCode: "audit-admin",
	}).Error)
	require.NoError(t, db.Create(&model.User{
		Id: userID, Username: "subscription-audit-user", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, AffCode: "audit-user",
	}).Error)

	timedCode := "subscription_audit_timed"
	creditCode := "subscription_audit_credit"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: 10_206, Title: "Timed", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode,
	}).Error)
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: 10_207, Title: "Credit balance", EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode,
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id: sourceID, UserId: userID, PlanId: 10_206,
		EntitlementType: model.SubscriptionEntitlementTimed,
		Status:          model.SubscriptionStatusConverted,
		ConvertedAt:     convertedAt, ConversionId: conversionID,
		ConvertedToSubscriptionId: targetID,
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id: targetID, UserId: userID, PlanId: 10_207,
		EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Status:          model.SubscriptionStatusActive,
	}).Error)
	require.NoError(t, db.Create(&model.SubscriptionConversion{
		Id: conversionID, UserId: userID, IdempotencyKey: "admin-audit-conversion",
		SourceSubscriptionId: sourceID, SourcePlanId: 10_206, SourcePlanTitle: "Timed",
		TargetSubscriptionId: targetID, TargetPlanId: 10_207, LedgerId: 10_208,
		SourceStatus: model.SubscriptionStatusExpired, ConvertedAt: convertedAt, CreatedAt: convertedAt,
	}).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("subscription-audit-secret"))))
	SetApiRouter(engine)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/subscription/admin/users/10202/subscriptions", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", "10201")
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response adminSubscriptionAuditRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	require.True(t, response.Success)
	require.Len(t, response.Data, 2)

	var convertedAudit *struct {
		ConversionId         string `json:"conversion_id"`
		SourceSubscriptionId string `json:"source_subscription_id"`
		TargetSubscriptionId string `json:"target_subscription_id"`
		SourceStatusBefore   string `json:"source_status_before"`
		SourceStatusAfter    string `json:"source_status_after"`
		TargetStatus         string `json:"target_status"`
		ConvertedAt          string `json:"converted_at"`
	}
	for _, record := range response.Data {
		if record.Subscription.Id == sourceID {
			assert.Equal(t, model.SubscriptionStatusConverted, record.Subscription.Status)
			convertedAudit = record.ConversionAudit
			assert.Nil(t, record.Subscription.ConversionId)
			assert.Nil(t, record.Subscription.ConvertedToSubscriptionId)
		} else {
			assert.Nil(t, record.ConversionAudit)
		}
	}
	require.NotNil(t, convertedAudit)
	assert.Equal(t, "9007199254740993", convertedAudit.ConversionId)
	assert.Equal(t, "10203", convertedAudit.SourceSubscriptionId)
	assert.Equal(t, "9007199254740997", convertedAudit.TargetSubscriptionId)
	assert.Equal(t, model.SubscriptionStatusExpired, convertedAudit.SourceStatusBefore)
	assert.Equal(t, model.SubscriptionStatusConverted, convertedAudit.SourceStatusAfter)
	assert.Equal(t, model.SubscriptionStatusActive, convertedAudit.TargetStatus)
	assert.Equal(t, "1789000000", convertedAudit.ConvertedAt)
}
