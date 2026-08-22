package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"runtime/debug"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

var _bp = func() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Path != "" {
		h := sha256.Sum256([]byte(bi.Main.Path))
		return hex.EncodeToString(h[:4])
	}
	return common.GetRandomString(8)
}()

const nginxRequestIDHeader = "X-Nginx-Request-ID"

func requestIDForContext(c *gin.Context) string {
	if c != nil && c.Request != nil {
		id := c.GetHeader(nginxRequestIDHeader)
		if len(id) == 32 {
			if _, err := hex.DecodeString(id); err == nil {
				return id
			}
		}
	}
	return common.GetTimeString() + _bp + common.GetRandomString(8)
}

func RequestId() func(c *gin.Context) {
	return func(c *gin.Context) {
		id := requestIDForContext(c)
		c.Set(common.RequestIdKey, id)
		ctx := context.WithValue(c.Request.Context(), common.RequestIdKey, id)
		c.Request = c.Request.WithContext(ctx)
		c.Header(common.RequestIdKey, id)
		c.Next()
	}
}
