package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionConcurrencyErrorToOpenAI429(t *testing.T) {
	apiErr := service.SubscriptionConcurrencyAPIError(5)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCode("subscription_concurrency_exceeded"), apiErr.GetErrorCode())
}
