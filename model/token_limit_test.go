package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestTokenLimitPreConsumeIgnoresLegacyQuotaAndIsIdempotent(t *testing.T) {
	setupTokenValidationTestDB(t)
	require.NoError(t, DB.Create(&Token{
		Id:                92001,
		UserId:            92002,
		Key:               "sk-token-limit",
		Status:            common.TokenStatusEnabled,
		ExpiredTime:       -1,
		RemainQuota:       0,
		UsedQuota:         999999,
		UnlimitedQuota:    false,
		TokenLimitEnabled: true,
		TokenLimit:        100,
		TokenUsed:         90,
	}).Error)

	ok, err := PreConsumeTokenLimit(92001, 92002, "req-token-limit", 10)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = PreConsumeTokenLimit(92001, 92002, "req-token-limit", 10)
	require.NoError(t, err)
	require.True(t, ok, "same request_id is idempotent")

	var token Token
	require.NoError(t, DB.First(&token, 92001).Error)
	require.Equal(t, int64(100), token.TokenUsed)
	require.Equal(t, 0, token.RemainQuota)
	require.Equal(t, 999999, token.UsedQuota)

	var records int64
	require.NoError(t, DB.Model(&TokenLimitPreConsumeRecord{}).Where("request_id = ?", "req-token-limit").Count(&records).Error)
	require.Equal(t, int64(1), records)

	ok, err = PreConsumeTokenLimit(92001, 92002, "req-token-limit-over", 1)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, DB.First(&token, 92001).Error)
	require.Equal(t, int64(100), token.TokenUsed)
	require.NoError(t, DB.Model(&TokenLimitPreConsumeRecord{}).Where("request_id = ?", "req-token-limit-over").Count(&records).Error)
	require.Equal(t, int64(0), records, "cap failure must roll back the idempotency record")
}

func TestTokenLimitSettleRefundResetAndCacheInvalidation(t *testing.T) {
	setupTokenValidationTestDB(t)
	require.NoError(t, DB.Create(&Token{
		Id:                92011,
		UserId:            92012,
		Key:               "sk-token-limit-cache",
		Status:            common.TokenStatusEnabled,
		ExpiredTime:       -1,
		TokenLimitEnabled: true,
		TokenLimit:        100,
		TokenUsed:         50,
	}).Error)

	ok, err := PreConsumeTokenLimit(92011, 92012, "req-token-limit-delta", 20)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, SettleTokenLimitPreConsume("req-token-limit-delta", 10))
	require.NoError(t, SettleTokenLimitPreConsume("req-token-limit-delta", 10))

	var token Token
	require.NoError(t, DB.First(&token, 92011).Error)
	require.Equal(t, int64(60), token.TokenUsed)
	var settled TokenLimitPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "req-token-limit-delta").First(&settled).Error)
	require.Equal(t, TokenLimitPreConsumeStatusSettled, settled.Status)
	require.Equal(t, int64(10), settled.ActualTokens)
	require.Equal(t, int64(-10), settled.DeltaTokens)

	ok, err = PreConsumeTokenLimit(92011, 92012, "req-token-limit-positive-delta", 5)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, SettleTokenLimitPreConsume("req-token-limit-positive-delta", 15))
	require.NoError(t, SettleTokenLimitPreConsume("req-token-limit-positive-delta", 15))
	require.NoError(t, DB.First(&token, 92011).Error)
	require.Equal(t, int64(75), token.TokenUsed)

	ok, err = PreConsumeTokenLimit(92011, 92012, "req-token-limit-refund", 10)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, RefundTokenLimitPreConsume("req-token-limit-refund", "upstream_failed"))
	require.NoError(t, RefundTokenLimitPreConsume("req-token-limit-refund", "upstream_failed"))
	require.NoError(t, DB.First(&token, 92011).Error)
	require.Equal(t, int64(75), token.TokenUsed)
	var refunded TokenLimitPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "req-token-limit-refund").First(&refunded).Error)
	require.Equal(t, TokenLimitPreConsumeStatusRefunded, refunded.Status)
	require.Equal(t, "upstream_failed", refunded.FailureCode)

	ok, err = PreConsumeTokenLimit(92011, 92012, "req-token-limit-reset", 15)
	require.NoError(t, err)
	require.True(t, ok)
	before, err := ResetTokenUsage(92011, 92012)
	require.NoError(t, err)
	require.Equal(t, int64(90), before)
	require.NoError(t, RefundTokenLimitPreConsume("req-token-limit-reset", "usage_reset_after_refund"))
	require.NoError(t, SettleTokenLimitPreConsume("req-token-limit-reset", 30))

	require.NoError(t, DB.First(&token, 92011).Error)
	require.Equal(t, int64(0), token.TokenUsed, "old in-flight record must not restore or subtract after reset")
	var resetRecord TokenLimitPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "req-token-limit-reset").First(&resetRecord).Error)
	require.Equal(t, TokenLimitPreConsumeStatusRefunded, resetRecord.Status)
	require.Equal(t, "usage_reset", resetRecord.FailureCode)
}

