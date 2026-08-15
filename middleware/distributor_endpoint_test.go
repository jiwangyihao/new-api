package middleware

import (
	"net/http/httptest"
	"testing"

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
