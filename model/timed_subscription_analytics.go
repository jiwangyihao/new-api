package model

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

const (
	adminTimedValuationBasisGrantTimeline = "timed_grant_timeline"
	adminTimedWarningMissingGrants        = "missing_timed_grants"
	adminTimedWarningInvalidGrant         = "invalid_timed_grant"
	adminTimedWarningOverlappingGrants    = "overlapping_grants"
	adminTimedSourceAttributionMixed      = "mixed_grants"
)

type adminTimedCurrencyValue struct {
	TimeMicros       int64
	TokenMicros      int64
	RecognizedMicros int64
}

type adminTimedSourceCurrencyKey struct {
	Source   dto.AdminAnalyticsSource
	Currency string
}

type adminTimedSourceValue struct {
	TimeMicros  int64
	TokenMicros int64
}

type adminTimedSubscriptionValue struct {
	ByCurrency       map[string]adminTimedCurrencyValue
	BySourceCurrency map[adminTimedSourceCurrencyKey]adminTimedSourceValue
	Sources          []dto.AdminAnalyticsSource
	Warnings         []string
	TokenAvailable   bool
	Unknown          bool
}

type adminTimedInterval struct {
	Start int64
	End   int64
}

func adminCalculateTimedSubscriptionValue(sub UserSubscription, grants []TimedSubscriptionValuationGrant, snapshotAt int64) (adminTimedSubscriptionValue, error) {
	result := adminTimedSubscriptionValue{
		ByCurrency:       map[string]adminTimedCurrencyValue{},
		BySourceCurrency: map[adminTimedSourceCurrencyKey]adminTimedSourceValue{},
		TokenAvailable:   sub.TokenLimit > 0,
	}
	if len(grants) == 0 {
		result.Unknown = true
		result.Warnings = []string{adminTimedWarningMissingGrants}
		return result, nil
	}

	sort.SliceStable(grants, func(i, j int) bool {
		if grants[i].CreatedAt != grants[j].CreatedAt {
			return grants[i].CreatedAt < grants[j].CreatedAt
		}
		return grants[i].Id < grants[j].Id
	})
	endTime := sub.EndTime
	if endTime <= snapshotAt {
		return result, nil
	}
	currentCycleEnd := endTime
	if sub.NextResetTime > snapshotAt && sub.NextResetTime < endTime {
		currentCycleEnd = sub.NextResetTime
	}
	remainingCredit := sub.TokenLimit - sub.TokenUsed
	if remainingCredit < 0 {
		remainingCredit = 0
	}
	if remainingCredit > sub.TokenLimit && sub.TokenLimit > 0 {
		remainingCredit = sub.TokenLimit
	}

	covered := make([]adminTimedInterval, 0, len(grants))
	sourceSet := map[dto.AdminAnalyticsSource]struct{}{}
	for i := range grants {
		grant := grants[i]
		currency := strings.ToUpper(strings.TrimSpace(grant.ValuationCurrency))
		if currency == "" {
			currency = strings.ToUpper(strings.TrimSpace(grant.SourceCurrency))
		}
		if grant.EventEndTime <= grant.EventStartTime || grant.ValuationAmountMicros < 0 || currency == "" {
			result.Unknown = true
			result.Warnings = appendStableWarning(result.Warnings, adminTimedWarningInvalidGrant)
			continue
		}
		start := maxInt64(snapshotAt, grant.EventStartTime)
		end := minInt64(endTime, grant.EventEndTime)
		if end <= start {
			continue
		}
		segments, overlap := adminTimedSubtractCovered(adminTimedInterval{Start: start, End: end}, covered)
		if overlap {
			result.Unknown = true
			result.Warnings = appendStableWarning(result.Warnings, adminTimedWarningOverlappingGrants)
		}
		covered = adminTimedAddCovered(covered, adminTimedInterval{Start: start, End: end})
		source := normalizeAdminTimedGrantSource(grant.SourceType)
		sourceSet[source] = struct{}{}
		for _, segment := range segments {
			timeMicros, err := mulDivFloor(grant.ValuationAmountMicros, segment.End-segment.Start, grant.EventEndTime-grant.EventStartTime)
			if err != nil {
				return adminTimedSubscriptionValue{}, err
			}
			tokenMicros := timeMicros
			if result.TokenAvailable && segment.Start < currentCycleEnd {
				currentEnd := minInt64(segment.End, currentCycleEnd)
				currentMicros, calcErr := mulDivFloor(grant.ValuationAmountMicros, currentEnd-segment.Start, grant.EventEndTime-grant.EventStartTime)
				if calcErr != nil {
					return adminTimedSubscriptionValue{}, calcErr
				}
				scaledCurrent, calcErr := mulDivFloor(currentMicros, remainingCredit, sub.TokenLimit)
				if calcErr != nil {
					return adminTimedSubscriptionValue{}, calcErr
				}
				tokenMicros = scaledCurrent
				if segment.End > currentCycleEnd {
					futureMicros, calcErr := mulDivFloor(grant.ValuationAmountMicros, segment.End-currentCycleEnd, grant.EventEndTime-grant.EventStartTime)
					if calcErr != nil {
						return adminTimedSubscriptionValue{}, calcErr
					}
					tokenMicros += futureMicros
				}
			}
			currencyValue := result.ByCurrency[currency]
			currencyValue.TimeMicros += timeMicros
			if result.TokenAvailable {
				currencyValue.TokenMicros += tokenMicros
			}
			result.ByCurrency[currency] = currencyValue
			sourceKey := adminTimedSourceCurrencyKey{Source: source, Currency: currency}
			sourceValue := result.BySourceCurrency[sourceKey]
			sourceValue.TimeMicros += timeMicros
			if result.TokenAvailable {
				sourceValue.TokenMicros += tokenMicros
			}
			result.BySourceCurrency[sourceKey] = sourceValue
		}
	}
	if !adminTimedCoverageComplete(covered, snapshotAt, endTime) {
		result.Unknown = true
		result.Warnings = appendStableWarning(result.Warnings, adminTimedWarningMissingGrants)
	}

	for currency, value := range result.ByCurrency {
		value.RecognizedMicros = value.TimeMicros
		if result.TokenAvailable && value.TokenMicros < value.RecognizedMicros {
			value.RecognizedMicros = value.TokenMicros
		}
		result.ByCurrency[currency] = value
	}
	result.Sources = make([]dto.AdminAnalyticsSource, 0, len(sourceSet))
	for source := range sourceSet {
		result.Sources = append(result.Sources, source)
	}
	sort.Slice(result.Sources, func(i, j int) bool { return result.Sources[i] < result.Sources[j] })
	return result, nil
}

