package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const (
	trialAbuseDefaultWindowSeconds = int64(14 * 24 * 60 * 60)
	trialAbuseMaxWindowSeconds     = int64(90 * 24 * 60 * 60)
)

var errTrialAbuseInvalidQuery = errors.New("invalid trial abuse query")

type trialAbuseQueryError string

func (e trialAbuseQueryError) Error() string {
	return string(e)
}

func (e trialAbuseQueryError) Is(target error) bool {
	return target == errTrialAbuseInvalidQuery
}

type TrialAbuseSummaryQuery struct {
	TrialEndStart   int64
	TrialEndEnd     int64
	RegisteredStart int64
	RegisteredEnd   int64
	SnapshotAt      int64
	MinConsumeCount int
	MinClusterSize  int
	RiskLimit       int
	GroupLimit      int
}

func GetTrialAbuseSummary(ctx context.Context, query TrialAbuseSummaryQuery) (*dto.TrialAbuseSummaryResponse, error) {
	normalized, err := normalizeTrialAbuseSummaryQuery(query, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	_ = ctx
	return model.GetTrialAbuseSummary(model.TrialAbuseQuery{
		TrialEndStart:   normalized.TrialEndStart,
		TrialEndEnd:     normalized.TrialEndEnd,
		RegisteredStart: normalized.RegisteredStart,
		RegisteredEnd:   normalized.RegisteredEnd,
		SnapshotAt:      normalized.SnapshotAt,
		MinConsumeCount: normalized.MinConsumeCount,
		MinClusterSize:  normalized.MinClusterSize,
		RiskLimit:       normalized.RiskLimit,
		GroupLimit:      normalized.GroupLimit,
	})
}

func IsTrialAbuseInvalidQueryError(err error) bool {
	return errors.Is(err, errTrialAbuseInvalidQuery)
}

func normalizeTrialAbuseSummaryQuery(query TrialAbuseSummaryQuery, now int64) (TrialAbuseSummaryQuery, error) {
	if query.SnapshotAt <= 0 {
		query.SnapshotAt = now
	}
	if query.TrialEndEnd <= 0 {
		query.TrialEndEnd = query.SnapshotAt
	}
	if query.TrialEndStart <= 0 {
		query.TrialEndStart = query.TrialEndEnd - trialAbuseDefaultWindowSeconds
	}
	if query.TrialEndStart < 0 || query.TrialEndEnd < 0 || query.TrialEndStart > query.TrialEndEnd {
		return TrialAbuseSummaryQuery{}, trialAbuseQueryError("invalid trial end range")
	}
	if query.TrialEndEnd-query.TrialEndStart > trialAbuseMaxWindowSeconds {
		return TrialAbuseSummaryQuery{}, trialAbuseQueryError("trial end range exceeds 90 days")
	}
	if query.RegisteredStart < 0 || query.RegisteredEnd < 0 || (query.RegisteredStart > 0 && query.RegisteredEnd > 0 && query.RegisteredStart > query.RegisteredEnd) {
		return TrialAbuseSummaryQuery{}, trialAbuseQueryError("invalid registered range")
	}
	if query.MinConsumeCount < 0 {
		return TrialAbuseSummaryQuery{}, trialAbuseQueryError("invalid min consume count")
	}
	if query.MinConsumeCount == 0 {
		query.MinConsumeCount = 500
	}
	if query.MinConsumeCount > 100000 {
		query.MinConsumeCount = 100000
	}
	if query.MinClusterSize != 0 && query.MinClusterSize < 2 {
		return TrialAbuseSummaryQuery{}, trialAbuseQueryError("invalid min cluster size")
	}
	if query.MinClusterSize == 0 {
		query.MinClusterSize = 2
	}
	if query.MinClusterSize > 100 {
		query.MinClusterSize = 100
	}
	if query.RiskLimit <= 0 {
		query.RiskLimit = 50
	}
	if query.RiskLimit > 200 {
		query.RiskLimit = 200
	}
	if query.GroupLimit <= 0 {
		query.GroupLimit = 20
	}
	if query.GroupLimit > 100 {
		query.GroupLimit = 100
	}
	return query, nil
}
