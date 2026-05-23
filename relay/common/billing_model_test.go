package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithCompactBillingModelSuffix(t *testing.T) {
	assert.Equal(t, "gpt-5.5-openai-compact", WithCompactBillingModelSuffix("gpt-5.5"))
	assert.Equal(t, "gpt-5.5-openai-compact", WithCompactBillingModelSuffix("gpt-5.5-openai-compact"))
}

func TestResolveBillingModelNameDefaultsToOriginModel(t *testing.T) {
	info := &RelayInfo{OriginModelName: "gpt-5.5"}

	assert.Equal(t, "gpt-5.5", ResolveBillingModelName(info))
}

func TestResolveBillingModelNameUsesBillingModelWhenSet(t *testing.T) {
	info := &RelayInfo{OriginModelName: "gpt-5.5", BillingModelName: "upstream-gpt-openai-compact"}

	assert.Equal(t, "upstream-gpt-openai-compact", ResolveBillingModelName(info))
}
