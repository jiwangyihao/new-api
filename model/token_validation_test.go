package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenValidationTestDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	originalRedisEnabled := common.RedisEnabled
	originalRDB := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		DB = originalDB
		common.RedisEnabled = originalRedisEnabled
		common.RDB = originalRDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&Token{}, &TokenLimitPreConsumeRecord{}))
}

func TestValidateUserTokenAllowsExhaustedQuotaForSubscriptionOnlyBilling(t *testing.T) {
	setupTokenValidationTestDB(t)
	require.NoError(t, DB.Create(&Token{
		Id:             9901,
		UserId:         9902,
		Key:            "sk-zero-quota",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    0,
		UsedQuota:      100,
		UnlimitedQuota: false,
	}).Error)

	token, err := ValidateUserToken("sk-zero-quota")

	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, common.TokenStatusEnabled, token.Status)
	assert.Equal(t, 0, token.RemainQuota)
	assert.False(t, token.TokenLimitEnabled)
	assert.Equal(t, int64(0), token.TokenLimit)
	assert.Equal(t, int64(0), token.TokenUsed)
}

func TestValidateUserTokenAllowsHistoricalExhaustedStatus(t *testing.T) {
	setupTokenValidationTestDB(t)
	require.NoError(t, DB.Create(&Token{
		Id:             9903,
		UserId:         9904,
		Key:            "sk-exhausted-status",
		Status:         common.TokenStatusExhausted,
		ExpiredTime:    -1,
		RemainQuota:    0,
		UsedQuota:      100,
		UnlimitedQuota: false,
	}).Error)

	token, err := ValidateUserToken("sk-exhausted-status")

	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, common.TokenStatusExhausted, token.Status)
}
