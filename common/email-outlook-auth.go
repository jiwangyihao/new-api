package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"sync"
	"time"
)

type outlookAuth struct {
	username, password string
}

func LoginAuth(username, password string) smtp.Auth {
	return &outlookAuth{username, password}
}

func (a *outlookAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *outlookAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unknown fromServer")
		}
	}
	return nil, nil
}

func isOutlookServer(server string) bool {
	// 兼容多地区的outlook邮箱和ofb邮箱
	// 其实应该加一个Option来区分是否用LOGIN的方式登录
	// 先临时兼容一下
	return strings.Contains(server, "outlook") || strings.Contains(server, "onmicrosoft")
}

// persistSMTPRefreshTokenHook is wired by the model package at init time to
// persist a rotated refresh token back into the options store. common cannot
// import model, so we use a function hook to avoid the import cycle.
var persistSMTPRefreshTokenHook func(string) error

func persistSMTPRefreshToken(token string) error {
	if persistSMTPRefreshTokenHook == nil {
		return nil
	}
	return persistSMTPRefreshTokenHook(token)
}

// SetPersistSMTPRefreshTokenHook lets the model package register the persistence
// callback used when Microsoft rotates the refresh token.
func SetPersistSMTPRefreshTokenHook(hook func(string) error) {
	persistSMTPRefreshTokenHook = hook
}

// ---- XOAUTH2 (OAuth2 bearer) support for Microsoft 365 / Outlook ----

type xoauth2Auth struct {
	username, accessToken string
}

// XOAuth2Auth returns an smtp.Auth implementing the SASL XOAUTH2 mechanism.
func XOAuth2Auth(username, accessToken string) smtp.Auth {
	return &xoauth2Auth{username, accessToken}
}

func (a *xoauth2Auth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	// base64 is applied by net/smtp; we return the raw initial response.
	resp := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.username, a.accessToken)
	return "XOAUTH2", []byte(resp), nil
}

func (a *xoauth2Auth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		// On failure the server sends a base64 JSON error challenge; replying
		// empty lets net/smtp surface the subsequent error code.
		return []byte{}, nil
	}
	return nil, nil
}

var smtpTokenMu sync.Mutex

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func smtpOAuthTokenEndpoint() string {
	tenant := SMTPOAuthTenantId
	if tenant == "" {
		tenant = "common"
	}
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenant)
}

// refreshSMTPOAuthToken exchanges the stored refresh token for a fresh access
// token. Microsoft rotates refresh tokens, so the returned refresh token is
// persisted back when present.
func refreshSMTPOAuthToken() error {
	if SMTPOAuthClientId == "" {
		return errors.New("SMTP OAuth client id 未配置")
	}
	if SMTPOAuthRefreshToken == "" {
		return errors.New("SMTP OAuth refresh token 未配置")
	}
	form := url.Values{}
	form.Set("client_id", SMTPOAuthClientId)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", SMTPOAuthRefreshToken)
	form.Set("scope", "https://outlook.office365.com/SMTP.Send offline_access")
	req, err := http.NewRequest(http.MethodPost, smtpOAuthTokenEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var tok oauthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return fmt.Errorf("解析 OAuth token 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK || tok.AccessToken == "" {
		if tok.Error != "" {
			return fmt.Errorf("刷新 OAuth token 失败: %s: %s", tok.Error, tok.ErrorDesc)
		}
		return fmt.Errorf("刷新 OAuth token 失败: HTTP %d", resp.StatusCode)
	}
	accessToken := tok.AccessToken
	expiry := time.Now().Unix() + tok.ExpiresIn - 120
	if tok.RefreshToken != "" && tok.RefreshToken != SMTPOAuthRefreshToken {
		SMTPOAuthRefreshToken = tok.RefreshToken
		// Persisting the rotated refresh token runs through the option map,
		// which clears the cached access token/expiry. Persist FIRST, then
		// publish the freshly obtained access token so it isn't wiped.
		if err := persistSMTPRefreshToken(tok.RefreshToken); err != nil {
			SysError(fmt.Sprintf("持久化 SMTP OAuth refresh token 失败: %v", err))
		}
	}
	SMTPOAuthAccessToken = accessToken
	SMTPOAuthAccessExpiry = expiry
	return nil
}

// getSMTPOAuthAccessToken returns a valid access token, refreshing if needed.
func getSMTPOAuthAccessToken() (string, error) {
	smtpTokenMu.Lock()
	defer smtpTokenMu.Unlock()
	if SMTPOAuthAccessToken != "" && time.Now().Unix() < SMTPOAuthAccessExpiry {
		return SMTPOAuthAccessToken, nil
	}
	if err := refreshSMTPOAuthToken(); err != nil {
		return "", err
	}
	return SMTPOAuthAccessToken, nil
}
