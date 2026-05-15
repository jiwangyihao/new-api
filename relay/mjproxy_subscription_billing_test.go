package relay

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMidjourneySubscriptionOnlyTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))
}

func midjourneySubscriptionOnlyContext(t *testing.T, userID int, tokenID int, tokenKey string) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/mj/submit/imagine", nil)
	ctx.Set(string(constant.ContextKeyUserId), userID)
	ctx.Set(string(constant.ContextKeyTokenId), tokenID)
	ctx.Set(string(constant.ContextKeyTokenKey), tokenKey)
	ctx.Set(string(constant.ContextKeyOriginalModel), "mj_imagine")
	ctx.Set(string(constant.ContextKeyUserQuota), 0)
	ctx.Set(string(constant.ContextKeyUserGroup), "default")
	ctx.Set(string(constant.ContextKeyUsingGroup), "default")
	ctx.Set(string(constant.ContextKeyUserSetting), dto.UserSetting{BillingPreference: "wallet_only"})
	return ctx
}

func TestMidjourneySubmissionRejectsWithoutSubscriptionBeforeWalletQuota(t *testing.T) {
	setupMidjourneySubscriptionOnlyTestDB(t)
	const userID = 9801
	const tokenID = 9802
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "mj_no_sub", Quota: 1000000, Status: common.UserStatusEnabled, AffCode: "aff9801"}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-mj-no-sub", Status: common.TokenStatusEnabled, RemainQuota: 1000000}).Error)
	ctx := midjourneySubscriptionOnlyContext(t, userID, tokenID, "sk-mj-no-sub")
	info := &relaycommon.RelayInfo{UserId: userID, TokenId: tokenID, TokenKey: "sk-mj-no-sub", RelayFormat: types.RelayFormatMjProxy, RelayMode: relayconstant.RelayModeMidjourneyImagine, OriginModelName: "mj_imagine", UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}

	mjErr := EnsureMidjourneySubscriptionBilling(ctx, info, 10)

	require.NotNil(t, mjErr)
	assert.Contains(t, mjErr.Description, "subscription")
	assert.Empty(t, info.BillingSource)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 1000000, user.Quota)
	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, 1000000, token.RemainQuota)
}

func TestMidjourneySubmissionRejectsWithDistributorSubscriptionWithoutWalletFallback(t *testing.T) {
	setupMidjourneySubscriptionOnlyTestDB(t)
	const userID = 9811
	const tokenID = 9812
	const planID = 9813
	const subID = 9814
	code := "mj-sub-only"
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "mj_with_sub", Quota: 1000000, Status: common.UserStatusEnabled, AffCode: "aff9811"}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-mj-with-sub", Status: common.TokenStatusEnabled, RemainQuota: 1000000}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "MJ Plan", Enabled: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subID, UserId: userID, PlanId: planID, TokenLimit: 1000, TokenUsed: 0, ConcurrencyLimit: 1, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order"}).Error)
	ctx := midjourneySubscriptionOnlyContext(t, userID, tokenID, "sk-mj-with-sub")
	info := &relaycommon.RelayInfo{UserId: userID, TokenId: tokenID, TokenKey: "sk-mj-with-sub", RelayFormat: types.RelayFormatMjProxy, RelayMode: relayconstant.RelayModeMidjourneyImagine, OriginModelName: "mj_imagine", UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}

	mjErr := EnsureMidjourneySubscriptionBilling(ctx, info, 10)

	require.NotNil(t, mjErr)
	assert.Contains(t, mjErr.Description, "subscription")
	assert.Empty(t, info.BillingSource)
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, subID).Error)
	assert.Equal(t, int64(0), sub.TokenUsed)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 1000000, user.Quota)
	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, 1000000, token.RemainQuota)
}
