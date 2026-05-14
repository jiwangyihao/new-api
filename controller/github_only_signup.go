package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
)

func rejectGitHubOnlySignup(c *gin.Context) bool {
	if !common.GitHubOnlySignupEnabled {
		return false
	}
	common.ApiErrorMsg(c, "当前仅允许通过 GitHub OAuth 创建新账号")
	return true
}

func rejectNonGitHubOAuthSignup(c *gin.Context) bool {
	if !common.GitHubOnlySignupEnabled {
		return false
	}
	common.ApiErrorMsg(c, "当前仅允许通过 GitHub OAuth 创建新账号")
	return true
}

func rejectGitHubOnlySignupI18n(c *gin.Context) bool {
	if !common.GitHubOnlySignupEnabled {
		return false
	}
	common.ApiErrorI18n(c, i18n.MsgUserRegisterDisabled)
	return true
}
