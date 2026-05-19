package openai

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

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

func TestResponsesConvertDoesNotInjectProviderNativeToolsFromMetadata(t *testing.T) {
	request := dto.OpenAIResponsesRequest{
		Model: "gpt-5",
		Metadata: []byte(`{
			"builtin_tools": {
				"web_search": true,
				"image_generation": {"enabled": true, "model": "gpt-image-1", "output_format": "png"}
			}
		}`),
	}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest := converted.(dto.OpenAIResponsesRequest)
	assert.Empty(t, convertedRequest.Tools)
	assert.JSONEq(t, string(request.Metadata), string(convertedRequest.Metadata))
	assert.Nil(t, info.ResponsesUsageInfo)
}

func TestResponsesConvertSkipsProviderNativeToolsForNonResponsesRelay(t *testing.T) {
	request := dto.OpenAIResponsesRequest{
		Model:    "gpt-5",
		Metadata: []byte(`{"builtin_tools":{"web_search":true}}`),
	}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
	require.NoError(t, err)
	convertedRequest := converted.(dto.OpenAIResponsesRequest)
	assert.Empty(t, convertedRequest.Tools)
	assert.JSONEq(t, `{"builtin_tools":{"web_search":true}}`, string(convertedRequest.Metadata))
}
