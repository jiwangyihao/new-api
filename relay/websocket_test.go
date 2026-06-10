package relay

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWssHelperPreservesAPIKeyTokenLimitExhaustedFromPostSettle(t *testing.T) {
	origin := types.NewOpenAIError(errors.New("api key token limit exhausted"), types.ErrorCodeAPIKeyTokenLimitExhausted, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())

	apiErr := postWssConsumeQuotaErrorToAPIError(origin)

	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeAPIKeyTokenLimitExhausted, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCodeAPIKeyTokenLimitExhausted, apiErr.ToOpenAIError().Code)
}