func adminTimedRecognizedSourceMicros(value adminTimedSubscriptionValue, key adminTimedSourceCurrencyKey) int64 {
	source := value.BySourceCurrency[key]
	total := value.ByCurrency[key.Currency]
	if !value.TokenAvailable || total.TimeMicros <= total.TokenMicros {
		return source.TimeMicros
	}
	return source.TokenMicros
}

func normalizeAdminTimedGrantSource(sourceType string) dto.AdminAnalyticsSource {
	switch strings.TrimSpace(sourceType) {
	case TimedSubscriptionGrantSourceOrder:
		return dto.AdminAnalyticsSourceOrder
	case TimedSubscriptionGrantSourceRedemption:
		return dto.AdminAnalyticsSourceRedemption
	case TimedSubscriptionGrantSourceAdmin:
		return dto.AdminAnalyticsSourceAdmin
	default:
		return dto.AdminAnalyticsSourceUnknown
	}
}

func adminTimedSubtractCovered(interval adminTimedInterval, covered []adminTimedInterval) ([]adminTimedInterval, bool) {
	segments := []adminTimedInterval{interval}
	overlap := false
	for _, existing := range covered {
		next := make([]adminTimedInterval, 0, len(segments)+1)
		for _, segment := range segments {
			if existing.End <= segment.Start || existing.Start >= segment.End {
				next = append(next, segment)
				continue
			}
			overlap = true
			if segment.Start < existing.Start {
				next = append(next, adminTimedInterval{Start: segment.Start, End: existing.Start})
			}
			if existing.End < segment.End {
				next = append(next, adminTimedInterval{Start: existing.End, End: segment.End})
			}
		}
		segments = next
		if len(segments) == 0 {
			break
		}
	}
	return segments, overlap
}

func adminTimedAddCovered(covered []adminTimedInterval, interval adminTimedInterval) []adminTimedInterval {
	covered = append(covered, interval)
	sort.Slice(covered, func(i, j int) bool { return covered[i].Start < covered[j].Start })
	merged := covered[:0]
	for _, candidate := range covered {
		if len(merged) == 0 || merged[len(merged)-1].End < candidate.Start {
			merged = append(merged, candidate)
			continue
		}
		if candidate.End > merged[len(merged)-1].End {
			merged[len(merged)-1].End = candidate.End
		}
	}
	return merged
}

func adminTimedCoverageComplete(covered []adminTimedInterval, start int64, end int64) bool {
	if end <= start {
		return true
	}
	cursor := start
	for _, interval := range covered {
		if interval.Start > cursor {
			return false
		}
		if interval.End > cursor {
			cursor = interval.End
		}
		if cursor >= end {
			return true
		}
	}
	return false
}

func appendStableWarning(warnings []string, warning string) []string {
	for _, existing := range warnings {
		if existing == warning {
			return warnings
		}
	}
	return append(warnings, warning)
}
