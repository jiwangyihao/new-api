package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type turnstileCheckResponse struct {
	Success bool `json:"success"`
}

var turnstileVerifier = verifyTurnstileTokenRemote

func VerifyTurnstileToken(ctx context.Context, token string, remoteIP string) (bool, error) {
	if !common.TurnstileCheckEnabled {
		return true, nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false, nil
	}
	return turnstileVerifier(ctx, token, remoteIP)
}

func SetTurnstileVerifierForTest(t interface{ Cleanup(func()) }, verifier func(context.Context, string, string) (bool, error)) {
	previous := turnstileVerifier
	turnstileVerifier = verifier
	t.Cleanup(func() { turnstileVerifier = previous })
}

func verifyTurnstileTokenRemote(ctx context.Context, token string, remoteIP string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, nil
	}
	values := url.Values{
		"secret":   {common.TurnstileSecretKey},
		"response": {token},
	}
	if remoteIP != "" {
		values.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(values.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rawRes, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer rawRes.Body.Close()
	if rawRes.StatusCode < 200 || rawRes.StatusCode >= 300 {
		return false, errors.New(rawRes.Status)
	}
	var res turnstileCheckResponse
	if err := json.NewDecoder(rawRes.Body).Decode(&res); err != nil {
		return false, err
	}
	return res.Success, nil
}

func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.TurnstileCheckEnabled {
			session := sessions.Default(c)
			turnstileChecked := session.Get("turnstile")
			if turnstileChecked != nil {
				c.Next()
				return
			}
			response := c.Query("turnstile")
			if response == "" {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile token 为空",
				})
				c.Abort()
				return
			}
			ok, err := VerifyTurnstileToken(c.Request.Context(), response, c.ClientIP())
			if err != nil {
				common.SysLog(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				c.Abort()
				return
			}
			if !ok {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验失败，请刷新重试！",
				})
				c.Abort()
				return
			}
			session.Set("turnstile", true)
			err = session.Save()
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"message": "无法保存会话信息，请重试",
					"success": false,
				})
				return
			}
		}
		c.Next()
	}
}