func TestTokenUpdateDoesNotOverwriteConcurrentTokenUsage(t *testing.T) {
	setupTokenValidationTestDB(t)
	token := Token{
		Id:                92031,
		UserId:            92032,
		Key:               "sk-token-update-stale-used",
		Status:            common.TokenStatusEnabled,
		Name:              "before",
		ExpiredTime:       -1,
		TokenLimitEnabled: true,
		TokenLimit:        1000,
		TokenUsed:         20,
	}
	require.NoError(t, DB.Create(&token).Error)
	stale := token
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("token_used", int64(45)).Error)

	stale.Name = "after"
	require.NoError(t, stale.Update())

	var got Token
	require.NoError(t, DB.First(&got, token.Id).Error)
	require.Equal(t, "after", got.Name)
	require.Equal(t, int64(45), got.TokenUsed)
}

func TestTokenLimitWritesInvalidateTokenCache(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, token Token)
	}{
		{
			name: "preconsume",
			run: func(t *testing.T, token Token) {
				ok, err := PreConsumeTokenLimit(token.Id, token.UserId, token.Key+":preconsume", 10)
				require.NoError(t, err)
				require.True(t, ok)
			},
		},
		{
			name: "settle negative delta",
			run: func(t *testing.T, token Token) {
				requestId := token.Key + ":settle-negative"
				ok, err := PreConsumeTokenLimit(token.Id, token.UserId, requestId, 20)
				require.NoError(t, err)
				require.True(t, ok)
				require.NoError(t, cacheSetToken(token))
				require.NoError(t, SettleTokenLimitPreConsume(requestId, 5))
			},
		},
		{
			name: "settle positive delta",
			run: func(t *testing.T, token Token) {
				requestId := token.Key + ":settle-positive"
				ok, err := PreConsumeTokenLimit(token.Id, token.UserId, requestId, 5)
				require.NoError(t, err)
				require.True(t, ok)
				require.NoError(t, cacheSetToken(token))
				require.NoError(t, SettleTokenLimitPreConsume(requestId, 15))
			},
		},
		{
			name: "refund",
			run: func(t *testing.T, token Token) {
				requestId := token.Key + ":refund"
				ok, err := PreConsumeTokenLimit(token.Id, token.UserId, requestId, 10)
				require.NoError(t, err)
				require.True(t, ok)
				require.NoError(t, cacheSetToken(token))
				require.NoError(t, RefundTokenLimitPreConsume(requestId, "test_refund"))
			},
		},
		{
			name: "increment",
			run: func(t *testing.T, token Token) {
				ok, err := ConsumeTokenLimitIncrement(token.Id, token.UserId, token.Key+":increment", 7)
				require.NoError(t, err)
				require.True(t, ok)
			},
		},
		{
			name: "reset usage",
			run: func(t *testing.T, token Token) {
				before, err := ResetTokenUsage(token.Id, token.UserId)
				require.NoError(t, err)
				require.Equal(t, token.TokenUsed, before)
			},
		},
		{
			name: "update enabled",
			run: func(t *testing.T, token Token) {
				token.TokenLimitEnabled = false
				token.TokenLimit = 0
				require.NoError(t, token.Update())
			},
		},
		{
			name: "update limit",
			run: func(t *testing.T, token Token) {
				token.TokenLimit = 250
				require.NoError(t, token.Update())
			},
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupTokenValidationTestDB(t)
			setupTokenLimitTestRedis(t)
			token := Token{
				Id:                93000 + i,
				UserId:            93100 + i,
				Key:               fmt.Sprintf("sk-cache-%d", i),
				Status:            common.TokenStatusEnabled,
				ExpiredTime:       -1,
				TokenLimitEnabled: true,
				TokenLimit:        100,
				TokenUsed:         20,
			}
			require.NoError(t, DB.Create(&token).Error)
			require.NoError(t, cacheSetToken(token))
			cached, err := cacheGetTokenByKey(token.Key)
			require.NoError(t, err)
			require.True(t, cached.TokenLimitEnabled)
			require.Equal(t, int64(100), cached.TokenLimit)
			require.Equal(t, int64(20), cached.TokenUsed)

			tc.run(t, token)

			var dbToken Token
			require.NoError(t, DB.First(&dbToken, token.Id).Error)
			fresh, err := GetTokenByKey(token.Key, false)
			require.NoError(t, err)
			require.Equal(t, dbToken.TokenLimitEnabled, fresh.TokenLimitEnabled)
			require.Equal(t, dbToken.TokenLimit, fresh.TokenLimit)
			require.Equal(t, dbToken.TokenUsed, fresh.TokenUsed)
		})
	}
}

func setupTokenLimitTestRedis(t *testing.T) {
	t.Helper()
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		server.Close()
	})
}
