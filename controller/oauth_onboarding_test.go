package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type onboardingMockProvider struct {
	name    string
	prefix  string
	enabled bool
	user    *oauth.OAuthUser
}

func (p *onboardingMockProvider) GetName() string             { return p.name }
func (p *onboardingMockProvider) PendingProviderName() string { return "github" }
func (p *onboardingMockProvider) IsEnabled() bool             { return p.enabled }
func (p *onboardingMockProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*oauth.OAuthToken, error) {
	return &oauth.OAuthToken{AccessToken: "mock-token"}, nil
}
func (p *onboardingMockProvider) GetUserInfo(ctx context.Context, token *oauth.OAuthToken) (*oauth.OAuthUser, error) {
	return p.user, nil
}
func (p *onboardingMockProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsGitHubIdAlreadyTaken(providerUserID)
}
func (p *onboardingMockProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.GitHubId = providerUserID
	return user.FillUserByGitHubId()
}
func (p *onboardingMockProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.GitHubId = providerUserID
}
func (p *onboardingMockProvider) GetProviderPrefix() string { return p.prefix }

func setupOAuthOnboardingControllerTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.TrialCode{}, &model.TrialRedemption{}, &model.OAuthProviderLock{}, &model.UserOAuthBinding{}))
	originalRegisterEnabled := common.RegisterEnabled
	originalTurnstileCheckEnabled := common.TurnstileCheckEnabled
	originalGitHubOAuthEnabled := common.GitHubOAuthEnabled
	common.RegisterEnabled = true
	common.TurnstileCheckEnabled = true
	common.GitHubOAuthEnabled = true
	middleware.SetTurnstileVerifierForTest(t, func(ctx context.Context, token string, remoteIP string) (bool, error) {
		return token == "valid-turnstile", nil
	})
	defaultOAuthOnboardingStore = &oauthOnboardingStore{sessions: make(map[string]OAuthOnboardingPendingSession)}
	t.Cleanup(func() {
		common.RegisterEnabled = originalRegisterEnabled
		common.TurnstileCheckEnabled = originalTurnstileCheckEnabled
		common.GitHubOAuthEnabled = originalGitHubOAuthEnabled
		oauth.Unregister("mockgithub")
	})
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	r.GET("/api/oauth/:provider", HandleOAuth)
	r.GET("/api/oauth-test/:provider", func(c *gin.Context) {
		provider := oauth.GetProvider(c.Param("provider"))
		require.NotNil(t, provider)
		oauthUser, err := provider.GetUserInfo(c.Request.Context(), &oauth.OAuthToken{AccessToken: "mock-token"})
		require.NoError(t, err)
		pending, err := CreateOAuthOnboardingPending(c.Param("provider"), provider, oauthUser, 0)
		require.NoError(t, err)
		respondOAuthOnboardingRequired(c, pending)
	})
	r.GET("/api/oauth/onboarding", GetOAuthOnboarding)
	r.POST("/api/oauth/onboarding", CompleteOAuthOnboarding)
	return r
}

func performOAuthOnboardingRequest(t *testing.T, router *gin.Engine, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	return recorder
}

func extractJSONPathString(t *testing.T, body []byte, path string) string {
	t.Helper()
	return gjson.GetBytes(body, path).String()
}

func TestGitHubOAuthOnboarding(t *testing.T) {
	router := setupOAuthOnboardingControllerTest(t)
	oauth.Register("mockgithub", &onboardingMockProvider{name: "GitHub", prefix: "github_", enabled: true, user: &oauth.OAuthUser{ProviderUserID: "gh-100", Username: "octocat", DisplayName: "Octo Cat", Email: "octo@example.com"}})

	callback := performOAuthOnboardingRequest(t, router, http.MethodGet, "/api/oauth-test/mockgithub", "")
	require.Equal(t, http.StatusOK, callback.Code)
	assert.Contains(t, callback.Body.String(), "oauth_onboarding_required")
	pendingToken := extractJSONPathString(t, callback.Body.Bytes(), "data.pending_token")
	require.NotEmpty(t, pendingToken)

	getPending := performOAuthOnboardingRequest(t, router, http.MethodGet, "/api/oauth/onboarding?pending_token="+pendingToken, "")
	require.Equal(t, http.StatusOK, getPending.Code)
	assert.Equal(t, "octocat", extractJSONPathString(t, getPending.Body.Bytes(), "data.login"))

	body := `{"pending_token":"` + pendingToken + `","password":"Passw0rd!","terms_accepted":true,"turnstile_token":"valid-turnstile"}`
	complete := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", body)
	require.Equal(t, http.StatusOK, complete.Code)
	assert.Contains(t, complete.Body.String(), `"success":true`)

	var user model.User
	require.NoError(t, model.DB.Where("github_id = ?", "gh-100").First(&user).Error)
	assert.Equal(t, "octocat", user.Username)
	assert.Equal(t, "octo@example.com", user.Email)
	assert.NotEmpty(t, user.Password)

	reused := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", body)
	require.Equal(t, http.StatusOK, reused.Code)
	assert.Contains(t, reused.Body.String(), `"success":false`)
}

func TestOAuthOnboardingRequiresTurnstile(t *testing.T) {
	router := setupOAuthOnboardingControllerTest(t)
	token, err := CreateOAuthOnboardingPendingForTest(OAuthOnboardingPendingInput{Provider: "github", ProviderUserID: "gh-turnstile", Login: "turnstile", Email: "turnstile@example.com"})
	require.NoError(t, err)

	missing := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", `{"pending_token":"`+token+`","terms_accepted":true}`)
	require.Equal(t, http.StatusOK, missing.Code)
	assert.Contains(t, missing.Body.String(), `"success":false`)

	queryOnly := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding?turnstile=valid-turnstile", `{"pending_token":"`+token+`","terms_accepted":true}`)
	require.Equal(t, http.StatusOK, queryOnly.Code)
	assert.Contains(t, queryOnly.Body.String(), `"success":false`)
}

