package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    []string{},
	})
}

func GetUserGroups(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    map[string]interface{}{},
	})
}
