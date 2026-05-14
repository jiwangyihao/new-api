package controller

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const oauthOnboardingTTLSeconds = 600

type pendingProviderNamer interface {
	PendingProviderName() string
}

type OAuthOnboardingPendingInput struct {
	Provider       string
	ProviderUserID string
	Login          string
	Email          string
	DisplayName    string
	Avatar         string
	InviterId      int
}

type OAuthOnboardingPendingSession struct {
	PendingToken   string
	Provider       string
	ProviderUserID string
	Login          string
	Email          string
	DisplayName    string
	Avatar         string
	InviterId      int
	ExpiresAt      int64
}

type oauthOnboardingStore struct {
	mu       sync.Mutex
	sessions map[string]OAuthOnboardingPendingSession
}

var defaultOAuthOnboardingStore = &oauthOnboardingStore{sessions: make(map[string]OAuthOnboardingPendingSession)}

func (s *oauthOnboardingStore) create(input OAuthOnboardingPendingInput) (string, error) {
	provider := strings.TrimSpace(input.Provider)
	providerUserID := strings.TrimSpace(input.ProviderUserID)
	if provider == "" || providerUserID == "" {
		return "", errors.New("invalid oauth pending session")
	}
	token := common.GetUUID()
	now := common.GetTimestamp()
	pending := OAuthOnboardingPendingSession{
		PendingToken:   token,
		Provider:       provider,
		ProviderUserID: providerUserID,
		Login:          strings.TrimSpace(input.Login),
		Email:          strings.TrimSpace(input.Email),
		DisplayName:    strings.TrimSpace(input.DisplayName),
		Avatar:         strings.TrimSpace(input.Avatar),
		InviterId:      input.InviterId,
		ExpiresAt:      now + oauthOnboardingTTLSeconds,
	}
	s.mu.Lock()
	s.sessions[token] = pending
	s.mu.Unlock()
	return token, nil
}

func (s *oauthOnboardingStore) get(token string) (OAuthOnboardingPendingSession, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return OAuthOnboardingPendingSession{}, false
	}
	now := common.GetTimestamp()
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.sessions[token]
	if !ok {
		return OAuthOnboardingPendingSession{}, false
	}
	if pending.ExpiresAt > 0 && pending.ExpiresAt <= now {
		delete(s.sessions, token)
		return OAuthOnboardingPendingSession{}, false
	}
	return pending, true
}

