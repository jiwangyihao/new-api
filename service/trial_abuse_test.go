package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTrialAbuseQueryDefaults(t *testing.T) {
	now := int64(1700000000)

	query, err := normalizeTrialAbuseSummaryQuery(TrialAbuseSummaryQuery{}, now)

	require.NoError(t, err)
	assert.Equal(t, now-14*24*60*60, query.TrialEndStart)
	assert.Equal(t, now, query.TrialEndEnd)
	assert.Equal(t, now, query.SnapshotAt)
	assert.Equal(t, 500, query.MinConsumeCount)
	assert.Equal(t, 2, query.MinClusterSize)
	assert.Equal(t, 50, query.RiskLimit)
	assert.Equal(t, 20, query.GroupLimit)
}

func TestNormalizeTrialAbuseQueryRejectsTooWideWindow(t *testing.T) {
	now := int64(1700000000)

	_, err := normalizeTrialAbuseSummaryQuery(TrialAbuseSummaryQuery{TrialEndStart: now - 91*24*60*60, TrialEndEnd: now}, now)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trial end range exceeds 90 days")
}

func TestNormalizeTrialAbuseQueryRejectsInvalidRanges(t *testing.T) {
	now := int64(1700000000)

	_, err := normalizeTrialAbuseSummaryQuery(TrialAbuseSummaryQuery{TrialEndStart: now, TrialEndEnd: now - 1}, now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trial end range")

	_, err = normalizeTrialAbuseSummaryQuery(TrialAbuseSummaryQuery{RegisteredStart: now, RegisteredEnd: now - 1}, now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid registered range")
}

func TestNormalizeTrialAbuseQueryClampsLimits(t *testing.T) {
	now := int64(1700000000)

	query, err := normalizeTrialAbuseSummaryQuery(TrialAbuseSummaryQuery{MinConsumeCount: 200000, MinClusterSize: 1000, RiskLimit: 999, GroupLimit: 999}, now)

	require.NoError(t, err)
	assert.Equal(t, 100000, query.MinConsumeCount)
	assert.Equal(t, 100, query.MinClusterSize)
	assert.Equal(t, 200, query.RiskLimit)
	assert.Equal(t, 100, query.GroupLimit)
}

func TestNormalizeTrialAbuseQueryRejectsInvalidThresholdLowerBounds(t *testing.T) {
	now := int64(1700000000)

	_, err := normalizeTrialAbuseSummaryQuery(TrialAbuseSummaryQuery{MinConsumeCount: -10}, now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid min consume count")

	_, err = normalizeTrialAbuseSummaryQuery(TrialAbuseSummaryQuery{MinClusterSize: 1}, now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid min cluster size")

	query, err := normalizeTrialAbuseSummaryQuery(TrialAbuseSummaryQuery{RiskLimit: -1, GroupLimit: 0}, now)
	require.NoError(t, err)
	assert.Equal(t, 50, query.RiskLimit)
	assert.Equal(t, 20, query.GroupLimit)
}
