package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestGetEndpointTypesByChannelTypeCodexIncludesResponsesCompact(t *testing.T) {
	endpoints := GetEndpointTypesByChannelType(constant.ChannelTypeCodex, "gpt-5.5")

	assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponse)
	assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
	assert.NotContains(t, endpoints, constant.EndpointTypeOpenAI)
}