func TestOAuthOnboardingIgnoresSessionTurnstileWithoutBodyToken(t *testing.T) {
	router := setupOAuthOnboardingControllerTest(t)
	token, err := CreateOAuthOnboardingPendingForTest(OAuthOnboardingPendingInput{Provider: "github", ProviderUserID: "gh-session-turnstile", Login: "sessionturnstile", Email: "sessionturnstile@example.com"})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/onboarding", bytes.NewBufferString(`{"pending_token":"`+token+`","terms_accepted":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
}

func TestOAuthOnboardingRejectsReusedPendingToken(t *testing.T) {
	router := setupOAuthOnboardingControllerTest(t)
	token, err := CreateOAuthOnboardingPendingForTest(OAuthOnboardingPendingInput{Provider: "github", ProviderUserID: "gh-reused", Login: "reused", Email: "reused@example.com"})
	require.NoError(t, err)
	body := `{"pending_token":"` + token + `","terms_accepted":true,"turnstile_token":"valid-turnstile"}`
	first := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", body)
	require.Equal(t, http.StatusOK, first.Code)
	assert.Contains(t, first.Body.String(), `"success":true`)
	second := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", body)
	require.Equal(t, http.StatusOK, second.Code)
	assert.Contains(t, second.Body.String(), `"success":false`)
}

func TestOAuthOnboardingKeepsPendingTokenAfterValidationError(t *testing.T) {
	router := setupOAuthOnboardingControllerTest(t)
	token, err := CreateOAuthOnboardingPendingForTest(OAuthOnboardingPendingInput{Provider: "github", ProviderUserID: "gh-keep-pending", Login: "keeppending"})
	require.NoError(t, err)

	missingEmail := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", `{"pending_token":"`+token+`","terms_accepted":true,"turnstile_token":"valid-turnstile"}`)
	require.Equal(t, http.StatusOK, missingEmail.Code)
	assert.Contains(t, missingEmail.Body.String(), `"success":false`)

	retry := performOAuthOnboardingRequest(t, router, http.MethodGet, "/api/oauth/onboarding?pending_token="+token, "")
	require.Equal(t, http.StatusOK, retry.Code)
	assert.Contains(t, retry.Body.String(), `"success":true`)
}

func TestOAuthOnboardingRequiredForNewOAuthUser(t *testing.T) {
	router := setupOAuthOnboardingControllerTest(t)
	oauth.Register("mockgithub", &onboardingMockProvider{name: "GitHub", prefix: "github_", enabled: true, user: &oauth.OAuthUser{ProviderUserID: "gh-required", Username: "required", Email: "required@example.com"}})
	callback := performOAuthOnboardingRequest(t, router, http.MethodGet, "/api/oauth-test/mockgithub", "")
	require.Equal(t, http.StatusOK, callback.Code)
	assert.Contains(t, callback.Body.String(), "oauth_onboarding_required")
}

func TestOAuthOnboardingRequiresEmailWhenProviderEmailMissing(t *testing.T) {
	router := setupOAuthOnboardingControllerTest(t)
	originalEmailVerificationEnabled := common.EmailVerificationEnabled
	common.EmailVerificationEnabled = true
	t.Cleanup(func() { common.EmailVerificationEnabled = originalEmailVerificationEnabled })
	token, err := CreateOAuthOnboardingPendingForTest(OAuthOnboardingPendingInput{Provider: "github", ProviderUserID: "gh-no-email", Login: "noemail"})
	require.NoError(t, err)

	missingEmail := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", `{"pending_token":"`+token+`","terms_accepted":true,"turnstile_token":"valid-turnstile"}`)
	require.Equal(t, http.StatusOK, missingEmail.Code)
	assert.Contains(t, missingEmail.Body.String(), `"success":false`)

	withUnverifiedEmail := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", `{"pending_token":"`+token+`","email":"provided@example.com","terms_accepted":true,"turnstile_token":"valid-turnstile"}`)
	require.Equal(t, http.StatusOK, withUnverifiedEmail.Code)
	assert.Contains(t, withUnverifiedEmail.Body.String(), `"success":false`)

	common.RegisterVerificationCodeWithKey("provided@example.com", "123456", common.EmailVerificationPurpose)
	withEmail := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", `{"pending_token":"`+token+`","email":"provided@example.com","verification_code":"123456","terms_accepted":true,"turnstile_token":"valid-turnstile"}`)
	require.Equal(t, http.StatusOK, withEmail.Code)
	assert.Contains(t, withEmail.Body.String(), `"success":true`)
}

func TestOAuthOnboardingRejectsProviderAlreadyBoundDuringCompletion(t *testing.T) {
	router := setupOAuthOnboardingControllerTest(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 7901, Username: "bound", Status: common.UserStatusEnabled, AffCode: "aff7901", GitHubId: "gh-bound"}).Error)
	token, err := CreateOAuthOnboardingPendingForTest(OAuthOnboardingPendingInput{Provider: "github", ProviderUserID: "gh-bound", Login: "bound2", Email: "bound2@example.com"})
	require.NoError(t, err)

	complete := performOAuthOnboardingRequest(t, router, http.MethodPost, "/api/oauth/onboarding", `{"pending_token":"`+token+`","terms_accepted":true,"turnstile_token":"valid-turnstile"}`)
	require.Equal(t, http.StatusOK, complete.Code)
	assert.Contains(t, complete.Body.String(), `"success":false`)
}
