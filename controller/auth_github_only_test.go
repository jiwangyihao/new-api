package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPasswordRegisterTrialTest(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.TrialCode{}, &model.TrialRedemption{}))
	originalRegisterEnabled := common.RegisterEnabled
	originalPasswordRegisterEnabled := common.PasswordRegisterEnabled
	originalEmailVerificationEnabled := common.EmailVerificationEnabled
	originalGenerateDefaultToken := constant.GenerateDefaultToken
	originalGitHubOnlySignupEnabled := common.GitHubOnlySignupEnabled
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.GitHubOnlySignupEnabled = false
	constant.GenerateDefaultToken = false
	t.Cleanup(func() {
		common.RegisterEnabled = originalRegisterEnabled
		common.PasswordRegisterEnabled = originalPasswordRegisterEnabled
		common.EmailVerificationEnabled = originalEmailVerificationEnabled
		common.GitHubOnlySignupEnabled = originalGitHubOnlySignupEnabled
		constant.GenerateDefaultToken = originalGenerateDefaultToken
	})
	return db
}

func seedControllerTrialPlan(t *testing.T, id int, code string) *model.SubscriptionPlan {
	t.Helper()
	plan := &model.SubscriptionPlan{Id: id, Title: code, DurationUnit: model.SubscriptionDurationHour, DurationValue: 24, Enabled: true, MonthlyTokenLimit: 0, ConcurrencyLimit: 1, IsTrial: true, PublicVisible: false, TrialDurationHours: 24, RewardEligible: false, BusinessCode: &code}
	require.NoError(t, model.DB.Create(plan).Error)
	return plan
}

func performPasswordRegister(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/register?turnstile=token", bytes.NewBufferString(body))
	Register(ctx)
	return recorder
}

func TestPasswordRegister_GrantsTrialCode(t *testing.T) {
	setupPasswordRegisterTrialTest(t)
	plan := seedControllerTrialPlan(t, 7801, "trial_24h")
	require.NoError(t, model.DB.Create(&model.TrialCode{Id: 7802, Code: "TRIAL7802", PlanId: plan.Id, Enabled: true, MaxRedemptions: 1}).Error)

	recorder := performPasswordRegister(t, `{"username":"trialuser","password":"Passw0rd!","trial_code":" trial7802 "}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "trialuser").First(&user).Error)
	var sub model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ?", user.Id).First(&sub).Error)
	assert.Equal(t, "trial_code", sub.GrantReason)
	assert.Equal(t, plan.Id, sub.PlanId)

	var redemption model.TrialRedemption
	require.NoError(t, model.DB.Where("user_id = ?", user.Id).First(&redemption).Error)
	assert.Equal(t, "TRIAL7802", redemption.Code)
}

func TestPasswordRegister_GrantsInviteTrialWithoutTrialCode(t *testing.T) {
	setupPasswordRegisterTrialTest(t)
	plan := seedControllerTrialPlan(t, 7811, "trial_24h")
	require.NoError(t, model.DB.Create(&model.User{Id: 7812, Username: "inviter", Status: common.UserStatusEnabled, AffCode: "INV7812"}).Error)

	recorder := performPasswordRegister(t, `{"username":"invitee","password":"Passw0rd!","aff_code":"INV7812"}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "invitee").First(&user).Error)
	var sub model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ?", user.Id).First(&sub).Error)
	assert.Equal(t, "invite_trial", sub.GrantReason)
	assert.Equal(t, plan.Id, sub.PlanId)
	assert.Equal(t, 7812, sub.GrantSourceUserId)
}

func TestPasswordRegister_AcceptsAffAliasFromDefaultFrontend(t *testing.T) {
	setupPasswordRegisterTrialTest(t)
	seedControllerTrialPlan(t, 7820, "trial_aff_alias")
	require.NoError(t, model.DB.Create(&model.User{Id: 7821, Username: "inviter_alias", Status: common.UserStatusEnabled, AffCode: "UxMF"}).Error)

	recorder := performPasswordRegister(t, `{"username":"aliasinvitee","password":"Passw0rd!","aff":"UxMF"}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "aliasinvitee").First(&user).Error)
	assert.Equal(t, 7821, user.InviterId)
}

func TestPasswordRegister_TreatsTrialCodeFieldAsInviteCode(t *testing.T) {
	setupPasswordRegisterTrialTest(t)
	plan := seedControllerTrialPlan(t, 7822, "trial_invite_manual")
	require.NoError(t, model.DB.Model(plan).Update("invite_trial", true).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 7823, Username: "inviter_manual", Status: common.UserStatusEnabled, AffCode: "Manual42"}).Error)

	recorder := performPasswordRegister(t, `{"username":"manualinvitee","password":"Passw0rd!","trial_code":" Manual42 "}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "manualinvitee").First(&user).Error)
	assert.Equal(t, 7823, user.InviterId)

	var sub model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ?", user.Id).First(&sub).Error)
	assert.Equal(t, "invite_trial", sub.GrantReason)
	assert.Equal(t, plan.Id, sub.PlanId)
	assert.Equal(t, 7823, sub.GrantSourceUserId)
}

func TestGitHubOnlySignupRejectsPasswordRegister(t *testing.T) {
	setupPasswordRegisterTrialTest(t)
	common.GitHubOnlySignupEnabled = true

	recorder := performPasswordRegister(t, `{"username":"githubonly","password":"Passw0rd!"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "GitHub")
	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "githubonly").Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestGitHubOnlySignupKeepsPasswordLogin(t *testing.T) {
	setupPasswordRegisterTrialTest(t)
	common.GitHubOnlySignupEnabled = true
	hashed, err := common.Password2Hash("Passw0rd!")
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.User{Id: 7901, Username: "github-created", Password: hashed, Status: common.UserStatusEnabled}).Error)

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	router.POST("/api/user/login", Login)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewBufferString(`{"username":"github-created","password":"Passw0rd!"}`))
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
}

func TestStatusIncludesGitHubOnlySignup(t *testing.T) {
	setupPasswordRegisterTrialTest(t)
	common.GitHubOnlySignupEnabled = true
	common.GitHubOAuthEnabled = true
	common.GitHubClientId = "github-client"

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	GetStatus(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"github_only_signup_enabled":true`)
}