func (s *oauthOnboardingStore) delete(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func CreateOAuthOnboardingPendingForTest(input OAuthOnboardingPendingInput) (string, error) {
	return defaultOAuthOnboardingStore.create(input)
}

func oauthOnboardingProviderKey(routeProviderName string, provider oauth.Provider) string {
	if namer, ok := provider.(pendingProviderNamer); ok {
		if name := strings.TrimSpace(namer.PendingProviderName()); name != "" {
			return name
		}
	}
	return strings.TrimSpace(routeProviderName)
}

func CreateOAuthOnboardingPending(providerName string, provider oauth.Provider, oauthUser *oauth.OAuthUser, inviterId int) (OAuthOnboardingPendingSession, error) {
	if provider == nil || oauthUser == nil {
		return OAuthOnboardingPendingSession{}, errors.New("invalid oauth onboarding pending input")
	}
	token, err := createOAuthOnboardingPending(OAuthOnboardingPendingInput{
		Provider:       oauthOnboardingProviderKey(providerName, provider),
		ProviderUserID: oauthUser.ProviderUserID,
		Login:          oauthUser.Username,
		Email:          oauthUser.Email,
		DisplayName:    oauthUser.DisplayName,
		InviterId:      inviterId,
	})
	if err != nil {
		return OAuthOnboardingPendingSession{}, err
	}
	pending, _ := defaultOAuthOnboardingStore.get(token)
	return pending, nil
}

func createOAuthOnboardingPending(input OAuthOnboardingPendingInput) (string, error) {
	return defaultOAuthOnboardingStore.create(input)
}

func respondOAuthOnboardingRequired(c *gin.Context, pending OAuthOnboardingPendingSession) {
	c.JSON(200, gin.H{
		"success": true,
		"message": "oauth_onboarding_required",
		"data": gin.H{
			"pending_token": pending.PendingToken,
			"provider":      pending.Provider,
			"login":         pending.Login,
			"email":         pending.Email,
		},
	})
}

func GetOAuthOnboarding(c *gin.Context) {
	pending, ok := defaultOAuthOnboardingStore.get(c.Query("pending_token"))
	if !ok {
		common.ApiErrorMsg(c, "OAuth 建号会话无效或已过期")
		return
	}
	respondOAuthOnboardingRequired(c, pending)
}

type completeOAuthOnboardingRequest struct {
	PendingToken     string `json:"pending_token"`
	Email            string `json:"email"`
	VerificationCode string `json:"verification_code"`
	TrialCode        string `json:"trial_code"`
	Password         string `json:"password"`
	TermsAccepted    bool   `json:"terms_accepted"`
	TurnstileToken   string `json:"turnstile_token"`
}

func CompleteOAuthOnboarding(c *gin.Context) {
	var req completeOAuthOnboardingRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid request body")
		return
	}
	if !req.TermsAccepted {
		common.ApiErrorMsg(c, "请先同意服务条款")
		return
	}
	ok, err := middleware.VerifyTurnstileToken(c.Request.Context(), req.TurnstileToken, c.ClientIP())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !ok {
		common.ApiErrorMsg(c, "Turnstile 校验失败，请刷新重试！")
		return
	}
	pending, ok := defaultOAuthOnboardingStore.get(req.PendingToken)
	if !ok {
		common.ApiErrorMsg(c, "OAuth 建号会话无效或已过期")
		return
	}
	user, err := completeOAuthOnboarding(c.Request.Context(), pending, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defaultOAuthOnboardingStore.delete(req.PendingToken)
	setupLogin(user, c)
}

func completeOAuthOnboarding(ctx context.Context, pending OAuthOnboardingPendingSession, req completeOAuthOnboardingRequest) (*model.User, error) {
	provider := oauth.GetProvider(pending.Provider)
	if provider == nil {
		return nil, errors.New("OAuth provider not found")
	}
	providerEmail := strings.TrimSpace(pending.Email)
	email := providerEmail
	if email == "" {
		email = strings.TrimSpace(req.Email)
	}
	if email == "" {
		return nil, errors.New("email is required")
	}
	if err := validateOAuthOnboardingEmail(email, req.VerificationCode, providerEmail == ""); err != nil {
		return nil, err
	}
	if req.Password != "" && (len(req.Password) < 8 || len(req.Password) > 20) {
		return nil, errors.New("password length must be between 8 and 20")
	}
	user := &model.User{
		Username:    oauthOnboardingUsername(provider, pending.Login),
		Password:    req.Password,
		DisplayName: oauthOnboardingDisplayName(provider, pending),
		Email:       email,
		InviterId:   pending.InviterId,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		providerLock, err := model.CreateOAuthProviderLockTx(tx, pending.Provider, pending.ProviderUserID)
		if err != nil {
			return errors.New("OAuth provider user already bound")
		}
		if err := ensureOAuthProviderAvailableForOnboardingTx(tx, pending.Provider, pending.ProviderUserID); err != nil {
			return err
		}
		if err := user.InsertWithTx(tx, pending.InviterId); err != nil {
			return err
		}
		if err := model.BindOAuthProviderLockTx(tx, providerLock.Id, user.Id); err != nil {
			return err
		}
		if err := bindOAuthProviderForOnboardingTx(tx, provider, pending.Provider, user, pending.ProviderUserID); err != nil {
			return err
		}
		if _, err := service.GrantTrialOnRegistration(tx, service.TrialGrantInput{UserId: user.Id, TrialCode: req.TrialCode, InviterId: pending.InviterId}); err != nil {
			return err
		}
		return user.FinalizeCreationTx(tx, pending.InviterId)
	})
	if err != nil {
		return nil, err
	}
	_ = ctx
	return user, nil
}

