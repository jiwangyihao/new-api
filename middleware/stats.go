package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

type StatsInfo = common.HTTPStatsInfo

// StatsMiddleware 统计中间件
func StatsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		common.IncrementActiveConnections()
		defer common.DecrementActiveConnections()

		c.Next()
	}
}

// GetStats 获取统计信息
func GetStats() StatsInfo {
	return common.GetHTTPStats()
}
