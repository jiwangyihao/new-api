package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRankingFreeUserLogCandidatesSelectDerivedColumnsWithoutLimit(t *testing.T) {
	truncateTables(t)

	const userID = 11001
	const start = int64(1800000000)
	meteredTokens := 1
	zeroTokens := int64(0)
	zeroSubscriptionID := userID
	blankOther := ""
	for i := 0; i < 25; i++ {
		subscriptionID := 12000 + i
		consumed := int64(100 + i)
		require.NoError(t, LOG_DB.Create(&Log{
			Id:                         13000 + i,
			UserId:                     userID,
			CreatedAt:                  start + int64(i),
			Type:                       LogTypeConsume,
			MeteredTokens:              &meteredTokens,
			SubscriptionID:              &subscriptionID,
			SubscriptionTokensConsumed: &consumed,
		}).Error)
	}
	fallbackOther := common.MapToJsonStr(map[string]interface{}{
		"subscription_id":              12999,
		"subscription_tokens_consumed": 999,
	})
	require.NoError(t, LOG_DB.Create(&Log{
		Id:            14000,
		UserId:        userID,
		CreatedAt:     start + 30,
		Type:          LogTypeConsume,
		MeteredTokens: &meteredTokens,
		Other:         fallbackOther,
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		Id:                         14001,
		UserId:                     userID,
		CreatedAt:                  start + 31,
		Type:                       LogTypeConsume,
		MeteredTokens:              &meteredTokens,
		SubscriptionID:              &zeroSubscriptionID,
		SubscriptionTokensConsumed: &zeroTokens,
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		Id:            14002,
		UserId:        userID,
		CreatedAt:     start + 32,
		Type:          LogTypeConsume,
		MeteredTokens: &meteredTokens,
		Other:         blankOther,
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		Id:            14003,
		UserId:        userID,
		CreatedAt:     start + 33,
		Type:          LogTypeError,
		MeteredTokens: &meteredTokens,
		Other:         fallbackOther,
	}).Error)

	rows, err := GetRankingFreeUserLogCandidates([]int{userID}, start, start+3600)

	require.NoError(t, err)
	require.Len(t, rows, 26)
	assert.Equal(t, 13000, rows[0].ID)
	require.NotNil(t, rows[0].SubscriptionID)
	assert.Equal(t, 12000, *rows[0].SubscriptionID)
	require.NotNil(t, rows[0].SubscriptionTokensConsumed)
	assert.EqualValues(t, 100, *rows[0].SubscriptionTokensConsumed)
	assert.Equal(t, fallbackOther, rows[25].Other)
	assert.Nil(t, rows[25].SubscriptionID)
	assert.Nil(t, rows[25].SubscriptionTokensConsumed)
}
