package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestEndpointTypeFromRequestPathResponsesCompact(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses/compact", nil)

	endpointType, ok := endpointTypeFromRequest(c)

	assert.True(t, ok)
	assert.Equal(t, constant.EndpointTypeOpenAIResponseCompact, endpointType)
}

func TestEndpointTypeFromRequestPathResponses(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	endpointType, ok := endpointTypeFromRequest(c)

	assert.True(t, ok)
	assert.Equal(t, constant.EndpointTypeOpenAIResponse, endpointType)
}

func TestEndpointTypeFromRequestRejectsNearMissPaths(t *testing.T) {
	for _, path := range []string{"/v1/responsesXYZ", "/v1/responses/compactXYZ", "/v1/responses/other"} {
		t.Run(path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", path, nil)

			endpointType, ok := endpointTypeFromRequest(c)

			assert.False(t, ok)
			assert.Empty(t, endpointType)
		})
	}
}

func TestRequestTimingIncludesProxyBufferDuration(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request.Header.Set(proxyRequestBufferTimeHeader, "1.250")
	now := time.Unix(100, 0)

	startTime, bufferTimeMs := requestTiming(c, now)

	assert.Equal(t, now.Add(-1250*time.Millisecond), startTime)
	assert.Equal(t, 1250, bufferTimeMs)
}

func TestRequestTimingRejectsInvalidProxyBufferDuration(t *testing.T) {
	for _, value := range []string{"", "NaN", "Inf", "-1", "601"} {
		t.Run(value, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			c.Request.Header.Set(proxyRequestBufferTimeHeader, value)
			now := time.Unix(100, 0)

			startTime, bufferTimeMs := requestTiming(c, now)

			assert.Equal(t, now, startTime)
			assert.Zero(t, bufferTimeMs)
		})
	}
}
