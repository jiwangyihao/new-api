package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
)

func GetWelcomePopup(c *gin.Context) {
	setting := console_setting.GetConsoleSetting()
	content := setting.WelcomePopupContent
	if !setting.WelcomePopupEnabled || strings.TrimSpace(content) == "" {
		content = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"enabled":   setting.WelcomePopupEnabled,
			"content":   content,
			"frequency": console_setting.NormalizeWelcomePopupFrequency(setting.WelcomePopupFrequency),
		},
	})
}
