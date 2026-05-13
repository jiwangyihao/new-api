package service

import "github.com/QuantumNous/new-api/dto"

func SubscriptionMeteredTokens(usage *dto.Usage) int64 {
	if usage == nil {
		return 0
	}
	if usage.TotalTokens > 0 && usage.UsageSemantic != "anthropic" {
		return int64(usage.TotalTokens)
	}

	total := usage.PromptTokens + usage.CompletionTokens
	if usage.UsageSemantic == "anthropic" {
		total += usage.PromptTokensDetails.CachedTokens
		cacheCreation5m, cacheCreation1h := NormalizeCacheCreationSplit(
			usage.PromptTokensDetails.CachedCreationTokens,
			usage.ClaudeCacheCreation5mTokens,
			usage.ClaudeCacheCreation1hTokens,
		)
		total += cacheCreation5m + cacheCreation1h
	}
	if total < 0 {
		return 0
	}
	return int64(total)
}
