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
