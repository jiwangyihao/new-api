package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestIDUsesValidNginxRequestID(t *testing.T) {
	const nginxID = "0123456789abcdef0123456789abcdef"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request.Header.Set(nginxRequestIDHeader, nginxID)

	RequestId()(c)

	storedID, ok := c.Get(common.RequestIdKey)
	require.True(t, ok)
	assert.Equal(t, nginxID, storedID)
	assert.Equal(t, nginxID, c.Request.Context().Value(common.RequestIdKey))
	assert.Equal(t, nginxID, recorder.Header().Get(common.RequestIdKey))
}

func TestRequestIDRejectsInvalidNginxRequestID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request.Header.Set(nginxRequestIDHeader, "not-a-valid-nginx-request-id")

	requestID := requestIDForContext(c)

	assert.NotEmpty(t, requestID)
	assert.NotEqual(t, "not-a-valid-nginx-request-id", requestID)
}
