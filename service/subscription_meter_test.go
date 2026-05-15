package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
)

func TestSubscriptionMeteredTokens_NilUsage(t *testing.T) {
	assert.Equal(t, int64(0), SubscriptionMeteredTokens(nil))
}

func TestSubscriptionMeteredTokens_OpenAITotalIncludesCachedTokens(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 40,
		},
	}

	assert.Equal(t, int64(150), SubscriptionMeteredTokens(usage))
}

func TestSubscriptionMeteredTokens_OpenAIPromptAlreadyIncludesCachedWhenTotalMissing(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 40,
		},
	}

	assert.Equal(t, int64(150), SubscriptionMeteredTokens(usage))
}

func TestSubscriptionMeteredTokens_AnthropicNativeCacheTokens(t *testing.T) {
	usage := &dto.Usage{
		UsageSemantic:    "anthropic",
		TotalTokens:      150,
		PromptTokens:     100,
		CompletionTokens: 50,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         30,
			CachedCreationTokens: 50,
		},
		ClaudeCacheCreation5mTokens: 7,
		ClaudeCacheCreation1hTokens: 11,
	}

	assert.Equal(t, int64(230), SubscriptionMeteredTokens(usage))
}

func TestSubscriptionMeteredTokens_AnthropicOpenAIStyleUsageDoesNotDoubleCountCache(t *testing.T) {
	usage := &dto.Usage{
		UsageSemantic:    "openai",
		UsageSource:      "anthropic",
		PromptTokens:     180,
		CompletionTokens: 50,
		TotalTokens:      230,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         30,
			CachedCreationTokens: 50,
		},
		ClaudeCacheCreation5mTokens: 7,
		ClaudeCacheCreation1hTokens: 11,
	}

	assert.Equal(t, int64(230), SubscriptionMeteredTokens(usage))
}

func TestSubscriptionMeteredTokens_GeminiCachedContentTokens(t *testing.T) {
	usage := &dto.Usage{
		UsageSemantic:    "gemini",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 40,
		},
	}

	assert.Equal(t, int64(150), SubscriptionMeteredTokens(usage))
}

func TestSubscriptionMeteredTokens_OpenAIAudioDetailsAreAdditional(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     4,
		CompletionTokens: 8,
		TotalTokens:      12,
		PromptTokensDetails: dto.InputTokenDetails{
			AudioTokens: 3,
		},
		CompletionTokenDetails: dto.OutputTokenDetails{
			AudioTokens: 5,
		},
	}

	assert.Equal(t, int64(20), SubscriptionMeteredTokens(usage))
}
