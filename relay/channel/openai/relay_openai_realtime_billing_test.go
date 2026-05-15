package openai

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeTransportErrorKeepsTransportErrorCode(t *testing.T) {
	apiErr, usage := realtimeErrorFromErrChan(errors.New("error writing to client: broken pipe"), nil)

	require.NotNil(t, apiErr)
	assert.Nil(t, usage)
	assert.Equal(t, types.ErrorCodeDoRequestFailed, apiErr.GetErrorCode())
	assert.NotEqual(t, types.ErrorCodeSubscriptionTokenExhausted, apiErr.GetErrorCode())
	assert.NotEqual(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestRealtimeSubscriptionErrorPreservesSubscriptionCode(t *testing.T) {
	origin := types.NewOpenAIError(errors.New("subscription token exhausted"), types.ErrorCodeSubscriptionTokenExhausted, http.StatusForbidden, types.ErrOptionWithSkipRetry())

	apiErr, usage := realtimeErrorFromErrChan(origin, nil)

	require.NotNil(t, apiErr)
	assert.Nil(t, usage)
	assert.Equal(t, types.ErrorCodeSubscriptionTokenExhausted, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}