func validateOAuthOnboardingEmail(email string, verificationCode string, requireVerification bool) error {
	if err := common.Validate.Var(email, "required,email"); err != nil {
		return errors.New("invalid email")
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return errors.New("invalid email")
	}
	localPart := parts[0]
	domainPart := parts[1]
	if common.EmailDomainRestrictionEnabled {
		allowed := false
		for _, domain := range common.EmailDomainWhitelist {
			if domainPart == domain {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.New("email domain is not allowed")
		}
	}
	if common.EmailAliasRestrictionEnabled && (strings.Contains(localPart, "+") || strings.Contains(localPart, ".")) {
		return errors.New("email alias is not allowed")
	}
	if model.IsEmailAlreadyTaken(email) {
		return errors.New("email already taken")
	}
	if requireVerification && common.EmailVerificationEnabled && !common.VerifyCodeWithKey(email, verificationCode, common.EmailVerificationPurpose) {
		return errors.New("email verification code error")
	}
	return nil
}

func oauthOnboardingUsername(provider oauth.Provider, login string) string {
	login = strings.TrimSpace(login)
	if login != "" {
		if exists, err := model.CheckUserExistOrDeleted(login, ""); err == nil && !exists && len(login) <= model.UserNameMaxLength {
			return login
		}
	}
	return provider.GetProviderPrefix() + common.GetRandomString(8)
}

func oauthOnboardingDisplayName(provider oauth.Provider, pending OAuthOnboardingPendingSession) string {
	if pending.DisplayName != "" {
		return pending.DisplayName
	}
	if pending.Login != "" {
		return pending.Login
	}
	return provider.GetName() + " User"
}

func ensureOAuthProviderAvailableForOnboardingTx(tx *gorm.DB, provider string, providerUserID string) error {
	if strings.TrimSpace(providerUserID) == "" {
		return errors.New("provider user id is required")
	}
	switch provider {
	case "github":
		return ensureBuiltInOAuthFieldAvailableTx(tx, "github_id", providerUserID)
	case "discord":
		return ensureBuiltInOAuthFieldAvailableTx(tx, "discord_id", providerUserID)
	case "oidc":
		return ensureBuiltInOAuthFieldAvailableTx(tx, "oidc_id", providerUserID)
	case "linuxdo":
		return ensureBuiltInOAuthFieldAvailableTx(tx, "linux_do_id", providerUserID)
	case "wechat":
		return ensureBuiltInOAuthFieldAvailableTx(tx, "wechat_id", providerUserID)
	case "telegram":
		return ensureBuiltInOAuthFieldAvailableTx(tx, "telegram_id", providerUserID)
	default:
		if gp, ok := oauth.GetProvider(provider).(*oauth.GenericOAuthProvider); ok {
			var count int64
			if err := tx.Model(&model.UserOAuthBinding{}).Where("provider_id = ? AND provider_user_id = ?", gp.GetProviderId(), providerUserID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("OAuth provider user already bound")
			}
		}
	}
	return nil
}

func ensureBuiltInOAuthFieldAvailableTx(tx *gorm.DB, column string, providerUserID string) error {
	var count int64
	if err := tx.Model(&model.User{}).Unscoped().Where(column+" = ?", providerUserID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("OAuth provider user already bound")
	}
	return nil
}

func bindOAuthProviderForOnboardingTx(tx *gorm.DB, provider oauth.Provider, providerName string, user *model.User, providerUserID string) error {
	if gp, ok := provider.(*oauth.GenericOAuthProvider); ok {
		binding := &model.UserOAuthBinding{UserId: user.Id, ProviderId: gp.GetProviderId(), ProviderUserId: providerUserID}
		return model.CreateUserOAuthBindingWithTx(tx, binding)
	}
	provider.SetProviderUserID(user, providerUserID)
	return tx.Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"github_id":    user.GitHubId,
		"discord_id":   user.DiscordId,
		"oidc_id":      user.OidcId,
		"linux_do_id":  user.LinuxDOId,
		"wechat_id":    user.WeChatId,
		"telegram_id":  user.TelegramId,
		"display_name": user.DisplayName,
	}).Error
}
